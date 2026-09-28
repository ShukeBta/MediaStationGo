package service

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"sort"
	"strings"
	"sync"
)

const (
	mediaTitleCleanupStageCleaning = "cleaning"
	mediaTitleCleanupMaxAttempts   = 2
)

var ErrAITitleCleanupUnavailable = errors.New("AI 标题清洗未配置")

type MediaTitleCleanupSource struct {
	MediaID         string `json:"media_id"`
	CurrentTitle    string `json:"current_title"`
	SourceDirectory string `json:"source_directory"`
	DirectoryChain  string `json:"directory_chain"`
	Filename        string `json:"filename"`
}

type MediaTitleCleanupGroup struct {
	SourceDirectory string                    `json:"source_directory"`
	Items           []MediaTitleCleanupSource `json:"items"`
	DirectoryKey    string                    `json:"-"`
}

type MediaTitleCleanupSuggestion struct {
	MediaID         string  `json:"media_id"`
	CurrentTitle    string  `json:"current_title,omitempty"`
	SourceDirectory string  `json:"source_directory,omitempty"`
	Filename        string  `json:"filename,omitempty"`
	Title           string  `json:"title"`
	Year            int     `json:"year,omitempty"`
	Confidence      float64 `json:"confidence"`
	Reason          string  `json:"reason,omitempty"`
}

type mediaTitleNormalization struct {
	MediaID    string
	Title      string
	Year       int
	Confidence float64
	Reason     string
}

type aiTitleNormalizationItem struct {
	MediaID    string   `json:"media_id"`
	Title      string   `json:"title"`
	Year       *int     `json:"year"`
	Confidence *float64 `json:"confidence"`
	Reason     string   `json:"reason"`
}

type aiTitleNormalizationResponse struct {
	Items *[]aiTitleNormalizationItem `json:"items"`
}

func (a *AIService) CleanMediaTitles(ctx context.Context, groups []MediaTitleCleanupGroup) ([]MediaTitleCleanupSuggestion, error) {
	return a.CleanMediaTitlesWithProgress(ctx, groups, nil)
}

// CleanMediaTitlesWithProgress only normalizes titles and years. Media
// relationships are managed explicitly by the manual grouping API.
func (a *AIService) CleanMediaTitlesWithProgress(
	ctx context.Context,
	groups []MediaTitleCleanupGroup,
	onProgress func(stage string, completed, total int),
) ([]MediaTitleCleanupSuggestion, error) {
	if len(groups) == 0 {
		return []MediaTitleCleanupSuggestion{}, nil
	}
	runtime := a.resolveRuntimeConfig(ctx)
	if !runtime.Enabled {
		return nil, ErrAITitleCleanupUnavailable
	}

	batches, err := runOrderedTitleCleanupStage(ctx, len(groups), func(stageCtx context.Context, index int) ([]mediaTitleNormalization, error) {
		return a.normalizeMediaTitleBatch(stageCtx, runtime, groups[index])
	}, func(completed, total int) {
		if onProgress != nil {
			onProgress(mediaTitleCleanupStageCleaning, completed, total)
		}
	})
	if err != nil {
		return nil, err
	}

	normalizedByID := make(map[string]mediaTitleNormalization)
	for _, batch := range batches {
		for _, item := range batch {
			if _, exists := normalizedByID[item.MediaID]; exists {
				return nil, fmt.Errorf("AI 标题清洗重复返回 media_id: %s", item.MediaID)
			}
			normalizedByID[item.MediaID] = item
		}
	}
	if len(normalizedByID) != mediaTitleCleanupItemCount(groups) {
		return nil, fmt.Errorf("AI 标题清洗结果数量不完整: got=%d want=%d", len(normalizedByID), mediaTitleCleanupItemCount(groups))
	}

	out := make([]MediaTitleCleanupSuggestion, 0, len(normalizedByID))
	for _, group := range groups {
		for _, source := range group.Items {
			normalized, ok := normalizedByID[source.MediaID]
			if !ok {
				return nil, fmt.Errorf("AI 标题清洗缺少 media_id: %s", source.MediaID)
			}
			out = append(out, MediaTitleCleanupSuggestion{
				MediaID: source.MediaID, CurrentTitle: source.CurrentTitle,
				SourceDirectory: source.SourceDirectory, Filename: source.Filename,
				Title: normalized.Title, Year: normalized.Year,
				Confidence: normalized.Confidence, Reason: normalized.Reason,
			})
		}
	}
	return validateMediaTitleCleanupSuggestions(groups, out)
}

