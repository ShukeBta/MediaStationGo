package service

import (
	"errors"
	"strings"
	"testing"
)

func TestTelegramMethodURLUsesCustomAPIBase(t *testing.T) {
	got, err := telegramMethodURL(map[string]string{
		"api_base_url": "https://tg.example.com/",
	}, "123456:ABC-def", "sendMessage")
	if err != nil {
		t.Fatalf("telegramMethodURL returned error: %v", err)
	}
	want := "https://tg.example.com/bot123456:ABC-def/sendMessage"
	if got != want {
		t.Fatalf("got %q, want %q", got, want)
	}
}

func TestSanitizeTelegramErrorRedactsBotToken(t *testing.T) {
	err := sanitizeTelegramError(errors.New(`Post "https://api.telegram.org/bot123456:SECRET/sendMessage": context deadline exceeded`))
	if err == nil {
		t.Fatal("expected error")
	}
	msg := err.Error()
	if strings.Contains(msg, "SECRET") || strings.Contains(msg, "123456:") {
		t.Fatalf("telegram token leaked in error: %s", msg)
	}
	if !strings.Contains(msg, "timeout") {
		t.Fatalf("expected timeout hint, got: %s", msg)
	}
}

func TestValidateTelegramChannelDoesNotRequireLegacyChatID(t *testing.T) {
	err := validateChannel(ChannelInput{
		Name: "Telegram",
		Type: "telegram",
		Config: map[string]any{
			"bot_token":      "123456:ABC-def",
			"admin_user_ids": "10001",
		},
	})
	if err != nil {
		t.Fatalf("validateChannel returned error: %v", err)
	}
}

func TestTelegramTargetChatIDsFallsBackToAdmins(t *testing.T) {
	got := telegramTargetChatIDs(map[string]string{
		"admin_user_ids": "10001, 10002",
	})
	if len(got) != 2 || got[0] != "10001" || got[1] != "10002" {
		t.Fatalf("got %#v, want admin user ids", got)
	}
}

func TestNormalizeTelegramChannelMigratesLegacyChatID(t *testing.T) {
	input := ChannelInput{
		Name: "Telegram",
		Type: "telegram",
		Config: map[string]any{
			"chat_id": "-10001",
		},
	}
	normalizeChannelInput(&input)
	if got := str(input.Config["group_chat_id"]); got != "-10001" {
		t.Fatalf("group_chat_id = %q, want -10001", got)
	}
}

func TestNormalizeTelegramChannelMigratesLegacyPrivateChatIDToAdmin(t *testing.T) {
	cfg := map[string]string{"chat_id": "5812333517"}
	normalizeTelegramConfig(cfg)
	if got := cfg["admin_user_ids"]; got != "5812333517" {
		t.Fatalf("admin_user_ids = %q, want legacy chat_id", got)
	}
}

func TestTelegramTargetChatIDsUsesLegacyPrivateChatID(t *testing.T) {
	got := telegramTargetChatIDs(map[string]string{
		"chat_id": "5812333517",
	})
	if len(got) != 1 || got[0] != "5812333517" {
		t.Fatalf("got %#v, want legacy private chat target", got)
	}
}
