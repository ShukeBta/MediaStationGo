package service

import (
	"context"
	"strings"
	"testing"
	"time"

	"go.uber.org/zap"

	"github.com/ShukeBta/MediaStationGo/internal/model"
)

func TestBotAdminUnbindMultipleUsers(t *testing.T) {
	ctx := context.Background()
	repos, bot := newBotTestService(t)
	admin := &model.User{Username: "root", PasswordHash: "x", Role: "admin", IsActive: true}
	viewer := &model.User{Username: "viewer", PasswordHash: "x", Role: "user", IsActive: true}
	guest := &model.User{Username: "guest", PasswordHash: "x", Role: "user", IsActive: true}
	for _, user := range []*model.User{admin, viewer, guest} {
		if err := repos.User.Create(ctx, user); err != nil {
			t.Fatal(err)
		}
	}
	bindings := []model.TelegramBinding{
		{TelegramUserID: 9401, TelegramName: "@root", ChatID: 9401, UserID: admin.ID},
		{TelegramUserID: 9402, TelegramName: "@viewer", ChatID: 9402, UserID: viewer.ID},
		{TelegramUserID: 9403, TelegramName: "@guest", ChatID: 9403, UserID: guest.ID},
	}
	for i := range bindings {
		if err := repos.DB.Create(&bindings[i]).Error; err != nil {
			t.Fatal(err)
		}
	}
	channel := &model.NotifyChannel{Name: "Telegram", Type: "telegram", Enabled: true, Config: `{"admin_user_ids":"9401"}`}
	msg := &TelegramMessage{From: TelegramUser{ID: 9401, Username: "root"}, Chat: TelegramChat{ID: 9401, Type: "private"}}

	reply, err := bot.executeCommand(ctx, channel, msg, "/unbind viewer,guest missing root")
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(reply.Text, "已解绑：<b>2</b>") || !strings.Contains(reply.Text, "root(管理员)") || !strings.Contains(reply.Text, "missing") {
		t.Fatalf("unexpected unbind reply: %q", reply.Text)
	}
	for _, user := range []*model.User{viewer, guest} {
		var count int64
		if err := repos.DB.Model(&model.TelegramBinding{}).Where("user_id = ?", user.ID).Count(&count).Error; err != nil {
			t.Fatal(err)
		}
		if count != 0 {
			t.Fatalf("%s binding count = %d, want 0", user.Username, count)
		}
	}
	if binding := bot.telegramBinding(ctx, 9401); binding == nil {
		t.Fatal("admin binding should be protected from /unbind by username")
	}
}

func TestBotAdminUnbindInactiveAndInvalidBindings(t *testing.T) {
	ctx := context.Background()
	repos, bot := newBotTestService(t)
	oldTime := time.Now().Add(-45 * 24 * time.Hour)
	recentTime := time.Now().Add(-2 * 24 * time.Hour)
	admin := &model.User{Username: "root", PasswordHash: "x", Role: "admin", IsActive: true, LastLoginAt: &oldTime}
	oldUser := &model.User{Username: "old", PasswordHash: "x", Role: "user", IsActive: true, LastLoginAt: &oldTime}
	realtimeUser := &model.User{Username: "realtime", PasswordHash: "x", Role: "user", IsActive: true, LastLoginAt: &oldTime}
	recentUser := &model.User{Username: "recent", PasswordHash: "x", Role: "user", IsActive: true, LastLoginAt: &recentTime}
	for _, user := range []*model.User{admin, oldUser, realtimeUser, recentUser} {
		if err := repos.User.Create(ctx, user); err != nil {
			t.Fatal(err)
		}
	}
	for _, binding := range []model.TelegramBinding{
		{TelegramUserID: 9501, TelegramName: "@root", ChatID: 9501, UserID: admin.ID},
		{TelegramUserID: 9502, TelegramName: "@old", ChatID: 9502, UserID: oldUser.ID},
		{TelegramUserID: 9503, TelegramName: "@recent", ChatID: 9503, UserID: recentUser.ID},
		{TelegramUserID: 9505, TelegramName: "@realtime", ChatID: 9505, UserID: realtimeUser.ID},
		{TelegramUserID: 9504, TelegramName: "@ghost", ChatID: 9504, UserID: "missing-user"},
	} {
		row := binding
		if err := repos.DB.Create(&row).Error; err != nil {
			t.Fatal(err)
		}
	}
	channel := &model.NotifyChannel{Name: "Telegram", Type: "telegram", Enabled: true, Config: `{"admin_user_ids":"9501"}`}
	msg := &TelegramMessage{From: TelegramUser{ID: 9501, Username: "root"}, Chat: TelegramChat{ID: 9501, Type: "private"}}
	tracker := NewSessionTrackerService(zap.NewNop())
	tracker.RecordActivity(ctx, realtimeUser.ID, realtimeUser.Username, "phone-1", "iPhone", "Infuse", "192.0.2.10")
	device := NewDeviceService(zap.NewNop(), repos)
	device.SetSessionTracker(tracker)
	bot.SetDeviceService(device)

	reply, err := bot.executeCommand(ctx, channel, msg, "/unbind_inactive 30")
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(reply.Text, "已解绑：<b>1</b>") || !strings.Contains(reply.Text, "old") {
		t.Fatalf("unexpected inactive unbind reply: %q", reply.Text)
	}
	if binding := bot.telegramBinding(ctx, 9502); binding != nil {
		t.Fatal("old user binding should be removed")
	}
	if binding := bot.telegramBinding(ctx, 9501); binding == nil {
		t.Fatal("admin binding should be skipped by inactive cleanup")
	}
	if binding := bot.telegramBinding(ctx, 9503); binding == nil {
		t.Fatal("recent user binding should remain")
	}
	if binding := bot.telegramBinding(ctx, 9505); binding == nil {
		t.Fatal("realtime active user binding should remain")
	}

	reply, err = bot.executeCommand(ctx, channel, msg, "/unbind_duplicates")
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(reply.Text, "已解绑：<b>1</b>") || !strings.Contains(reply.Text, "tg:9504") {
		t.Fatalf("unexpected duplicate cleanup reply: %q", reply.Text)
	}
	if binding := bot.telegramBinding(ctx, 9504); binding != nil {
		t.Fatal("invalid binding should be removed")
	}
}

func TestTelegramMembershipChatIDsIncludesCommandChatID(t *testing.T) {
	_, bot := newBotTestService(t)
	channel := &model.NotifyChannel{Name: "Telegram", Type: "telegram", Enabled: true, Config: `{"command_chat_id":"-100123"}`}
	got := bot.telegramMembershipChatIDs(channel)
	if len(got) != 1 || got[0] != "-100123" {
		t.Fatalf("telegramMembershipChatIDs() = %#v, want command_chat_id", got)
	}
}

func TestTelegramMembershipChatIDsDedupesGroupChannelAndCommandIDs(t *testing.T) {
	_, bot := newBotTestService(t)
	channel := &model.NotifyChannel{Name: "Telegram", Type: "telegram", Enabled: true, Config: `{"group_chat_id":"-100123","channel_chat_id":"-100124","command_chat_id":"-100123"}`}
	got := bot.telegramMembershipChatIDs(channel)
	if len(got) != 2 || got[0] != "-100123" || got[1] != "-100124" {
		t.Fatalf("telegramMembershipChatIDs() = %#v, want deduped ids", got)
	}
}
