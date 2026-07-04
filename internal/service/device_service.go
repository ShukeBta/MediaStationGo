package service

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"strings"
	"time"

	"go.uber.org/zap"

	"github.com/ShukeBta/MediaStationGo/internal/model"
	"github.com/ShukeBta/MediaStationGo/internal/repository"
)

// DeviceService implements device/session tracking and the two decoupled
// enforcement policies requested by the operator:
//
//	① 防共享: too many concurrent playbacks / logged-in clients disables the
//	   account immediately; fingerprint mismatch is warning-based and disables
//	   the account after the configured warning threshold.
//	② Mgo 保号规则: admins define one or more keep rules; a sweep deletes
//	   accounts only when none of the enabled keep rules match.
//
// Safeguards: admin / protected accounts are never auto disabled or deleted;
// a Telegram notification is sent before a destructive action; every policy
// defaults to OFF.
type DeviceService struct {
	log      *zap.Logger
	repo     *repository.Container
	sessions *SessionTrackerService

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

func (s *DeviceService) SetSessionTracker(tracker *SessionTrackerService) {
	s.sessions = tracker
}

// fingerprint derives a stable terminal hash from the device name. Client/app
// names are deliberately ignored so one phone/TV/PC using multiple apps is
// still counted as one terminal device; Client remains a login channel label.
func fingerprint(client, deviceName string) string {
	terminal := strings.ToLower(strings.TrimSpace(deviceName))
	if terminal == "" {
		terminal = strings.ToLower(strings.TrimSpace(client))
	}
	sum := sha256.Sum256([]byte(terminal))
	return hex.EncodeToString(sum[:])[:16]
}

// isProtected reports whether a user must never be auto disabled/deleted.
// Admins are always protected; the earliest admin (default admin) too.
func (s *DeviceService) isProtected(ctx context.Context, u *model.User) bool {
	return UserIsProtectedAccount(ctx, s.repo, u)
}

// RecordLogin records (or refreshes) a login channel at authentication time and
// runs the terminal-device + fingerprint anti-share checks. It is safe to call
// on every Emby/Jellyfin AuthenticateByName request.
func (s *DeviceService) RecordLogin(ctx context.Context, userID, deviceID, deviceName, client, ip string) {
	if userID == "" {
		return
	}
	username := ""
	if u, _ := s.repo.User.FindByID(ctx, userID); u != nil {
		username = u.Username
	}
	if deviceID == "" {
		// Fall back to a fingerprint-derived id so headless clients still count.
		deviceID = "fp-" + fingerprint(client, deviceName)
	}
	if s.sessions != nil {
		s.sessions.RecordLogin(ctx, userID, username, deviceID, deviceName, client, ip)
	}
	fp := fingerprint(client, deviceName)
	now := s.now()

	existing, _ := s.findTerminalDevice(ctx, userID, deviceID, fp)
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
		existing.DeviceID = deviceID
		existing.DeviceName = deviceName
		existing.Client = client
		existing.Fingerprint = fp
		existing.LastIP = ip
		existing.LastSeenAt = now
		existing.Kicked = false
		_ = s.repo.UserDevice.Save(ctx, existing)
		s.deleteStaleTerminalDeviceRows(ctx, userID, fp, existing.ID)
	}

	cfg := loadBotConfig(ctx, s.repo)
	if !cfg.AntiShareEnabled {
		return
	}

	if mismatch {
		s.registerFingerprintWarning(ctx, userID, fmt.Sprintf("设备指纹变更（设备：%s）", deviceLabel(deviceName, client)), cfg)
		return
	}
	since := now.Add(-time.Duration(cfg.ClientActiveDays) * 24 * time.Hour)
	if n, err := s.repo.UserDevice.CountActiveClients(ctx, userID, since); err == nil && int(n) > cfg.MaxLoggedClients {
		s.disableForPolicy(ctx, userID, fmt.Sprintf("同时登录终端设备 %d 台，超过上限 %d 台", n, cfg.MaxLoggedClients))
	}
}

// RecordPlayback marks a device as actively playing and runs the concurrent
// playback anti-share check. Call from playback-progress reporting.
func (s *DeviceService) RecordPlayback(ctx context.Context, userID, deviceID, deviceName, client string) {
	if userID == "" {
		return
	}
	username := ""
	if u, _ := s.repo.User.FindByID(ctx, userID); u != nil {
		username = u.Username
	}
	if deviceID == "" {
		deviceID = "fp-" + fingerprint(client, deviceName)
	}
	if s.sessions != nil {
		s.sessions.RecordPlayback(ctx, userID, username, deviceID, deviceName, client, "", "", 0, 0, false)
	}
	now := s.now()
	fp := fingerprint(client, deviceName)
	existing, _ := s.findTerminalDevice(ctx, userID, deviceID, fp)
	if existing == nil {
		existing = &model.UserDevice{
			UserID:      userID,
			DeviceID:    deviceID,
			DeviceName:  deviceName,
			Client:      client,
			Fingerprint: fp,
			FirstSeenAt: now,
			LastSeenAt:  now,
		}
		existing.LastPlayAt = &now
		_ = s.repo.UserDevice.Create(ctx, existing)
	} else {
		existing.DeviceID = deviceID
		existing.DeviceName = deviceName
		existing.Client = client
		existing.Fingerprint = fp
		existing.LastSeenAt = now
		existing.LastPlayAt = &now
		_ = s.repo.UserDevice.Save(ctx, existing)
		s.deleteStaleTerminalDeviceRows(ctx, userID, fp, existing.ID)
	}

	cfg := loadBotConfig(ctx, s.repo)
	if !cfg.AntiShareEnabled {
		return
	}
	since := now.Add(-time.Duration(cfg.PlayWindowSeconds) * time.Second)
	if n, err := s.repo.UserDevice.CountConcurrentPlaying(ctx, userID, since); err == nil && int(n) > cfg.MaxConcurrentPlay {
		s.disableForPolicy(ctx, userID, fmt.Sprintf("同时播放终端设备 %d 台，超过上限 %d 台", n, cfg.MaxConcurrentPlay))
	}
}

