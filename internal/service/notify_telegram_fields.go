package service

import (
	"fmt"
	"strconv"
	"strings"
)

func telegramFirstValue(data map[string]interface{}, keys ...string) string {
	for _, key := range keys {
		if value := telegramDataString(data, key); value != "" {
			return value
		}
	}
	return ""
}

func telegramMessageFieldValue(message string, keys ...string) string {
	if strings.TrimSpace(message) == "" {
		return ""
	}
	for _, line := range strings.Split(message, "\n") {
		key, value, ok := splitTelegramField(line)
		if !ok {
			continue
		}
		for _, want := range keys {
			if strings.EqualFold(strings.TrimSpace(key), strings.TrimSpace(want)) {
				return value
			}
		}
	}
	return ""
}

func telegramMediaCategory(data map[string]interface{}) string {
	if category := telegramFirstValue(data, "media_category", "category"); category != "" {
		return category
	}
	switch strings.ToLower(telegramFirstValue(data, "media_type")) {
	case "movie":
		return "电影"
	case "tv", "series", "show":
		return "剧集"
	case "anime":
		return "动漫"
	case "variety":
		return "综艺"
	case "documentary":
		return "纪录片"
	default:
		return telegramFirstValue(data, "media_type")
	}
}

func telegramLanguageName(raw string) string {
	raw = strings.TrimSpace(raw)
	if raw == "" {
		return ""
	}
	parts := strings.FieldsFunc(raw, func(r rune) bool {
		return r == ',' || r == '，' || r == '/' || r == '|' || r == '、'
	})
	if len(parts) == 0 {
		parts = []string{raw}
	}
	seen := map[string]struct{}{}
	out := []string{}
	for _, part := range parts {
		part = strings.TrimSpace(strings.Trim(part, "[]"))
		if part == "" {
			continue
		}
		lower := strings.ToLower(strings.ReplaceAll(part, "_", "-"))
		name := part
		switch {
		case strings.HasPrefix(lower, "zh") || lower == "cn" || lower == "cmn":
			name = "中文"
		case lower == "en" || strings.HasPrefix(lower, "en-"):
			name = "英语"
		case lower == "ja" || lower == "jp" || strings.HasPrefix(lower, "ja-"):
			name = "日语"
		case lower == "ko" || lower == "kr" || strings.HasPrefix(lower, "ko-"):
			name = "韩语"
		case lower == "fr" || strings.HasPrefix(lower, "fr-"):
			name = "法语"
		case lower == "de" || strings.HasPrefix(lower, "de-"):
			name = "德语"
		case lower == "es" || strings.HasPrefix(lower, "es-"):
			name = "西班牙语"
		case lower == "it" || strings.HasPrefix(lower, "it-"):
			name = "意大利语"
		case lower == "ru" || strings.HasPrefix(lower, "ru-"):
			name = "俄语"
		case lower == "th" || strings.HasPrefix(lower, "th-"):
			name = "泰语"
		}
		if _, ok := seen[name]; ok {
			continue
		}
		seen[name] = struct{}{}
		out = append(out, name)
	}
	return strings.Join(out, "、")
}

func telegramSeasonEpisodeValue(event NotifyEvent) string {
	if value := telegramFirstValue(event.Data, "season_episode", "episode_tag"); value != "" {
		return strings.ToUpper(value)
	}
	for _, raw := range []string{
		telegramFirstValue(event.Data, "resource_title", "torrent_title", "release_title"),
		telegramFirstValue(event.Data, "title", "name"),
		event.Message,
	} {
		if value := telegramExtractSeasonEpisode(raw); value != "" {
			return value
		}
	}
	season := telegramFirstValue(event.Data, "season")
	episode := telegramFirstValue(event.Data, "episode")
	if season != "" && episode != "" {
		return fmt.Sprintf("S%02dE%02d", telegramEpisodeNumber(season), telegramEpisodeNumber(episode))
	}
	return ""
}

func telegramEpisodeNumber(raw string) int {
	raw = strings.TrimSpace(strings.TrimLeft(strings.ToUpper(raw), "SE"))
	raw = strings.TrimLeft(raw, "0")
	if raw == "" {
		return 0
	}
	n, _ := strconv.Atoi(raw)
	return n
}

func telegramExtractSeasonEpisode(raw string) string {
	raw = strings.TrimSpace(raw)
	if raw == "" {
		return ""
	}
	return strings.ToUpper(telegramSeasonEpisodePattern.FindString(raw))
}

func telegramSizeValue(data map[string]interface{}) string {
	size := telegramFirstValue(data, "size")
	bitrate := telegramFirstValue(data, "bitrate")
	if size != "" && bitrate != "" {
		return size + " / " + bitrate
	}
	if size != "" {
		return size
	}
	return bitrate
}

func telegramVersionValue(event NotifyEvent, seasonEpisode string) string {
	if version := telegramFirstValue(event.Data, "version", "release_group"); version != "" && !strings.EqualFold(version, "best") {
		return version
	}
	return telegramVersionFromResourceTitle(
		telegramFirstValue(event.Data, "resource_title", "torrent_title", "release_title"),
		seasonEpisode,
		telegramFirstValue(event.Data, "year", "release_year"),
	)
}

func telegramVersionFromResourceTitle(raw, seasonEpisode, year string) string {
	raw = strings.TrimSpace(raw)
	if raw == "" {
		return ""
	}
	tail := ""
	if seasonEpisode != "" {
		upperRaw := strings.ToUpper(raw)
		upperEpisode := strings.ToUpper(seasonEpisode)
		if idx := strings.Index(upperRaw, upperEpisode); idx >= 0 {
			tail = raw[idx+len(seasonEpisode):]
		}
	}
	if tail == "" && year != "" {
		if idx := strings.LastIndex(raw, year); idx >= 0 {
			tail = raw[idx+len(year):]
		}
	}
	tail = strings.Trim(tail, " \t\r\n._-[]()【】")
	if tail == "" {
		return ""
	}
	tail = strings.TrimSuffix(tail, ".torrent")
	tail = strings.TrimSuffix(tail, ".mkv")
	tail = strings.TrimSuffix(tail, ".mp4")
	tail = strings.Join(strings.Fields(tail), ".")
	if len([]rune(tail)) > 72 {
		tail = string([]rune(tail)[:72]) + "..."
	}
	return tail
}

func telegramGenresValue(raw string) string {
	raw = strings.TrimSpace(strings.Trim(raw, "[]"))
	if raw == "" {
		return ""
	}
	parts := strings.FieldsFunc(raw, func(r rune) bool {
		return r == ',' || r == '，' || r == '/' || r == '|' || r == '、'
	})
	if len(parts) <= 1 {
		return raw
	}
	out := make([]string, 0, len(parts))
	seen := map[string]struct{}{}
	for _, part := range parts {
		part = strings.TrimSpace(part)
		if part == "" {
			continue
		}
		if _, ok := seen[part]; ok {
			continue
		}
		seen[part] = struct{}{}
		out = append(out, part)
	}
	return strings.Join(out, "、")
}
