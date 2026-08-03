// Package service — M-Team site adapter.
package service

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"net/url"
	"regexp"
	"sort"
	"strconv"
	"strings"
	"time"
)

// ─── MTeam 适配器 ────────────────────────────────────────────────────────────

// MTeamAdapter MTeam.cc 独立站适配器。
type MTeamAdapter struct {
	client *http.Client
}

// NewMTeamAdapter 创建 MTeam 适配器。
func NewMTeamAdapter() *MTeamAdapter {
	return &MTeamAdapter{
		client: &http.Client{Timeout: 60 * time.Second},
	}
}

func (a *MTeamAdapter) Authenticate(ctx context.Context, cfg SiteConfig) error {
	if strings.TrimSpace(cfg.APIKey) == "" {
		return fmt.Errorf("M-Team 需要填写 API Access Token（控制台 → 实验室 → 存取令牌），不能使用 Cookie 访问开放 API")
	}
	// 与旧版参考实现对齐：
	// 用 camelCase 参数（pageNumber / pageSize），同时接受 code 为字符串 "0"
	// 或数值 0；兼容 M-Team v3 API 不同版本的返回。
	if err := reserveMTeamAPIQuota(ctx, cfg, mteamAPIEndpointSearch); err != nil {
		return fmt.Errorf("authenticate: %w", err)
	}
	u := cfg.URL + "/api/torrent/search"
	payload := `{"pageNumber":1,"pageSize":1,"mode":"all"}`
	data, status, err := doRequestJSON(ctx, a.client, "POST", u, cfg, []byte(payload))
	if err != nil {
		return mteamRequestError("authenticate", cfg, err)
	}
	preview := string(data)
	if len(preview) > 400 {
		preview = preview[:400] + "..."
	}
	if status == http.StatusUnauthorized || status == http.StatusForbidden {
		return fmt.Errorf("authentication failed: status %d, body=%s", status, preview)
	}
	if status >= 300 && status < 400 {
		return fmt.Errorf("authentication failed: HTTP %d (API Key 无效或未登录), body=%s", status, preview)
	}
	if status != http.StatusOK {
		return fmt.Errorf("authenticate failed: status %d, body=%s", status, preview)
	}
	var resp map[string]interface{}
	if err := json.Unmarshal(data, &resp); err != nil {
		return fmt.Errorf("parse response: %w (body=%s)", err, preview)
	}
	if mteamCodeOK(resp["code"]) {
		return nil
	}
	msg, _ := resp["message"].(string)
	if msg == "" {
		msg = fmt.Sprintf("code=%s", mteamCodeString(resp["code"]))
	}
	return fmt.Errorf("authentication failed: %s (body=%s)", msg, preview)
}

func (a *MTeamAdapter) Search(ctx context.Context, cfg SiteConfig, keyword string, page int) (*SiteSearchResult, error) {
	return a.SearchWithCategoryMode(ctx, cfg, keyword, "", page, false)
}

func (a *MTeamAdapter) SearchWithCategory(ctx context.Context, cfg SiteConfig, keyword, category string, page int) (*SiteSearchResult, error) {
	return a.SearchWithCategoryMode(ctx, cfg, keyword, category, page, false)
}

func (a *MTeamAdapter) SearchWithCategoryMode(ctx context.Context, cfg SiteConfig, keyword, category string, page int, includeAdult bool) (*SiteSearchResult, error) {
	// 与参考项目对齐：使用 camelCase 字段名，page 从 1 开始。
	if page <= 0 {
		page = 1
	}
	payload := mteamSearchPayload(keyword, category, page, includeAdult)
	body, _ := json.Marshal(payload)

	if err := reserveMTeamAPIQuota(ctx, cfg, mteamAPIEndpointSearch); err != nil {
		return nil, err
	}
	u := cfg.URL + "/api/torrent/search"
	data, status, err := doRequestJSON(ctx, a.client, "POST", u, cfg, body)
	if err != nil {
		return nil, mteamRequestError("search", cfg, err)
	}
	if status != http.StatusOK {
		return nil, fmt.Errorf("search failed: status %d", status)
	}

	return parseMTeamJSON(data, cfg.Name, cfg.URL)
}

func (a *MTeamAdapter) Browse(ctx context.Context, cfg SiteConfig, category string, page int) (*SiteSearchResult, error) {
	return a.BrowseWithMode(ctx, cfg, category, page, false)
}

func (a *MTeamAdapter) BrowseWithMode(ctx context.Context, cfg SiteConfig, category string, page int, includeAdult bool) (*SiteSearchResult, error) {
	if page <= 0 {
		page = 1
	}
	payload := mteamSearchPayload("", category, page, includeAdult)
	body, _ := json.Marshal(payload)

	if err := reserveMTeamAPIQuota(ctx, cfg, mteamAPIEndpointSearch); err != nil {
		return nil, err
	}
	u := cfg.URL + "/api/torrent/search"
	data, status, err := doRequestJSON(ctx, a.client, "POST", u, cfg, body)
	if err != nil {
		return nil, mteamRequestError("browse", cfg, err)
	}
	if status != http.StatusOK {
		return nil, fmt.Errorf("browse failed: status %d", status)
	}

	return parseMTeamJSON(data, cfg.Name, cfg.URL)
}

func mteamSearchPayload(keyword, category string, page int, includeAdult bool) map[string]interface{} {
	payload := map[string]interface{}{
		"keyword":    strings.TrimSpace(keyword),
		"pageNumber": page,
		"pageSize":   50,
		"mode":       mteamSearchMode(category, includeAdult),
	}
	if categories := mteamCategoryIDs(category); len(categories) > 0 {
		payload["categories"] = categories
	}
	return payload
}

func mteamSearchMode(category string, includeAdult bool) string {
	category = strings.ToLower(strings.TrimSpace(category))
	if includeAdult || looksAdultPTResource(category) {
		return "adult"
	}
	switch category {
	case "movie", "music", "tvshow", "waterfall", "rss", "rankings", "all":
		return category
	default:
		return "normal"
	}
}

func mteamCategoryIDs(category string) []int64 {
	category = strings.TrimSpace(category)
	if category == "" {
		return nil
	}
	out := []int64{}
	for _, part := range strings.FieldsFunc(category, func(r rune) bool {
		return r == ',' || r == '，' || r == '/' || r == '|' || r == '、'
	}) {
		part = strings.TrimSpace(part)
		if part == "" {
			continue
		}
		if id, err := strconv.ParseInt(part, 10, 64); err == nil && id > 0 {
			out = append(out, id)
		}
	}
	return out
}