func (s *DeviceService) findTerminalDevice(ctx context.Context, userID, deviceID, fp string) (*model.UserDevice, error) {
	existing, err := s.repo.UserDevice.Find(ctx, userID, deviceID)
	if err != nil || existing != nil {
		return existing, err
	}
	if strings.TrimSpace(fp) == "" {
		return nil, nil
	}
	return s.repo.UserDevice.FindByFingerprint(ctx, userID, fp)
}

func (s *DeviceService) deleteStaleTerminalDeviceRows(ctx context.Context, userID, fp, keepID string) {
	if strings.TrimSpace(fp) == "" || strings.TrimSpace(keepID) == "" {
		return
	}
	if err := s.repo.UserDevice.DeleteByFingerprintExcept(ctx, userID, fp, keepID); err != nil && s.log != nil {
		s.log.Warn("device terminal cleanup failed",
			zap.String("user_id", userID),
			zap.String("fingerprint", fp),
			zap.Error(err))
	}
}

// registerFingerprintWarning increments the fingerprint warning counter for a
// user and disables the account once warnings exceed the threshold. Violations
// are debounced so a single burst counts at most once per minute.
func (s *DeviceService) registerFingerprintWarning(ctx context.Context, userID, reason string, cfg botConfig) {
	u, err := s.repo.User.FindByID(ctx, userID)
	if err != nil || u == nil {
		return
	}
	if s.isProtected(ctx, u) {
		s.log.Info("anti-share: skipping protected account", zap.String("user", u.Username), zap.String("reason", reason))
		return
	}
	now := s.now()
	if u.LastShareWarnAt != nil && now.Sub(*u.LastShareWarnAt) < time.Minute {
		return // debounce burst
	}

	warnings := u.ShareWarnings + 1
	if warnings > cfg.WarnThreshold {
		s.disableForPolicy(ctx, userID, fmt.Sprintf("多次设备指纹异常：%s", reason))
		s.log.Warn("anti-share: disabling account after fingerprint warnings",
			zap.String("user", u.Username), zap.Int("warnings", u.ShareWarnings), zap.String("reason", reason))
		return
	}
	_ = s.repo.User.UpdateFields(ctx, userID, map[string]any{
		"share_warnings":     warnings,
		"last_share_warn_at": &now,
	})
	left := cfg.WarnThreshold + 1 - warnings
	s.notify(ctx, userID, fmt.Sprintf("⚠️ 账号 <b>%s</b> 触发设备指纹警告：%s\n这是第 <b>%d</b> 次警告，再异常 <b>%d</b> 次将禁用账号。请使用 Bot 的「我的设备」踢下线异常设备。", u.Username, reason, warnings, left))
	s.log.Info("anti-share: warning issued", zap.String("user", u.Username), zap.Int("warnings", warnings), zap.String("reason", reason))
}

func (s *DeviceService) disableForPolicy(ctx context.Context, userID, reason string) {
	u, err := s.repo.User.FindByID(ctx, userID)
	if err != nil || u == nil || !u.IsActive {
		return
	}
	if s.isProtected(ctx, u) {
		s.log.Info("device policy: skipping protected account", zap.String("user", u.Username), zap.String("reason", reason))
		return
	}
	now := s.now()
	_ = s.repo.User.UpdateFields(ctx, userID, map[string]any{
		"is_active":          false,
		"last_share_warn_at": &now,
	})
	_ = s.repo.UserDevice.SetKickedByUser(ctx, userID, true)
	s.notify(ctx, userID, fmt.Sprintf("⛔️ 账号 <b>%s</b> 因触发设备规则已被禁用：%s\n请联系管理员解除禁用，或通过「我的设备」踢下线多余设备后再申请恢复。", u.Username, reason))
	s.log.Warn("device policy: disabled account", zap.String("user", u.Username), zap.String("reason", reason))
}

func (s *DeviceService) UserRecentlyActive(ctx context.Context, userID string, within time.Duration) bool {
	return s.sessions != nil && s.sessions.UserRecentlyActive(ctx, userID, within)
}

func (s *DeviceService) now() time.Time {
	if s != nil && s.sessions != nil && s.sessions.now != nil {
		return s.sessions.now()
	}
	return time.Now()
}

func (s *DeviceService) notify(ctx context.Context, userID, text string) {
	if s.notifyUser != nil {
		s.notifyUser(ctx, userID, text)
	}
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
