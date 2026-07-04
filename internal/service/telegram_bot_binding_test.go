package service

import (
	"encoding/json"
	"errors"
	"strings"
	"testing"

	"go.uber.org/zap"

	"github.com/ShukeBta/MediaStationGo/internal/model"
)

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

func TestTelegramStartReplacesAccountBindingFromAnotherTelegram(t *testing.T) {
	ctx := t.Context()
	repos, auth, _, _ := newAuthTestServices(t)
	user, _, err := auth.Register(ctx, "viewer", "secret-pass")
	if err != nil {
		t.Fatalf("register: %v", err)
	}
	if err := repos.DB.Create(&model.TelegramBinding{
		TelegramUserID: 20001,
		TelegramName:   "@viewer-one",
		ChatID:         20001,
		UserID:         user.ID,
	}).Error; err != nil {
		t.Fatalf("create binding: %v", err)
	}

	bot := NewTelegramBotService(zap.NewNop(), repos, nil, auth)
	cfgJSON, _ := json.Marshal(map[string]string{"admin_user_ids": "20002"})
	if err := repos.DB.AutoMigrate(&model.NotifyChannel{}); err != nil {
		t.Fatalf("migrate notify channel: %v", err)
	}
	if err := repos.DB.Create(&model.NotifyChannel{Name: "Telegram", Type: "telegram", Enabled: true, Config: string(cfgJSON)}).Error; err != nil {
		t.Fatalf("create notify channel: %v", err)
	}
	msg := &TelegramMessage{
		From: TelegramUser{ID: 20002, Username: "viewer-two", FirstName: "Viewer Two"},
		Chat: TelegramChat{ID: 20002, Type: "private"},
	}
	reply := bot.cmdStart(ctx, msg, []string{"viewer", "secret-pass"})

	if !strings.Contains(reply.Text, "绑定成功") {
		t.Fatalf("expected new telegram account to replace old binding, got %q", reply.Text)
	}
	var accountBindings int64
	if err := repos.DB.Model(&model.TelegramBinding{}).Where("user_id = ?", user.ID).Count(&accountBindings).Error; err != nil {
		t.Fatalf("count account bindings: %v", err)
	}
	if accountBindings != 1 {
		t.Fatalf("account should keep exactly one telegram binding, got %d", accountBindings)
	}
	if binding := bot.telegramBinding(ctx, 20002); binding == nil || binding.UserID != user.ID {
		t.Fatalf("new telegram account should be bound to user, got %#v", binding)
	}
	if binding := bot.telegramBinding(ctx, 20001); binding != nil {
		t.Fatalf("old telegram binding should be removed, got %#v", binding)
	}
}

func TestTelegramStartUnbindsWhenBoundPasswordChanged(t *testing.T) {
	ctx := t.Context()
	repos, auth, _, _ := newAuthTestServices(t)
	user, _, err := auth.Register(ctx, "viewer", "old-password")
	if err != nil {
		t.Fatalf("register: %v", err)
	}
	if err := repos.DB.Create(&model.TelegramBinding{
		TelegramUserID: 20003,
		TelegramName:   "@viewer",
		ChatID:         20003,
		UserID:         user.ID,
	}).Error; err != nil {
		t.Fatalf("create binding: %v", err)
	}
	if err := auth.ResetPassword(ctx, user.ID, "new-password"); err != nil {
		t.Fatalf("reset password: %v", err)
	}
	if err := repos.DB.AutoMigrate(&model.NotifyChannel{}); err != nil {
		t.Fatalf("migrate notify channel: %v", err)
	}
	cfgJSON, _ := json.Marshal(map[string]string{"admin_user_ids": "20003"})
	if err := repos.DB.Create(&model.NotifyChannel{Name: "Telegram", Type: "telegram", Enabled: true, Config: string(cfgJSON)}).Error; err != nil {
		t.Fatalf("create notify channel: %v", err)
	}

	bot := NewTelegramBotService(zap.NewNop(), repos, nil, auth)
	msg := &TelegramMessage{
		From: TelegramUser{ID: 20003, Username: "viewer", FirstName: "Viewer"},
		Chat: TelegramChat{ID: 20003, Type: "private"},
	}
	reply := bot.cmdStart(ctx, msg, []string{"viewer", "old-password"})

	if !strings.Contains(reply.Text, "已自动解绑") {
		t.Fatalf("expected auto unbind reply, got %q", reply.Text)
	}
	if binding := bot.telegramBinding(ctx, 20003); binding != nil {
		t.Fatal("stale binding should be removed after password mismatch")
	}
}

