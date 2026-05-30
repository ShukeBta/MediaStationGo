package service

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"math/rand"
	"strings"
	"time"

	"go.uber.org/zap"

	"github.com/ShukeBta/MediaStationGo/internal/model"
	"github.com/ShukeBta/MediaStationGo/internal/repository"
)

// DeviceService implements device/session tracking and the two decoupled
// enforcement policies requested by the operator:
//
//	① 防共享 (anti-sharing, warning-based): too many concurrent playbacks,
//	   too many logged-in clients, or a device-fingerprint mismatch each emit a
//	   warning. After the configured number of warnings, a re-offence deletes
//	   the account. These three sub-rules share one per-user warning counter.
//	② 不活跃清理 (inactivity cleanup, independent toggle): watching less than
//	   the configured hours over a random 3–5 day window deletes the account.
//
// Safeguards (always enforced): admin / protected accounts are never auto
// disabled or deleted; a Telegram notification is sent before any destructive
// action; every threshold is configurable and both policies default to OFF.
type DeviceService struct {
	log  *zap.Logger
	repo *repository.Container

	// notifyUser sends a Telegram message to the local user (resolved to their
	// Telegram binding). Wired by the bot service; nil disables notifications.
	notifyUser func(ctx context.Context, userID, text string)
}

// NewDeviceService constructs a DeviceService.
func NewDeviceService(log *zap.Logger, repo *repository.Container) *DeviceService {
	return &DeviceService{log: log, repo: repo}
}

// SetNotifier wires the per-user Telegram notification callback.
func (s *DeviceService) SetNotifier(fn func(ctx context.Context, userID, text string)) {
	s.notifyUser = fn
}

// fingerprint derives a stable short hash from the client + device name. A
// changed fingerprint for the same device id signals the session was cloned
// onto different hardware/software.
func fingerprint(client, deviceName string) string {
	sum := sha256.Sum256([]byte(strings.ToLower(strings.TrimSpace(client)) + "|" + strings.ToLower(strings.TrimSpace(deviceName))))
	return hex.EncodeToString(sum[:])[:16]
}

// isProtected reports whether a user must never be auto disabled/deleted.
// Admins are always protected; the earliest admin (default admin) too.
func (s *DeviceService) isProtected(ctx context.Context, u *model.User) bool {
	if u == nil {
		return true
	}
	if u.Role == "admin" {
		return true
	}
	if first, err := s.repo.User.FirstAdmin(ctx); err == nil && first != nil && first.ID == u.ID {
		return true
	}
	return false
}

// RecordLogin records (or refreshes) a device session at authentication time
// and runs the logged-in-client + fingerprint anti-share checks. It is safe to
// call on every Emby/Jellyfin AuthenticateByName request.
func (s *DeviceService) RecordLogin(ctx context.Context, userID, deviceID, deviceName, client, ip string) {
	if userID == "" {
		return
	}
	if deviceID == "" {
		// Fall back to a fingerprint-derived id so headless clients still count.
		deviceID = "fp-" + fingerprint(client, deviceName)
	}
	fp := fingerprint(client, deviceName)
	now := time.Now()

	existing, _ := s.repo.UserDevice.Find(ctx, userID, deviceID)
	mismatch := false
	if existing == nil {
		_ = s.repo.UserDevice.Create(ctx, &model.UserDevice{
			UserID:      userID,
			DeviceID:    deviceID,
			DeviceName:  deviceName,
			Client:      client,
			Fingerprint: fp,
			LastIP:      ip,
			FirstSeenAt: now,
			LastSeenAt:  now,
		})
	} else {
		if existing.Fingerprint != "" && existing.Fingerprint != fp {
			mismatch = true
		}
		existing.DeviceName = deviceName
		existing.Client = client
		existing.Fingerprint = fp
		existing.LastIP = ip
		existing.LastSeenAt = now
		existing.Kicked = false
		_ = s.repo.UserDevice.Save(ctx, existing)
	}

	cfg := loadBotConfig(ctx, s.repo)
	if !cfg.AntiShareEnabled {
		return
	}

	if mismatch {
		s.registerViolation(ctx, userID, fmt.Sprintf("设备指纹变更（设备：%s）", deviceLabel(deviceName, client)), cfg)
		return
	}
	since := now.Add(-time.Duration(cfg.ClientActiveDays) * 24 * time.Hour)
	if n, err := s.repo.UserDevice.CountActiveClients(ctx, userID, since); err == nil && int(n) > cfg.MaxLoggedClients {
		s.registerViolation(ctx, userID, fmt.Sprintf("同时登录客户端 %d 台，超过上限 %d 台", n, cfg.MaxLoggedClients), cfg)
	}
}

