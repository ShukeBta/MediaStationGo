package service

import (
	"context"
	"testing"
	"time"

	"github.com/ShukeBta/MediaStationGo/internal/model"
)

func TestRegistrationCodeRedeemOnce(t *testing.T) {
	ctx := context.Background()
	repos, bot := newBotTestService(t)

	code, err := bot.generateCode(ctx, model.RegistrationCodeRenew, 30, 0, "")
	if err != nil {
		t.Fatal(err)
	}
	// 首次校验通过
	rc, msg := bot.lookupRedeemableCode(ctx, code.Code, model.RegistrationCodeRenew)
	if rc == nil {
		t.Fatalf("expected valid code, got msg=%q", msg)
	}
	// 标记使用后不可再用
	if err := repos.RegCode.MarkUsed(ctx, rc.ID, "user-1"); err != nil {
		t.Fatal(err)
	}
	if _, msg := bot.lookupRedeemableCode(ctx, code.Code, model.RegistrationCodeRenew); msg == "" {
		t.Fatal("used code must not validate again")
	}
	// 第二次 MarkUsed 应失败（防止双花）
	if err := repos.RegCode.MarkUsed(ctx, rc.ID, "user-2"); err == nil {
		t.Fatal("double-spend should be rejected")
	}
	// 类型不匹配应被拒
	reg, _ := bot.generateCode(ctx, model.RegistrationCodeRegister, 0, 0, "")
	if _, msg := bot.lookupRedeemableCode(ctx, reg.Code, model.RegistrationCodeRenew); msg == "" {
		t.Fatal("register code should not validate as renew")
	}
}

func TestRegistrationCodeCanBeGeneratedForMultipleUses(t *testing.T) {
	ctx := context.Background()
	repos, bot := newBotTestService(t)

	code, err := bot.generateCodeWithUses(ctx, model.RegistrationCodeRenew, 30, 0, 2, "")
	if err != nil {
		t.Fatal(err)
	}
	rc, msg := bot.lookupRedeemableCode(ctx, code.Code, model.RegistrationCodeRenew)
	if rc == nil {
		t.Fatalf("expected valid code, got msg=%q", msg)
	}
	if err := repos.RegCode.MarkUsed(ctx, rc.ID, "user-1"); err != nil {
		t.Fatal(err)
	}
	rc, msg = bot.lookupRedeemableCode(ctx, code.Code, model.RegistrationCodeRenew)
	if rc == nil {
		t.Fatalf("code should remain redeemable after first use, got msg=%q", msg)
	}
	if err := repos.RegCode.MarkUsed(ctx, rc.ID, "user-2"); err != nil {
		t.Fatal(err)
	}
	if _, msg := bot.lookupRedeemableCode(ctx, code.Code, model.RegistrationCodeRenew); msg == "" {
		t.Fatal("code should be exhausted after max uses")
	}
	var used model.RegistrationCode
	if err := repos.DB.Where("id = ?", code.ID).First(&used).Error; err != nil {
		t.Fatal(err)
	}
	if used.UsedCount != 2 || used.UsedAt == nil {
		t.Fatalf("expected exhausted code with used_count=2, got %+v", used)
	}
}

func TestRenewalClearsExpiry(t *testing.T) {
	ctx := context.Background()
	repos, bot := newBotTestService(t)
	past := time.Now().Add(-time.Hour)
	u := &model.User{Username: "bob", PasswordHash: "x", Role: "user", IsActive: false, ExpiredAt: &past}
	if err := repos.User.Create(ctx, u); err != nil {
		t.Fatal(err)
	}
	if err := bot.applyRenewal(ctx, u.ID, 30); err != nil {
		t.Fatal(err)
	}
	got, _ := repos.User.FindByID(ctx, u.ID)
	if !got.IsActive {
		t.Fatal("renewal should re-activate account")
	}
	if got.ExpiredAt == nil || got.ExpiredAt.Before(time.Now()) {
		t.Fatalf("renewal should set future expiry, got %v", got.ExpiredAt)
	}
}
