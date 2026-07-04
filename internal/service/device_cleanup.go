package service

import (
	"context"
	"fmt"
	"strings"
	"time"

	"go.uber.org/zap"

	"github.com/ShukeBta/MediaStationGo/internal/model"
)

// SweepInactiveUsers is kept for compatibility with the existing scheduler; it
// now delegates to the custom account-cleanup policy.
func (s *DeviceService) SweepInactiveUsers(ctx context.Context) (int, error) {
	return s.SweepAccountCleanup(ctx)
}

// SweepAccountCleanup runs the admin-defined account cleanup policy once.
// Users are kept when they satisfy any enabled keep rule. Users that do not
// meet any enabled rule are deleted.
func (s *DeviceService) SweepAccountCleanup(ctx context.Context) (int, error) {
	cfg := loadBotConfig(ctx, s.repo)
	if !cfg.AccountCleanupEnabled {
		return 0, nil
	}
	candidates, err := s.accountCleanupCandidates(ctx, cfg)
	if err != nil {
		return 0, err
	}
	removed := 0
	for _, candidate := range candidates {
		s.notify(ctx, candidate.UserID, fmt.Sprintf("⛔️ 账号 <b>%s</b> 未满足保号规则，已被清理。\n规则结果：%s\n如需恢复请联系管理员。", candidate.Username, candidate.Details))
		s.log.Warn("account cleanup: deleting account", zap.String("user", candidate.Username), zap.String("details", candidate.Details))
		_ = s.repo.UserDevice.DeleteByUser(ctx, candidate.UserID)
		if err := s.repo.User.Delete(ctx, candidate.UserID); err == nil {
			removed++
		}
	}
	return removed, nil
}

type accountCleanupCandidate struct {
	UserID   string
	Username string
	Details  string
}

func (s *DeviceService) PreviewAccountCleanup(ctx context.Context) ([]accountCleanupCandidate, error) {
	cfg := loadBotConfig(ctx, s.repo)
	if !cfg.AccountCleanupEnabled {
		return nil, nil
	}
	return s.accountCleanupCandidates(ctx, cfg)
}

func (s *DeviceService) accountCleanupCandidates(ctx context.Context, cfg botConfig) ([]accountCleanupCandidate, error) {
	users, err := s.repo.User.List(ctx)
	if err != nil {
		return nil, err
	}
	candidates := make([]accountCleanupCandidate, 0)
	for i := range users {
		u := &users[i]
		if s.isProtected(ctx, u) || !u.IsActive {
			continue
		}
		keep, details := s.userMatchesCleanupPolicy(ctx, u, cfg)
		if keep {
			continue
		}
		candidates = append(candidates, accountCleanupCandidate{
			UserID:   u.ID,
			Username: u.Username,
			Details:  details,
		})
	}
	return candidates, nil
}

// randomWindowDays returns a random integer in [min,max]. The window is
// intentionally non-fixed per the operator's requirement ("随机触发").
func randomWindowDays(min, max int) int {
	if min < 1 {
		min = 1
	}
	if max < min {
		max = min
	}
	if max == min {
		return min
	}
	return min + secureRandomIntn(max-min+1)
}

func (s *DeviceService) userMatchesCleanupPolicy(ctx context.Context, u *model.User, cfg botConfig) (bool, string) {
	rules := make([]accountCleanupRule, 0, len(cfg.AccountCleanupRules))
	for _, r := range cfg.AccountCleanupRules {
		if r.Enabled {
			rules = append(rules, r)
		}
	}
	if len(rules) == 0 {
		return true, "无启用规则，跳过"
	}
	matches := 0
	parts := make([]string, 0, len(rules))
	for _, r := range rules {
		ok, detail := s.userMatchesCleanupRule(ctx, u, r)
		if ok {
			matches++
			parts = append(parts, "✅ "+detail)
		} else {
			parts = append(parts, "❌ "+detail)
		}
	}
	required := 1
	return matches >= required, fmt.Sprintf("满足 %d/%d 条，需要 %d 条；%s", matches, len(rules), required, strings.Join(parts, "；"))
}

func (s *DeviceService) userMatchesCleanupRule(ctx context.Context, u *model.User, r accountCleanupRule) (bool, string) {
	switch r.Type {
	case "watch_hours":
		windowDays := randomWindowDays(r.WindowDaysMin, r.WindowDaysMax)
		since := time.Now().Add(-time.Duration(windowDays) * 24 * time.Hour)
		watched, _ := s.repo.UserDevice.WatchedMillisSince(ctx, u.ID, since)
		hours := float64(watched) / 3600000
		return hours >= r.MinHours, fmt.Sprintf("%s：近 %d 天观看 %.1f/%.1f 小时", r.Name, windowDays, hours, r.MinHours)
	case "recent_login":
		days := r.WindowDaysMax
		if days < 1 {
			days = r.WindowDaysMin
		}
		cutoff := time.Now().Add(-time.Duration(days) * 24 * time.Hour)
		ok := u.LastLoginAt != nil && u.LastLoginAt.After(cutoff)
		if !ok && s.sessions != nil {
			ok = s.sessions.UserRecentlyActive(ctx, u.ID, time.Duration(days)*24*time.Hour)
		}
		return ok, fmt.Sprintf("%s：%d 天内登录", r.Name, days)
	case "signin_streak":
		rec, _ := s.repo.SignIn.Get(ctx, u.ID)
		streak := 0
		if rec != nil {
			streak = rec.StreakDays
		}
		return streak >= r.MinCount, fmt.Sprintf("%s：连续签到 %d/%d 天", r.Name, streak, r.MinCount)
	case "account_age_grace":
		days := r.MinCount
		if days < 1 {
			days = r.WindowDaysMax
		}
		ok := u.CreatedAt.After(time.Now().Add(-time.Duration(days) * 24 * time.Hour))
		return ok, fmt.Sprintf("%s：新账号 %d 天宽限", r.Name, days)
	default:
		return false, r.Name + "：未知规则"
	}
}
