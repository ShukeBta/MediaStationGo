package service

import (
	"context"
	"strings"
	"testing"
	"time"

	"github.com/ShukeBta/MediaStationGo/internal/model"
)

func TestBotCleanupRulesDefaultToEmpty(t *testing.T) {
	ctx := context.Background()
	repos, _ := newBotTestService(t)

	cfg := loadBotConfig(ctx, repos)
	if len(cfg.AccountCleanupRules) != 0 {
		t.Fatalf("default cleanup rules should be empty, got %+v", cfg.AccountCleanupRules)
	}
}

func TestBotCleanupRulesCanBeDeletedUntilEmpty(t *testing.T) {
	ctx := context.Background()
	repos, bot := newBotTestService(t)
	admin := &model.User{Username: "root", PasswordHash: "x", Role: "admin", IsActive: true}
	if err := repos.User.Create(ctx, admin); err != nil {
		t.Fatal(err)
	}
	channel := &model.NotifyChannel{Name: "Telegram", Type: "telegram", Enabled: true, Config: `{"admin_user_ids":"9001"}`}
	msg := &TelegramMessage{From: TelegramUser{ID: 9001, Username: "root"}, Chat: TelegramChat{ID: 9001, Type: "private"}}

	if _, err := bot.executeCommand(ctx, channel, msg, "/cleanup_rule add watch_hours watch_3_5d_6h 观看3到5天满6小时 3 5 6"); err != nil {
		t.Fatal(err)
	}
	reply, err := bot.executeCommand(ctx, channel, msg, "/cleanup_rule del watch_3_5d_6h")
	if err != nil {
		t.Fatal(err)
	}
	cfg := loadBotConfig(ctx, repos)
	if len(cfg.AccountCleanupRules) != 0 {
		t.Fatalf("cleanup rules should stay empty after deleting the last rule; reply=%q rules=%+v", reply.Text, cfg.AccountCleanupRules)
	}

	reply, err = bot.executeCommand(ctx, channel, msg, "/cleanup_rule list")
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(reply.Text, "暂无规则") {
		t.Fatalf("expected empty rule list, got %q", reply.Text)
	}
}

func TestBotCleanupRunPreviewsBeforeConfirm(t *testing.T) {
	ctx := context.Background()
	repos, bot := newBotTestService(t)
	admin := &model.User{Username: "root", PasswordHash: "x", Role: "admin", IsActive: true}
	if err := repos.User.Create(ctx, admin); err != nil {
		t.Fatal(err)
	}
	now := time.Now()
	old := now.Add(-30 * 24 * time.Hour)
	stale := &model.User{Username: "stale", PasswordHash: "x", Role: "user", IsActive: true}
	stale.CreatedAt = old
	stale.LastLoginAt = &old
	recent := &model.User{Username: "recent", PasswordHash: "x", Role: "user", IsActive: true}
	recent.CreatedAt = old
	recent.LastLoginAt = &now
	newUser := &model.User{Username: "newbie", PasswordHash: "x", Role: "user", IsActive: true}
	newUser.CreatedAt = now
	for _, user := range []*model.User{stale, recent, newUser} {
		if err := repos.User.Create(ctx, user); err != nil {
			t.Fatal(err)
		}
	}
	if err := repos.Setting.Set(ctx, SettingAccountCleanupEnabled, "true"); err != nil {
		t.Fatal(err)
	}
	if err := repos.Setting.Set(ctx, SettingAccountCleanupKeepMode, "any"); err != nil {
		t.Fatal(err)
	}
	if err := repos.Setting.Set(ctx, SettingAccountCleanupRules, `[
		{"id":"login_7d","name":"最近登录","type":"recent_login","enabled":true,"window_days_max":7},
		{"id":"new_7d","name":"新号宽限","type":"account_age_grace","enabled":true,"min_count":7}
	]`); err != nil {
		t.Fatal(err)
	}
	channel := &model.NotifyChannel{Name: "Telegram", Type: "telegram", Enabled: true, Config: `{"admin_user_ids":"9001"}`}
	msg := &TelegramMessage{From: TelegramUser{ID: 9001, Username: "root"}, Chat: TelegramChat{ID: 9001, Type: "private"}}

	reply, err := bot.executeCommand(ctx, channel, msg, "/cleanup run")
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(reply.Text, "当前只是预览") || !strings.Contains(reply.Text, "stale") || !strings.Contains(reply.Text, "/cleanup run confirm") {
		t.Fatalf("cleanup run should preview candidates and confirmation command, got %q", reply.Text)
	}
	if got, _ := repos.User.FindByID(ctx, stale.ID); got == nil {
		t.Fatal("cleanup preview must not delete the stale user")
	}

	reply, err = bot.executeCommand(ctx, channel, msg, "/deleted")
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(reply.Text, "当前只是预览") {
		t.Fatalf("/deleted alias should preview only, got %q", reply.Text)
	}
	if got, _ := repos.User.FindByID(ctx, stale.ID); got == nil {
		t.Fatal("/deleted preview alias must not delete users")
	}

	reply, err = bot.executeCommand(ctx, channel, msg, "/cleanup run confirm")
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(reply.Text, "已清理 <b>1</b>") {
		t.Fatalf("cleanup confirm should delete exactly one stale user, got %q", reply.Text)
	}
	if got, _ := repos.User.FindByID(ctx, stale.ID); got != nil {
		t.Fatal("stale user should be deleted after explicit confirmation")
	}
	for _, user := range []*model.User{recent, newUser, admin} {
		if got, _ := repos.User.FindByID(ctx, user.ID); got == nil {
			t.Fatalf("%s should be kept by保号 rules/protection", user.Username)
		}
	}
}

