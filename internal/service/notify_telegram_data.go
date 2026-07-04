package service

import (
	"fmt"
	"net/url"
	"sort"
	"strings"
)

type telegramDataField struct {
	key   string
	value string
}

func telegramDisplayData(data map[string]interface{}) []telegramDataField {
	if len(data) == 0 {
		return nil
	}
	keys := make([]string, 0, len(data))
	for key := range data {
		if telegramHiddenDataKey(key) {
			continue
		}
		keys = append(keys, key)
	}
	sort.Strings(keys)
	fields := make([]telegramDataField, 0, len(keys))
	for _, key := range keys {
		value := strings.TrimSpace(fmt.Sprint(data[key]))
		if value == "" || value == "<nil>" {
			continue
		}
		fields = append(fields, telegramDataField{key: key, value: value})
	}
	return fields
}

func telegramHiddenDataKey(key string) bool {
	switch strings.ToLower(strings.TrimSpace(key)) {
	case "photo_url", "poster_url", "poster", "image_url", "backdrop_url",
		"tmdb_url", "imdb_url", "douban_url", "detail_url", "external_url",
		"resource_title", "torrent_title", "release_title":
		return true
	default:
		return false
	}
}

func telegramFieldLabel(key string) string {
	switch strings.ToLower(strings.TrimSpace(key)) {
	case "title", "name":
		return "标题"
	case "original_title":
		return "原始片名"
	case "original_language":
		return "原始语言"
	case "year", "release_year":
		return "发行年份"
	case "save_path":
		return "保存路径"
	case "hash":
		return "Hash"
	case "media_type":
		return "媒体类型"
	case "media_category":
		return "类别"
	case "season_episode":
		return "季集"
	case "size", "bitrate":
		return "大小"
	case "version", "release_group":
		return "版本"
	case "rating":
		return "评分"
	case "genres":
		return "类型"
	case "overview":
		return "简介"
	case "subscription":
		return "订阅"
	case "queued":
		return "新增资源"
	default:
		return strings.TrimSpace(key)
	}
}

func telegramExternalLinks(data map[string]interface{}) string {
	if len(data) == 0 {
		return ""
	}
	links := []string{}
	for _, item := range []struct {
		key  string
		name string
	}{
		{key: "tmdb_url", name: "TMDB"},
		{key: "imdb_url", name: "IMDB"},
		{key: "douban_url", name: "豆瓣"},
	} {
		value := telegramDataString(data, item.key)
		if isTelegramRemotePhotoURL(value) {
			links = append(links, fmt.Sprintf(`<a href="%s">%s</a>`, escapeHTML(value), escapeHTML(item.name)))
		}
	}
	if len(links) == 0 {
		return ""
	}
	return "🔗 外链：" + strings.Join(links, " / ")
}

func telegramEventPhotoURL(event NotifyEvent) string {
	for _, key := range []string{"photo_url", "poster_url", "poster", "image_url", "backdrop_url"} {
		value := telegramDataString(event.Data, key)
		if isTelegramRemotePhotoURL(value) {
			return value
		}
	}
	return ""
}

func telegramDataString(data map[string]interface{}, key string) string {
	if len(data) == 0 {
		return ""
	}
	for k, value := range data {
		if strings.EqualFold(strings.TrimSpace(k), key) {
			return telegramValueString(value)
		}
	}
	return ""
}

func telegramValueString(value interface{}) string {
	switch v := value.(type) {
	case nil:
		return ""
	case string:
		return strings.TrimSpace(v)
	case []string:
		return strings.TrimSpace(strings.Join(v, ","))
	case []interface{}:
		out := make([]string, 0, len(v))
		for _, item := range v {
			if s := telegramValueString(item); s != "" {
				out = append(out, s)
			}
		}
		return strings.Join(out, ",")
	case float32:
		return strings.TrimRight(strings.TrimRight(fmt.Sprintf("%.1f", v), "0"), ".")
	case float64:
		return strings.TrimRight(strings.TrimRight(fmt.Sprintf("%.1f", v), "0"), ".")
	default:
		text := strings.TrimSpace(fmt.Sprint(value))
		if text == "<nil>" {
			return ""
		}
		return text
	}
}

func isTelegramRemotePhotoURL(raw string) bool {
	raw = strings.TrimSpace(raw)
	if raw == "" {
		return false
	}
	u, err := url.Parse(raw)
	return err == nil && (u.Scheme == "http" || u.Scheme == "https") && u.Host != ""
}
