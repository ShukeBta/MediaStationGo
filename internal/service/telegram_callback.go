package service

import (
	"context"
	"strings"
	"time"

	"go.uber.org/zap"

	"github.com/ShukeBta/MediaStationGo/internal/model"
)

func (s *TelegramBotService) handleCallback(ctx context.Context, cb *TelegramCallbackQuery, channelHint *model.NotifyChannel) error {
	if cb == nil || cb.Message == nil {
		return nil
	}
	msg := *cb.Message
	msg.From = cb.From
	channel := s.channelForMessage(ctx, &msg, channelHint)
	if channel == nil {
		channel = s.findChannelByChatID(ctx, cb.Message.Chat.ID)
	}
	// 立即应答回调，关闭按钮上的加载状态，避免客户端长时间转圈。
	if telegramIsGroupChat(cb.Message.Chat.Type) {
		s.answerCallbackWithText(ctx, channel, cb.ID, "为了隐私，群组内按钮面板已禁用。请私聊 Bot 或在群里发送 /menu，我会把面板私聊给你。", true)
		s.deleteTelegramSourceMessage(channel, cb.Message.Chat.ID, cb.Message.MessageID)
		return nil
	}
	if cb.Message.Chat.Type == "private" && cb.Message.Chat.ID != cb.From.ID {
		s.answerCallbackWithText(ctx, channel, cb.ID, "这个面板不属于你，请发送 /menu 打开自己的面板。", true)
		return nil
	}
	s.answerCallback(ctx, channel, cb.ID)
	data := strings.TrimSpace(cb.Data)
	if data == "adult_toggle" {
		reply := s.cmdHideAdult(ctx, &msg, nil)
		if reply.Text != "" {
			err := s.reply(ctx, channel, cb.Message.Chat.ID, reply)
			s.deleteTelegramSourceMessage(channel, cb.Message.Chat.ID, cb.Message.MessageID)
			return err
		}
		return nil
	}
	if reply, handled := s.handleMenuCallback(ctx, channel, &msg, data); handled {
		if reply.Text != "" {
			err := s.reply(ctx, channel, cb.Message.Chat.ID, reply)
			s.deleteTelegramSourceMessage(channel, cb.Message.Chat.ID, cb.Message.MessageID)
			return err
		}
	}
	return nil
}

// answerCallback 应答 Telegram 回调查询，关闭按钮上的加载提示。
func (s *TelegramBotService) answerCallback(ctx context.Context, channel *model.NotifyChannel, callbackID string) {
	s.answerCallbackWithText(ctx, channel, callbackID, "", false)
}

func (s *TelegramBotService) answerCallbackWithText(ctx context.Context, channel *model.NotifyChannel, callbackID, text string, showAlert bool) {
	if channel == nil || strings.TrimSpace(callbackID) == "" {
		return
	}
	cfg := s.telegramChannelConfig(channel)
	if strings.TrimSpace(cfg["bot_token"]) == "" {
		return
	}
	payload := map[string]interface{}{
		"callback_query_id": callbackID,
	}
	if strings.TrimSpace(text) != "" {
		payload["text"] = text
		payload["show_alert"] = showAlert
	}
	if err := telegramPostJSON(ctx, cfg, "answerCallbackQuery", payload, 8*time.Second); err != nil {
		s.log.Debug("telegram answerCallbackQuery failed", zap.Error(sanitizeTelegramError(err)))
	}
}
