package service

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"go.uber.org/zap"

	"github.com/ShukeBta/MediaStationGo/internal/config"
)

func TestCleanMediaTitlesOnlyRunsNormalization(t *testing.T) {
	var calls atomic.Int32
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		system, _ := decodeAIRequestMessages(t, r)
		calls.Add(1)
		if !strings.Contains(system, "标题标准化器") || !strings.Contains(system, "不判断、推测或输出文件之间的聚合关系") {
			t.Fatalf("unexpected prompt: %s", system)
		}
		w.Header().Set("Content-Type", "application/json")
		writeAIContent(t, w, `{"items":[{"media_id":"a","title":"作品 A","year":2025,"confidence":0.96,"reason":"去除画质"},{"media_id":"b","title":"作品 A","year":2025,"confidence":0.91,"reason":"去除编码"}]}`)
	}))
	defer server.Close()

	items, err := newTestTitleCleanupAI(server.URL).CleanMediaTitles(t.Context(), []MediaTitleCleanupGroup{{
		SourceDirectory: "作品 A",
		Items: []MediaTitleCleanupSource{
			{MediaID: "a", CurrentTitle: "A.1080p", SourceDirectory: "作品 A", Filename: "A.1080p.mkv"},
			{MediaID: "b", CurrentTitle: "A.2160p", SourceDirectory: "作品 A", Filename: "A.2160p.mkv"},
		},
	}})
	if err != nil {
		t.Fatal(err)
	}
	if calls.Load() != 1 || len(items) != 2 {
		t.Fatalf("calls=%d items=%#v", calls.Load(), items)
	}
	if items[0].Title != "作品 A" || items[0].Confidence != 0.96 || items[0].CurrentTitle != "A.1080p" {
		t.Fatalf("unexpected suggestion: %#v", items[0])
	}
}

func TestCleanMediaTitlesRetriesInvalidNormalizationWithValidationFeedback(t *testing.T) {
	var calls atomic.Int32
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		system, user := decodeAIRequestMessages(t, r)
		call := calls.Add(1)
		w.Header().Set("Content-Type", "application/json")
		if call == 1 {
			writeAIContent(t, w, `{"items":[{"media_id":"a","title":"","year":0,"confidence":0.9,"reason":"空标题"}]}`)
			return
		}
		if !strings.Contains(system, "上一轮输出未通过服务端校验") ||
			!strings.Contains(user, "无效标题") || !strings.Contains(user, `"previous_output"`) {
			t.Fatalf("correction request lacks validation context: system=%s user=%s", system, user)
		}
		writeAIContent(t, w, `{"items":[{"media_id":"a","title":"作品 A","year":0,"confidence":0.9,"reason":"修正空标题"}]}`)
	}))
	defer server.Close()

	items, err := newTestTitleCleanupAI(server.URL).CleanMediaTitles(t.Context(), []MediaTitleCleanupGroup{{
		Items: []MediaTitleCleanupSource{{MediaID: "a", Filename: "a.mp4"}},
	}})
	if err != nil {
		t.Fatal(err)
	}
	if calls.Load() != 2 || len(items) != 1 || items[0].Title != "作品 A" {
		t.Fatalf("calls=%d items=%#v", calls.Load(), items)
	}
}

func TestCleanMediaTitlePromptPrioritizesChineseDescriptions(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		system, _ := decodeAIRequestMessages(t, r)
		for _, rule := range []string{
			"标题优先提取有语义的中文描述",
			"数字只是弱证据",
			"孤立数字 + 空格 + 中文描述",
			"当前标题只是弱参考",
			"不得输出“第10段”",
			"人物或主体 + 核心情节/场景 + 一个有辨识度的特征",
			"不得只输出“小敏儿 以性换租”",
			"不得输出 relation、group_key、group_title、part_index",
		} {
			if !strings.Contains(system, rule) {
				t.Fatalf("normalization prompt missing rule %q", rule)
			}
		}
		w.Header().Set("Content-Type", "application/json")
		writeAIContent(t, w, `{"items":[{"media_id":"main","title":"玩偶姐姐 穿着可爱的黑色JK超诱惑黑丝","year":0,"confidence":0.9,"reason":"中文描述优先"},{"media_id":"ad","title":"私房猛药 提高硬度 延时不射","year":0,"confidence":0.8,"reason":"保留文件独立语义"}]}`)
	}))
	defer server.Close()

	items, err := newTestTitleCleanupAI(server.URL).CleanMediaTitles(t.Context(), []MediaTitleCleanupGroup{{
		SourceDirectory: "玩偶姐姐hongkongdoll-111最新会员私信短片 穿着可爱的黑色JK超诱惑黑丝",
		Items: []MediaTitleCleanupSource{
			{MediaID: "main", Filename: "玩偶姐姐hongkongdoll-111.mp4"},
			{MediaID: "ad", Filename: "私房猛药，提高硬度，延时不射.mp4"},
		},
	}})
	if err != nil {
		t.Fatal(err)
	}
	if len(items) != 2 || strings.Contains(items[0].Title, "111") {
		t.Fatalf("unexpected suggestions: %#v", items)
	}
}

