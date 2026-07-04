package service

import (
	"context"
	"strings"
	"testing"
	"time"

	"go.uber.org/zap"

	"github.com/ShukeBta/MediaStationGo/internal/model"
)

func TestBotRegistrationCommandUsesOpenRegQuota(t *testing.T) {
	ctx := context.Background()
	repos, bot := newBotTestService(t)
	admin := &model.User{Username: "root", PasswordHash: "x", Role: "admin", IsActive: true}
	if err := repos.User.Create(ctx, admin); err != nil {
		t.Fatal(err)
	}
	if err := repos.Setting.Set(ctx, SettingOpenRegEnabled, "false"); err != nil {
		t.Fatal(err)
	}
	if err := repos.DB.Create(&model.TelegramBinding{
		TelegramUserID: 9051,
		TelegramName:   "@root",
		ChatID:         9051,
		UserID:         admin.ID,
	}).Error; err != nil {
		t.Fatal(err)
	}
	channel := &model.NotifyChannel{Name: "Telegram", Type: "telegram", Enabled: true, Config: `{"admin_user_ids":"9051"}`}
	msg := &TelegramMessage{From: TelegramUser{ID: 9051, Username: "root"}, Chat: TelegramChat{ID: 9051, Type: "private"}}

	reply, err := bot.executeCommand(ctx, channel, msg, "/registration on 2")
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(reply.Text, "2 个名额") {
		t.Fatalf("expected quota feedback, got %q", reply.Text)
	}
	capacity := bot.loadCapacity(ctx)
	if !capacity.OpenRegOn || capacity.OpenRegLimit != 2 || capacity.OpenRegUsed != 0 {
		t.Fatalf("registration command should open quota-aware registration, got %+v", capacity)
	}
}

func TestBotUserCommandsAndAdminGate(t *testing.T) {
	ctx := context.Background()
	repos, bot := newBotTestService(t)
	user := &model.User{Username: "viewer", PasswordHash: "x", Role: "user", IsActive: true}
	if err := repos.User.Create(ctx, user); err != nil {
		t.Fatal(err)
	}
	if err := repos.DB.Create(&model.TelegramBinding{
		TelegramUserID: 9101,
		TelegramName:   "@viewer",
		ChatID:         9101,
		UserID:         user.ID,
	}).Error; err != nil {
		t.Fatal(err)
	}
	now := time.Now()
	if err := repos.UserDevice.Create(ctx, &model.UserDevice{
		UserID: user.ID, DeviceID: "dev-1", DeviceName: "iPhone", Client: "Infuse", FirstSeenAt: now, LastSeenAt: now,
	}); err != nil {
		t.Fatal(err)
	}
	channel := &model.NotifyChannel{Name: "Telegram", Type: "telegram", Enabled: true, Config: `{"admin_user_ids":"9001"}`}
	msg := &TelegramMessage{From: TelegramUser{ID: 9101, Username: "viewer"}, Chat: TelegramChat{ID: 9101, Type: "private"}}

	reply, err := bot.executeCommand(ctx, channel, msg, "/antishare on")
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(reply.Text, "仅管理员") {
		t.Fatalf("regular user should not manage policy, got %q", reply.Text)
	}

	reply, err = bot.executeCommand(ctx, channel, msg, "/devices")
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(reply.Text, "我的登录设备") {
		t.Fatalf("expected device list, got %q", reply.Text)
	}

	reply, err = bot.executeCommand(ctx, channel, msg, "/kick 1")
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(reply.Text, "已踢下线") {
		t.Fatalf("expected kick feedback, got %q", reply.Text)
	}
	if kicked := bot.device; kicked != nil {
		t.Fatal("test should not require wired device service")
	}
	if ok := NewDeviceService(zap.NewNop(), repos).IsDeviceKicked(ctx, user.ID, "dev-1"); !ok {
		t.Fatal("device should be marked kicked")
	}
}

func TestBotAdminCodeAndUserCommands(t *testing.T) {
	ctx := context.Background()
	repos, bot := newBotTestService(t)
	admin := &model.User{Username: "root", PasswordHash: "x", Role: "admin", IsActive: true}
	user := &model.User{Username: "viewer", PasswordHash: "x", Role: "user", IsActive: true}
	if err := repos.User.Create(ctx, admin); err != nil {
		t.Fatal(err)
	}
	if err := repos.User.Create(ctx, user); err != nil {
		t.Fatal(err)
	}
	if err := repos.DB.Create(&model.TelegramBinding{
		TelegramUserID: 9301,
		TelegramName:   "@root",
		ChatID:         9301,
		UserID:         admin.ID,
	}).Error; err != nil {
		t.Fatal(err)
	}
	channel := &model.NotifyChannel{Name: "Telegram", Type: "telegram", Enabled: true, Config: `{"admin_user_ids":"9301"}`}
	msg := &TelegramMessage{From: TelegramUser{ID: 9301, Username: "root"}, Chat: TelegramChat{ID: 9301, Type: "private"}}

	reply, err := bot.executeCommand(ctx, channel, msg, "/gencode renew 90 7")
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(reply.Text, "已生成续期码") {
		t.Fatalf("expected generated renew code, got %q", reply.Text)
	}

	reply, err = bot.executeCommand(ctx, channel, msg, "/renew_user viewer 30")
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(reply.Text, "viewer") {
		t.Fatalf("renew command should return user actions, got %q", reply.Text)
	}
	updated, _ := repos.User.FindByID(ctx, user.ID)
	if updated.ExpiredAt == nil || updated.ExpiredAt.Before(time.Now()) {
		t.Fatalf("renew_user should set future expiry, got %v", updated.ExpiredAt)
	}

	reply, err = bot.executeCommand(ctx, channel, msg, "/delete_user viewer")
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(reply.Text, "需要确认") {
		t.Fatalf("delete without confirm should be rejected, got %q", reply.Text)
	}
}

func TestBotGroupMenuShowsAdminActionsOnlyForAdmins(t *testing.T) {
	ctx := context.Background()
	_, bot := newBotTestService(t)
	channel := &model.NotifyChannel{Name: "Telegram", Type: "telegram", Enabled: true, Config: `{"admin_user_ids":"9301","group_chat_id":"-1001"}`}
	adminMsg := &TelegramMessage{From: TelegramUser{ID: 9301, Username: "admin"}, Chat: TelegramChat{ID: -1001, Type: "group"}}
	reply, err := bot.executeCommand(ctx, channel, adminMsg, "/menu")
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(reply.Text, "管理员入口") || len(reply.Buttons) == 0 {
		t.Fatalf("admin group menu should expose management actions, got %#v", reply)
	}

	userMsg := &TelegramMessage{From: TelegramUser{ID: 9302, Username: "user"}, Chat: TelegramChat{ID: -1001, Type: "group"}}
	reply, err = bot.executeCommand(ctx, channel, userMsg, "/menu")
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(reply.Text, "管理员入口") {
		t.Fatalf("non-admin group menu must not expose management actions, got %#v", reply)
	}
}
