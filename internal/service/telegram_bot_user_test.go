package service

import (
	"encoding/json"
	"strings"
	"testing"

	"go.uber.org/zap"

	"github.com/ShukeBta/MediaStationGo/internal/model"
)

func TestTelegramUpdateActionableDispatchesCallbackQuery(t *testing.T) {
	if !telegramUpdateActionable(TelegramUpdate{CallbackQuery: &TelegramCallbackQuery{Data: "adult_toggle"}}) {
		t.Fatal("callback_query update must be dispatched, otherwise inline buttons break")
	}
	if !telegramUpdateActionable(TelegramUpdate{Message: &TelegramMessage{Text: "/help"}}) {
		t.Fatal("text command message must be dispatched")
	}
	if telegramUpdateActionable(TelegramUpdate{}) {
		t.Fatal("empty update must be skipped")
	}
	if telegramUpdateActionable(TelegramUpdate{Message: &TelegramMessage{}}) {
		t.Fatal("message without text must be skipped")
	}
}

func TestTelegramCallbackTogglesAdultVisibility(t *testing.T) {
	ctx := t.Context()
	repos, auth, _, _ := newAuthTestServices(t)
	user, _, err := auth.Register(ctx, "viewer", "secret-pass")
	if err != nil {
		t.Fatalf("register user: %v", err)
	}
	if err := repos.DB.Create(&model.TelegramBinding{
		TelegramUserID: 30001,
		TelegramName:   "@viewer",
		ChatID:         30001,
		UserID:         user.ID,
	}).Error; err != nil {
		t.Fatalf("create binding: %v", err)
	}
	if err := repos.DB.AutoMigrate(&model.NotifyChannel{}); err != nil {
		t.Fatalf("migrate notify_channels: %v", err)
	}
	// 配置一个绑定该 Telegram 用户的渠道（无 bot_token，避免测试触发网络请求）。
	cfg, _ := json.Marshal(map[string]string{"admin_user_ids": "30001"})
	if err := repos.DB.Create(&model.NotifyChannel{
		Name:    "Telegram",
		Type:    "telegram",
		Enabled: true,
		Config:  string(cfg),
	}).Error; err != nil {
		t.Fatalf("create channel: %v", err)
	}

	before, err := repos.User.FindByID(ctx, user.ID)
	if err != nil || before == nil {
		t.Fatalf("load user before toggle: %v", err)
	}

	bot := NewTelegramBotService(zap.NewNop(), repos, nil)
	update, _ := json.Marshal(TelegramUpdate{
		UpdateID: 1,
		CallbackQuery: &TelegramCallbackQuery{
			ID:      "cb1",
			From:    TelegramUser{ID: 30001, Username: "viewer", FirstName: "Viewer"},
			Message: &TelegramMessage{MessageID: 5, Chat: TelegramChat{ID: 30001, Type: "private"}},
			Data:    "adult_toggle",
		},
	})
	// reply 因 bot_token 为空会返回错误，但成人目录状态应已在数据库中被切换。
	_ = bot.HandleWebhook(ctx, update)

	updated, err := repos.User.FindByID(ctx, user.ID)
	if err != nil || updated == nil {
		t.Fatalf("reload user: %v", err)
	}
	if updated.HideAdult == before.HideAdult {
		t.Fatalf("adult_toggle callback should have flipped HideAdult (was %v)", before.HideAdult)
	}
}

func TestTelegramStartClearsStaleUserBinding(t *testing.T) {
	repos, _, _, _ := newAuthTestServices(t)
	if err := repos.DB.Create(&model.TelegramBinding{
		TelegramUserID: 20001,
		TelegramName:   "@viewer",
		ChatID:         20001,
		UserID:         "deleted-user",
	}).Error; err != nil {
		t.Fatalf("create binding: %v", err)
	}
	bot := NewTelegramBotService(zap.NewNop(), repos, nil)

	reply := bot.cmdStart(t.Context(), &TelegramMessage{
		From: TelegramUser{ID: 20001, Username: "viewer", FirstName: "Viewer"},
		Chat: TelegramChat{ID: 20001, Type: "private"},
	}, nil)

	if !strings.Contains(reply.Text, "已不存在") {
		t.Fatalf("expected stale binding message, got %q", reply.Text)
	}
	var count int64
	if err := repos.DB.Model(&model.TelegramBinding{}).Where("telegram_user_id = ?", 20001).Count(&count).Error; err != nil {
		t.Fatalf("count binding: %v", err)
	}
	if count != 0 {
		t.Fatalf("stale binding should be removed, got %d", count)
	}
}