func TestTelegramSelfSetNameRequiresCurrentPassword(t *testing.T) {
	ctx := t.Context()
	repos, auth, _, _ := newAuthTestServices(t)
	user, _, err := auth.Register(ctx, "viewer", "old-password")
	if err != nil {
		t.Fatalf("register: %v", err)
	}
	if err := repos.DB.Create(&model.TelegramBinding{
		TelegramUserID: 20004,
		TelegramName:   "@viewer",
		ChatID:         20004,
		UserID:         user.ID,
	}).Error; err != nil {
		t.Fatalf("create binding: %v", err)
	}

	bot := NewTelegramBotService(zap.NewNop(), repos, nil, auth)
	msg := &TelegramMessage{From: TelegramUser{ID: 20004, Username: "viewer"}, Chat: TelegramChat{ID: 20004, Type: "private"}}
	if reply := bot.selfSetName(ctx, msg, "renamed"); !strings.Contains(reply.Text, "当前密码 新用户名") {
		t.Fatalf("expected usage reply, got %q", reply.Text)
	}
	if reply := bot.selfSetName(ctx, msg, "old-password renamed"); !strings.Contains(reply.Text, "用户名已修改") {
		t.Fatalf("expected rename success, got %q", reply.Text)
	}
	updated, _ := repos.User.FindByID(ctx, user.ID)
	if updated == nil || updated.Username != "renamed" {
		t.Fatalf("username not updated: %#v", updated)
	}
}

func TestTelegramSelfSetPassWrongCurrentPasswordUnbinds(t *testing.T) {
	ctx := t.Context()
	repos, auth, _, _ := newAuthTestServices(t)
	user, _, err := auth.Register(ctx, "viewer", "old-password")
	if err != nil {
		t.Fatalf("register: %v", err)
	}
	if err := repos.DB.Create(&model.TelegramBinding{
		TelegramUserID: 20005,
		TelegramName:   "@viewer",
		ChatID:         20005,
		UserID:         user.ID,
	}).Error; err != nil {
		t.Fatalf("create binding: %v", err)
	}

	bot := NewTelegramBotService(zap.NewNop(), repos, nil, auth)
	msg := &TelegramMessage{From: TelegramUser{ID: 20005, Username: "viewer"}, Chat: TelegramChat{ID: 20005, Type: "private"}}
	reply := bot.selfSetPass(ctx, msg, "wrong-password new-password")

	if !strings.Contains(reply.Text, "已自动解绑") {
		t.Fatalf("expected auto unbind reply, got %q", reply.Text)
	}
	if binding := bot.telegramBinding(ctx, 20005); binding != nil {
		t.Fatal("binding should be removed after wrong current password")
	}
	if _, err := auth.Login(ctx, "viewer", "old-password"); err != nil {
		t.Fatalf("old password should remain valid after failed change: %v", err)
	}
}