func (a *MTeamAdapter) Categories(ctx context.Context, cfg SiteConfig) ([]SiteCategory, error) {
	var (
		lastErr   error
		best      []SiteCategory
		bestScore int
	)
	for _, endpoint := range []struct {
		method string
		path   string
		body   []byte
	}{
		{method: "POST", path: "/api/torrent/categoryList", body: []byte("{}")},
		{method: "GET", path: "/api/torrent/categoryList"},
		{method: "GET", path: "/api/torrent/categories"},
		{method: "GET", path: "/api/torrent/category"},
	} {
		data, status, err := doRequestJSON(ctx, a.client, endpoint.method, cfg.URL+endpoint.path, cfg, endpoint.body)
		if err != nil {
			lastErr = mteamRequestError("categories", cfg, err)
			continue
		}
		if status != http.StatusOK {
			lastErr = fmt.Errorf("category %s failed: status %d", endpoint.path, status)
			continue
		}
		cats, err := parseMTeamCategoriesJSON(data, cfg.Type)
		if err != nil {
			lastErr = err
			continue
		}
		if len(cats) > 0 {
			score := siteCategoryQualityScore(cats)
			if score > bestScore {
				best = cats
				bestScore = score
			}
		}
	}
	if len(best) > 0 {
		return best, nil
	}
	if lastErr != nil {
		return nil, lastErr
	}
	return nil, fmt.Errorf("mteam categories unavailable")
}

func siteCategoryQualityScore(cats []SiteCategory) int {
	score := len(cats)
	for _, cat := range cats {
		if strings.TrimSpace(cat.Name) != "" && strings.TrimSpace(cat.Name) != strings.TrimSpace(cat.ID) {
			score += 10
		}
	}
	return score
}

func (a *MTeamAdapter) GetDetail(ctx context.Context, cfg SiteConfig, id string) (*TorrentDetail, error) {
	var (
		data    []byte
		status  int
		err     error
		lastErr error
	)
	for _, attempt := range []struct {
		method string
		url    string
		body   []byte
	}{
		{method: "POST", url: cfg.URL + "/api/torrent/detail?id=" + url.QueryEscape(id) + "&origin=0", body: []byte("{}")},
		{method: "POST", url: cfg.URL + "/api/torrent/detail?id=" + url.QueryEscape(id), body: []byte("{}")},
		{method: "POST", url: cfg.URL + "/api/torrent/detail", body: []byte(`{"id":"` + id + `"}`)},
		{method: "GET", url: cfg.URL + "/api/torrent/detail?id=" + url.QueryEscape(id)},
	} {
		data, status, err = doRequestJSON(ctx, a.client, attempt.method, attempt.url, cfg, attempt.body)
		if err != nil {
			lastErr = mteamRequestError("detail", cfg, err)
			continue
		}
		if status == http.StatusOK && mteamResponseCodeOK(data) {
			lastErr = nil
			break
		}
		if status == http.StatusOK {
			lastErr = mteamResponseError(data)
			if isSitePortalRateLimitError(lastErr) {
				break
			}
		} else {
			lastErr = fmt.Errorf("detail failed: status %d", status)
			if status == http.StatusTooManyRequests {
				break
			}
		}
	}
	if lastErr != nil {
		return nil, lastErr
	}

	var resp map[string]interface{}
	if err := json.Unmarshal(data, &resp); err != nil {
		return nil, fmt.Errorf("parse response: %w", err)
	}

	dataField, ok := resp["data"].(map[string]interface{})
	if !ok {
		return nil, fmt.Errorf("detail not found")
	}

	detail := &TorrentDetail{
		ID:        id,
		DetailURL: cfg.URL + "/detail/" + id,
	}

	releaseTitle := mteamStringField(dataField, "name", "title")
	detail.Title = mteamDisplayTitle(dataField, releaseTitle)
	detail.Subtitle = mteamSubtitle(dataField, detail.Title, releaseTitle)
	detail.Category = mteamCategoryName(dataField)
	detail.PosterURL = mteamPosterURL(dataField, cfg.URL)
	detail.BackdropURL = mteamBackdropURL(dataField, cfg.URL)
	detail.Size = mteamSize(dataField)
	detail.Seeders, detail.Leechers, detail.Snatched = mteamStatus(dataField)
	detail.Free = mteamFree(dataField)
	detail.UploadTime = mteamUploadTime(dataField)
	detail.DownloadURL = firstNonEmpty(
		mteamStringField(dataField, "download", "downloadUrl", "downloadURL"),
		cfg.URL+"/api/torrent/genDlToken?id="+id,
	)
	detail.InfoHash = mteamStringField(dataField, "infoHash", "info_hash", "hash")
	detail.ImdbID = NormalizeIMDBID(mteamStringField(dataField, "imdb", "imdbId", "imdbID", "imdb_id"))
	detail.DoubanID = NormalizeDoubanID(mteamStringField(dataField, "douban", "doubanId", "doubanID", "douban_id"))
	if tmdbID := NormalizeTMDbID(firstNonEmpty(
		mteamStringField(dataField, "tmdb", "tmdbId", "tmdbID", "tmdb_id"),
		mteamNestedStringField(dataField, "movieInfo", "tmdb", "tmdbId", "tmdbID", "id"),
	)); tmdbID > 0 {
		detail.TMDbID = strconv.Itoa(tmdbID)
	}
	detail.Year = mteamYear(dataField)
	detail.Rating = mteamRating(dataField)
	detail.Genres = mteamStringList(dataField, "genres", "genre")
	detail.Tags = mteamTags(dataField)
	detail.Images = mteamImages(dataField, cfg.URL)
	detail.Description = mteamDescription(dataField, cfg.URL)
	detail.Files = mteamFiles(dataField)
	if detail.PosterURL == "" && len(detail.Images) > 0 {
		detail.PosterURL = detail.Images[0]
	}
	if detail.BackdropURL == "" && len(detail.Images) > 1 {
		detail.BackdropURL = detail.Images[1]
	}

	return detail, nil
}

// GetDownloadURL 解析 M-Team 种子的真实下载链接。
//
// M-Team v3 流程：
//
//	POST /api/torrent/genDlToken?id={tid}     (带 x-api-key)
//	→ {"code":"0","data":"https://api.m-team.cc/api/rss/dlv2?sign=..."}
//
// 拿到的 sign URL 可被任何下载客户端无认证地直接 GET。这是旧版参考实现
// _download_torrent_file 方法的子集。
func (a *MTeamAdapter) GetDownloadURL(ctx context.Context, cfg SiteConfig, id string) (string, error) {
	if err := reserveMTeamAPIQuota(ctx, cfg, mteamAPIEndpointDownload); err != nil {
		return "", err
	}
	u := cfg.URL + "/api/torrent/genDlToken?id=" + id
	// genDlToken 是 POST 但参数走 query string；body 留空。
	data, status, err := doRequestJSON(ctx, a.client, "POST", u, cfg, []byte("{}"))
	if err != nil {
		return "", mteamRequestError("genDlToken", cfg, err)
	}
	if status >= 300 {
		return "", fmt.Errorf("genDlToken: HTTP %d", status)
	}
	var resp map[string]interface{}
	if err := json.Unmarshal(data, &resp); err != nil {
		return "", fmt.Errorf("genDlToken parse: %w", err)
	}
	codeStr := ""
	switch v := resp["code"].(type) {
	case string:
		codeStr = v
	case float64:
		codeStr = strconv.Itoa(int(v))
	}
	if codeStr != "0" && codeStr != "200" {
		msg, _ := resp["message"].(string)
		if msg == "" {
			msg = "unknown error"
		}
		return "", fmt.Errorf("genDlToken: %s", msg)
	}
	dl, _ := resp["data"].(string)
	if dl == "" {
		return "", fmt.Errorf("genDlToken: empty data field")
	}
	return dl, nil
}