func runOrderedTitleCleanupStage[T any](
	ctx context.Context,
	count int,
	worker func(context.Context, int) ([]T, error),
	onProgress func(completed, total int),
) ([][]T, error) {
	const concurrency = 3
	results := make([][]T, count)
	if count == 0 {
		return results, nil
	}
	stageCtx, cancel := context.WithCancel(ctx)
	defer cancel()
	work := make(chan int)
	var wg sync.WaitGroup
	var mu sync.Mutex
	var firstErr error
	completed := 0
	workerCount := minInt(concurrency, count)
	for i := 0; i < workerCount; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for index := range work {
				items, err := worker(stageCtx, index)
				mu.Lock()
				if err != nil && firstErr == nil {
					firstErr = err
					cancel()
				}
				if err == nil {
					results[index] = items
					completed++
					if onProgress != nil {
						onProgress(completed, count)
					}
				}
				mu.Unlock()
				if err != nil {
					return
				}
			}
		}()
	}
sendLoop:
	for index := 0; index < count; index++ {
		select {
		case work <- index:
		case <-stageCtx.Done():
			break sendLoop
		}
	}
	close(work)
	wg.Wait()
	if firstErr != nil {
		return nil, firstErr
	}
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	return results, nil
}

func (a *AIService) normalizeMediaTitleBatch(
	ctx context.Context,
	runtime aiRuntimeConfig,
	group MediaTitleCleanupGroup,
) ([]mediaTitleNormalization, error) {
	payload, err := json.Marshal(map[string]any{
		"source_directory": group.SourceDirectory,
		"items":            group.Items,
	})
	if err != nil {
		return nil, err
	}
	const system = `你是媒体文件标题标准化器。只做标题和年份清洗，不判断、推测或输出文件之间的聚合关系。
只能依据输入的目录名、目录层级、当前标题和文件名，不得虚构作品、演员或外部元数据。
标题优先提取有语义的中文描述：保留人物或作品名、场景、情节、主题等能区分内容的中文短语，不得只剩下人名、品牌名或一串数字。
只删除站点域名、引流前后缀、画质、编码、发布组、“AI增强”等制作标记和无意义分隔符。不要把有语义的中文内容描述当成噪声删除。
数字只是弱证据：当前标题中的数字、“-数字”、括号数字或文件名尾号，不得单独解释为年份、集数或分段。只有原始文字明确出现“第N集/第N段/Part N/CD N/上中下”等语义标记时，才在标题中保留该序号。
标题开头的“孤立数字 + 空格 + 中文描述”通常是来源排序号，应删除这个孤立数字；但“3月”、“12月新作”、“第3季”等数字与语义单位直接相连的内容必须保留。例如“22 8月新品，极品母狗…”应从“8月新品，极品母狗…”开始。
当前标题可能是旧清洗结果，如果它的分段数字只来自“mp4_ (8)”这类无语义文件名，必须忽略该数字，不得继续输出“第8段”。
文件名末尾的裸数字不得在标题清洗阶段改写成“第N段”。例如“我与护士小母狗2.mp4”和“我与护士小母狗3.mp4”都不能仅凭尾号生成分段标题；“无套内射18岁小嫩逼-10.mp4”也不得输出“第10段”。
当前标题只是弱参考，可能来自过去的过度清洗。如果 source_directory 或 filename 包含更丰富的中文描述，不得直接复制较短的 current_title，必须从原始描述重建标题。
对于中文描述丰富的资源，清洗后至少保留“人物或主体 + 核心情节/场景 + 一个有辨识度的特征”，不得缩成“人名 + 极短动作”。例如“小敏儿 粉色性感连衣裙小学妹以性换租”不得只输出“小敏儿 以性换租”。
文件名无语义而目录名有明确中文描述时，以描述性目录名为标题基础。但广告、药品推广、引流等文件名本身已有独立语义时，不得用父目录影片标题覆盖它。
例如：“玩偶姐姐hongkongdoll-111最新会员私信短片 穿着可爱的黑色JK超诱惑黑丝”应提取为“玩偶姐姐 穿着可爱的黑色JK超诱惑黑丝”，不得输出“第111段”。
每个 media_id 必须返回且只能返回一次。year 不确定时返回 0。不得输出 relation、group_key、group_title、part_index 等聚合字段。只输出纯 JSON。
格式：{"items":[{"media_id":"...","title":"...","year":0,"confidence":0.0,"reason":"简短依据"}]}`
	requestSystem := system
	requestUser := string(payload)
	var firstValidationErr error
	for attempt := 0; attempt < mediaTitleCleanupMaxAttempts; attempt++ {
		raw, completeErr := a.completeBatch(ctx, runtime, requestSystem, requestUser, 0)
		if completeErr != nil {
			return nil, completeErr
		}
		items, validationErr := parseMediaTitleNormalizationResponse(group, raw)
		if validationErr == nil {
			return items, nil
		}
		if firstValidationErr == nil {
			firstValidationErr = validationErr
		}
		if attempt+1 >= mediaTitleCleanupMaxAttempts {
			return nil, mediaTitleCleanupRetryError("标题清洗", firstValidationErr, validationErr)
		}
		requestSystem, requestUser, err = mediaTitleCleanupCorrectionRequest(system, string(payload), raw, validationErr)
		if err != nil {
			return nil, err
		}
	}
	return nil, errors.New("AI 标题清洗未生成有效结果")
}

