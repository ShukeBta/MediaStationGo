package service

import (
	"context"
	"encoding/json"
	"strconv"
	"strings"

	"github.com/ShukeBta/MediaStationGo/internal/model"
)

// findChannelByChatID 根据 chat_id 查找已配置的通知渠道。
func (s *TelegramBotService) findChannelByChatID(ctx context.Context, chatID int) *model.NotifyChannel {
	channels, err := s.repo.NotifyChannel.ListByType(ctx, "telegram")
	if err != nil {
		return nil
	}
	target := strconv.Itoa(chatID)
	for _, ch := range channels {
		if !ch.Enabled {
			continue
		}
		configStr := ch.Config
		if s.crypto != nil && configStr != "" {
			configStr = s.crypto.Decrypt(configStr)
		}
		var cfg map[string]string
		if err := json.Unmarshal([]byte(configStr), &cfg); err != nil {
			continue
		}
		if cfg["chat_id"] == target || cfg["command_chat_id"] == target ||
			cfg["group_chat_id"] == target || cfg["channel_chat_id"] == target {
			return &ch
		}
	}
	if len(channels) == 1 && channels[0].Enabled {
		return &channels[0]
	}
	return nil
}

func (s *TelegramBotService) findChannelForMessage(ctx context.Context, msg *TelegramMessage) *model.NotifyChannel {
	if msg == nil {
		return nil
	}
	if msg.Chat.Type != "" && msg.Chat.Type != "private" {
		return s.findChannelByChatID(ctx, msg.Chat.ID)
	}
	channels, err := s.repo.NotifyChannel.ListByType(ctx, "telegram")
	if err != nil {
		return nil
	}
	var first *model.NotifyChannel
	for i := range channels {
		ch := channels[i]
		if !ch.Enabled {
			continue
		}
		if first == nil {
			first = &ch
		}
		if s.telegramUserIsAdmin(ctx, &ch, msg.From.ID) || s.telegramUserCanBind(ctx, &ch, msg.From.ID) {
			return &ch
		}
	}
	return first
}

func (s *TelegramBotService) channelForMessage(ctx context.Context, msg *TelegramMessage, hint *model.NotifyChannel) *model.NotifyChannel {
	if hint == nil {
		return s.findChannelForMessage(ctx, msg)
	}
	if msg == nil {
		return hint
	}
	if msg.Chat.Type != "" && msg.Chat.Type != "private" && !s.telegramChatAllowed(hint, msg.Chat.ID) {
		return nil
	}
	return hint
}

func (s *TelegramBotService) telegramChatAllowed(channel *model.NotifyChannel, chatID int) bool {
	if channel == nil {
		return false
	}
	configStr := channel.Config
	if s.crypto != nil && configStr != "" {
		configStr = s.crypto.Decrypt(configStr)
	}
	var cfg map[string]string
	if err := json.Unmarshal([]byte(configStr), &cfg); err != nil {
		return false
	}
	target := strconv.Itoa(chatID)
	for _, key := range []string{"group_chat_id", "channel_chat_id", "command_chat_id"} {
		if configured := strings.TrimSpace(cfg[key]); configured != "" && configured == target {
			return true
		}
	}
	if strings.TrimSpace(cfg["group_chat_id"]) != "" || strings.TrimSpace(cfg["channel_chat_id"]) != "" || strings.TrimSpace(cfg["command_chat_id"]) != "" {
		return false
	}
	return strings.TrimSpace(cfg["chat_id"]) == target
}

func (s *TelegramBotService) telegramUserIDConfigured(channel *model.NotifyChannel, telegramUserID int) bool {
	if channel == nil || telegramUserID == 0 {
		return false
	}
	cfg := s.telegramChannelConfig(channel)
	target := strconv.Itoa(telegramUserID)
	for _, value := range telegramConfiguredUserIDs(cfg["admin_user_ids"]) {
		if value == target {
			return true
		}
	}
	if strings.TrimSpace(cfg["admin_user_ids"]) == "" && strings.TrimSpace(cfg["chat_id"]) == target {
		return true
	}
	return false
}

func (s *TelegramBotService) telegramChannelConfig(channel *model.NotifyChannel) map[string]string {
	return telegramConfigFromChannel(s.crypto, channel)
}

func telegramConfigFromChannel(crypto *CryptoService, channel *model.NotifyChannel) map[string]string {
	if channel == nil {
		return map[string]string{}
	}
	configStr := channel.Config
	if crypto != nil && configStr != "" {
		configStr = crypto.Decrypt(configStr)
	}
	var cfg map[string]string
	if err := json.Unmarshal([]byte(configStr), &cfg); err != nil || cfg == nil {
		return map[string]string{}
	}
	normalizeTelegramConfig(cfg)
	return cfg
}

func normalizeTelegramConfig(cfg map[string]string) {
	if cfg == nil {
		return
	}
	chatID := strings.TrimSpace(cfg["chat_id"])
	if chatID == "" {
		return
	}
	if strings.HasPrefix(chatID, "-") {
		if strings.TrimSpace(cfg["group_chat_id"]) == "" && strings.TrimSpace(cfg["channel_chat_id"]) == "" && strings.TrimSpace(cfg["command_chat_id"]) == "" {
			cfg["group_chat_id"] = chatID
		}
		return
	}
	if strings.TrimSpace(cfg["admin_user_ids"]) == "" {
		cfg["admin_user_ids"] = chatID
	}
}