func TestBotCleanupLegacyCountModeStillKeepsSingleMatchedRule(t *testing.T) {
	ctx := context.Background()
	repos, bot := newBotTestService(t)
	admin := &model.User{Username: "root", PasswordHash: "x", Role: "admin", IsActive: true}
	if err := repos.User.Create(ctx, admin); err != nil {
		t.Fatal(err)
	}
	now := time.Now()
	old := now.Add(-30 * 24 * time.Hour)
	recent := &model.User{Username: "recent", PasswordHash: "x", Role: "user", IsActive: true}
	recent.CreatedAt = old
	recent.LastLoginAt = &now
	stale := &model.User{Username: "stale", PasswordHash: "x", Role: "user", IsActive: true}
	stale.CreatedAt = old
	stale.LastLoginAt = &old
	for _, user := range []*model.User{recent, stale} {
		if err := repos.User.Create(ctx, user); err != nil {
			t.Fatal(err)
		}
	}
	if err := repos.Setting.Set(ctx, SettingAccountCleanupEnabled, "true"); err != nil {
		t.Fatal(err)
	}
	if err := repos.Setting.Set(ctx, SettingAccountCleanupKeepMode, "count"); err != nil {
		t.Fatal(err)
	}
	if err := repos.Setting.Set(ctx, SettingAccountCleanupRequiredCount, "2"); err != nil {
		t.Fatal(err)
	}
	if err := repos.Setting.Set(ctx, SettingAccountCleanupRules, `[
		{"id":"login_7d","name":"最近登录","type":"recent_login","enabled":true,"window_days_max":7},
		{"id":"new_7d","name":"新号宽限","type":"account_age_grace","enabled":true,"min_count":7}
	]`); err != nil {
		t.Fatal(err)
	}
	channel := &model.NotifyChannel{Name: "Telegram", Type: "telegram", Enabled: true, Config: `{"admin_user_ids":"9001"}`}
	msg := &TelegramMessage{From: TelegramUser{ID: 9001, Username: "root"}, Chat: TelegramChat{ID: 9001, Type: "private"}}

	reply, err := bot.executeCommand(ctx, channel, msg, "/cleanup run")
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(reply.Text, "recent") {
		t.Fatalf("user matching one keep rule must not be a cleanup candidate, got %q", reply.Text)
	}
	if !strings.Contains(reply.Text, "stale") {
		t.Fatalf("user matching no keep rules should be a candidate, got %q", reply.Text)
	}

	reply, err = bot.executeCommand(ctx, channel, msg, "/cleanup run confirm")
	if err != nil {
		t.Fatal(err)
	}
	if got, _ := repos.User.FindByID(ctx, recent.ID); got == nil {
		t.Fatal("legacy count mode must not delete a user matching one keep rule")
	}
	if got, _ := repos.User.FindByID(ctx, stale.ID); got != nil {
		t.Fatalf("stale user should be deleted after confirm, reply=%q", reply.Text)
	}
}

