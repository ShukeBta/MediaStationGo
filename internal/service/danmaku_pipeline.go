package service

import (
	"context"
	"net/url"
)

// danmakuPipelineClient 的 HTTP 实现：转发到 media-pipeline 的 /v1/danmaku/*。
//
// 弹弹play 的 AppId/AppSecret 只存在于 media-pipeline 一侧的环境变量里，
// MediaStationGo 既不下发也不保存第三方弹幕凭证。

type danmakuMatchEnvelope struct {
	MediaID string             `json:"media_id"`
	Target  map[string]any     `json:"target"`
	Match   DanmakuMatchResult `json:"match"`
}

func (c *resourcePipelineHTTPClient) MatchDanmaku(ctx context.Context, mediaID string) (DanmakuMatchResult, error) {
	var out danmakuMatchEnvelope
	if err := c.doJSON(ctx, "POST", "/v1/danmaku/match", map[string]any{
		"media_id": mediaID,
	}, "", &out); err != nil {
		return DanmakuMatchResult{}, err
	}
	if out.Match.Status == "" {
		if out.Match.Matched {
			out.Match.Status = DanmakuStatusMatched
		} else {
			out.Match.Status = DanmakuStatusUnmatched
		}
	}
	return out.Match, nil
}

func (c *resourcePipelineHTTPClient) FetchDanmaku(ctx context.Context, request DanmakuFetchRequest) (DanmakuPayload, error) {
	var out DanmakuPayload
	err := c.doJSON(ctx, "POST", "/v1/danmaku/comment", map[string]any{
		"media_id":               request.MediaID,
		"episode_id":             request.EpisodeID,
		"source":                 request.Source,
		"ch_convert":             request.ChConvert,
		"offset_seconds":         request.OffsetSeconds,
		"provider_shift_seconds": request.ProviderShiftSeconds,
		"anime_title":            request.AnimeTitle,
		"episode_title":          request.EpisodeTitle,
		"match_mode":             request.MatchMode,
		"with_related":           request.WithRelated,
	}, "", &out)
	return out, err
}

func (c *resourcePipelineHTTPClient) SearchDanmaku(ctx context.Context, keyword string, episode int) (DanmakuSearchResult, error) {
	var out DanmakuSearchResult
	body := map[string]any{"keyword": keyword}
	if episode > 0 {
		body["episode"] = episode
	}
	err := c.doJSON(ctx, "POST", "/v1/danmaku/search", body, "", &out)
	return out, err
}

// ParseDanmaku 让管线解析本地弹幕文件；解析与归一化都只发生在管线一侧。
func (c *resourcePipelineHTTPClient) ParseDanmaku(ctx context.Context, request DanmakuParseRequest) (DanmakuPayload, error) {
	var out DanmakuPayload
	err := c.doJSON(ctx, "POST", "/v1/danmaku/parse", map[string]any{
		"content":        request.Content,
		"format":         request.Format,
		"offset_seconds": request.OffsetSeconds,
		"ch_convert":     request.ChConvert,
		"title":          request.Title,
	}, "", &out)
	return out, err
}

// StartDanmakuPrewarm 触发管线侧的一次整季预热；任务在管线后台串行执行。
func (c *resourcePipelineHTTPClient) StartDanmakuPrewarm(ctx context.Context, request DanmakuPrewarmRequest) (DanmakuPrewarmTask, error) {
	var out DanmakuPrewarmTask
	err := c.doJSON(ctx, "POST", "/v1/danmaku/season/prewarm", map[string]any{
		"owner_id": request.OwnerID,
		"media_id": request.MediaID,
		"season":   request.Season,
		"episodes": request.Episodes,
	}, "", &out)
	return out, err
}

func (c *resourcePipelineHTTPClient) GetDanmakuPrewarm(ctx context.Context, taskID string) (DanmakuPrewarmTask, error) {
	var out DanmakuPrewarmTask
	endpoint := "/v1/danmaku/season/prewarm/" + url.PathEscape(taskID)
	if err := c.doJSON(ctx, "GET", endpoint, nil, "", &out); err != nil {
		return DanmakuPrewarmTask{}, err
	}
	return out, nil
}