func TestCleanMediaTitlesRunsWithBoundedConcurrency(t *testing.T) {
	var active atomic.Int32
	var maxActive atomic.Int32
	var calls atomic.Int32
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_, user := decodeAIRequestMessages(t, r)
		current := active.Add(1)
		for {
			previous := maxActive.Load()
			if current <= previous || maxActive.CompareAndSwap(previous, current) {
				break
			}
		}
		time.Sleep(60 * time.Millisecond)
		active.Add(-1)
		calls.Add(1)
		id := cleanupRequestMediaID(t, user)
		w.Header().Set("Content-Type", "application/json")
		writeAIContent(t, w, `{"items":[{"media_id":"`+id+`","title":"作品 `+id+`","year":0,"confidence":0.9,"reason":"清洗"}]}`)
	}))
	defer server.Close()

	groups := make([]MediaTitleCleanupGroup, 3)
	for i, id := range []string{"a", "b", "c"} {
		groups[i] = MediaTitleCleanupGroup{SourceDirectory: id, Items: []MediaTitleCleanupSource{{MediaID: id, Filename: id + ".mp4"}}}
	}
	var mu sync.Mutex
	progress := 0
	items, err := newTestTitleCleanupAI(server.URL).CleanMediaTitlesWithProgress(t.Context(), groups, func(stage string, done, total int) {
		mu.Lock()
		defer mu.Unlock()
		if stage != mediaTitleCleanupStageCleaning || total != 3 || done <= progress {
			t.Fatalf("progress stage=%s value=%d/%d previous=%d", stage, done, total, progress)
		}
		progress = done
	})
	if err != nil {
		t.Fatal(err)
	}
	if len(items) != 3 || progress != 3 || calls.Load() != 3 || maxActive.Load() < 2 || maxActive.Load() > 3 {
		t.Fatalf("items=%d progress=%d calls=%d max_active=%d", len(items), progress, calls.Load(), maxActive.Load())
	}
}

func TestCleanMediaTitlesRejectsAggregationFields(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		writeAIContent(t, w, `{"items":[{"media_id":"a","title":"作品 A","year":0,"confidence":0.9,"relation":"part"}]}`)
	}))
	defer server.Close()

	_, err := newTestTitleCleanupAI(server.URL).CleanMediaTitles(t.Context(), []MediaTitleCleanupGroup{{
		Items: []MediaTitleCleanupSource{{MediaID: "a", Filename: "a.mp4"}},
	}})
	if err == nil || !strings.Contains(err.Error(), "unknown field") {
		t.Fatalf("expected strict unknown-field error, got %v", err)
	}
}

func newTestTitleCleanupAI(apiBase string) *AIService {
	return NewAIService(&config.Config{AI: config.AIConfig{
		Enabled: true, APIBase: apiBase, APIKey: "test", Model: "test",
	}}, zap.NewNop(), nil)
}

func decodeAIRequestMessages(t *testing.T, r *http.Request) (string, string) {
	t.Helper()
	var request struct {
		Stream      bool    `json:"stream"`
		Temperature float64 `json:"temperature"`
		Messages    []struct {
			Content string `json:"content"`
		} `json:"messages"`
	}
	if err := json.NewDecoder(r.Body).Decode(&request); err != nil {
		t.Fatal(err)
	}
	if request.Stream || request.Temperature != 0 || len(request.Messages) < 2 {
		t.Fatalf("unexpected AI request: %#v", request)
	}
	return request.Messages[0].Content, request.Messages[1].Content
}

func writeAIContent(t *testing.T, w http.ResponseWriter, content string) {
	t.Helper()
	body, err := json.Marshal(map[string]any{"choices": []any{map[string]any{"message": map[string]any{"content": content}}}})
	if err != nil {
		t.Fatal(err)
	}
	_, _ = w.Write(body)
}

func cleanupRequestMediaID(t *testing.T, user string) string {
	t.Helper()
	var payload map[string]json.RawMessage
	if err := json.Unmarshal([]byte(user), &payload); err != nil {
		t.Fatal(err)
	}
	var items []struct {
		MediaID string `json:"media_id"`
	}
	if raw, ok := payload["items"]; ok && json.Unmarshal(raw, &items) == nil && len(items) > 0 {
		return items[0].MediaID
	}
	t.Fatalf("media id not found in request: %s", user)
	return ""
}