func TestBotCleanupConfirmRequiresEnabledRules(t *testing.T) {
	ctx := context.Background()
	repos, bot := newBotTestService(t)
	user := &model.User{Username: "viewer", PasswordHash: "x", Role: "user", IsActive: true}
	user.CreatedAt = time.Now().Add(-30 * 24 * time.Hour)
	if err := repos.User.Create(ctx, user); err != nil {
		t.Fatal(err)
	}
	if err := repos.Setting.Set(ctx, SettingAccountCleanupEnabled, "true"); err != nil {
		t.Fatal(err)
	}
	channel := &model.NotifyChannel{Name: "Telegram", Type: "telegram", Enabled: true, Config: `{"admin_user_ids":"9001"}`}
	msg := &TelegramMessage{From: TelegramUser{ID: 9001, Username: "root"}, Chat: TelegramChat{ID: 9001, Type: "private"}}

	reply, err := bot.executeCommand(ctx, channel, msg, "/cleanup run confirm")
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(reply.Text, "没有启用的保号规则") {
		t.Fatalf("cleanup confirm without rules should be blocked, got %q", reply.Text)
	}
	if got, _ := repos.User.FindByID(ctx, user.ID); got == nil {
		t.Fatal("cleanup confirm without enabled rules must not delete users")
	}
}

func TestBotCleanupRuleListInfersDaysAndHidesDuplicateNames(t *testing.T) {
	ctx := context.Background()
	repos, bot := newBotTestService(t)
	admin := &model.User{Username: "root", PasswordHash: "x", Role: "admin", IsActive: true}
	if err := repos.User.Create(ctx, admin); err != nil {
		t.Fatal(err)
	}
	if err := repos.Setting.Set(ctx, SettingAccountCleanupRules, `[
		{"id":"login_7d","name":"login_7d","type":"recent_login","enabled":true,"window_days_min":1,"window_days_max":5,"min_count":1},
		{"id":"new_7d","name":"new_7d","type":"account_age_grace","enabled":true,"window_days_min":1,"window_days_max":1,"min_count":1}
	]`); err != nil {
		t.Fatal(err)
	}
	channel := &model.NotifyChannel{Name: "Telegram", Type: "telegram", Enabled: true, Config: `{"admin_user_ids":"9001"}`}
	msg := &TelegramMessage{From: TelegramUser{ID: 9001, Username: "root"}, Chat: TelegramChat{ID: 9001, Type: "private"}}

	reply, err := bot.executeCommand(ctx, channel, msg, "/cleanup_rule")
	if err != nil {
		t.Fatal(err)
	}
	for _, bad := range []string{"login_7d</code> · login_7d", "new_7d</code> · new_7d", "5 天内登录", "新号宽限 1 天", "add watch_hours", "Mgo 保号规则命令"} {
		if strings.Contains(reply.Text, bad) {
			t.Fatalf("rule list still contains bad fragment %q: %s", bad, reply.Text)
		}
	}
	for _, want := range []string{"login_7d", "7 天内登录", "new_7d", "新号宽限 7 天"} {
		if !strings.Contains(reply.Text, want) {
			t.Fatalf("rule list missing %q: %s", want, reply.Text)
		}
	}
}
