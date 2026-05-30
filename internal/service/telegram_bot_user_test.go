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

	bot := NewTelegramBotService(zap.NewNop(), repos, nil, auth)
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

func TestTelegramRegisterRespectsAdminToggle(t *testing.T) {
	ctx := t.Context()
	repos, auth, _, _ := newAuthTestServices(t)
	if err := repos.DB.AutoMigrate(&model.Setting{}, &model.NotifyChannel{}); err != nil {
		t.Fatalf("migrate: %v", err)
	}
	// 预置一个管理员，确保通过 Bot 注册的用户是普通角色而非首个管理员。
	if _, _, err := auth.Register(ctx, "rootadmin", "admin-pass"); err != nil {
		t.Fatalf("seed admin: %v", err)
	}

	bot := NewTelegramBotService(zap.NewNop(), repos, nil, auth)
	// 把注册者放进 admin_user_ids，即可让 telegramUserCanBind 通过（私聊场景，
	// 无需走 getChatMember 网络校验）；注册流程本身不依赖角色。
	cfgJSON, _ := json.Marshal(map[string]string{"admin_user_ids": "999"})
	channel := &model.NotifyChannel{Name: "Telegram", Type: "telegram", Enabled: true, Config: string(cfgJSON)}

	msg := &TelegramMessage{From: TelegramUser{ID: 999, Username: "newbie", FirstName: "Newbie"}, Chat: TelegramChat{ID: 999, Type: "private"}}

	// 默认关闭：拒绝且不创建用户。
	if reply := bot.cmdRegister(ctx, channel, msg, []string{"newbie", "secret-pass"}); !strings.Contains(reply.Text, "未开放") {
		t.Fatalf("registration disabled by default, got %q", reply.Text)
	}
	if u, _ := repos.User.FindByUsername(ctx, "newbie"); u != nil {
		t.Fatal("no user should be created while registration disabled")
	}

	// 管理员开启后注册成功并自动绑定。
	if err := bot.setRegistrationEnabled(ctx, true); err != nil {
		t.Fatalf("enable registration: %v", err)
	}
	reply := bot.cmdRegister(ctx, channel, msg, []string{"newbie", "secret-pass"})
	if !strings.Contains(reply.Text, "注册并绑定成功") {
		t.Fatalf("expected success reply, got %q", reply.Text)
	}
	created, err := repos.User.FindByUsername(ctx, "newbie")
	if err != nil || created == nil {
		t.Fatalf("user should be created after enabling: %v", err)
	}
	if created.Role != "user" {
		t.Fatalf("bot-registered account should be a regular user, got role %q", created.Role)
	}
	if binding := bot.telegramBinding(ctx, 999); binding == nil || binding.UserID != created.ID {
		t.Fatalf("telegram should be bound to the newly registered user")
	}

	// 重复注册：已绑定 → 提示无需重复注册。
	if reply := bot.cmdRegister(ctx, channel, msg, []string{"another", "pass-2"}); !strings.Contains(reply.Text, "无需重复注册") {
		t.Fatalf("expected already-bound reply, got %q", reply.Text)
	}
}

func TestTelegramStartClearsStaleUserBinding(t *testing.T) {
	repos, auth, _, _ := newAuthTestServices(t)
	if err := repos.DB.Create(&model.TelegramBinding{
		TelegramUserID: 20001,
		TelegramName:   "@viewer",
		ChatID:         20001,
		UserID:         "deleted-user",
	}).Error; err != nil {
		t.Fatalf("create binding: %v", err)
	}
	bot := NewTelegramBotService(zap.NewNop(), repos, nil, auth)

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
