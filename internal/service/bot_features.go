package service

import (
	"context"
	"crypto/rand"
	"fmt"
	"strconv"
	"strings"
	"time"

	"github.com/ShukeBta/MediaStationGo/internal/model"
)

// ── 容量 / 开注名额 ──────────────────────────────────────────────────────────

// capacityInfo 描述当前用户容量（随凭证授权实时变化）与开注名额状态。
type capacityInfo struct {
	UsedUsers    int64
	MaxUsers     int64 // 来自 LicensedMaxUsers，随授权实时变化
	OpenRegOn    bool
	OpenRegLimit int // 0 = 不限（仅受 MaxUsers 约束）
	OpenRegUsed  int
}

// Remaining 返回还能注册多少个账号（同时受授权上限与开注名额约束）。
func (c capacityInfo) Remaining() int64 {
	byLicense := c.MaxUsers - c.UsedUsers
	if byLicense < 0 {
		byLicense = 0
	}
	if c.OpenRegLimit > 0 {
		byQuota := int64(c.OpenRegLimit - c.OpenRegUsed)
		if byQuota < 0 {
			byQuota = 0
		}
		if byQuota < byLicense {
			return byQuota
		}
	}
	return byLicense
}

// loadCapacity reads live capacity + open-reg quota state.
func (s *TelegramBotService) loadCapacity(ctx context.Context) capacityInfo {
	used, _ := s.repo.User.Count(ctx)
	info := capacityInfo{
		UsedUsers:    used,
		MaxUsers:     LicensedMaxUsers(ctx, s.repo),
		OpenRegOn:    s.openRegEnabled(ctx),
		OpenRegLimit: s.intSetting(ctx, SettingOpenRegLimit, 0),
		OpenRegUsed:  s.intSetting(ctx, SettingOpenRegUsed, 0),
	}
	return info
}

func (s *TelegramBotService) intSetting(ctx context.Context, key string, fallback int) int {
	v, err := s.repo.Setting.Get(ctx, key)
	if err != nil {
		return fallback
	}
	return parseIntSettingDefault(v, fallback)
}

// openRegEnabled reports whether bot registration is currently open. It honours
// both the new open-reg switch and the legacy registration switch.
func (s *TelegramBotService) openRegEnabled(ctx context.Context) bool {
	if v, _ := s.repo.Setting.Get(ctx, SettingOpenRegEnabled); v != "" {
		return parseBoolSetting(v, false)
	}
	return s.registrationEnabled(ctx)
}

// openRegistration opens registration for `limit` new accounts (0 = unlimited,
// bounded only by the license). Resets the used counter.
func (s *TelegramBotService) openRegistration(ctx context.Context, limit int) error {
	if limit < 0 {
		limit = 0
	}
	if err := s.repo.Setting.Set(ctx, SettingOpenRegEnabled, "true"); err != nil {
		return err
	}
	if err := s.repo.Setting.Set(ctx, SettingOpenRegLimit, strconv.Itoa(limit)); err != nil {
		return err
	}
	if err := s.repo.Setting.Set(ctx, SettingOpenRegUsed, "0"); err != nil {
		return err
	}
	// 与旧开关同步，兼容系统设置页。
	return s.setRegistrationEnabled(ctx, true)
}

// closeRegistration disables bot registration.
func (s *TelegramBotService) closeRegistration(ctx context.Context) error {
	if err := s.repo.Setting.Set(ctx, SettingOpenRegEnabled, "false"); err != nil {
		return err
	}
	return s.setRegistrationEnabled(ctx, false)
}

// consumeOpenRegSlot increments the used counter and auto-closes registration
// once the quota is exhausted. Call after a successful bot registration.
func (s *TelegramBotService) consumeOpenRegSlot(ctx context.Context) {
	limit := s.intSetting(ctx, SettingOpenRegLimit, 0)
	used := s.intSetting(ctx, SettingOpenRegUsed, 0) + 1
	_ = s.repo.Setting.Set(ctx, SettingOpenRegUsed, strconv.Itoa(used))
	if limit > 0 && used >= limit {
		_ = s.closeRegistration(ctx)
	}
}

// ── 兑换码 ──────────────────────────────────────────────────────────────────

// generateCode creates a random redemption code of the given kind. durationDays
// sets the account validity granted on redeem (0 = permanent). validDays sets
// how long the code itself stays redeemable (0 = never expires).
func (s *TelegramBotService) generateCode(ctx context.Context, kind string, durationDays, validDays int, createdBy string) (*model.RegistrationCode, error) {
	if kind != model.RegistrationCodeRegister && kind != model.RegistrationCodeRenew {
		kind = model.RegistrationCodeRegister
	}
	code := &model.RegistrationCode{
		Code:         randomCode(12),
		Kind:         kind,
		DurationDays: durationDays,
		CreatedByID:  createdBy,
	}
	if validDays > 0 {
		exp := time.Now().Add(time.Duration(validDays) * 24 * time.Hour)
		code.ExpiresAt = &exp
	}
	if err := s.repo.RegCode.Create(ctx, code); err != nil {
		return nil, err
	}
	return code, nil
}