func mteamRequestError(action string, cfg SiteConfig, err error) error {
	if err == nil {
		return nil
	}
	if isSiteRequestTimeout(err) {
		timeout := cfg.Timeout
		if timeout <= 0 {
			timeout = 30 * time.Second
		}
		return fmt.Errorf("%s: M-Team API request timed out after %s; check Docker/IPv6/proxy access to api.m-team.cc or increase the site timeout to 45-60s: %w",
			action, timeout.Round(time.Second), err)
	}
	return fmt.Errorf("%s: %w", action, err)
}

func isSiteRequestTimeout(err error) bool {
	if errors.Is(err, context.DeadlineExceeded) {
		return true
	}
	var timeout interface{ Timeout() bool }
	return errors.As(err, &timeout) && timeout.Timeout()
}

// parseMTeamJSON 解析 MTeam v3 JSON 响应。
//
// 响应结构（与旧版参考实现一致）：
//
//	{
//	  "code": "0",          // 字符串 "0" 表示成功
//	  "message": "SUCCESS",
//	  "data": {
//	    "total": "123",
//	    "data": [ ... ]    // 旧字段名 "lists" 已被替换为 "data"
//	  }
//	}
func parseMTeamJSON(data []byte, siteName, baseURL string) (*SiteSearchResult, error) {
	// 用 map 反序列化以兼容 code/total 既可能是字符串又可能是数字。
	var raw map[string]interface{}
	if err := json.Unmarshal(data, &raw); err != nil {
		return nil, fmt.Errorf("parse JSON: %w", err)
	}

	// code 兼容字符串与数字。
	codeStr := ""
	switch v := raw["code"].(type) {
	case string:
		codeStr = v
	case float64:
		codeStr = strconv.Itoa(int(v))
	}
	if codeStr != "" && codeStr != "0" && codeStr != "200" {
		msg, _ := raw["message"].(string)
		if msg == "" {
			msg = fmt.Sprintf("code=%s", codeStr)
		}
		return nil, fmt.Errorf("mteam: %s", msg)
	}

	dataField, _ := raw["data"].(map[string]interface{})
	if dataField == nil {
		return &SiteSearchResult{SiteName: siteName, Items: []TorrentItem{}}, nil
	}

	// total 兼容字符串与数字。
	total := 0
	switch v := dataField["total"].(type) {
	case string:
		total, _ = strconv.Atoi(v)
	case float64:
		total = int(v)
	}

	// data.data（v3）优先；兜底兼容旧的 data.lists。
	var rows []interface{}
	switch v := dataField["data"].(type) {
	case []interface{}:
		rows = v
	}
	if rows == nil {
		if v, ok := dataField["lists"].([]interface{}); ok {
			rows = v
		}
	}

	result := &SiteSearchResult{
		SiteName: siteName,
		Items:    []TorrentItem{},
		Total:    total,
	}

	for _, rawT := range rows {
		t, ok := rawT.(map[string]interface{})
		if !ok {
			continue
		}
		item := TorrentItem{}
		if v, ok := t["id"].(string); ok {
			item.ID = v
		} else if v, ok := t["id"].(float64); ok {
			item.ID = strconv.Itoa(int(v))
		}
		releaseTitle := mteamStringField(t, "name", "title")
		item.Title = mteamDisplayTitle(t, releaseTitle)
		item.Subtitle = mteamSubtitle(t, item.Title, releaseTitle)
		listImages := mteamImageList(t, baseURL)
		if len(listImages) > 0 {
			item.PosterURL = listImages[0]
		}
		if len(listImages) > 1 {
			item.BackdropURL = listImages[1]
		}
		if item.PosterURL == "" {
			item.PosterURL = mteamPosterURL(t, baseURL)
		}
		if item.BackdropURL == "" {
			item.BackdropURL = mteamBackdropURL(t, baseURL)
		}
		if item.PosterURL == "" || item.BackdropURL == "" {
			images := mteamImages(t, baseURL)
			if item.PosterURL == "" && len(images) > 0 {
				item.PosterURL = images[0]
			}
			if item.BackdropURL == "" && len(images) > 1 {
				item.BackdropURL = images[1]
			}
		}
		item.Category = mteamCategoryName(t)
		item.Size = mteamSize(t)
		item.Seeders, item.Leechers, item.Snatched = mteamStatus(t)
		item.Free = mteamFree(t)
		item.UploadTime = mteamUploadTime(t)

		item.DetailURL = baseURL + "/detail/" + item.ID
		// 标记 download_url 指向 genDlToken；真正的下载链接由 handler 层
		// 在用户点"下载"时通过 MTeamAdapter.GetDownloadURL 解析。
		// 这样前端 SiteSearchPage 才知道这一行有可用的下载入口。
		item.DownloadURL = baseURL + "/api/torrent/genDlToken?id=" + item.ID
		result.Items = append(result.Items, item)
	}

	return result, nil
}

func mteamResponseCodeOK(data []byte) bool {
	var raw map[string]interface{}
	if err := json.Unmarshal(data, &raw); err != nil {
		return true
	}
	code := mteamCodeString(raw["code"])
	return code == "" || code == "0" || code == "200"
}

func mteamResponseError(data []byte) error {
	var raw map[string]interface{}
	if err := json.Unmarshal(data, &raw); err != nil {
		return fmt.Errorf("mteam detail failed: parse response: %w", err)
	}
	code := mteamCodeString(raw["code"])
	msg := mteamStringField(raw, "message", "msg", "error")
	if msg == "" {
		msg = fmt.Sprintf("code=%s", firstNonEmpty(code, "unknown"))
	}
	return fmt.Errorf("mteam: %s", msg)
}

