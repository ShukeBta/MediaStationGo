package service

import (
	"context"
	"testing"
	"time"

	"github.com/glebarez/sqlite"
	"go.uber.org/zap"
	"gorm.io/gorm"

	"github.com/ShukeBta/MediaStationGo/internal/config"
	"github.com/ShukeBta/MediaStationGo/internal/model"
	"github.com/ShukeBta/MediaStationGo/internal/repository"
)

func newBotTestService(t *testing.T) (*repository.Container, *TelegramBotService) {
	t.Helper()
	db, err := gorm.Open(sqlite.Open(":memory:"), &gorm.Config{})
	if err != nil {
		t.Fatal(err)
	}
	if err := db.AutoMigrate(model.AllModels()...); err != nil {
		t.Fatal(err)
	}
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

func TestDeviceKickAndConcurrency(t *testing.T) {
	ctx := context.Background()
	repos, _ := newBotTestService(t)
	dev := NewDeviceService(zap.NewNop(), repos)
	u := &model.User{Username: "carol", PasswordHash: "x", Role: "user", IsActive: true}
	if err := repos.User.Create(ctx, u); err != nil {
		t.Fatal(err)
	}

	dev.RecordLogin(ctx, u.ID, "dev-1", "iPhone", "Infuse", "1.2.3.4")
	dev.RecordPlayback(ctx, u.ID, "dev-1", "iPhone", "Infuse")
	devices, _ := dev.ListDevices(ctx, u.ID)
	if len(devices) != 1 {
		t.Fatalf("expected 1 device, got %d", len(devices))
	}

	// 踢下线后命中 kicked
	if err := dev.KickDevice(ctx, u.ID, "dev-1"); err != nil {
		t.Fatal(err)
	}
	if !dev.IsDeviceKicked(ctx, u.ID, "dev-1") {
		t.Fatal("device should be kicked")
	}
	// 重新登录清除 kicked
	dev.RecordLogin(ctx, u.ID, "dev-1", "iPhone", "Infuse", "1.2.3.4")
	if dev.IsDeviceKicked(ctx, u.ID, "dev-1") {
		t.Fatal("re-login should clear kicked flag")
	}

	// 并发播放计数
	now := time.Now()
	for i, id := range []string{"d1", "d2", "d3", "d4"} {
		_ = repos.UserDevice.Create(ctx, &model.UserDevice{
			UserID: u.ID, DeviceID: id, FirstSeenAt: now, LastSeenAt: now, LastPlayAt: &now,
		})
		_ = i
	}
	n, err := repos.UserDevice.CountConcurrentPlaying(ctx, u.ID, now.Add(-time.Minute))
	if err != nil {
		t.Fatal(err)
	}
	if n < 4 {
		t.Fatalf("expected >=4 concurrent playing, got %d", n)
	}
}

func TestProtectedAdminNeverViolated(t *testing.T) {
	ctx := context.Background()
	repos, _ := newBotTestService(t)
	dev := NewDeviceService(zap.NewNop(), repos)
	admin := &model.User{Username: "root", PasswordHash: "x", Role: "admin", IsActive: true}
	if err := repos.User.Create(ctx, admin); err != nil {
		t.Fatal(err)
	}
	_ = repos.Setting.Set(ctx, SettingAntiShareEnabled, "true")
	cfg := loadBotConfig(ctx, repos)
	// 多次违规也不应删除/警告管理员
	for i := 0; i < 5; i++ {
		dev.registerViolation(ctx, admin.ID, "test", cfg)
	}
	got, _ := repos.User.FindByID(ctx, admin.ID)
	if got == nil {
		t.Fatal("admin must never be auto-deleted")
	}
	if got.ShareWarnings != 0 {
		t.Fatalf("admin should accrue no warnings, got %d", got.ShareWarnings)
	}
}
