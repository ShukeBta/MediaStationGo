package service

import (
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync/atomic"
	"testing"

	"github.com/ShukeBta/MediaStationGo/internal/model"
)

func TestTelegramWebhookRegistersPersistentSecretAndAuthenticates(t *testing.T) {
	repos, bot := newBotTestService(t)
	var registeredSecret string
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if strings.HasSuffix(r.URL.Path, "/setWebhook") {
			var payload struct {
				Secret string `json:"secret_token"`
				URL    string `json:"url"`
			}
			if err := json.NewDecoder(r.Body).Decode(&payload); err != nil {
				t.Errorf("decode registration: %v", err)
			}
			registeredSecret = payload.Secret
			if payload.URL != "https://example.test/api/telegram/webhook" {
				t.Error("webhook URL not preserved")
			}
			persisted, err := repos.Setting.Get(r.Context(), telegramWebhookSecretKey("100:bot-a"))
			if err != nil || persisted != payload.Secret || len(payload.Secret) != 64 {
				t.Error("source secret must be securely generated and persisted before registration")
			}
		}
		_, _ = w.Write([]byte(`{"ok":true,"result":true}`))
	}))
	defer server.Close()
	addWebhookTestChannel(t, bot, "100:bot-a", "111", server.URL, true)
	update := []byte(`{"message":{"from":{"id":111},"chat":{"id":111,"type":"private"},"text":"/openreg on"}}`)
	if err := bot.HandleAuthenticatedWebhook(t.Context(), update, "unregistered"); !errors.Is(err, ErrTelegramWebhookUnauthorized) {
		t.Fatalf("unregistered webhook was not rejected: %v", err)
	}
	if err := bot.SetWebhook(t.Context(), "100:bot-a", "https://example.test/api/telegram/webhook"); err != nil {
		t.Fatalf("register: %v", err)
	}
	for _, supplied := range []string{"", "wrong-secret"} {
		if err := bot.HandleAuthenticatedWebhook(t.Context(), update, supplied); !errors.Is(err, ErrTelegramWebhookUnauthorized) {
			t.Fatalf("invalid credentials were not rejected: %v", err)
		}
		if bot.registrationEnabled(t.Context()) {
			t.Fatal("spoofed administrator update changed registration settings")
		}
	}
	// A new service instance represents a restart; verification cannot depend on memory.
	restarted := NewTelegramBotService(bot.log, repos, bot.crypto, bot.auth)
	if err := restarted.HandleAuthenticatedWebhook(t.Context(), update, registeredSecret); err != nil {
		t.Fatalf("authenticated update after restart: %v", err)
	}
	if !bot.registrationEnabled(t.Context()) {
		t.Fatal("valid administrator update was not dispatched")
	}
}

func TestTelegramWebhookRegistrationFailureDoesNotRotateSecret(t *testing.T) {
	_, bot := newBotTestService(t)
	var reject atomic.Bool
	var requests atomic.Int32
	var secrets []string
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		requests.Add(1)
		if strings.HasSuffix(r.URL.Path, "/setWebhook") {
			var payload struct {
				Secret string `json:"secret_token"`
			}
			_ = json.NewDecoder(r.Body).Decode(&payload)
			secrets = append(secrets, payload.Secret)
			if reject.Load() {
				_, _ = w.Write([]byte(`{"ok":false,"description":"registration failed"}`))
				return
			}
		}
		_, _ = w.Write([]byte(`{"ok":true,"result":true}`))
	}))
	defer server.Close()
	addWebhookTestChannel(t, bot, "100:bot-a", "111", server.URL, true)
	reject.Store(true)
	if err := bot.SetWebhook(t.Context(), "100:bot-a", "https://example.test/webhook"); err == nil {
		t.Fatal("Telegram ok:false must fail registration")
	}
	reject.Store(false)
	if err := bot.SetWebhook(t.Context(), "100:bot-a", "https://example.test/webhook"); err != nil {
		t.Fatal(err)
	}
	reject.Store(true)
	if err := bot.SetWebhook(t.Context(), "100:bot-a", "https://example.test/webhook"); err == nil {
		t.Fatal("failed re-registration must be reported")
	}
	if len(secrets) != 3 || secrets[0] == "" || secrets[0] != secrets[1] || secrets[1] != secrets[2] {
		t.Fatal("registration retries changed the persisted secret")
	}
	if err := bot.HandleAuthenticatedWebhook(t.Context(), []byte(`{}`), secrets[0]); err != nil {
		t.Fatalf("failed re-registration invalidated the active secret: %v", err)
	}
	before := requests.Load()
	if err := bot.SetWebhook(t.Context(), "200:unconfigured", "https://example.test/webhook"); err == nil {
		t.Fatal("unconfigured Bot registration must fail")
	}
	if err := bot.repo.DB.Migrator().DropTable(&model.Setting{}); err != nil {
		t.Fatal(err)
	}
	if err := bot.SetWebhook(t.Context(), "100:bot-a", "https://example.test/webhook"); err == nil {
		t.Fatal("database failure must fail registration")
	}
	if requests.Load() != before {
		t.Fatal("failed local validation/persistence made remote requests")
	}
}