func parseMTeamCategoriesJSON(data []byte, siteType string) ([]SiteCategory, error) {
	var raw map[string]interface{}
	if err := json.Unmarshal(data, &raw); err != nil {
		return nil, fmt.Errorf("parse categories: %w", err)
	}
	code := mteamCodeString(raw["code"])
	if code != "" && code != "0" && code != "200" {
		msg, _ := raw["message"].(string)
		if msg == "" {
			msg = fmt.Sprintf("code=%s", code)
		}
		return nil, fmt.Errorf("mteam categories: %s", msg)
	}
	payload := any(raw)
	if dataField, ok := raw["data"]; ok && dataField != nil {
		payload = dataField
	}
	cats := mteamCollectSiteCategoriesFromJSON(payload, siteType, "")
	cats = mteamDedupeSiteCategories(cats)
	if strings.EqualFold(strings.TrimSpace(siteType), "mteam") {
		cats = normalizeMTeamOfficialCategories(cats)
	}
	return cats, nil
}

func mteamCollectSiteCategoriesFromJSON(value any, siteType, group string) []SiteCategory {
	switch v := value.(type) {
	case []interface{}:
		out := make([]SiteCategory, 0, len(v))
		for _, item := range v {
			out = append(out, mteamCollectSiteCategoriesFromJSON(item, siteType, group)...)
		}
		return out
	case map[string]interface{}:
		if cat, ok := categoryFromJSONObject(v, siteType, group); ok {
			out := []SiteCategory{cat}
			for _, key := range []string{"children", "items", "list", "data", "subCategories"} {
				if child, exists := v[key]; exists {
					out = append(out, mteamCollectSiteCategoriesFromJSON(child, siteType, cat.Name)...)
				}
			}
			return out
		}
		out := []SiteCategory{}
		for key, child := range v {
			nextGroup := group
			if key != "data" && key != "list" && key != "items" && key != "children" {
				nextGroup = inferSiteCategoryGroup(key, key)
			}
			if label, ok := child.(string); ok && strings.TrimSpace(label) != "" && looksCategoryID(key) {
				out = append(out, SiteCategory{
					ID:       strings.TrimSpace(key),
					Name:     strings.TrimSpace(label),
					Group:    inferSiteCategoryGroup(label, key),
					SiteType: siteType,
					Adult:    looksAdultPTResource(label + " " + key),
				})
				continue
			}
			out = append(out, mteamCollectSiteCategoriesFromJSON(child, siteType, nextGroup)...)
		}
		return out
	default:
		return nil
	}
}

func categoryFromJSONObject(obj map[string]interface{}, siteType, group string) (SiteCategory, bool) {
	id := mteamStringField(obj, "id", "value", "categoryId", "cat", "code")
	parentID := mteamStringField(obj, "parent", "parentId", "parentID", "pid")
	name := mteamStringField(
		obj,
		"categoryName", "displayName", "className",
		"nameChs", "nameCht", "nameEng",
		"zhName", "cnName", "nameZh", "nameCn", "chs", "zh",
		"titleZh", "titleCn", "labelZh", "labelCn",
		"name", "label", "title", "text",
	)
	if cjk := firstCJKString(
		mteamStringField(obj, "nameChs", "nameCht", "zhName", "cnName", "nameZh", "nameCn", "chs", "zh", "titleZh", "titleCn", "labelZh", "labelCn"),
		name,
	); cjk != "" {
		name = cjk
	}
	if id == "" && name == "" {
		return SiteCategory{}, false
	}
	if name == "" || strings.EqualFold(strings.TrimSpace(name), strings.TrimSpace(id)) {
		if fallback := defaultSiteCategoryName(siteType, id); fallback != "" {
			name = fallback
		} else if name == "" {
			name = "原站分类 " + id
		}
	}
	if group == "" {
		group = inferSiteCategoryGroup(name, id)
	}
	return SiteCategory{
		ID:       strings.TrimSpace(id),
		Name:     strings.TrimSpace(name),
		Group:    group,
		ParentID: strings.TrimSpace(parentID),
		SiteType: siteType,
		Adult:    looksAdultPTResource(name + " " + id + " " + group),
	}, true
}

func mteamVisibleVideoCategory(cat SiteCategory) bool {
	if strings.TrimSpace(cat.ID) == "" && (strings.TrimSpace(cat.Name) == "" || strings.TrimSpace(cat.Name) == "全部") {
		return true
	}
	if mteamKnownVideoCategoryName(cat.ID) != "" || mteamKnownVideoCategoryName(cat.ParentID) != "" {
		return true
	}
	text := strings.ToLower(strings.TrimSpace(cat.Name + " " + cat.Group))
	switch {
	case strings.Contains(text, "movie") || strings.Contains(text, "电影") || strings.Contains(text, "電影"):
		return true
	case strings.Contains(text, "tv") || strings.Contains(text, "剧") || strings.Contains(text, "劇") || strings.Contains(text, "番剧") || strings.Contains(text, "番劇"):
		return true
	case strings.Contains(text, "anime") || strings.Contains(text, "animation") || strings.Contains(text, "动漫") || strings.Contains(text, "動畫") || strings.Contains(text, "动画"):
		return true
	case strings.Contains(text, "variety") || strings.Contains(text, "综艺") || strings.Contains(text, "綜藝"):
		return true
	case strings.Contains(text, "documentary") || strings.Contains(text, "纪录") || strings.Contains(text, "紀錄"):
		return true
	case (strings.Contains(text, "adult") || strings.Contains(text, "成人")) && (strings.Contains(text, "视频") || strings.Contains(text, "視頻") || strings.Contains(text, "影像") || strings.Contains(text, "图片") || strings.Contains(text, "圖片") || strings.Contains(text, "写真") || strings.Contains(text, "寫真") || strings.Contains(text, "动漫") || strings.Contains(text, "動畫")):
		return true
	default:
		return false
	}
}

func normalizeMTeamOfficialCategories(cats []SiteCategory) []SiteCategory {
	byID := map[string]SiteCategory{}
	for _, cat := range cats {
		if strings.TrimSpace(cat.ID) != "" {
			byID[strings.TrimSpace(cat.ID)] = cat
		}
	}
	out := make([]SiteCategory, 0, len(cats))
	for _, cat := range cats {
		cat.ID = strings.TrimSpace(cat.ID)
		cat.Name = strings.TrimSpace(cat.Name)
		cat.ParentID = strings.TrimSpace(cat.ParentID)
		if cat.Name == "" || strings.EqualFold(cat.Name, cat.ID) || strings.HasPrefix(cat.Name, "原站分类 ") {
			if name := mteamKnownVideoCategoryName(cat.ID); name != "" {
				cat.Name = name
			}
		}
		cat.Adult = cat.Adult || looksAdultPTResource(cat.Name+" "+cat.Group+" "+cat.ID)
		if cat.Adult {
			cat.Group = "成人"
		} else if parent, ok := byID[cat.ParentID]; ok && strings.TrimSpace(parent.Name) != "" {
			cat.Group = strings.TrimSpace(parent.Name)
		} else if name := mteamKnownVideoCategoryName(cat.ParentID); name != "" {
			cat.Group = name
		} else if mteamKnownVideoCategoryName(cat.ID) != "" {
			cat.Group = "影视"
		} else if cat.Group == "" || cat.Group == "原站" {
			cat.Group = inferSiteCategoryGroup(cat.Name, cat.ID)
		}
		out = append(out, cat)
	}
	return mteamDedupeSiteCategories(out)
}

