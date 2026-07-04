package service

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/ShukeBta/MediaStationGo/internal/model"
)

func TestRegisterTelegramBotCommands(t *testing.T) {
	var gotPath string
	var payloads []struct {
		Commands []telegramBotCommand `json:"commands"`
		Scope    map[string]any       `json:"scope"`
	}
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotPath = r.URL.Path
		var payload struct {
			Commands []telegramBotCommand `json:"commands"`
			Scope    map[string]any       `json:"scope"`
		}
		if err := json.NewDecoder(r.Body).Decode(&payload); err != nil {
			t.Fatalf("decode payload: %v", err)
		}
		payloads = append(payloads, payload)
		_, _ = w.Write([]byte(`{"ok":true}`))
	}))
	defer server.Close()

	err := registerTelegramBotCommands(t.Context(), map[string]string{
		"bot_token":    "123456:ABC",
		"api_base_url": server.URL,
	})
	if err != nil {
		t.Fatal(err)
	}
	if gotPath != "/bot123456:ABC/setMyCommands" {
		t.Fatalf("path = %q", gotPath)
	}
	if len(payloads) < 3 {
		t.Fatalf("expected default/private/group command registrations, got %d", len(payloads))
	}
	if len(payloads[0].Commands) == 0 || payloads[0].Commands[0].Command != "start" {
		t.Fatalf("commands not registered: %#v", payloads[0].Commands)
	}
	var groupCommands []telegramBotCommand
	for _, payload := range payloads {
		if payload.Scope["type"] == "all_group_chats" {
			groupCommands = payload.Commands
			break
		}
	}
	if len(groupCommands) == 0 {
		t.Fatal("group command scope was not registered")
	}
	for _, command := range groupCommands {
		if command.Command == "users" || command.Command == "status" || command.Command == "cleanup" || command.Command == "register" || command.Command == "redeem" {
			t.Fatalf("group commands must not expose private/admin command %q", command.Command)
		}
	}
}

func TestDeleteTelegramWebhookBeforePolling(t *testing.T) {
	var gotPath string
	var payload map[string]any
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotPath = r.URL.Path
		if err := json.NewDecoder(r.Body).Decode(&payload); err != nil {
			t.Fatalf("decode payload: %v", err)
		}
		_, _ = w.Write([]byte(`{"ok":true}`))
	}))
	defer server.Close()

	err := deleteTelegramWebhook(t.Context(), map[string]string{
		"bot_token":    "123456:ABC",
		"api_base_url": server.URL,
	})
	if err != nil {
		t.Fatal(err)
	}
	if gotPath != "/bot123456:ABC/deleteWebhook" {
		t.Fatalf("path = %q", gotPath)
	}
	if got := payload["drop_pending_updates"]; got != false {
		t.Fatalf("drop_pending_updates = %#v, want false", got)
	}
}

