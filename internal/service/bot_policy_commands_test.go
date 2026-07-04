package service

import (
	"context"
	"strings"
	"testing"

	"github.com/ShukeBta/MediaStationGo/internal/model"
)

func TestBotAdminCommandsManageDevicePolicy(t *testing.T) {
	ctx := context.Background()
	repos, bot := newBotTestService(t)
	admin := &model.User{Username: "root", PasswordHash: "x", Role: "admin", IsActive: true}
	if err := repos.User.Create(ctx, admin); err != nil {
		t.Fatal(err)
	}
	if err := repos.DB.Create(&model.TelegramBinding{
		TelegramUserID: 9001,
		TelegramName:   "@root",
		ChatID:         9001,
		UserID:         admin.ID,
	}).Error; err != nil {
		t.Fatal(err)
	}
	channel := &model.NotifyChannel{Name: "Telegram", Type: "telegram", Enabled: true, Config: `{"admin_user_ids":"9001"}`}
	msg := &TelegramMessage{From: TelegramUser{ID: 9001, Username: "root"}, Chat: TelegramChat{ID: 9001, Type: "private"}}

	reply, err := bot.executeCommand(ctx, channel, msg, "/antishare on play=4 login=5 warn=3")
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(reply.Text, "防共享：<b>已开启</b>") {
		t.Fatalf("expected antishare enabled reply, got %q", reply.Text)
	}
	cfg := loadBotConfig(ctx, repos)
	if !cfg.AntiShareEnabled || cfg.MaxConcurrentPlay != 4 || cfg.MaxLoggedClients != 5 || cfg.WarnThreshold != 3 {
		t.Fatalf("unexpected device policy: %+v", cfg)
	}

	reply, err = bot.executeCommand(ctx, channel, msg, "/cleanup_mode count 2")
	if err != nil {
		t.Fatal(err)
	}
	cfg = loadBotConfig(ctx, repos)
	if cfg.AccountCleanupKeepMode != "any" || cfg.AccountCleanupRequiredCount != 1 {
		t.Fatalf("unexpected cleanup mode: %+v; reply=%q", cfg, reply.Text)
	}
	if !strings.Contains(reply.Text, "满足任意一条") {
		t.Fatalf("cleanup mode should explain fixed any-rule policy, got %q", reply.Text)
	}

	reply, err = bot.executeCommand(ctx, channel, msg, "/cleanup_rule add recent_login login_7d 七天内登录 7")
	if err != nil {
		t.Fatal(err)
	}
	cfg = loadBotConfig(ctx, repos)
	found := false
	for _, rule := range cfg.AccountCleanupRules {
		if rule.ID == "login_7d" && rule.Type == "recent_login" && rule.WindowDaysMax == 7 {
			found = true
		}
	}
	if !found {
		t.Fatalf("cleanup rule not added; reply=%q rules=%+v", reply.Text, cfg.AccountCleanupRules)
	}

	reply, err = bot.executeCommand(ctx, channel, msg, "/cleanup_rule edit recent_login login_7d 十四天内登录 14")
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(reply.Text, "已更新规则") {
		t.Fatalf("expected cleanup rule update reply, got %q", reply.Text)
	}
	cfg = loadBotConfig(ctx, repos)
	matches := 0
	for _, rule := range cfg.AccountCleanupRules {
		if rule.ID == "login_7d" {
			matches++
			if rule.Type != "recent_login" || rule.WindowDaysMax != 14 || rule.Name != "十四天内登录" {
				t.Fatalf("cleanup rule should be updated in place, got %+v", rule)
			}
		}
	}
	if matches != 1 {
		t.Fatalf("cleanup rule update should not create duplicates, got %d rules=%+v", matches, cfg.AccountCleanupRules)
	}

	reply, err = bot.executeCommand(ctx, channel, msg, "/cleanup_rule 修改 recent_login login_7d 二十一天内登录 21")
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(reply.Text, "已更新规则") {
		t.Fatalf("expected Chinese cleanup rule update reply, got %q", reply.Text)
	}
	cfg = loadBotConfig(ctx, repos)
	matches = 0
	for _, rule := range cfg.AccountCleanupRules {
		if rule.ID == "login_7d" {
			matches++
			if rule.WindowDaysMax != 21 || rule.Name != "二十一天内登录" {
				t.Fatalf("Chinese cleanup rule update should update values, got %+v", rule)
			}
		}
	}
	if matches != 1 {
		t.Fatalf("Chinese cleanup rule update should not create duplicates, got %d rules=%+v", matches, cfg.AccountCleanupRules)
	}

	reply, err = bot.executeCommand(ctx, channel, msg, "/cleanup_rule add account_age_grace new_7d 7")
	if err != nil {
		t.Fatal(err)
	}
	cfg = loadBotConfig(ctx, repos)
	found = false
	for _, rule := range cfg.AccountCleanupRules {
		if rule.ID == "new_7d" && rule.Type == "account_age_grace" && rule.MinCount == 7 {
			found = true
		}
	}
	if !found {
		t.Fatalf("cleanup shorthand rule not added; reply=%q rules=%+v", reply.Text, cfg.AccountCleanupRules)
	}
}