func mteamKnownVideoCategoryName(id string) string {
	switch strings.TrimSpace(id) {
	case "100":
		return "电影"
	case "105":
		return "剧集"
	case "110":
		return "综艺"
	case "115":
		return "动漫"
	case "120":
		return "纪录片"
	case "401":
		return "电影"
	case "402":
		return "剧集"
	case "403":
		return "综艺"
	case "404":
		return "动漫"
	case "405":
		return "纪录片"
	case "420":
		return "成人视频"
	case "421":
		return "成人写真"
	case "422":
		return "成人动漫"
	case "423":
		return "成人三级"
	case "424":
		return "成人高清"
	case "425":
		return "成人无码"
	case "426":
		return "成人有码"
	case "427":
		return "成人素人"
	case "429":
		return "成人写真"
	case "430":
		return "成人影像"
	case "431":
		return "成人图片"
	case "432":
		return "成人动漫"
	case "433":
		return "成人游戏"
	case "434":
		return "成人电子书"
	case "435":
		return "成人软件"
	case "436":
		return "成人音乐"
	case "437":
		return "成人MV"
	case "438":
		return "成人纪录片"
	case "439":
		return "成人综艺"
	case "440":
		return "成人其他"
	case "442":
		return "写真"
	case "444":
		return "成人影像"
	case "445":
		return "成人图片"
	case "446":
		return "成人写真"
	case "447":
		return "成人视频"
	case "448":
		return "成人动漫"
	case "449":
		return "成人游戏"
	case "450":
		return "成人小说"
	case "451":
		return "成人其他"
	default:
		return ""
	}
}

func mteamDedupeSiteCategories(in []SiteCategory) []SiteCategory {
	out := make([]SiteCategory, 0, len(in))
	seen := map[string]struct{}{}
	for _, cat := range in {
		if strings.TrimSpace(cat.Name) == "" && strings.TrimSpace(cat.ID) == "" {
			continue
		}
		key := strings.ToLower(strings.TrimSpace(cat.ID) + "\x00" + strings.TrimSpace(cat.Name))
		if _, ok := seen[key]; ok {
			continue
		}
		seen[key] = struct{}{}
		out = append(out, cat)
	}
	sort.SliceStable(out, func(i, j int) bool {
		if out[i].Adult != out[j].Adult {
			return !out[i].Adult
		}
		if out[i].Group != out[j].Group {
			return out[i].Group < out[j].Group
		}
		return out[i].Name < out[j].Name
	})
	return out
}

func looksCategoryID(value string) bool {
	value = strings.TrimSpace(value)
	if value == "" || len(value) > 16 {
		return false
	}
	for _, ch := range value {
		if (ch < '0' || ch > '9') && (ch < 'a' || ch > 'z') && (ch < 'A' || ch > 'Z') && ch != '_' && ch != '-' {
			return false
		}
	}
	return true
}

func mteamDisplayTitle(obj map[string]interface{}, fallback string) string {
	candidates := []string{
		mteamNestedStringField(obj, "movieInfo", "chineseTitle", "titleZh", "titleCn", "nameZh", "nameCn", "cnName", "zhName", "translatedTitle", "localizedTitle"),
		mteamStringField(obj, "chineseTitle", "titleZh", "titleCn", "nameZh", "nameCn", "cnName", "zhName", "translatedTitle", "localizedTitle"),
		mteamNestedStringField(obj, "movieInfo", "title", "name"),
		mteamStringField(obj, "smallDescr", "smallDescription", "subTitle", "subtitle"),
		fallback,
		mteamStringField(obj, "name", "title"),
	}
	if cjk := firstCJKString(candidates...); cjk != "" {
		return cjk
	}
	return firstNonEmpty(candidates...)
}

func mteamSubtitle(obj map[string]interface{}, title, releaseTitle string) string {
	parts := []string{
		releaseTitle,
		mteamStringField(obj, "smallDescr", "smallDescription", "subTitle", "subtitle"),
		mteamNestedStringField(obj, "movieInfo", "originalTitle", "originalName", "originTitle"),
		mteamStringField(obj, "descriptionSmall", "descrSmall"),
	}
	return strings.Join(dedupeNonEmptyExcept(parts, title), " · ")
}

func mteamCategoryName(obj map[string]interface{}) string {
	if cat, ok := obj["category"].(map[string]interface{}); ok {
		if parsed, ok := categoryFromJSONObject(cat, "mteam", ""); ok {
			return parsed.Name
		}
	}
	if value := mteamStringField(obj, "categoryName", "categoryTitle", "typeName", "catName", "category"); value != "" {
		return value
	}
	if cat, ok := obj["category"].(string); ok {
		return strings.TrimSpace(cat)
	}
	return ""
}

func mteamPosterURL(obj map[string]interface{}, baseURL string) string {
	for _, parent := range []string{"movieInfo", "movie_info", "movie", "mediaInfo", "media_info", "media", "video", "tmdb"} {
		movie, ok := obj[parent].(map[string]interface{})
		if !ok {
			continue
		}
		if path := mteamStringField(movie, "posterPath", "poster_path"); path != "" {
			return tmdbImageURL(path, "w500")
		}
		if value := mteamStringField(movie, "poster", "posterUrl", "posterURL", "cover", "coverUrl", "image", "imageUrl", "smallCover", "thumbnail", "thumbnailUrl", "thumb", "thumbUrl", "pic", "picUrl", "img", "imgUrl"); value != "" {
			return mteamAbsolutizeImageURL(baseURL, value)
		}
	}
	if path := mteamStringField(obj, "posterPath", "poster_path"); path != "" {
		return tmdbImageURL(path, "w500")
	}
	if value := mteamStringField(obj, "poster", "posterUrl", "posterURL", "cover", "coverUrl", "image", "imageUrl", "smallCover", "coverImage", "coverImageUrl", "thumbnail", "thumbnailUrl", "thumb", "thumbUrl", "pic", "picUrl", "img", "imgUrl"); value != "" {
		return mteamAbsolutizeImageURL(baseURL, value)
	}
	return ""
}

