package service

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/ShukeBta/MediaStationGo/internal/model"
	"go.uber.org/zap"
)

func TestTelegramReplyAutoDeletesSentMessage(t *testing.T) {
	requests := make(chan string, 4)
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch {
		case strings.HasSuffix(r.URL.Path, "/sendMessage"):
			requests <- "sendMessage"
			w.Header().Set("Content-Type", "application/json")
			_, _ = w.Write([]byte(`{"ok":true,"result":{"message_id":777}}`))
		case strings.HasSuffix(r.URL.Path, "/deleteMessage"):
			requests <- "deleteMessage"
			w.Header().Set("Content-Type", "application/json")
			_, _ = w.Write([]byte(`{"ok":true,"result":true}`))
		default:
			http.NotFound(w, r)
		}
	}))
	defer server.Close()

	cfg, _ := json.Marshal(map[string]string{
		"bot_token":           "123456:ABC-def",
		"api_base_url":        server.URL,
		"auto_delete_seconds": "0",
	})
	_, bot := newBotTestService(t)
	channel := &model.NotifyChannel{Name: "Telegram", Type: "telegram", Enabled: true, Config: string(cfg)}
	if err := bot.reply(context.Background(), channel, 42, telegramCommandReply{Text: "hello"}); err != nil {
		t.Fatalf("reply: %v", err)
	}
	waitForTelegramMethod(t, requests, "sendMessage")
	waitForTelegramMethod(t, requests, "deleteMessage")
}

func TestTelegramGroupCommandSendsPanelInGroup(t *testing.T) {
	var payloads []struct {
		ChatID      any            `json:"chat_id"`
		Text        string         `json:"text"`
		ReplyMarkup map[string]any `json:"reply_markup"`
	}
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if !strings.HasSuffix(r.URL.Path, "/sendMessage") {
			http.NotFound(w, r)
			return
		}
		var payload struct {
			ChatID      any            `json:"chat_id"`
			Text        string         `json:"text"`
			ReplyMarkup map[string]any `json:"reply_markup"`
		}
		if err := json.NewDecoder(r.Body).Decode(&payload); err != nil {
			t.Fatalf("decode sendMessage: %v", err)
		}
		payloads = append(payloads, payload)
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"ok":true,"result":{"message_id":777}}`))
	}))
	defer server.Close()

	repos, bot := newBotTestService(t)
	cfg, _ := json.Marshal(map[string]string{
		"bot_token":           "123456:ABC-def",
		"api_base_url":        server.URL,
		"group_chat_id":       "-100123",
		"auto_delete_seconds": "-1",
	})
	if err := repos.DB.Create(&model.NotifyChannel{Name: "Telegram", Type: "telegram", Enabled: true, Config: string(cfg)}).Error; err != nil {
		t.Fatalf("create channel: %v", err)
	}
	update, _ := json.Marshal(TelegramUpdate{
		UpdateID: 1,
		Message: &TelegramMessage{
			MessageID: 55,
			From:      TelegramUser{ID: 9002, Username: "viewer", FirstName: "Viewer"},
			Chat:      TelegramChat{ID: -100123, Type: "supergroup"},
			Text:      "/menu",
		},
	})
	if err := bot.HandleWebhook(t.Context(), update); err != nil {
		t.Fatalf("handle webhook: %v", err)
	}
	if len(payloads) != 1 {
		t.Fatalf("sendMessage count = %d, payloads=%#v", len(payloads), payloads)
	}
	if got := fmt.Sprint(payloads[0].ChatID); got != "-100123" {
		t.Fatalf("message should stay in group, chat_id=%s payload=%#v", got, payloads[0])
	}
	if strings.Contains(payloads[0].Text, "管理员入口") {
		t.Fatalf("normal group user must not see admin panel: %#v", payloads[0])
	}
}

func TestTelegramGroupCallbackIsRejected(t *testing.T) {
	var callbackPayloads []struct {
		CallbackID string `json:"callback_query_id"`
		Text       string `json:"text"`
		ShowAlert  bool   `json:"show_alert"`
	}
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if !strings.HasSuffix(r.URL.Path, "/answerCallbackQuery") {
			http.NotFound(w, r)
			return
		}
		var payload struct {
			CallbackID string `json:"callback_query_id"`
			Text       string `json:"text"`
			ShowAlert  bool   `json:"show_alert"`
		}
		if err := json.NewDecoder(r.Body).Decode(&payload); err != nil {
			t.Fatalf("decode answerCallbackQuery: %v", err)
		}
		callbackPayloads = append(callbackPayloads, payload)
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"ok":true,"result":true}`))
	}))
	defer server.Close()

	ctx := t.Context()
	repos, auth, _, _ := newAuthTestServices(t)
	user, _, err := auth.Register(ctx, "viewer", "secret-pass")
	if err != nil {
		t.Fatalf("register user: %v", err)
	}
	if err := repos.DB.AutoMigrate(&model.NotifyChannel{}); err != nil {
		t.Fatalf("migrate notify channel: %v", err)
	}
	if err := repos.DB.Create(&model.TelegramBinding{
		TelegramUserID: 9002,
		TelegramName:   "@viewer",
		ChatID:         9002,
		UserID:         user.ID,
	}).Error; err != nil {
		t.Fatalf("create binding: %v", err)
	}
	cfg, _ := json.Marshal(map[string]string{
		"bot_token":           "123456:ABC-def",
		"api_base_url":        server.URL,
		"group_chat_id":       "-100123",
		"auto_delete_seconds": "-1",
	})
	if err := repos.DB.Create(&model.NotifyChannel{Name: "Telegram", Type: "telegram", Enabled: true, Config: string(cfg)}).Error; err != nil {
		t.Fatalf("create channel: %v", err)
	}
	before, _ := repos.User.FindByID(ctx, user.ID)
	bot := NewTelegramBotService(zap.NewNop(), repos, nil, auth)
	update, _ := json.Marshal(TelegramUpdate{
		UpdateID: 2,
		CallbackQuery: &TelegramCallbackQuery{
			ID:      "cb-group",
			From:    TelegramUser{ID: 9002, Username: "viewer", FirstName: "Viewer"},
			Message: &TelegramMessage{MessageID: 56, Chat: TelegramChat{ID: -100123, Type: "supergroup"}},
			Data:    "adult_toggle",
		},
	})
	if err := bot.HandleWebhook(ctx, update); err != nil {
		t.Fatalf("handle webhook: %v", err)
	}
	if len(callbackPayloads) != 1 {
		t.Fatalf("answerCallbackQuery count = %d", len(callbackPayloads))
	}
	if !callbackPayloads[0].ShowAlert || !strings.Contains(callbackPayloads[0].Text, "群组内按钮面板已禁用") {
		t.Fatalf("unexpected callback answer: %#v", callbackPayloads[0])
	}
	after, _ := repos.User.FindByID(ctx, user.ID)
	if before == nil || after == nil || before.HideAdult != after.HideAdult {
		t.Fatalf("group callback should not mutate user adult visibility: before=%#v after=%#v", before, after)
	}
}

func waitForTelegramMethod(t *testing.T, requests <-chan string, want string) {
	t.Helper()
	deadline := time.After(2 * time.Second)
	for {
		select {
		case got := <-requests:
			if got == want {
				return
			}
		case <-deadline:
			t.Fatalf("timed out waiting for telegram %s", want)
		}
	}
}