func TestTelegramWebhookBindsSourceSecretToEnabledBot(t *testing.T) {
	_, bot := newBotTestService(t)
	var paths []string
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		paths = append(paths, r.URL.Path)
		_, _ = w.Write([]byte(`{"ok":true,"result":true}`))
	}))
	defer server.Close()
	addWebhookTestChannel(t, bot, "100:bot-a", "111", server.URL, true)
	channelB := addWebhookTestChannel(t, bot, "200:bot-b", "222", server.URL, true)
	secretA, err := bot.ensureTelegramWebhookSecret(t.Context(), "100:bot-a")
	if err != nil {
		t.Fatal(err)
	}
	secretB, err := bot.ensureTelegramWebhookSecret(t.Context(), "200:bot-b")
	if err != nil {
		t.Fatal(err)
	}
	if secretA == secretB {
		t.Fatal("different Bots must have independent secrets")
	}
	update := []byte(`{"message":{"from":{"id":222},"chat":{"id":222,"type":"private"},"text":"/openreg on"}}`)
	if err := bot.HandleAuthenticatedWebhook(t.Context(), update, secretA); err != nil {
		t.Fatal(err)
	}
	if bot.registrationEnabled(t.Context()) {
		t.Fatal("Bot A secret allowed a command using Bot B's administrator configuration")
	}
	for _, path := range paths {
		if !strings.HasPrefix(path, "/bot100:bot-a/") {
			t.Fatal("authenticated Bot A update was routed to another Bot")
		}
	}
	if err := bot.HandleAuthenticatedWebhook(t.Context(), update, secretB); err != nil {
		t.Fatal(err)
	}
	if !bot.registrationEnabled(t.Context()) {
		t.Fatal("Bot B's own administrator command was not dispatched")
	}
	channelB.Enabled = false
	if err := bot.repo.NotifyChannel.Update(t.Context(), channelB); err != nil {
		t.Fatal(err)
	}
	if err := bot.HandleAuthenticatedWebhook(t.Context(), update, secretB); !errors.Is(err, ErrTelegramWebhookUnauthorized) {
		t.Fatalf("disabled Bot secret accepted: %v", err)
	}
	if err := bot.repo.NotifyChannel.Delete(t.Context(), channelB.ID); err != nil {
		t.Fatal(err)
	}
	if err := bot.HandleAuthenticatedWebhook(t.Context(), update, secretB); !errors.Is(err, ErrTelegramWebhookUnauthorized) {
		t.Fatalf("deleted Bot secret accepted: %v", err)
	}
}

func TestTelegramWebhookSelectsGroupWithinAuthenticatedBot(t *testing.T) {
	_, bot := newBotTestService(t)
	var sentChats []string
	var requests atomic.Int32
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		requests.Add(1)
		if strings.HasSuffix(r.URL.Path, "/sendMessage") {
			var payload struct {
				ChatID string `json:"chat_id"`
			}
			if err := json.NewDecoder(r.Body).Decode(&payload); err != nil {
				t.Errorf("decode message: %v", err)
			}
			sentChats = append(sentChats, payload.ChatID)
		}
		_, _ = w.Write([]byte(`{"ok":true,"result":true}`))
	}))
	defer server.Close()
	for _, entry := range []struct{ token, group string }{{"100:bot-a", "-1001"}, {"100:bot-a", "-1002"}, {"200:bot-b", "-2001"}} {
		ch := addWebhookTestChannel(t, bot, entry.token, "111", server.URL, true)
		cfg := bot.telegramChannelConfig(ch)
		cfg["group_chat_id"] = entry.group
		encoded, _ := json.Marshal(cfg)
		ch.Config = bot.crypto.Encrypt(string(encoded))
		if err := bot.repo.NotifyChannel.Update(t.Context(), ch); err != nil {
			t.Fatal(err)
		}
	}
	secret, err := bot.ensureTelegramWebhookSecret(t.Context(), "100:bot-a")
	if err != nil {
		t.Fatal(err)
	}
	if err := bot.HandleAuthenticatedWebhook(t.Context(), []byte(`{"message":{"from":{"id":111},"chat":{"id":-1002,"type":"supergroup"},"text":"/menu"}}`), secret); err != nil {
		t.Fatal(err)
	}
	if len(sentChats) != 1 || sentChats[0] != "-1002" {
		t.Fatalf("same-Bot second group was not selected: %v", sentChats)
	}
	before := requests.Load()
	for _, update := range []string{
		`{"message":{"from":{"id":111},"chat":{"id":-2001,"type":"supergroup"},"text":"/menu"}}`,
		`{"callback_query":{"id":"callback","from":{"id":111},"message":{"chat":{"id":-2001,"type":"supergroup"}},"data":"adm_openreg_set:0"}}`,
	} {
		if err := bot.HandleAuthenticatedWebhook(t.Context(), []byte(update), secret); err != nil {
			t.Fatal(err)
		}
	}
	if requests.Load() != before || bot.registrationEnabled(t.Context()) {
		t.Fatal("group update crossed the authenticated Bot boundary")
	}
}

func addWebhookTestChannel(t *testing.T, bot *TelegramBotService, token, adminID, apiBase string, enabled bool) *model.NotifyChannel {
	t.Helper()
	cfg, err := json.Marshal(map[string]string{
		"bot_token": token, "admin_user_ids": adminID, "api_base_url": apiBase, "auto_delete_seconds": "-1",
	})
	if err != nil {
		t.Fatal(err)
	}
	channel := &model.NotifyChannel{Name: token, Type: "telegram", Enabled: enabled, Config: bot.crypto.Encrypt(string(cfg))}
	if err := bot.repo.NotifyChannel.Create(t.Context(), channel); err != nil {
		t.Fatal(err)
	}
	return channel
}
