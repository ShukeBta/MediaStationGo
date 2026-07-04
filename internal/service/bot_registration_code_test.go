package service

import (
	"context"
	"encoding/json"
	"strings"
	"testing"

	"github.com/ShukeBta/MediaStationGo/internal/model"
)

func TestBotRedeemRegisterRequiresAllowedTelegramUser(t *testing.T) {
	ctx := context.Background()
	_, bot := newBotTestService(t)
	code, err := bot.generateCode(ctx, model.RegistrationCodeRegister, 30, 0, "")
	if err != nil {
		t.Fatal(err)
	}
	channel := &model.NotifyChannel{Name: "Telegram", Type: "telegram", Enabled: true, Config: `{"admin_user_ids":"9001"}`}
	msg := &TelegramMessage{From: TelegramUser{ID: 9201, Username: "outsider"}, Chat: TelegramChat{ID: 9201, Type: "private"}}

	reply, err := bot.executeCommand(ctx, channel, msg, "/redeem_register "+code.Code)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(reply.Text, "不在管理员配置") {
		t.Fatalf("outsider should not redeem register code, got %q", reply.Text)
	}

	channel.Config = `{"admin_user_ids":"9201"}`
	reply, err = bot.executeCommand(ctx, channel, msg, "/redeem_register "+code.Code)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(reply.Text, "兑换成功") {
		t.Fatalf("allowed user should redeem register code, got %q", reply.Text)
	}
	if binding := bot.telegramBinding(ctx, 9201); binding == nil {
		t.Fatal("redeemed account should be bound to telegram user")
	}
}

func TestBotRedeemRegisterCodeCreatesOnlyOneAccount(t *testing.T) {
	ctx := context.Background()
	repos, bot := newBotTestService(t)
	code, err := bot.generateCode(ctx, model.RegistrationCodeRegister, 30, 0, "")
	if err != nil {
		t.Fatal(err)
	}
	channel := &model.NotifyChannel{Name: "Telegram", Type: "telegram", Enabled: true, Config: `{"admin_user_ids":"9201,9202"}`}

	first := &TelegramMessage{From: TelegramUser{ID: 9201, Username: "first"}, Chat: TelegramChat{ID: 9201, Type: "private"}}
	reply, err := bot.executeCommand(ctx, channel, first, "/redeem_register "+code.Code)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(reply.Text, "兑换成功") {
		t.Fatalf("first redeem should succeed, got %q", reply.Text)
	}

	second := &TelegramMessage{From: TelegramUser{ID: 9202, Username: "second"}, Chat: TelegramChat{ID: 9202, Type: "private"}}
	reply, err = bot.executeCommand(ctx, channel, second, "/redeem_register "+code.Code)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(reply.Text, "兑换码已被使用") && !strings.Contains(reply.Text, "兑换码刚刚被使用") {
		t.Fatalf("second redeem should be rejected as used, got %q", reply.Text)
	}
	var users int64
	if err := repos.DB.Model(&model.User{}).Count(&users).Error; err != nil {
		t.Fatal(err)
	}
	if users != 1 {
		t.Fatalf("one register code must create exactly one user, got %d", users)
	}
	if binding := bot.telegramBinding(ctx, 9202); binding != nil {
		t.Fatal("second telegram user must not be bound by an already-used register code")
	}
}

func TestBotRegisterCommandAcceptsRegistrationCode(t *testing.T) {
	ctx := context.Background()
	_, bot := newBotTestService(t)
	code, err := bot.generateCode(ctx, model.RegistrationCodeRegister, 30, 0, "")
	if err != nil {
		t.Fatal(err)
	}
	channel := &model.NotifyChannel{Name: "Telegram", Type: "telegram", Enabled: true, Config: `{"admin_user_ids":"9301"}`}
	msg := &TelegramMessage{From: TelegramUser{ID: 9301, Username: "codeuser"}, Chat: TelegramChat{ID: 9301, Type: "private"}}

	reply, err := bot.executeCommand(ctx, channel, msg, "/register "+strings.ToLower(code.Code[:4])+"-"+strings.ToLower(code.Code[4:]))
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(reply.Text, "兑换成功") {
		t.Fatalf("/register CODE should redeem registration code, got %q", reply.Text)
	}
	if binding := bot.telegramBinding(ctx, 9301); binding == nil {
		t.Fatal("register code should bind the newly created account")
	}
}

func TestBotPlainRegistrationCodeMessageRedeems(t *testing.T) {
	ctx := context.Background()
	repos, bot := newBotTestService(t)
	code, err := bot.generateCode(ctx, model.RegistrationCodeRegister, 30, 0, "")
	if err != nil {
		t.Fatal(err)
	}
	if err := repos.DB.Create(&model.NotifyChannel{
		Name:    "Telegram",
		Type:    "telegram",
		Enabled: true,
		Config:  `{"admin_user_ids":"9302"}`,
	}).Error; err != nil {
		t.Fatal(err)
	}
	update, _ := json.Marshal(TelegramUpdate{
		UpdateID: 1,
		Message: &TelegramMessage{
			MessageID: 12,
			Text:      strings.ToLower(code.Code),
			From:      TelegramUser{ID: 9302, Username: "plaincode"},
			Chat:      TelegramChat{ID: 9302, Type: "private"},
		},
	})

	if err := bot.HandleWebhook(ctx, update); err != nil {
		t.Fatal(err)
	}
	if binding := bot.telegramBinding(ctx, 9302); binding == nil {
		t.Fatal("plain code private message should redeem and bind account")
	}
	var used model.RegistrationCode
	if err := repos.DB.Where("code = ?", code.Code).First(&used).Error; err != nil {
		t.Fatal(err)
	}
	if used.UsedAt == nil || used.UsedByUserID == "" {
		t.Fatal("plain code message should mark registration code as used")
	}
}