func mteamBackdropURL(obj map[string]interface{}, baseURL string) string {
	for _, parent := range []string{"movieInfo", "movie_info", "movie", "mediaInfo", "media_info", "media", "video", "tmdb"} {
		movie, ok := obj[parent].(map[string]interface{})
		if !ok {
			continue
		}
		if path := mteamStringField(movie, "backdropPath", "backdrop_path"); path != "" {
			return tmdbImageURL(path, "w1280")
		}
		if value := mteamStringField(movie, "backdrop", "backdropUrl", "backdropURL", "background", "banner", "backgroundUrl", "backgroundImage", "backgroundImageUrl", "screenshot", "screenshotUrl"); value != "" {
			return mteamAbsolutizeImageURL(baseURL, value)
		}
	}
	if path := mteamStringField(obj, "backdropPath", "backdrop_path"); path != "" {
		return tmdbImageURL(path, "w1280")
	}
	if value := mteamStringField(obj, "backdrop", "backdropUrl", "backdropURL", "background", "banner", "backgroundUrl", "backgroundImage", "backgroundImageUrl", "screenshot", "screenshotUrl"); value != "" {
		return mteamAbsolutizeImageURL(baseURL, value)
	}
	return ""
}

func mteamAbsolutizeImageURL(baseURL, raw string) string {
	raw = strings.TrimSpace(raw)
	if raw == "" {
		return ""
	}
	if strings.HasPrefix(raw, "//") {
		return "https:" + raw
	}
	return absolutizeURL(baseURL, raw)
}

func tmdbImageURL(path, size string) string {
	path = strings.TrimSpace(path)
	if path == "" {
		return ""
	}
	if strings.HasPrefix(path, "http://") || strings.HasPrefix(path, "https://") {
		return path
	}
	if !strings.HasPrefix(path, "/") {
		path = "/" + path
	}
	if size == "" {
		size = "w500"
	}
	return "https://image.tmdb.org/t/p/" + size + path
}

func mteamSize(obj map[string]interface{}) int64 {
	if n := int64FromAny(obj["size"]); n > 0 {
		return n
	}
	raw := mteamStringField(obj, "sizeText", "size_text")
	if raw == "" {
		return 0
	}
	if m := regexp.MustCompile(`(?i)([0-9]+(?:\.[0-9]+)?)\s*([KMGT]?B)`).FindStringSubmatch(raw); len(m) == 3 {
		return parseSizeString(m[1], m[2])
	}
	return 0
}

func mteamStatus(obj map[string]interface{}) (int, int, int) {
	status, _ := obj["status"].(map[string]interface{})
	seeders := intFromAny(firstExisting(obj, status, "seeders", "seeder", "seed"))
	leechers := intFromAny(firstExisting(obj, status, "leechers", "leecher", "leech"))
	snatched := intFromAny(firstExisting(obj, status, "completed", "snatched", "finish", "downloads"))
	return seeders, leechers, snatched
}

func mteamFree(obj map[string]interface{}) bool {
	if boolFromAny(obj["free"]) || boolFromAny(obj["isFree"]) || boolFromAny(obj["freeTorrent"]) {
		return true
	}
	status, _ := obj["status"].(map[string]interface{})
	for _, raw := range []string{
		mteamStringField(obj, "discount", "discountType", "promotion", "saleStatus", "spState"),
		mteamStringField(status, "discount", "discountType", "promotion", "saleStatus", "spState"),
	} {
		lower := strings.ToLower(strings.TrimSpace(raw))
		if lower == "" {
			continue
		}
		if strings.Contains(lower, "free") || lower == "0" || lower == "100" || strings.Contains(lower, "免费") || strings.Contains(lower, "免費") {
			return true
		}
	}
	return false
}

func mteamUploadTime(obj map[string]interface{}) time.Time {
	for _, key := range []string{"uploadTime", "createdDate", "createdAt", "createTime", "publishTime", "added", "created"} {
		if ts := timeFromAny(obj[key]); !ts.IsZero() {
			return ts
		}
	}
	return time.Time{}
}

func mteamYear(obj map[string]interface{}) string {
	for _, raw := range []string{
		mteamNestedStringField(obj, "movieInfo", "year", "releaseYear"),
		mteamStringField(obj, "year", "releaseYear"),
		mteamNestedStringField(obj, "movieInfo", "releaseDate", "release_date", "firstAirDate"),
	} {
		if len(raw) >= 4 {
			return raw[:4]
		}
	}
	return ""
}

func mteamRating(obj map[string]interface{}) string {
	for _, raw := range []string{
		mteamNestedStringField(obj, "movieInfo", "voteAverage", "rating", "score"),
		mteamStringField(obj, "rating", "score"),
	} {
		if raw != "" && raw != "0" {
			return raw
		}
	}
	return ""
}

func mteamDescription(obj map[string]interface{}, baseURL string) string {
	parts := []string{
		mteamNestedStringField(obj, "movieInfo", "overview", "summary", "plot"),
		mteamCleanDescription(mteamStringField(obj, "description", "descr", "body", "intro")),
		mteamStringField(obj, "smallDescr", "smallDescription"),
	}
	return strings.Join(dedupeNonEmptyExcept(parts, ""), "\n\n")
}

func mteamImages(obj map[string]interface{}, baseURL string) []string {
	images := []string{}
	images = append(images, mteamImageList(obj, baseURL)...)
	for _, value := range []string{
		mteamPosterURL(obj, baseURL),
		mteamBackdropURL(obj, baseURL),
		mteamStringField(obj, "coverImage", "coverImageUrl", "image", "imageUrl"),
		mteamNestedStringField(obj, "movieInfo", "poster", "posterUrl", "cover", "coverUrl", "image", "imageUrl"),
		mteamNestedStringField(obj, "movieInfo", "backdrop", "backdropUrl", "background", "backgroundUrl"),
	} {
		if value != "" {
			images = append(images, mteamAbsolutizeImageURL(baseURL, value))
		}
	}
	for _, key := range []string{"description", "descr", "body", "intro"} {
		images = append(images, mteamImageURLsFromMarkup(baseURL, mteamStringField(obj, key))...)
	}
	images = append(images, mteamImageURLsFromAny(baseURL, obj, "", 0)...)
	return mteamDedupeStrings(images)
}

func mteamImageList(obj map[string]interface{}, baseURL string) []string {
	if obj == nil {
		return nil
	}
	images := []string{}
	for _, key := range []string{
		"imageList", "images", "imageUrls", "imageURLs", "imageURLList",
		"coverImages", "screenshots", "screenshotList", "photoList", "picList",
	} {
		if value, ok := obj[key]; ok {
			images = append(images, mteamImageURLsFromImageListValue(baseURL, value)...)
		}
	}
	for _, parent := range []string{"movieInfo", "movie_info", "movie", "mediaInfo", "media_info", "media", "video"} {
		if child, ok := obj[parent].(map[string]interface{}); ok {
			images = append(images, mteamImageList(child, baseURL)...)
		}
	}
	return mteamDedupeStrings(images)
}