// RecordPlayback marks a device as actively playing and runs the concurrent
// playback anti-share check. Call from playback-progress reporting.
func (s *DeviceService) RecordPlayback(ctx context.Context, userID, deviceID, deviceName, client string) {
	if userID == "" {
		return
	}
	if deviceID == "" {
		deviceID = "fp-" + fingerprint(client, deviceName)
	}
	now := time.Now()
	existing, _ := s.repo.UserDevice.Find(ctx, userID, deviceID)
	if existing == nil {
		existing = &model.UserDevice{
			UserID:      userID,
			DeviceID:    deviceID,
			DeviceName:  deviceName,
			Client:      client,
			Fingerprint: fingerprint(client, deviceName),
			FirstSeenAt: now,
			LastSeenAt:  now,
		}
		existing.LastPlayAt = &now
		_ = s.repo.UserDevice.Create(ctx, existing)
	} else {
		existing.LastSeenAt = now
		existing.LastPlayAt = &now
		_ = s.repo.UserDevice.Save(ctx, existing)
	}

	cfg := loadBotConfig(ctx, s.repo)
	if !cfg.AntiShareEnabled {
		return
	}
	since := now.Add(-time.Duration(cfg.PlayWindowSeconds) * time.Second)
	if n, err := s.repo.UserDevice.CountConcurrentPlaying(ctx, userID, since); err == nil && int(n) > cfg.MaxConcurrentPlay {
		s.registerViolation(ctx, userID, fmt.Sprintf("同时播放 %d 台，超过上限 %d 台", n, cfg.MaxConcurrentPlay), cfg)
	}
}

// registerViolation increments the shared anti-share warning counter for a
// user, notifies them, and deletes the account once warnings exceed the
// threshold. Protected accounts are never auto-deleted. Violations are
// debounced so a single burst counts at most once per minute.
func (s *DeviceService) registerViolation(ctx context.Context, userID, reason string, cfg botConfig) {
	u, err := s.repo.User.FindByID(ctx, userID)
	if err != nil || u == nil {
		return
	}
	if s.isProtected(ctx, u) {
		s.log.Info("anti-share: skipping protected account", zap.String("user", u.Username), zap.String("reason", reason))
		return
	}
	now := time.Now()
	if u.LastShareWarnAt != nil && now.Sub(*u.LastShareWarnAt) < time.Minute {
		return // debounce burst
	}

	warnings := u.ShareWarnings + 1
	if warnings > cfg.WarnThreshold {
		// Exhausted warnings → delete (notify first).
		s.notify(ctx, userID, fmt.Sprintf("⛔️ 账号 <b>%s</b> 因多次触发防共享规则（%s）已被删除。如有疑问请联系管理员。", u.Username, reason))
		s.log.Warn("anti-share: deleting account after warnings",
			zap.String("user", u.Username), zap.Int("warnings", u.ShareWarnings), zap.String("reason", reason))
		_ = s.repo.UserDevice.DeleteByUser(ctx, userID)
		_ = s.repo.User.Delete(ctx, userID)
		return
	}
	_ = s.repo.User.UpdateFields(ctx, userID, map[string]any{
		"share_warnings":     warnings,
		"last_share_warn_at": &now,
	})
	left := cfg.WarnThreshold + 1 - warnings
	s.notify(ctx, userID, fmt.Sprintf("⚠️ 账号 <b>%s</b> 触发防共享规则：%s\n这是第 <b>%d</b> 次警告，再违规 <b>%d</b> 次将删除账号。请使用 Bot 的「我的设备」一键踢下线多余设备。", u.Username, reason, warnings, left))
	s.log.Info("anti-share: warning issued", zap.String("user", u.Username), zap.Int("warnings", warnings), zap.String("reason", reason))
}

