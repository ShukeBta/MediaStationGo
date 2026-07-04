package service

import (
	"regexp"
	"strings"
)

const (
	telegramMediaTemplateHeader    = "🐈‍⬛🐈‍⬛ MediaStationGo 更新啦 🐈‍⬛🐈‍⬛"
	telegramMediaTemplateSeparator = "--------------------------------"
)

var telegramSeasonEpisodePattern = regexp.MustCompile(`(?i)S\d{1,2}E\d{1,3}(?:[\-.~_ ]?E?\d{1,3})?`)

func formatTelegramMediaNotification(event NotifyEvent) string {
	if strings.TrimSpace(event.Type) == EventDownloadComplete {
		return formatTelegramDownloadCompleteNotification(event)
	}

	tag := telegramMediaTag(event.Data)
	if tag == "" {
		return ""
	}

	title := telegramFirstValue(event.Data, "chinese_title", "title", "name", "media_title")
	if title == "" {
		title = telegramMessageFieldValue(event.Message, "任务", "订阅", "媒体", "资源")
	}
	originalTitle := telegramFirstValue(event.Data, "original_title", "original_name")
	originalLanguage := telegramLanguageName(telegramFirstValue(event.Data, "original_language", "language", "languages"))
	year := telegramFirstValue(event.Data, "year", "release_year")
	category := telegramMediaCategory(event.Data)
	seasonEpisode := telegramSeasonEpisodeValue(event)
	size := telegramSizeValue(event.Data)
	version := telegramVersionValue(event, seasonEpisode)
	rating := telegramFirstValue(event.Data, "rating", "score")
	genres := telegramGenresValue(telegramFirstValue(event.Data, "genres", "genre", "type"))
	overview := telegramFirstValue(event.Data, "overview", "summary", "description")
	links := telegramExternalLinks(event.Data)

	topFields := []string{}
	if title != "" {
		topFields = append(topFields, "📺 中文片名："+escapeHTML(title))
	}
	if originalTitle != "" && !strings.EqualFold(originalTitle, title) {
		topFields = append(topFields, "🧿 原始片名："+escapeHTML(originalTitle))
	}
	if originalLanguage != "" {
		topFields = append(topFields, "🌐 原始语言："+escapeHTML(originalLanguage))
	}
	if year != "" && year != "0" {
		topFields = append(topFields, "📅 发行年份："+escapeHTML(year))
	}

	mediaFields := []string{}
	if category != "" {
		mediaFields = append(mediaFields, "🐈‍⬛ 类别："+escapeHTML(category))
	}
	if seasonEpisode != "" {
		mediaFields = append(mediaFields, "🫧 季集："+escapeHTML(seasonEpisode))
	}
	if size != "" {
		mediaFields = append(mediaFields, "🔎 大小："+escapeHTML(size))
	}
	if version != "" {
		mediaFields = append(mediaFields, "📁 版本："+escapeHTML(version))
	}

	infoFields := []string{}
	if rating != "" && rating != "0" {
		infoFields = append(infoFields, "⭐️ 评分："+escapeHTML(rating))
	}
	if genres != "" {
		infoFields = append(infoFields, "💎 类型："+escapeHTML(genres))
	}
	if overview != "" {
		infoFields = append(infoFields, "🪬 简介：\n"+escapeHTML(overview))
	}

	if len(topFields) == 0 && len(mediaFields) == 0 && len(infoFields) == 0 && links == "" {
		return ""
	}

	lines := []string{telegramMediaTemplateHeader, telegramMediaTemplateSeparator, tag}
	lines = append(lines, topFields...)
	if len(mediaFields) > 0 {
		if len(topFields) > 0 {
			lines = append(lines, "")
		}
		lines = append(lines, mediaFields...)
	}
	if len(infoFields) > 0 {
		if len(topFields) > 0 || len(mediaFields) > 0 {
			lines = append(lines, "")
		}
		lines = append(lines, infoFields...)
	}
	if links != "" {
		lines = append(lines, "", telegramMediaTemplateSeparator, links)
	}
	return strings.TrimSpace(strings.Join(lines, "\n"))
}

func formatTelegramDownloadCompleteNotification(event NotifyEvent) string {
	title := telegramMessageFieldValue(event.Message, "任务", "媒体", "资源")
	if title == "" {
		title = telegramFirstValue(event.Data, "title", "name", "media_title", "chinese_title", "resource_title")
	}
	if title == "" {
		title = "下载任务"
	}
	return "#下载完成\n📺 任务：" + escapeHTML(title)
}

func telegramMediaTag(data map[string]interface{}) string {
	for _, key := range []string{"media_category", "category", "media_type"} {
		value := strings.TrimSpace(telegramDataString(data, key))
		if value == "" {
			continue
		}
		lower := strings.ToLower(value)
		switch {
		case strings.Contains(value, "电影") || lower == "movie":
			return "#电影"
		case strings.Contains(value, "剧") || lower == "tv" || lower == "series" || lower == "show":
			return "#剧集"
		case strings.Contains(value, "动漫") || strings.Contains(value, "动画") || lower == "anime":
			return "#动漫"
		case strings.Contains(value, "综艺") || lower == "variety":
			return "#综艺"
		case strings.Contains(value, "纪录") || lower == "documentary":
			return "#纪录片"
		default:
			return "#" + escapeHTML(strings.ReplaceAll(value, " ", ""))
		}
	}
	return ""
}
