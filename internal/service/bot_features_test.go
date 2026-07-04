package service

import (
	"context"
	"testing"
	"time"

	"github.com/ShukeBta/MediaStationGo/internal/config"
	"github.com/ShukeBta/MediaStationGo/internal/model"
	"github.com/ShukeBta/MediaStationGo/internal/repository"
	"go.uber.org/zap"
)

func newBotTestService(t *testing.T) (*repository.Container, *TelegramBotService) {
	t.Helper()
	db := newServiceTestDB(t, model.AllModels()...)
	repos := repository.New(db)
	cfg := &config.Config{}
	cfg.Secrets.JWTSecret = "test-secret"
	log := zap.NewNop()
	perms := NewPermissionService(log, repos)
	tokenSvc := NewTokenService(cfg, log, repos)
	auth := NewAuthService(cfg, log, repos, tokenSvc, perms)
	crypto := NewCryptoService("test-secret", log)
	bot := NewTelegramBotService(log, repos, crypto, auth)
	return repos, bot
}

// ── pure logic ──────────────────────────────────────────────────────────────

func TestRenewExpiry(t *testing.T) {
	// 永久（0 天）→ nil
	if got := renewExpiry(nil, 0); got != nil {
		t.Fatalf("expected nil for permanent, got %v", got)
	}
	// 从现在起 +30 天（当前为空）
	got := renewExpiry(nil, 30)
	if got == nil || got.Before(time.Now().Add(29*24*time.Hour)) {
		t.Fatalf("expected ~30d expiry, got %v", got)
	}
	// 已有未来到期 → 在原到期基础上叠加
	future := time.Now().Add(10 * 24 * time.Hour)
	got = renewExpiry(&future, 30)
	if got == nil || got.Before(future.Add(29*24*time.Hour)) {
		t.Fatalf("expected stacking on future expiry, got %v", got)
	}
	// 已过期 → 从现在起算
	past := time.Now().Add(-10 * 24 * time.Hour)
	got = renewExpiry(&past, 5)
	if got == nil || got.Before(time.Now().Add(4*24*time.Hour)) {
		t.Fatalf("expected fresh window from now, got %v", got)
	}
}

func TestCapacityRemaining(t *testing.T) {
	cases := []struct {
		name string
		c    capacityInfo
		want int64
	}{
		{"license only", capacityInfo{UsedUsers: 5, MaxUsers: 20}, 15},
		{"quota tighter", capacityInfo{UsedUsers: 5, MaxUsers: 100, OpenRegLimit: 10, OpenRegUsed: 3}, 7},
		{"license tighter", capacityInfo{UsedUsers: 95, MaxUsers: 100, OpenRegLimit: 50, OpenRegUsed: 0}, 5},
		{"full", capacityInfo{UsedUsers: 20, MaxUsers: 20}, 0},
		{"quota exhausted", capacityInfo{UsedUsers: 1, MaxUsers: 100, OpenRegLimit: 5, OpenRegUsed: 5}, 0},
	}
	for _, tc := range cases {
		if got := tc.c.Remaining(); got != tc.want {
			t.Errorf("%s: Remaining()=%d want %d", tc.name, got, tc.want)
		}
	}
}

func TestRandomWindowDays(t *testing.T) {
	for i := 0; i < 200; i++ {
		d := randomWindowDays(3, 5)
		if d < 3 || d > 5 {
			t.Fatalf("randomWindowDays(3,5)=%d out of range", d)
		}
	}
	if d := randomWindowDays(4, 4); d != 4 {
		t.Fatalf("randomWindowDays(4,4)=%d want 4", d)
	}
}

func TestFingerprintStability(t *testing.T) {
	a := fingerprint("Infuse", "iPhone")
	b := fingerprint("infuse", " iPhone ")
	if a != b {
		t.Fatalf("fingerprint should be case/space-insensitive: %s != %s", a, b)
	}
	if a != fingerprint("Emby", "iPhone") {
		t.Fatal("different apps on the same terminal must share one fingerprint")
	}
	if a == fingerprint("Infuse", "iPad") {
		t.Fatal("different device names must yield different fingerprints")
	}
}

// ── DB-backed ─────────────────────────────────────────────────────────────

func TestSignInStreak(t *testing.T) {
	ctx := context.Background()
	repos, bot := newBotTestService(t)
	u := &model.User{Username: "alice", PasswordHash: "x", Role: "user"}
	if err := repos.User.Create(ctx, u); err != nil {
		t.Fatal(err)
	}

	res, err := bot.signIn(ctx, u.ID)
	if err != nil || res.Streak != 1 || res.Total != 1 {
		t.Fatalf("first sign-in: %+v err=%v", res, err)
	}
	// 同日重复签到 → 不增长
	res, _ = bot.signIn(ctx, u.ID)
	if !res.AlreadySigned || res.Streak != 1 {
		t.Fatalf("same-day re-signin should be no-op: %+v", res)
	}
	// 模拟昨天签到 → 连续 +1
	rec, _ := repos.SignIn.Get(ctx, u.ID)
	rec.LastSignIn = time.Now().Add(-24 * time.Hour)
	_ = repos.SignIn.Save(ctx, rec)
	res, _ = bot.signIn(ctx, u.ID)
	if res.Streak != 2 || res.Total != 2 {
		t.Fatalf("consecutive day should bump streak: %+v", res)
	}
	// 中断（前天）→ 重置为 1
	rec, _ = repos.SignIn.Get(ctx, u.ID)
	rec.LastSignIn = time.Now().Add(-72 * time.Hour)
	_ = repos.SignIn.Save(ctx, rec)
	res, _ = bot.signIn(ctx, u.ID)
	if res.Streak != 1 {
		t.Fatalf("broken streak should reset to 1: %+v", res)
	}
}
