package handler

import (
	"crypto/sha256"
	"encoding/hex"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/ShukeBta/MediaStationGo/internal/model"
	"github.com/ShukeBta/MediaStationGo/internal/repository"
	"github.com/ShukeBta/MediaStationGo/internal/service"
	"github.com/gin-gonic/gin"
	"github.com/glebarez/sqlite"
	"go.uber.org/zap"
	"gorm.io/gorm"
)

func TestTelegramWebhookHTTPRequiresRegisteredSourceSecret(t *testing.T) {
	gin.SetMode(gin.TestMode)
	db, err := gorm.Open(sqlite.Open(":memory:"), &gorm.Config{})
	if err != nil {
		t.Fatal(err)
	}
	if err := db.AutoMigrate(&model.NotifyChannel{}, &model.Setting{}); err != nil {
		t.Fatal(err)
	}
	repos := repository.New(db)
	log := zap.NewNop()
	svc := &service.Container{Repo: repos, Log: log, TelegramBot: service.NewTelegramBotService(log, repos, nil, nil)}
	router := gin.New()
	router.POST("/api/telegram/webhook", telegramWebhookHandler(svc))
	request := func(secret, body string, want int) {
		t.Helper()
		req := httptest.NewRequest(http.MethodPost, "/api/telegram/webhook", strings.NewReader(body))
		if secret != "" {
			req.Header.Set("X-Telegram-Bot-Api-Secret-Token", secret)
		}
		response := httptest.NewRecorder()
		router.ServeHTTP(response, req)
		if response.Code != want {
			t.Fatalf("webhook returned %d, want %d: %s", response.Code, want, response.Body.String())
		}
	}
	request("", "invalid json", http.StatusForbidden)
	request("unconfigured", "invalid json", http.StatusForbidden)
	channel := &model.NotifyChannel{Type: "telegram", Name: "Bot", Enabled: true, Config: `{"bot_token":"100:bot-a"}`}
	if err := repos.NotifyChannel.Create(t.Context(), channel); err != nil {
		t.Fatal(err)
	}
	request("unregistered", "invalid json", http.StatusForbidden)
	digest := sha256.Sum256([]byte("100:bot-a"))
	if err := repos.Setting.Set(t.Context(), "telegram.webhook."+hex.EncodeToString(digest[:])+".secret", "test-source-secret"); err != nil {
		t.Fatal(err)
	}
	request("wrong-secret", `{"message":{"from":{"id":111},"chat":{"id":111,"type":"private"},"text":"/openreg on"}}`, http.StatusForbidden)
	request("test-source-secret", `{"update_id":1}`, http.StatusOK)
	channel.Enabled = false
	if err := repos.NotifyChannel.Update(t.Context(), channel); err != nil {
		t.Fatal(err)
	}
	request("test-source-secret", `{"update_id":1}`, http.StatusForbidden)
}
