package service

import (
	"regexp"
	"strings"
)

var discoverShortDramaTitle = regexp.MustCompile(`(?i)(?:^|[\[【(（:：|｜])\s*(?:微短剧|微短劇|网络短剧|網絡短劇|竖屏短剧|豎屏短劇|短剧|短劇|short[- ]form drama|short drama|micro[- ]drama|vertical drama)\s*(?:$|[\]】)）:：|｜])`)
var discoverShortDramaDescription = regexp.MustCompile(`(?i)(?:本剧|本劇|该剧|該劇|这是一部|這是一部|本作|是一部)[^。.!！?？\n]{0,24}(?:微短剧|微短劇|竖屏短剧|豎屏短劇|网络短剧|網絡短劇|短剧|短劇)(?:[，。,:：\s]|$)|\b(?:a|an|original)\s+(?:short[- ]form|micro[- ]|vertical)\s*drama\b`)

// FilterDiscoverRecommendations removes explicitly identified short dramas only
// from recommendation rails. Nationality, language, runtime and episode count
// are deliberately not evidence; ordinary animation and variety stay visible.
// Sparse upstream lists may omit labels, so unknown works remain eligible.
func FilterDiscoverRecommendations(items []ExternalMediaResult) []ExternalMediaResult {
	out := make([]ExternalMediaResult, 0, len(items))
	for _, item := range items {
		if !discoverRecommendationIsShortDrama(item) {
			out = append(out, item)
		}
	}
	return out
}

func discoverRecommendationIsShortDrama(item ExternalMediaResult) bool {
	for _, genre := range item.Genres {
		switch strings.ToLower(strings.TrimSpace(genre)) {
		case "短剧", "短劇", "微短剧", "微短劇", "网络短剧", "網絡短劇", "竖屏短剧", "豎屏短劇", "short drama", "short-form drama", "microdrama", "micro-drama", "vertical drama":
			return true
		}
	}
	if strings.ToLower(strings.TrimSpace(item.MediaType)) != "tv" {
		return false
	}
	for _, genre := range item.Genres {
		switch strings.ToLower(strings.TrimSpace(genre)) {
		case "动画", "動畫", "animation", "纪录", "纪录片", "紀錄片", "documentary", "真人秀", "reality", "脱口秀", "talk", "综艺", "綜藝", "variety":
			return false
		}
	}
	return discoverShortDramaTitle.MatchString(item.Title) ||
		discoverShortDramaTitle.MatchString(item.OriginalName) ||
		discoverShortDramaDescription.MatchString(item.Overview)
}