func parseMediaTitleNormalizationResponse(group MediaTitleCleanupGroup, raw string) ([]mediaTitleNormalization, error) {
	var response aiTitleNormalizationResponse
	if err := decodeStrictAIJSON(raw, &response); err != nil {
		return nil, fmt.Errorf("AI 标题清洗返回了非法 JSON: %w", err)
	}
	if response.Items == nil {
		return nil, errors.New("AI 标题清洗缺少 items")
	}
	expected := make(map[string]MediaTitleCleanupSource, len(group.Items))
	for _, source := range group.Items {
		expected[source.MediaID] = source
	}
	if len(*response.Items) != len(expected) {
		return nil, fmt.Errorf("AI 标题清洗结果数量不完整: got=%d want=%d", len(*response.Items), len(expected))
	}
	seen := make(map[string]struct{}, len(expected))
	out := make([]mediaTitleNormalization, 0, len(expected))
	for _, wire := range *response.Items {
		id := strings.TrimSpace(wire.MediaID)
		if _, ok := expected[id]; !ok {
			return nil, fmt.Errorf("AI 标题清洗返回了未知 media_id: %s", id)
		}
		if _, exists := seen[id]; exists {
			return nil, fmt.Errorf("AI 标题清洗重复返回 media_id: %s", id)
		}
		seen[id] = struct{}{}
		title := strings.Join(strings.Fields(strings.TrimSpace(wire.Title)), " ")
		if title == "" || len([]rune(title)) > 255 {
			return nil, fmt.Errorf("AI 标题清洗返回了无效标题: %s", id)
		}
		if wire.Year == nil || *wire.Year < 0 || *wire.Year > 2100 {
			return nil, fmt.Errorf("AI 标题清洗返回了无效年份: %s", id)
		}
		if wire.Confidence == nil || *wire.Confidence < 0 || *wire.Confidence > 1 {
			return nil, fmt.Errorf("AI 标题清洗返回了无效置信度: %s", id)
		}
		out = append(out, mediaTitleNormalization{
			MediaID: id, Title: title, Year: *wire.Year,
			Confidence: *wire.Confidence, Reason: strings.TrimSpace(wire.Reason),
		})
	}
	sort.SliceStable(out, func(i, j int) bool {
		return mediaTitleCleanupSourceOrder(group.Items, out[i].MediaID) < mediaTitleCleanupSourceOrder(group.Items, out[j].MediaID)
	})
	return out, nil
}

func mediaTitleCleanupCorrectionRequest(system, originalInput, previousOutput string, validationErr error) (string, string, error) {
	var input any
	if err := json.Unmarshal([]byte(originalInput), &input); err != nil {
		return "", "", fmt.Errorf("构建 AI 纠错请求失败: %w", err)
	}
	payload, err := json.Marshal(map[string]any{
		"original_input":   input,
		"previous_output":  previousOutput,
		"validation_error": validationErr.Error(),
	})
	if err != nil {
		return "", "", err
	}
	correction := `

上一轮输出未通过服务端校验。用户消息会同时提供 original_input、previous_output 和 validation_error。
必须针对 validation_error 修正上一轮结果，仍完整返回 original_input 中的每个 media_id；不得解释、不得遗漏、不得输出额外字段。`
	return system + correction, string(payload), nil
}