func TestTelegramCommandMenusSeparateGroupAndAdminCommands(t *testing.T) {
	privateNames := telegramCommandNames(telegramPrivateBotCommandMenu())
	for _, required := range []string{"setname", "setpass"} {
		if !privateNames[required] {
			t.Fatalf("private menu should include %s", required)
		}
	}
	for _, hiddenAlias := range []string{"myinfo", "count"} {
		if privateNames[hiddenAlias] {
			t.Fatalf("private menu should hide compatibility alias %s", hiddenAlias)
		}
		if !telegramSupportedCommand("/" + hiddenAlias) {
			t.Fatalf("compatibility alias /%s should remain executable", hiddenAlias)
		}
	}

	groupNames := telegramCommandNames(telegramGroupBotCommandMenu())
	for _, forbidden := range []string{"status", "search", "downloads", "stats", "users", "cleanup", "cleanup_rule", "register", "redeem"} {
		if groupNames[forbidden] {
			t.Fatalf("group menu should not expose %s", forbidden)
		}
	}
	for _, required := range []string{"start", "menu", "help", "account", "signin", "devices", "kick", "hideadult"} {
		if !groupNames[required] {
			t.Fatalf("group menu should include %s", required)
		}
	}
	adminCommands := telegramAdminBotCommandMenu()
	adminNames := telegramCommandNames(adminCommands)
	for _, required := range []string{"users", "status", "cleanup_mode", "cleanup_rule", "ucr", "uinfo", "rmemby", "only_rm_record", "renewall", "userip", "auditip", "auditdevice", "auditclient", "udeviceid", "syncunbound", "syncgroupm", "check_ex", "deleted", "embyadmin", "banall", "unbanall", "prouser", "revuser", "embylibs_blockall", "embylibs_unblockall", "proadmin", "revadmin", "backup_db", "restore_from_db"} {
		if !adminNames[required] {
			t.Fatalf("admin menu should include %s", required)
		}
	}
	for _, hiddenAlias := range []string{"myinfo", "count", "low_activity", "urm", "only_rm_emby", "extraembylibs_blockall", "extraembylibs_unblockall"} {
		if adminNames[hiddenAlias] {
			t.Fatalf("admin menu should hide compatibility alias %s", hiddenAlias)
		}
	}
	for _, command := range adminCommands {
		if strings.Contains(command.Description, "Mgo 兼容") {
			t.Fatalf("admin menu command %s should use native Mgo wording: %q", command.Command, command.Description)
		}
	}
	help := telegramMgoAdminCommandHelp()
	for _, want := range []string{"用户：", "审计：", "清理：", "权限：", "运维："} {
		if !strings.Contains(help, want) {
			t.Fatalf("mgo admin help should include category %q in %q", want, help)
		}
	}
	if strings.Contains(help, "/setpass") {
		t.Fatalf("mgo admin help should not include user self-service command /setpass")
	}
}

func telegramCommandNames(commands []telegramBotCommand) map[string]bool {
	names := make(map[string]bool, len(commands))
	for _, command := range commands {
		names[command.Command] = true
	}
	return names
}

func TestTelegramCommandFiltering(t *testing.T) {
	if telegramIsCommandText("今天看什么") {
		t.Fatal("plain chat message should not be treated as command")
	}
	if !telegramIsCommandText("/start user pass") {
		t.Fatal("/start should be treated as command")
	}
	if got := telegramCommandName("/hideadult@MediaStationGoBot on"); got != "/hideadult" {
		t.Fatalf("telegramCommandName = %q, want /hideadult", got)
	}
	if telegramSupportedCommand("/签到") {
		t.Fatal("unrelated group bot command should not be handled")
	}
	for _, cmd := range []string{"/signin", "/redeem", "/gencode", "/users", "/renew_user", "/delete_user", "/cleanup_rule"} {
		if !telegramSupportedCommand(cmd) {
			t.Fatalf("%s should be supported so group slash commands get feedback", cmd)
		}
	}
	for _, cmd := range []string{"/restart", "/update_bot", "/coins", "/red", "/white_channel", "/config"} {
		if telegramSupportedCommand(cmd) {
			t.Fatalf("%s should not be treated as supported until it has a real Mgo implementation", cmd)
		}
	}
}

func TestTelegramSupportedCommandSetMatchesRegistry(t *testing.T) {
	_, bot := newBotTestService(t)
	channel := &model.NotifyChannel{Name: "Telegram", Type: "telegram", Enabled: true, Config: `{"admin_user_ids":"9001"}`}
	msg := &TelegramMessage{From: TelegramUser{ID: 9001, Username: "admin"}, Chat: TelegramChat{ID: 9001, Type: "private"}}
	for _, def := range bot.telegramCommandDefinitions(t.Context(), channel, msg) {
		for _, alias := range def.Aliases {
			if !telegramSupportedCommand(alias) {
				t.Fatalf("registered command %s must be in telegramSupportedCommandSet", alias)
			}
		}
	}
}