// SweepInactiveUsers runs the inactivity cleanup policy once. It picks a random
// window in [min,max] days, then deletes non-protected users (past their grace
// period) who watched less than the configured hours in that window. Returns
// the number of accounts removed. No-op when the policy is disabled.
func (s *DeviceService) SweepInactiveUsers(ctx context.Context) (int, error) {
	cfg := loadBotConfig(ctx, s.repo)
	if !cfg.InactiveEnabled {
		return 0, nil
	}
	windowDays := randomWindowDays(cfg.InactiveWindowMin, cfg.InactiveWindowMax)
	since := time.Now().Add(-time.Duration(windowDays) * 24 * time.Hour)
	graceCutoff := time.Now().Add(-time.Duration(cfg.InactiveGraceDays) * 24 * time.Hour)
	minMs := int64(cfg.InactiveMinHours) * 3600 * 1000

	users, err := s.repo.User.List(ctx)
	if err != nil {
		return 0, err
	}
	removed := 0
	for i := range users {
		u := &users[i]
		if s.isProtected(ctx, u) || !u.IsActive {
			continue
		}
		if u.CreatedAt.After(graceCutoff) {
			continue // still within new-account grace period
		}
		watched, err := s.repo.UserDevice.WatchedMillisSince(ctx, u.ID, since)
		if err != nil {
			continue
		}
		if watched >= minMs {
			continue // active enough → keep
		}
		s.notify(ctx, u.ID, fmt.Sprintf("⛔️ 账号 <b>%s</b> 因近 %d 天观看时长不足 %d 小时（不活跃）已被清理。如需恢复请联系管理员。", u.Username, windowDays, cfg.InactiveMinHours))
		s.log.Warn("inactivity: deleting inactive account",
			zap.String("user", u.Username), zap.Int("window_days", windowDays), zap.Int64("watched_ms", watched))
		_ = s.repo.UserDevice.DeleteByUser(ctx, u.ID)
		if err := s.repo.User.Delete(ctx, u.ID); err == nil {
			removed++
		}
	}
	return removed, nil
}

// KickDevice marks a device as kicked so the next request from it is rejected
// (the client must log in again). Returns the affected device for messaging.
func (s *DeviceService) KickDevice(ctx context.Context, userID, deviceID string) error {
	d, err := s.repo.UserDevice.Find(ctx, userID, deviceID)
	if err != nil {
		return err
	}
	if d == nil {
		return fmt.Errorf("device not found")
	}
	return s.repo.UserDevice.SetKicked(ctx, d.ID, true)
}

// ListDevices returns the device sessions for a user.
func (s *DeviceService) ListDevices(ctx context.Context, userID string) ([]model.UserDevice, error) {
	return s.repo.UserDevice.ListByUser(ctx, userID)
}

// IsDeviceKicked reports whether a (user, device) pair was kicked and should be
// forced to re-authenticate.
func (s *DeviceService) IsDeviceKicked(ctx context.Context, userID, deviceID string) bool {
	if userID == "" || deviceID == "" {
		return false
	}
	d, err := s.repo.UserDevice.Find(ctx, userID, deviceID)
	return err == nil && d != nil && d.Kicked
}

func (s *DeviceService) notify(ctx context.Context, userID, text string) {
	if s.notifyUser != nil {
		s.notifyUser(ctx, userID, text)
	}
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
	return min + rand.Intn(max-min+1)
}

func deviceLabel(name, client string) string {
	name = strings.TrimSpace(name)
	client = strings.TrimSpace(client)
	switch {
	case name != "" && client != "":
		return name + " / " + client
	case name != "":
		return name
	case client != "":
		return client
	default:
		return "未知设备"
	}
}
