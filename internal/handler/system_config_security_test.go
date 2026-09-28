package handler

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/gin-gonic/gin"
	"github.com/glebarez/sqlite"
	"gorm.io/gorm"

	"github.com/ShukeBta/MediaStationGo/internal/middleware"
	"github.com/ShukeBta/MediaStationGo/internal/model"
	"github.com/ShukeBta/MediaStationGo/internal/repository"
	"github.com/ShukeBta/MediaStationGo/internal/service"
)

func TestSystemConfigProtectsPrivateSettingsAndPreservesAdminAccess(t *testing.T) {
	gin.SetMode(gin.TestMode)
	db, err := gorm.Open(sqlite.Open(":memory:"), &gorm.Config{})
	if err != nil {
		t.Fatal(err)
	}
	sqlDB, err := db.DB()
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = sqlDB.Close() })
	if err := db.AutoMigrate(&model.Setting{}); err != nil {
		t.Fatal(err)
	}
	repos := repository.New(db)
	svc := &service.Container{Repo: repos}

	privateSettings := map[string]string{
		"notify.telegram.bot_token": "telegram-bot-credential",
		"license.hmac_secret":       "license-signing-credential",
		"notify.bark.key":           "bark-credential",
		"notify.wechat.sendkey":     "wechat-credential",
		"telegram.webhook." + strings.Repeat("a", 64) + ".secret": "telegram-webhook-credential",
		"qbittorrent.password": "download-password",
		"provider.api_key":     "provider-credential",
		"provider.cookie":      "provider-session",
		"adult.pin":            "123456",
		// Credentials also occur in URLs and structured values with no
		// secret-looking suffix. New keys must be private by default.
		"notify.webhook.url":        "https://notify.example/send?key=webhook-credential",
		"notify.telegram.proxy_url": "https://user:proxy-credential@proxy.example",
		"custom.integration":        `{"authorization":"integration-credential"}`,
		"transcode.access_token":    "transcode-credential",
	}
	publicSettings := map[string]string{
		"tmdb.language":           "zh-CN",
		"adult.enabled":           "true",
		"adult.require_pin":       "true",
		"transcode.enabled":       "true",
		"transcoder.enabled":      "true",
		"transcode.hw_accel":      "qsv",
		"transcode.max_height":    "1080",
		"transcode.video_bitrate": "6M",
	}
	for _, values := range []map[string]string{privateSettings, publicSettings} {
		for key, value := range values {
			if err := repos.Setting.Set(t.Context(), key, value); err != nil {
				t.Fatal(err)
			}
		}
	}

	for _, role := range []string{"user", "admin", "unknown", ""} {
		name := role
		if name == "" {
			name = "missing-role"
		}
		t.Run(name, func(t *testing.T) {
			router := gin.New()
			router.Use(func(c *gin.Context) {
				if role != "" {
					c.Set(middleware.CtxUserRole, role)
				}
				c.Next()
			})
			router.GET("/api/system/config", listSystemConfigHandler(svc))
			w := httptest.NewRecorder()
			router.ServeHTTP(w, httptest.NewRequest(http.MethodGet, "/api/system/config", nil))
			if w.Code != http.StatusOK {
				t.Fatalf("status = %d, body = %s", w.Code, w.Body.String())
			}
			var response struct {
				Items []model.Setting `json:"items"`
			}
			if err := json.Unmarshal(w.Body.Bytes(), &response); err != nil {
				t.Fatal(err)
			}
			if len(response.Items) != len(privateSettings)+len(publicSettings) {
				t.Fatalf("expected all setting rows for compatibility, got %d", len(response.Items))
			}
			values := make(map[string]string, len(response.Items))
			for _, row := range response.Items {
				values[row.Key] = row.Value
			}
			for key, original := range privateSettings {
				want := "********"
				if role == "admin" {
					want = original
				}
				if values[key] != want {
					t.Errorf("private setting %q is not correctly protected for role %q", key, role)
				}
				if role != "admin" && strings.Contains(w.Body.String(), original) {
					t.Errorf("response contains private value for %q", key)
				}
			}
			for key, want := range publicSettings {
				if values[key] != want {
					t.Errorf("public setting %q = %q, want %q", key, values[key], want)
				}
			}
		})
	}

	for key, want := range privateSettings {
		got, err := repos.Setting.Get(t.Context(), key)
		if err != nil {
			t.Fatal(err)
		}
		if got != want {
			t.Errorf("reading config changed stored value for %q", key)
		}
	}
}