func mteamImageURLsFromImageListValue(baseURL string, value interface{}) []string {
	if value == nil {
		return nil
	}
	out := []string{}
	switch v := value.(type) {
	case []interface{}:
		for _, child := range v {
			out = append(out, mteamImageURLsFromImageListValue(baseURL, child)...)
		}
	case []string:
		for _, child := range v {
			out = append(out, mteamImageURLsFromImageListValue(baseURL, child)...)
		}
	case map[string]interface{}:
		for _, key := range []string{"url", "src", "href", "image", "imageUrl", "imageURL", "cover", "coverUrl", "thumb", "thumbUrl"} {
			if raw := mteamStringField(v, key); raw != "" {
				if u := mteamCleanImageURL(baseURL, raw); u != "" {
					out = append(out, u)
				}
			}
		}
		out = append(out, mteamImageURLsFromAny(baseURL, v, "imageList", 0)...)
	case string:
		out = append(out, mteamImageURLsFromMarkup(baseURL, v)...)
		if u := mteamCleanImageURL(baseURL, v); u != "" {
			out = append(out, u)
		}
	}
	return mteamDedupeStrings(out)
}

func mteamImageURLsFromAny(baseURL string, value interface{}, keyHint string, depth int) []string {
	if depth > 6 || value == nil {
		return nil
	}
	out := []string{}
	switch v := value.(type) {
	case map[string]interface{}:
		for key, child := range v {
			out = append(out, mteamImageURLsFromAny(baseURL, child, key, depth+1)...)
		}
	case []interface{}:
		for _, child := range v {
			out = append(out, mteamImageURLsFromAny(baseURL, child, keyHint, depth+1)...)
		}
	case []string:
		for _, child := range v {
			out = append(out, mteamImageURLsFromAny(baseURL, child, keyHint, depth+1)...)
		}
	case string:
		out = append(out, mteamImageURLsFromMarkup(baseURL, v)...)
		if mteamLooksImageField(keyHint) || mteamLooksImageURL(v) {
			if u := mteamCleanImageURL(baseURL, v); u != "" {
				out = append(out, u)
			}
		}
	}
	return mteamDedupeStrings(out)
}

func mteamLooksImageField(key string) bool {
	key = strings.ToLower(strings.TrimSpace(key))
	if key == "" {
		return false
	}
	for _, part := range []string{"poster", "cover", "image", "img", "thumb", "thumbnail", "backdrop", "background", "banner", "screenshot", "photo", "pic"} {
		if strings.Contains(key, part) {
			return true
		}
	}
	return false
}

func mteamLooksImageURL(raw string) bool {
	raw = strings.TrimSpace(strings.Trim(raw, `"'`))
	if raw == "" {
		return false
	}
	lower := strings.ToLower(raw)
	if strings.HasPrefix(lower, "http://") || strings.HasPrefix(lower, "https://") || strings.HasPrefix(lower, "//") || strings.HasPrefix(lower, "/") {
		return strings.Contains(lower, ".jpg") || strings.Contains(lower, ".jpeg") || strings.Contains(lower, ".png") || strings.Contains(lower, ".webp") || strings.Contains(lower, ".gif") || strings.Contains(lower, "/images/")
	}
	return false
}

func mteamImageURLsFromMarkup(baseURL, body string) []string {
	body = strings.TrimSpace(body)
	if body == "" {
		return nil
	}
	out := []string{}
	for _, re := range []*regexp.Regexp{
		regexp.MustCompile(`(?is)<img[^>]+(?:src|data-src|data-original)=["']([^"']+)["']`),
		regexp.MustCompile(`(?is)!\[[^\]]*\]\(\s*([^) \t\r\n]+)[^)]*\)`),
		regexp.MustCompile(`(?is)\[img[^\]]*\]\s*(https?://[^\s\[]+)\s*(?:\[/img\])?`),
	} {
		for _, match := range re.FindAllStringSubmatch(body, -1) {
			if len(match) < 2 {
				continue
			}
			if u := mteamCleanImageURL(baseURL, match[1]); u != "" {
				out = append(out, u)
			}
		}
	}
	return mteamDedupeStrings(out)
}

func mteamCleanImageURL(baseURL, raw string) string {
	raw = strings.TrimSpace(raw)
	raw = strings.Trim(raw, `"'`)
	raw = strings.TrimRight(raw, ")]}")
	if raw == "" {
		return ""
	}
	u := mteamAbsolutizeImageURL(baseURL, raw)
	lower := strings.ToLower(u)
	if strings.Contains(lower, "cat") || strings.Contains(lower, "icon") || strings.Contains(lower, "spacer") {
		return ""
	}
	return u
}

func mteamCleanDescription(raw string) string {
	raw = strings.TrimSpace(raw)
	if raw == "" {
		return ""
	}
	cleaned := raw
	for _, re := range []*regexp.Regexp{
		regexp.MustCompile(`(?is)<img[^>]*>`),
		regexp.MustCompile(`(?is)!\[[^\]]*\]\(\s*[^)]+?\)`),
		regexp.MustCompile(`(?is)\[img[^\]]*\]\s*https?://[^\s\[]+\s*(?:\[/img\])?`),
	} {
		cleaned = re.ReplaceAllString(cleaned, "\n")
	}
	cleaned = stripHTML(cleaned)
	for _, re := range []*regexp.Regexp{
		regexp.MustCompile(`(?is)\[(?:/?)(?:b|i|u|size|color|font|align|quote|url)[^\]]*\]`),
		regexp.MustCompile(`\r\n?`),
		regexp.MustCompile(`[ \t]+\n`),
		regexp.MustCompile(`\n{3,}`),
	} {
		repl := ""
		if re.String() == `\r\n?` {
			repl = "\n"
		} else if re.String() == `[ \t]+\n` {
			repl = "\n"
		} else if re.String() == `\n{3,}` {
			repl = "\n\n"
		}
		cleaned = re.ReplaceAllString(cleaned, repl)
	}
	return strings.TrimSpace(cleaned)
}

func mteamTags(obj map[string]interface{}) []string {
	tags := []string{}
	tags = append(tags, mteamStringList(obj, "labels", "tags", "tagList")...)
	tags = append(tags, mteamStringListFromNested(obj, "movieInfo", "genres", "genre")...)
	return mteamDedupeStrings(tags)
}

func mteamStringList(obj map[string]interface{}, keys ...string) []string {
	for _, key := range keys {
		if value, ok := obj[key]; ok {
			if out := stringListFromAny(value); len(out) > 0 {
				return out
			}
		}
	}
	if movie, ok := obj["movieInfo"].(map[string]interface{}); ok {
		return mteamStringListFromNested(map[string]interface{}{"movieInfo": movie}, "movieInfo", keys...)
	}
	return nil
}

func mteamStringListFromNested(obj map[string]interface{}, parent string, keys ...string) []string {
	nested, ok := obj[parent].(map[string]interface{})
	if !ok {
		return nil
	}
	for _, key := range keys {
		if value, ok := nested[key]; ok {
			if out := stringListFromAny(value); len(out) > 0 {
				return out
			}
		}
	}
	return nil
}