func mediaTitleCleanupRetryError(stage string, firstErr, lastErr error) error {
	if firstErr == nil {
		return fmt.Errorf("AI %s未通过校验: %w", stage, lastErr)
	}
	return fmt.Errorf("AI %s纠错后仍未通过校验（首次：%v）: %w", stage, firstErr, lastErr)
}

func decodeStrictAIJSON(raw string, target any) error {
	raw = strings.TrimSpace(raw)
	if strings.HasPrefix(raw, "```") {
		firstLineEnd := strings.IndexByte(raw, '\n')
		if firstLineEnd < 0 || !strings.HasSuffix(raw, "```") {
			return errors.New("代码块格式不完整")
		}
		fence := strings.TrimSpace(raw[:firstLineEnd])
		if fence != "```" && !strings.EqualFold(fence, "```json") {
			return fmt.Errorf("不支持的代码块类型: %s", fence)
		}
		raw = strings.TrimSpace(raw[firstLineEnd+1 : len(raw)-3])
	}
	decoder := json.NewDecoder(strings.NewReader(raw))
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(target); err != nil {
		return err
	}
	var extra any
	if err := decoder.Decode(&extra); !errors.Is(err, io.EOF) {
		if err == nil {
			return errors.New("JSON 后存在额外内容")
		}
		return err
	}
	return nil
}

func validateMediaTitleCleanupSuggestions(groups []MediaTitleCleanupGroup, suggestions []MediaTitleCleanupSuggestion) ([]MediaTitleCleanupSuggestion, error) {
	expected := make(map[string]MediaTitleCleanupSource)
	order := make(map[string]int)
	for _, group := range groups {
		for _, item := range group.Items {
			id := strings.TrimSpace(item.MediaID)
			if id == "" {
				return nil, errors.New("AI 标题清洗输入包含空 media_id")
			}
			if _, duplicate := expected[id]; duplicate {
				return nil, fmt.Errorf("AI 标题清洗输入重复 media_id: %s", id)
			}
			order[id] = len(order)
			expected[id] = item
		}
	}
	if len(suggestions) != len(expected) {
		return nil, fmt.Errorf("AI 标题清洗结果数量不完整: got=%d want=%d", len(suggestions), len(expected))
	}

	seen := make(map[string]struct{}, len(suggestions))
	for i := range suggestions {
		item := &suggestions[i]
		item.MediaID = strings.TrimSpace(item.MediaID)
		source, ok := expected[item.MediaID]
		if !ok {
			return nil, fmt.Errorf("AI 标题清洗返回了未知 media_id: %s", item.MediaID)
		}
		if _, exists := seen[item.MediaID]; exists {
			return nil, fmt.Errorf("AI 标题清洗重复返回 media_id: %s", item.MediaID)
		}
		seen[item.MediaID] = struct{}{}
		item.Title = strings.Join(strings.Fields(strings.TrimSpace(item.Title)), " ")
		if item.Title == "" || len([]rune(item.Title)) > 255 {
			return nil, fmt.Errorf("AI 标题清洗返回了无效标题: %s", item.MediaID)
		}
		if item.Year < 0 || item.Year > 2100 {
			return nil, fmt.Errorf("AI 标题清洗返回了无效年份: %s", item.MediaID)
		}
		if item.Confidence < 0 || item.Confidence > 1 {
			return nil, fmt.Errorf("AI 标题清洗返回了无效置信度: %s", item.MediaID)
		}
		item.Reason = strings.TrimSpace(item.Reason)
		item.CurrentTitle = source.CurrentTitle
		item.SourceDirectory = source.SourceDirectory
		item.Filename = source.Filename
	}
	sort.SliceStable(suggestions, func(i, j int) bool {
		return order[suggestions[i].MediaID] < order[suggestions[j].MediaID]
	})
	return suggestions, nil
}

func mediaTitleCleanupItemCount(groups []MediaTitleCleanupGroup) int {
	total := 0
	for _, group := range groups {
		total += len(group.Items)
	}
	return total
}

func mediaTitleCleanupSourceOrder(items []MediaTitleCleanupSource, mediaID string) int {
	for index, item := range items {
		if item.MediaID == mediaID {
			return index
		}
	}
	return len(items)
}
