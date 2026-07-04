package service

import (
	"fmt"
	"strings"
)

func telegramShouldShowEventHeading(event NotifyEvent) bool {
	if telegramMediaTag(event.Data) != "" {
		return false
	}
	switch strings.TrimSpace(event.Type) {
	case EventSubscriptionHit, EventDownloadComplete:
		return false
	default:
		return true
	}
}

func telegramEventTag(event NotifyEvent) string {
	if tag := telegramMediaTag(event.Data); tag != "" {
		return tag
	}
	switch strings.TrimSpace(event.Type) {
	case EventSubscriptionHit:
		return "#订阅"
	case EventDownloadComplete:
		return "#下载完成"
	case EventScrapeFailed:
		return "#刮削失败"
	case EventSystemAlert:
		return "#系统提醒"
	case EventLibraryIngest:
		return "#入库"
	default:
		title := strings.TrimSpace(strings.TrimPrefix(event.Title, "MediaStationGo "))
		if title == "" {
			return "#MediaStationGo"
		}
		return "#" + escapeHTML(strings.ReplaceAll(title, " ", ""))
	}
}

func telegramEventHeading(event NotifyEvent) string {
	title := strings.TrimSpace(event.Title)
	title = strings.TrimSpace(strings.TrimPrefix(title, "MediaStationGo "))
	if title == "" {
		title = "MediaStationGo 通知"
	}
	icon := "🔔"
	switch strings.TrimSpace(event.Type) {
	case EventSubscriptionHit:
		icon = "🎯"
	case EventDownloadComplete:
		icon = "✅"
	case EventScrapeFailed:
		icon = "⚠️"
	case EventSystemAlert:
		icon = "🚨"
	case EventLibraryIngest:
		icon = "📚"
	}
	return fmt.Sprintf("%s <b>%s</b>", icon, escapeHTML(title))
}

func formatTelegramBody(message string) string {
	lines := strings.Split(message, "\n")
	out := make([]string, 0, len(lines))
	for _, line := range lines {
		line = strings.TrimSpace(line)
		if line == "" {
			out = append(out, "")
			continue
		}
		if strings.HasPrefix(line, "- ") {
			out = append(out, "- "+escapeHTML(strings.TrimSpace(strings.TrimPrefix(line, "- "))))
			continue
		}
		if key, ok := trimTelegramEmptyField(line); ok {
			out = append(out, fmt.Sprintf("%s <b>%s</b>：", telegramFieldIcon(telegramFieldLabel(key)), escapeHTML(telegramFieldLabel(key))))
			continue
		}
		if key, value, ok := splitTelegramField(line); ok {
			out = append(out, formatTelegramField(key, value))
			continue
		}
		out = append(out, escapeHTML(line))
	}
	return strings.TrimSpace(strings.Join(out, "\n"))
}

func trimTelegramEmptyField(line string) (string, bool) {
	line = strings.TrimSpace(line)
	for _, suffix := range []string{"：", ":"} {
		if strings.HasSuffix(line, suffix) {
			key := strings.TrimSpace(strings.TrimSuffix(line, suffix))
			if key != "" && len([]rune(key)) <= 16 {
				return key, true
			}
		}
	}
	return "", false
}

func splitTelegramField(line string) (string, string, bool) {
	idx := strings.Index(line, "：")
	sepLen := len("：")
	if idx < 0 {
		idx = strings.Index(line, ":")
		sepLen = len(":")
	}
	if idx <= 0 {
		return "", "", false
	}
	key := strings.TrimSpace(line[:idx])
	value := strings.TrimSpace(line[idx+sepLen:])
	if key == "" || value == "" || len([]rune(key)) > 16 {
		return "", "", false
	}
	return key, value, true
}

func formatTelegramField(key, value string) string {
	key = telegramFieldLabel(key)
	escapedValue := escapeHTML(strings.TrimSpace(value))
	if telegramCodeField(key) {
		escapedValue = "<code>" + escapedValue + "</code>"
	}
	return fmt.Sprintf("%s <b>%s</b>：%s", telegramFieldIcon(key), escapeHTML(key), escapedValue)
}

func telegramCodeField(key string) bool {
	key = strings.ToLower(strings.TrimSpace(key))
	return strings.Contains(key, "hash") ||
		strings.Contains(key, "路径") ||
		strings.Contains(key, "path") ||
		strings.Contains(key, "id")
}

func telegramFieldIcon(key string) string {
	switch strings.ToLower(strings.TrimSpace(key)) {
	case "中文片名", "标题", "任务", "媒体", "资源":
		return "📺"
	case "原始片名":
		return "🧿"
	case "原始语言", "语言":
		return "🌐"
	case "发行年份", "年份":
		return "📅"
	case "类别", "分类", "媒体类型":
		return "🐈‍⬛"
	case "季集", "集数":
		return "🫧"
	case "大小", "质量", "规格":
		return "🔎"
	case "版本", "保存路径":
		return "📁"
	case "评分":
		return "⭐️"
	case "类型":
		return "💎"
	case "简介":
		return "🪬"
	case "订阅":
		return "🎯"
	case "新增资源":
		return "✨"
	case "Hash":
		return "🧿"
	case "错误":
		return "⚠️"
	default:
		return "•"
	}
}
