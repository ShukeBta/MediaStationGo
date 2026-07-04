// Package service — Telegram 通知 Provider。
package service

import (
	"context"
	"fmt"
	"strings"
	"time"
	"unicode/utf8"
)

// TelegramProvider 通过 Telegram Bot API 发送通知。
type TelegramProvider struct{}

// Send 发送 Telegram 消息。
func (p *TelegramProvider) Send(ctx context.Context, cfg map[string]string, event NotifyEvent) error {
	botToken := cfg["bot_token"]
	chatIDs := telegramTargetChatIDs(cfg)
	parseMode := cfg["parse_mode"]
	if parseMode == "" {
		parseMode = "HTML"
	}

	if botToken == "" || len(chatIDs) == 0 {
		return fmt.Errorf("telegram: bot_token and group_chat_id/channel_chat_id are required")
	}

	text := formatTelegramMessage(event, parseMode)
	photoURL := telegramEventPhotoURL(event)

	var firstErr error
	for _, chatID := range chatIDs {
		if photoURL != "" && utf8.RuneCountInString(text) <= 1024 {
			payload := map[string]string{
				"chat_id":    chatID,
				"photo":      photoURL,
				"caption":    text,
				"parse_mode": parseMode,
			}
			if err := telegramPostJSON(ctx, cfg, "sendPhoto", payload, 15*time.Second); err == nil {
				continue
			} else if firstErr == nil {
				firstErr = err
			}
			if photo, _, err := telegramFetchRemotePhoto(ctx, cfg, photoURL, 15*time.Second); err == nil {
				fields := map[string]string{
					"chat_id":    chatID,
					"caption":    text,
					"parse_mode": parseMode,
				}
				if err := telegramPostMultipart(ctx, cfg, "sendPhoto", fields, "photo", "poster.jpg", photo, 20*time.Second); err == nil {
					continue
				} else if firstErr == nil {
					firstErr = err
				}
			} else if firstErr == nil {
				firstErr = err
			}
		}
		payload := map[string]string{
			"chat_id":    chatID,
			"text":       text,
			"parse_mode": parseMode,
		}
		if err := telegramPostJSON(ctx, cfg, "sendMessage", payload, 15*time.Second); err != nil && firstErr == nil {
			firstErr = err
		}
	}
	return firstErr
}

// ValidateConfig 验证 Telegram 配置。
func (p *TelegramProvider) ValidateConfig(cfg map[string]string) error {
	if cfg["bot_token"] == "" {
		return fmt.Errorf("telegram: bot_token is required")
	}
	if len(telegramTargetChatIDs(cfg)) == 0 {
		return fmt.Errorf("telegram: group_chat_id or channel_chat_id is required")
	}
	return nil
}

// formatTelegramMessage 格式化消息内容。
func formatTelegramMessage(event NotifyEvent, parseMode string) string {
	text := formatTelegramNotification(event)
	if parseMode == "HTML" || parseMode == "" {
		return text
	}
	result := text
	result = strings.ReplaceAll(result, "<b>", "**")
	result = strings.ReplaceAll(result, "</b>", "**")
	result = strings.ReplaceAll(result, "<code>", "`")
	result = strings.ReplaceAll(result, "</code>", "`")
	result = strings.ReplaceAll(result, "&lt;", "<")
	result = strings.ReplaceAll(result, "&gt;", ">")
	result = strings.ReplaceAll(result, "&amp;", "&")
	return result
}

func formatTelegramNotification(event NotifyEvent) string {
	if text := formatTelegramMediaNotification(event); text != "" {
		return text
	}

	var sb strings.Builder
	if tag := telegramEventTag(event); tag != "" {
		sb.WriteString(tag)
		if telegramShouldShowEventHeading(event) {
			sb.WriteString("\n")
		}
	}
	if telegramShouldShowEventHeading(event) {
		sb.WriteString(telegramEventHeading(event))
	}

	message := strings.TrimSpace(event.Message)
	if message != "" {
		if sb.Len() > 0 {
			sb.WriteString("\n\n")
		}
		sb.WriteString(formatTelegramBody(message))
	}

	fields := telegramDisplayData(event.Data)
	if len(fields) > 0 {
		if sb.Len() > 0 {
			sb.WriteString("\n")
		}
		sb.WriteString("\n")
		for _, field := range fields {
			sb.WriteString(formatTelegramField(field.key, field.value))
			sb.WriteString("\n")
		}
	}
	if links := telegramExternalLinks(event.Data); links != "" {
		sb.WriteString("\n")
		sb.WriteString(links)
	}

	return strings.TrimSpace(sb.String())
}
