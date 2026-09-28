package service

import (
	"context"
	"errors"
	"strings"
	"time"

	"go.uber.org/zap"
)

// SetWebhook 注册 Telegram Bot Webhook URL。
func (s *TelegramBotService) SetWebhook(ctx context.Context, botToken, webhookURL string) error {
	botToken = strings.TrimSpace(botToken)
	channels, err := s.repo.NotifyChannel.ListByType(ctx, "telegram")
	if err != nil {
		return err
	}
	var cfg map[string]string
	for i := range channels {
		candidate := s.telegramChannelConfig(&channels[i])
		if channels[i].Enabled && botToken != "" && strings.TrimSpace(candidate["bot_token"]) == botToken {
			cfg = candidate
			cfg["bot_token"] = botToken
			break
		}
	}
	if cfg == nil {
		return errors.New("webhook requires an enabled Telegram channel with the supplied bot token")
	}
	secret, err := s.ensureTelegramWebhookSecret(ctx, botToken)
	if err != nil {
		return err
	}
	if err := registerTelegramBotCommands(ctx, cfg); err != nil && s.log != nil {
		s.log.Warn("telegram setMyCommands failed", zap.Error(sanitizeTelegramError(err)))
	}
	payload := map[string]interface{}{
		"url":             webhookURL,
		"secret_token":    secret,
		"allowed_updates": []string{"message", "callback_query"},
	}
	var result struct {
		OK bool `json:"ok"`
	}
	if err := telegramPostJSONDecode(ctx, cfg, "setWebhook", payload, 15*time.Second, &result); err != nil {
		return err
	}
	if !result.OK {
		return errors.New("Telegram rejected webhook registration")
	}
	return nil
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
