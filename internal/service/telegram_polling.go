package service

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"strings"
	"time"

	"go.uber.org/zap"

	"github.com/ShukeBta/MediaStationGo/internal/model"
)

// TelegramPollingStartResult describes what happened when local long polling
// was requested. The admin UI uses it to avoid a silent "started" toast when
// no Telegram channel can actually poll.
type TelegramPollingStartResult struct {
	Message        string   `json:"message"`
	Started        int      `json:"started"`
	AlreadyRunning int      `json:"already_running"`
	Skipped        int      `json:"skipped"`
	Errors         []string `json:"errors,omitempty"`
}

// StartPolling 为所有已启用的 Telegram 通知渠道启动长轮询。
func (s *TelegramBotService) StartPolling(ctx context.Context) TelegramPollingStartResult {
	result := TelegramPollingStartResult{Message: "telegram polling started"}
	channels, err := s.repo.NotifyChannel.ListByType(ctx, "telegram")
	if err != nil {
		s.log.Error("failed to list telegram channels for polling", zap.Error(err))
		result.Message = "failed to list telegram channels"
		result.Errors = append(result.Errors, err.Error())
		return result
	}
	if len(channels) == 0 {
		result.Message = "no telegram channels configured"
		result.Errors = append(result.Errors, "没有配置 Telegram 通知渠道")
		return result
	}

	for _, ch := range channels {
		if !ch.Enabled {
			result.Skipped++
			result.Errors = append(result.Errors, ch.Name+": 通知渠道未启用")
			continue
		}
		configStr := ch.Config
		if s.crypto != nil && configStr != "" {
			configStr = s.crypto.Decrypt(configStr)
		}
		var rawCfg map[string]any
		if err := json.Unmarshal([]byte(configStr), &rawCfg); err != nil {
			result.Skipped++
			result.Errors = append(result.Errors, ch.Name+": Telegram 配置解析失败: "+err.Error())
			continue
		}
		cfg := telegramStringConfigFromAny(rawCfg)
		botToken := cfg["bot_token"]
		if botToken == "" {
			result.Skipped++
			result.Errors = append(result.Errors, ch.Name+": Telegram Bot Token 为空")
			continue
		}
		s.pollingMu.Lock()
		if _, running := s.pollingCancel[botToken]; running {
			s.pollingMu.Unlock()
			result.AlreadyRunning++
			continue
		}
		s.pollingMu.Unlock()

		if err := registerTelegramBotCommands(ctx, cfg); err != nil && s.log != nil {
			s.log.Warn("telegram setMyCommands failed", zap.Error(sanitizeTelegramError(err)))
		}
		if err := deleteTelegramWebhook(ctx, cfg); err != nil {
			result.Skipped++
			result.Errors = append(result.Errors, ch.Name+": "+sanitizeTelegramError(err).Error())
			continue
		}

		s.pollingMu.Lock()
		if _, running := s.pollingCancel[botToken]; running {
			s.pollingMu.Unlock()
			result.AlreadyRunning++
			continue
		}
		pollCtx, cancel := context.WithCancel(context.Background())
		s.pollingCancel[botToken] = cancel
		s.pollingMu.Unlock()

		channel := ch
		go s.pollLoop(pollCtx, cfg, &channel)
		result.Started++
		s.log.Info("started telegram polling", zap.String("channel", ch.Name))
	}
	if result.Started == 0 && result.AlreadyRunning == 0 {
		result.Message = "no enabled telegram channels started"
	}
	return result
}

// StopPolling 停止所有 Telegram 长轮询。
func (s *TelegramBotService) StopPolling() int {
	s.pollingMu.Lock()
	defer s.pollingMu.Unlock()
	stopped := 0
	for token, cancel := range s.pollingCancel {
		cancel()
		delete(s.pollingCancel, token)
		stopped++
	}
	s.log.Info("telegram polling stopped")
	return stopped
}

// pollLoop 对单个 Bot Token 执行长轮询。
func (s *TelegramBotService) pollLoop(ctx context.Context, cfg map[string]string, channel *model.NotifyChannel) {
	var offset int64 = 0
	pollURL, err := telegramMethodURL(cfg, cfg["bot_token"], "getUpdates")
	if err != nil {
		s.log.Warn("telegram polling config invalid", zap.Error(err))
		return
	}
	clients := telegramHTTPClients(45*time.Second, cfg)

	for {
		select {
		case <-ctx.Done():
			return
		default:
		}

		reqBody, _ := json.Marshal(map[string]interface{}{
			"offset":  offset,
			"timeout": 30,
		})
		respBody, err := telegramPollingRequest(ctx, clients, pollURL, string(reqBody))
		if err != nil {
			s.log.Debug("telegram polling failed", zap.Error(err))
			time.Sleep(5 * time.Second)
			continue
		}

		var result struct {
			OK     bool             `json:"ok"`
			Result []TelegramUpdate `json:"result"`
		}
		if err := json.Unmarshal(respBody, &result); err != nil || !result.OK {
			time.Sleep(3 * time.Second)
			continue
		}

		for _, upd := range result.Result {
			if upd.UpdateID >= int(offset) {
				offset = int64(upd.UpdateID) + 1
			}
			if !telegramUpdateActionable(upd) {
				continue
			}
			go func(u TelegramUpdate) {
				handlerCtx, cancel := context.WithTimeout(ctx, 2*time.Minute)
				defer cancel()
				_ = s.handleTelegramUpdate(handlerCtx, u, channel)
			}(upd)
		}
	}
}

// telegramUpdateActionable 判断一条 update 是否需要分发处理。
// 长轮询默认会返回 message 与 callback_query 两类更新；命令消息需有文本，
// 而内联按钮回调（callback_query）必须被分发，否则成人目录显隐开关会失效。
func telegramUpdateActionable(upd TelegramUpdate) bool {
	if upd.CallbackQuery != nil {
		return true
	}
	return upd.Message != nil && upd.Message.Text != ""
}

func telegramPollingRequest(ctx context.Context, clients []*http.Client, pollURL, body string) ([]byte, error) {
	var lastErr error
	for _, client := range clients {
		req, err := http.NewRequestWithContext(ctx, http.MethodPost, pollURL, strings.NewReader(body))
		if err != nil {
			return nil, err
		}
		req.Header.Set("Content-Type", "application/json")
		resp, err := client.Do(req)
		if err != nil {
			lastErr = sanitizeTelegramError(err)
			continue
		}
		respBody, _ := io.ReadAll(resp.Body)
		_ = resp.Body.Close()
		if resp.StatusCode >= 400 {
			lastErr = fmt.Errorf("telegram api error %d: %s", resp.StatusCode, sanitizeTelegramText(string(respBody)))
			continue
		}
		return respBody, nil
	}
	if lastErr != nil {
		return nil, lastErr
	}
	return nil, errors.New("telegram polling failed")
}