func mteamFiles(obj map[string]interface{}) []string {
	for _, key := range []string{"files", "fileList", "file_list", "contents"} {
		if out := fileListFromAny(obj[key]); len(out) > 0 {
			return out
		}
	}
	return nil
}

func mteamStringField(obj map[string]interface{}, keys ...string) string {
	if obj == nil {
		return ""
	}
	for _, key := range keys {
		if v, ok := obj[key]; ok {
			switch val := v.(type) {
			case string:
				if strings.TrimSpace(val) != "" {
					return strings.TrimSpace(val)
				}
			case float64:
				if val != 0 {
					return strconv.Itoa(int(val))
				}
			case int:
				if val != 0 {
					return strconv.Itoa(val)
				}
			case json.Number:
				return val.String()
			}
		}
	}
	return ""
}

func mteamNestedStringField(obj map[string]interface{}, parent string, keys ...string) string {
	if nested, ok := obj[parent].(map[string]interface{}); ok {
		return mteamStringField(nested, keys...)
	}
	return ""
}

func firstCJKString(values ...string) string {
	for _, value := range values {
		value = strings.TrimSpace(value)
		if value != "" && mteamContainsCJK(value) {
			return value
		}
	}
	return ""
}

func mteamContainsCJK(value string) bool {
	for _, r := range value {
		if (r >= '\u4e00' && r <= '\u9fff') || (r >= '\u3400' && r <= '\u4dbf') {
			return true
		}
	}
	return false
}

func dedupeNonEmptyExcept(values []string, exclude string) []string {
	exclude = strings.TrimSpace(exclude)
	out := []string{}
	seen := map[string]struct{}{}
	for _, value := range values {
		value = strings.TrimSpace(value)
		if value == "" || strings.EqualFold(value, exclude) {
			continue
		}
		key := strings.ToLower(value)
		if _, ok := seen[key]; ok {
			continue
		}
		seen[key] = struct{}{}
		out = append(out, value)
	}
	return out
}

func mteamDedupeStrings(values []string) []string {
	out := []string{}
	seen := map[string]struct{}{}
	for _, value := range values {
		value = strings.TrimSpace(value)
		if value == "" {
			continue
		}
		key := strings.ToLower(value)
		if _, ok := seen[key]; ok {
			continue
		}
		seen[key] = struct{}{}
		out = append(out, value)
	}
	return out
}

func stringListFromAny(value interface{}) []string {
	switch v := value.(type) {
	case nil:
		return nil
	case string:
		parts := strings.FieldsFunc(v, func(r rune) bool {
			return r == ',' || r == '，' || r == '/' || r == '|' || r == '、'
		})
		return mteamDedupeStrings(parts)
	case []string:
		return mteamDedupeStrings(v)
	case []interface{}:
		out := []string{}
		for _, item := range v {
			switch row := item.(type) {
			case string:
				out = append(out, row)
			case map[string]interface{}:
				out = append(out, firstNonEmpty(
					mteamStringField(row, "zhName", "nameZh", "cnName", "nameCn"),
					mteamStringField(row, "name", "label", "title", "value"),
				))
			default:
				if s := strings.TrimSpace(fmt.Sprint(row)); s != "" && s != "<nil>" {
					out = append(out, s)
				}
			}
		}
		return mteamDedupeStrings(out)
	default:
		if s := strings.TrimSpace(fmt.Sprint(value)); s != "" && s != "<nil>" {
			return []string{s}
		}
	}
	return nil
}

func fileListFromAny(value interface{}) []string {
	switch v := value.(type) {
	case []string:
		return mteamDedupeStrings(v)
	case []interface{}:
		out := []string{}
		for _, item := range v {
			switch row := item.(type) {
			case string:
				out = append(out, row)
			case map[string]interface{}:
				name := firstNonEmpty(
					mteamStringField(row, "name", "path", "filename", "fileName"),
					mteamStringField(row, "title"),
				)
				if size := int64FromAny(row["size"]); size > 0 {
					name = strings.TrimSpace(name + " (" + formatSize(size) + ")")
				}
				out = append(out, name)
			}
		}
		return mteamDedupeStrings(out)
	default:
		return nil
	}
}

func firstExisting(primary, secondary map[string]interface{}, keys ...string) interface{} {
	for _, key := range keys {
		if primary != nil {
			if value, ok := primary[key]; ok {
				return value
			}
		}
		if secondary != nil {
			if value, ok := secondary[key]; ok {
				return value
			}
		}
	}
	return nil
}

func intFromAny(value interface{}) int {
	return int(int64FromAny(value))
}

func int64FromAny(value interface{}) int64 {
	switch v := value.(type) {
	case int:
		return int64(v)
	case int64:
		return v
	case float64:
		return int64(v)
	case json.Number:
		n, _ := v.Int64()
		return n
	case string:
		raw := strings.TrimSpace(v)
		if raw == "" {
			return 0
		}
		if n, err := strconv.ParseInt(raw, 10, 64); err == nil {
			return n
		}
		if m := regexp.MustCompile(`(?i)([0-9]+(?:\.[0-9]+)?)\s*([KMGT]?B)`).FindStringSubmatch(raw); len(m) == 3 {
			return parseSizeString(m[1], m[2])
		}
	}
	return 0
}

func boolFromAny(value interface{}) bool {
	switch v := value.(type) {
	case bool:
		return v
	case string:
		switch strings.ToLower(strings.TrimSpace(v)) {
		case "true", "1", "yes", "free", "免費", "免费":
			return true
		}
	case float64:
		return v != 0
	case int:
		return v != 0
	}
	return false
}

func timeFromAny(value interface{}) time.Time {
	switch v := value.(type) {
	case time.Time:
		return v
	case float64:
		return unixTimeFromNumber(int64(v))
	case int64:
		return unixTimeFromNumber(v)
	case int:
		return unixTimeFromNumber(int64(v))
	case string:
		raw := strings.TrimSpace(v)
		if raw == "" {
			return time.Time{}
		}
		if n, err := strconv.ParseInt(raw, 10, 64); err == nil {
			return unixTimeFromNumber(n)
		}
		for _, layout := range []string{
			time.RFC3339,
			"2006-01-02 15:04:05",
			"2006-01-02",
			"2006/01/02 15:04:05",
		} {
			if ts, err := time.ParseInLocation(layout, raw, time.Local); err == nil {
				return ts
			}
		}
	}
	return time.Time{}
}

func unixTimeFromNumber(n int64) time.Time {
	if n <= 0 {
		return time.Time{}
	}
	if n > 1_000_000_000_000 {
		n = n / 1000
	}
	return time.Unix(n, 0)
}
