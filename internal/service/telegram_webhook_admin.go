package service

import (
	"context"
	"time"

	"go.uber.org/zap"
)

// SetWebhook 注册 Telegram Bot Webhook URL。
func (s *TelegramBotService) SetWebhook(ctx context.Context, botToken, webhookURL string) error {
	cfg := map[string]string{"bot_token": botToken}
	if err := registerTelegramBotCommands(ctx, cfg); err != nil && s.log != nil {
		s.log.Warn("telegram setMyCommands failed", zap.Error(sanitizeTelegramError(err)))
	}
	payload := map[string]interface{}{
		"url":             webhookURL,
		"allowed_updates": []string{"message", "callback_query"},
	}
	return telegramPostJSON(ctx, cfg, "setWebhook", payload, 15*time.Second)
}

// GetWebhookInfo 获取 Webhook 配置信息。
func (s *TelegramBotService) GetWebhookInfo(ctx context.Context, botToken string) (map[string]interface{}, error) {
	cfg := map[string]string{"bot_token": botToken}
	var result map[string]interface{}
	if err := telegramGetJSONDecode(ctx, cfg, "getWebhookInfo", 10*time.Second, &result); err != nil {
		return nil, err
	}
	return result, nil
}