func TestTelegramSelfSetPassChangesPasswordWithCurrentPassword(t *testing.T) {
	ctx := t.Context()
	repos, auth, _, _ := newAuthTestServices(t)
	user, _, err := auth.Register(ctx, "viewer", "old-password")
	if err != nil {
		t.Fatalf("register: %v", err)
	}
	if err := repos.DB.Create(&model.TelegramBinding{
		TelegramUserID: 20006,
		TelegramName:   "@viewer",
		ChatID:         20006,
		UserID:         user.ID,
	}).Error; err != nil {
		t.Fatalf("create binding: %v", err)
	}

	bot := NewTelegramBotService(zap.NewNop(), repos, nil, auth)
	msg := &TelegramMessage{From: TelegramUser{ID: 20006, Username: "viewer"}, Chat: TelegramChat{ID: 20006, Type: "private"}}
	reply := bot.selfSetPass(ctx, msg, "old-password new-password")

	if !strings.Contains(reply.Text, "密码已修改") {
		t.Fatalf("expected password change success, got %q", reply.Text)
	}
	if _, err := auth.Login(ctx, "viewer", "old-password"); !errors.Is(err, ErrInvalidCredentials) {
		t.Fatalf("old password should fail, got %v", err)
	}
	if _, err := auth.Login(ctx, "viewer", "new-password"); err != nil {
		t.Fatalf("new password should login: %v", err)
	}
	if binding := bot.telegramBinding(ctx, 20006); binding == nil {
		t.Fatal("successful password change should keep telegram binding")
	}
}

func TestTelegramBindingFromGroupStoresPrivateUserChatID(t *testing.T) {
	ctx := t.Context()
	repos, auth, _, _ := newAuthTestServices(t)
	user, _, err := auth.Register(ctx, "viewer", "secret-pass")
	if err != nil {
		t.Fatalf("register: %v", err)
	}
	bot := NewTelegramBotService(zap.NewNop(), repos, nil, auth)
	msg := &TelegramMessage{
		From: TelegramUser{ID: 21001, Username: "viewer", FirstName: "Viewer"},
		Chat: TelegramChat{ID: -100123456, Type: "group"},
	}

	if err := bot.upsertTelegramBinding(ctx, msg, user.ID); err != nil {
		t.Fatalf("upsert binding: %v", err)
	}
	binding := bot.telegramBinding(ctx, 21001)
	if binding == nil {
		t.Fatal("binding should be created")
	}
	if binding.ChatID != 21001 {
		t.Fatalf("group binding must store private user chat id, got %d", binding.ChatID)
	}
}

func TestTelegramBindingFromGroupPreservesExistingPrivateChatID(t *testing.T) {
	ctx := t.Context()
	repos, auth, _, _ := newAuthTestServices(t)
	user, _, err := auth.Register(ctx, "viewer", "secret-pass")
	if err != nil {
		t.Fatalf("register: %v", err)
	}
	if err := repos.DB.Create(&model.TelegramBinding{
		TelegramUserID: 21002,
		TelegramName:   "@viewer",
		ChatID:         987654,
		UserID:         user.ID,
	}).Error; err != nil {
		t.Fatalf("seed binding: %v", err)
	}
	bot := NewTelegramBotService(zap.NewNop(), repos, nil, auth)
	msg := &TelegramMessage{
		From: TelegramUser{ID: 21002, Username: "viewer", FirstName: "Viewer"},
		Chat: TelegramChat{ID: -100123456, Type: "supergroup"},
	}

	if err := bot.upsertTelegramBinding(ctx, msg, user.ID); err != nil {
		t.Fatalf("upsert binding: %v", err)
	}
	binding := bot.telegramBinding(ctx, 21002)
	if binding == nil {
		t.Fatal("binding should exist")
	}
	if binding.ChatID != 987654 {
		t.Fatalf("group command must not overwrite existing private chat id, got %d", binding.ChatID)
	}
}

func TestTelegramPrivateNotifyChatIDFallsBackFromLegacyGroupBinding(t *testing.T) {
	binding := model.TelegramBinding{
		TelegramUserID: 21003,
		ChatID:         -100123456,
	}
	if got := telegramPrivateChatIDFromBinding(binding); got != 21003 {
		t.Fatalf("legacy group binding should notify private user chat, got %d", got)
	}
}