// lookupRedeemableCode validates a code without consuming it. Callers mark it
// used only after the dependent action (account create / renew) succeeds, so a
// failed action never burns a code.
func (s *TelegramBotService) lookupRedeemableCode(ctx context.Context, raw, wantKind string) (*model.RegistrationCode, string) {
	code := strings.TrimSpace(raw)
	if code == "" {
		return nil, "请提供兑换码。"
	}
	rc, err := s.repo.RegCode.FindByCode(ctx, code)
	if err != nil || rc == nil {
		return nil, "兑换码无效。"
	}
	if rc.IsUsed() {
		return nil, "兑换码已被使用。"
	}
	if rc.IsExpired() {
		return nil, "兑换码已过期。"
	}
	if wantKind != "" && rc.Kind != wantKind {
		switch rc.Kind {
		case model.RegistrationCodeRenew:
			return nil, "这是续期兑换码，请在「我的账号」里使用它续期。"
		default:
			return nil, "这是注册兑换码，请用于注册新账号。"
		}
	}
	return rc, ""
}

// ── 续期 ────────────────────────────────────────────────────────────────────

// renewUser extends a user's expiry by durationDays. A nil/zero current expiry
// starts from now; a future expiry is extended from that point. durationDays<=0
// sets the account to never expire (permanent).
func renewExpiry(current *time.Time, durationDays int) *time.Time {
	if durationDays <= 0 {
		return nil // permanent
	}
	base := time.Now()
	if current != nil && current.After(base) {
		base = *current
	}
	exp := base.Add(time.Duration(durationDays) * 24 * time.Hour)
	return &exp
}

// applyRenewal renews a user account and clears any expiry-related suspension.
func (s *TelegramBotService) applyRenewal(ctx context.Context, userID string, durationDays int) error {
	u, err := s.repo.User.FindByID(ctx, userID)
	if err != nil || u == nil {
		return fmt.Errorf("user not found")
	}
	exp := renewExpiry(u.ExpiredAt, durationDays)
	updates := map[string]any{"expired_at": exp, "is_active": true}
	return s.repo.User.UpdateFields(ctx, userID, updates)
}

// ── 签到 ────────────────────────────────────────────────────────────────────

// signInResult 描述一次签到的结果。
type signInResult struct {
	AlreadySigned bool
	Streak        int
	Total         int
}

// signIn records a daily sign-in for the user, tracking consecutive-day streaks
// only (no points). A second sign-in on the same calendar day is a no-op.
func (s *TelegramBotService) signIn(ctx context.Context, userID string) (signInResult, error) {
	now := time.Now()
	today := now.Truncate(24 * time.Hour)
	rec, err := s.repo.SignIn.Get(ctx, userID)
	if err != nil {
		return signInResult{}, err
	}
	if rec == nil {
		rec = &model.SignIn{UserID: userID, LastSignIn: now, StreakDays: 1, TotalDays: 1}
		if err := s.repo.SignIn.Save(ctx, rec); err != nil {
			return signInResult{}, err
		}
		return signInResult{Streak: 1, Total: 1}, nil
	}
	last := rec.LastSignIn.Truncate(24 * time.Hour)
	switch {
	case last.Equal(today):
		return signInResult{AlreadySigned: true, Streak: rec.StreakDays, Total: rec.TotalDays}, nil
	case last.Equal(today.Add(-24 * time.Hour)):
		rec.StreakDays++
	default:
		rec.StreakDays = 1 // streak broken
	}
	rec.TotalDays++
	rec.LastSignIn = now
	if err := s.repo.SignIn.Save(ctx, rec); err != nil {
		return signInResult{}, err
	}
	return signInResult{Streak: rec.StreakDays, Total: rec.TotalDays}, nil
}

// ── helpers ─────────────────────────────────────────────────────────────────

const codeAlphabet = "ABCDEFGHJKLMNPQRSTUVWXYZ23456789" // no ambiguous 0/O/1/I

func randomCode(n int) string {
	b := make([]byte, n)
	if _, err := rand.Read(b); err != nil {
		// crypto/rand failure is extremely unlikely; fall back to time noise.
		seed := time.Now().UnixNano()
		for i := range b {
			b[i] = byte(seed >> (uint(i%8) * 8))
		}
	}
	out := make([]byte, n)
	for i := range b {
		out[i] = codeAlphabet[int(b[i])%len(codeAlphabet)]
	}
	return string(out)
}

// formatExpiry renders a user's expiry status for display.
func formatExpiry(t *time.Time) string {
	if t == nil {
		return "永久有效"
	}
	if time.Now().After(*t) {
		return "已过期（" + t.Format("2006-01-02") + "）"
	}
	days := int(time.Until(*t).Hours() / 24)
	return fmt.Sprintf("%s（剩 %d 天）", t.Format("2006-01-02"), days)
}
