package service

import (
	"context"
	"fmt"
	"strconv"
	"strings"
	"time"

	"go.uber.org/zap"

	"github.com/ShukeBta/MediaStationGo/internal/model"
)

const defaultTelegramMessageDeleteDelay = 120 * time.Second

type telegramSendMessageResponse struct {
	OK     bool `json:"ok"`
	Result struct {
		MessageID int `json:"message_id"`
	} `json:"result"`
}

// reply 通过 Telegram Bot API 发送回复消息。
func (s *TelegramBotService) reply(ctx context.Context, channel *model.NotifyChannel, chatID int, reply telegramCommandReply) error {
	cfg := s.telegramChannelConfig(channel)
	if strings.TrimSpace(cfg["bot_token"]) == "" {
		return fmt.Errorf("bot_token not configured")
	}

	payload := map[string]interface{}{
		"chat_id":    strconv.Itoa(chatID),
		"text":       reply.Text,
		"parse_mode": "HTML",
	}
	if len(reply.Buttons) > 0 {
		keyboard := make([][]map[string]string, 0, len(reply.Buttons))
		for _, row := range reply.Buttons {
			buttons := make([]map[string]string, 0, len(row))
			for _, button := range row {
				buttons = append(buttons, map[string]string{
					"text":          button.Text,
					"callback_data": button.Data,
				})
			}
			keyboard = append(keyboard, buttons)
		}
		payload["reply_markup"] = map[string]interface{}{"inline_keyboard": keyboard}
	}
	var sent telegramSendMessageResponse
	if err := telegramPostJSONDecode(ctx, cfg, "sendMessage", payload, 15*time.Second, &sent); err != nil {
		return err
	}
	if sent.Result.MessageID > 0 {
		s.scheduleTelegramMessageDelete(cfg, chatID, sent.Result.MessageID)
	}
	return nil
}

func (s *TelegramBotService) replyForMessage(ctx context.Context, channel *model.NotifyChannel, msg *TelegramMessage, reply telegramCommandReply) error {
	if msg == nil {
		return nil
	}
	if strings.TrimSpace(reply.Text) == "" {
		return nil
	}
	return s.reply(ctx, channel, msg.Chat.ID, reply)
}

func (s *TelegramBotService) deleteTelegramSourceMessage(channel *model.NotifyChannel, chatID, messageID int) {
	if messageID <= 0 {
		return
	}
	s.scheduleTelegramMessageDelete(s.telegramChannelConfig(channel), chatID, messageID)
}

func (s *TelegramBotService) scheduleTelegramMessageDelete(cfg map[string]string, chatID, messageID int) {
	if chatID == 0 || messageID <= 0 || strings.TrimSpace(cfg["bot_token"]) == "" {
		return
	}
	delay := telegramMessageDeleteDelay(cfg)
	if delay < 0 {
		return
	}
	cfgCopy := make(map[string]string, len(cfg))
	for k, v := range cfg {
		cfgCopy[k] = v
	}
	go func() {
		if delay > 0 {
			timer := time.NewTimer(delay)
			defer timer.Stop()
			<-timer.C
		}
		deleteCtx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
		defer cancel()
		err := telegramPostJSON(deleteCtx, cfgCopy, "deleteMessage", map[string]interface{}{
			"chat_id":    strconv.Itoa(chatID),
			"message_id": messageID,
		}, 10*time.Second)
		if err != nil && s.log != nil {
			s.log.Debug("telegram deleteMessage failed",
				zap.Int("chat_id", chatID),
				zap.Int("message_id", messageID),
				zap.Error(sanitizeTelegramError(err)),
			)
		}
	}()
}

func telegramMessageDeleteDelay(cfg map[string]string) time.Duration {
	for _, key := range []string{"auto_delete_seconds", "message_delete_seconds", "delete_after_seconds"} {
		raw := strings.TrimSpace(cfg[key])
		if raw == "" {
			continue
		}
		seconds, err := strconv.Atoi(raw)
		if err != nil {
			continue
		}
		if seconds < 0 {
			return -1
		}
		return time.Duration(seconds) * time.Second
	}
	return defaultTelegramMessageDeleteDelay
}
