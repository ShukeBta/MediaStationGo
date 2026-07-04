package service

import (
	"strings"
	"testing"
	"time"

	"go.uber.org/zap"

	"github.com/ShukeBta/MediaStationGo/internal/model"
	"github.com/ShukeBta/MediaStationGo/internal/repository"
)

func TestSessionTrackerAppliesRealtimeActivityToUsers(t *testing.T) {
	tracker := NewSessionTrackerService(zap.NewNop())
	now := time.Date(2026, 6, 21, 10, 0, 0, 0, time.UTC)
	tracker.now = func() time.Time { return now }
	old := now.Add(-8 * time.Hour)
	users := []model.User{{Base: model.Base{ID: "u1"}, Username: "admin", LastLoginAt: &old}}

	tracker.RecordLogin(t.Context(), "u1", "admin", "web-1", "Web", "Browser", "127.0.0.1")
	tracker.ApplyToUsers(t.Context(), users)

	if users[0].LastLoginAt == nil || !users[0].LastLoginAt.Equal(now) {
		t.Fatalf("last_login_at = %v, want realtime %v", users[0].LastLoginAt, now)
	}
	if !users[0].RealtimeOnline || users[0].RealtimeDeviceCount != 1 {
		t.Fatalf("realtime flags online=%v devices=%d", users[0].RealtimeOnline, users[0].RealtimeDeviceCount)
	}
}

func TestSessionTrackerDeduplicatesAppsOnSameTerminal(t *testing.T) {
	tracker := NewSessionTrackerService(zap.NewNop())
	now := time.Date(2026, 6, 21, 10, 30, 0, 0, time.UTC)
	tracker.now = func() time.Time { return now }
	users := []model.User{{Base: model.Base{ID: "u1"}, Username: "viewer"}}

	tracker.RecordActivity(t.Context(), "u1", "viewer", "infuse-device", "iPhone", "Infuse", "10.0.0.8")
	now = now.Add(time.Minute)
	tracker.RecordActivity(t.Context(), "u1", "viewer", "emby-device", " IPHONE ", "Emby", "10.0.0.8")

	sessions := tracker.List(t.Context())
	if len(sessions) != 1 {
		t.Fatalf("sessions = %#v, want one merged terminal", sessions)
	}
	if sessions[0].DeviceID != "emby-device" || sessions[0].Client != "Emby" {
		t.Fatalf("merged session should keep latest client details, got %#v", sessions[0])
	}
	tracker.ApplyToUsers(t.Context(), users)
	if !users[0].RealtimeOnline || users[0].RealtimeDeviceCount != 1 {
		t.Fatalf("same terminal should count as one online device, online=%v devices=%d", users[0].RealtimeOnline, users[0].RealtimeDeviceCount)
	}
}

func TestSessionTrackerLogoutUsesLatestMergedDeviceID(t *testing.T) {
	tracker := NewSessionTrackerService(zap.NewNop())
	now := time.Date(2026, 6, 21, 10, 45, 0, 0, time.UTC)
	tracker.now = func() time.Time { return now }

	tracker.RecordActivity(t.Context(), "u1", "viewer", "infuse-device", "iPhone", "Infuse", "10.0.0.8")
	now = now.Add(time.Minute)
	tracker.RecordActivity(t.Context(), "u1", "viewer", "emby-device", "iPhone", "Emby", "10.0.0.8")
	tracker.Logout(t.Context(), "u1", "emby-device", "10.0.0.8")

	if sessions := tracker.List(t.Context()); len(sessions) != 0 {
		t.Fatalf("merged terminal should logout by latest device id, got %#v", sessions)
	}
}

func TestDeviceListMergesRealtimeSessions(t *testing.T) {
	repos := newSessionTrackerTestRepos(t)
	user := model.User{Base: model.Base{ID: "u1"}, Username: "viewer", PasswordHash: "x", Role: "user", IsActive: true}
	if err := repos.User.Create(t.Context(), &user); err != nil {
		t.Fatal(err)
	}
	tracker := NewSessionTrackerService(zap.NewNop())
	now := time.Date(2026, 6, 21, 11, 0, 0, 0, time.UTC)
	tracker.now = func() time.Time { return now }
	tracker.RecordPlayback(t.Context(), user.ID, user.Username, "dev-1", "Apple TV", "Yamby", "10.0.0.8", "media-1", 123, 456, false)

	device := NewDeviceService(zap.NewNop(), repos)
	device.SetSessionTracker(tracker)
	rows, err := device.ListDevices(t.Context(), user.ID)
	if err != nil {
		t.Fatal(err)
	}
	if len(rows) != 1 {
		t.Fatalf("devices = %d, want 1", len(rows))
	}
	if !rows[0].Realtime || !rows[0].Online || !rows[0].Playing {
		t.Fatalf("realtime device flags = realtime:%v online:%v playing:%v", rows[0].Realtime, rows[0].Online, rows[0].Playing)
	}
	if rows[0].DeviceName != "Apple TV" || rows[0].Client != "Yamby" || !rows[0].LastSeenAt.Equal(now) {
		t.Fatalf("device row = %#v", rows[0])
	}
}

func TestActivityRefreshKeepsPlaybackState(t *testing.T) {
	tracker := NewSessionTrackerService(zap.NewNop())
	now := time.Date(2026, 6, 21, 11, 30, 0, 0, time.UTC)
	tracker.now = func() time.Time { return now }
	tracker.RecordPlayback(t.Context(), "u1", "viewer", "dev-1", "Apple TV", "Yamby", "10.0.0.8", "media-1", 123, 456, false)
	now = now.Add(time.Minute)
	tracker.RecordActivity(t.Context(), "u1", "viewer", "dev-1", "Apple TV", "Yamby", "10.0.0.8")

	sessions := tracker.List(t.Context())
	if len(sessions) != 1 {
		t.Fatalf("sessions = %#v, want one", sessions)
	}
	if !sessions[0].IsPlaying || sessions[0].ItemID != "media-1" || sessions[0].PositionTicks != 123 || sessions[0].RuntimeTicks != 456 {
		t.Fatalf("activity refresh should keep playback state, got %#v", sessions[0])
	}
	if !sessions[0].LastActivityAt.Equal(now) {
		t.Fatalf("last activity = %v, want %v", sessions[0].LastActivityAt, now)
	}
}

func TestLogoutKeepsRealtimeLastActivityWithoutOnlineSession(t *testing.T) {
	tracker := NewSessionTrackerService(zap.NewNop())
	now := time.Date(2026, 6, 21, 12, 30, 0, 0, time.UTC)
	tracker.now = func() time.Time { return now }
	old := now.Add(-8 * time.Hour)
	users := []model.User{{Base: model.Base{ID: "u1"}, Username: "viewer", LastLoginAt: &old}}

	tracker.RecordActivity(t.Context(), "u1", "viewer", "dev-1", "iPhone", "Infuse", "10.0.0.8")
	now = now.Add(time.Minute)
	tracker.Logout(t.Context(), "u1", "dev-1", "10.0.0.8")
	tracker.ApplyToUsers(t.Context(), users)

	if users[0].LastLoginAt == nil || !users[0].LastLoginAt.Equal(now) {
		t.Fatalf("last_login_at = %v, want logout activity %v", users[0].LastLoginAt, now)
	}
	if users[0].RealtimeOnline || users[0].RealtimeDeviceCount != 0 {
		t.Fatalf("logged-out user should keep last activity but no online devices, online=%v devices=%d", users[0].RealtimeOnline, users[0].RealtimeDeviceCount)
	}
}

func TestBotDevicesIncludesRealtimeSessionOnlyDevices(t *testing.T) {
	repos, bot := newBotTestService(t)
	user := model.User{Base: model.Base{ID: "u1"}, Username: "viewer", PasswordHash: "x", Role: "user", IsActive: true}
	if err := repos.User.Create(t.Context(), &user); err != nil {
		t.Fatal(err)
	}
	if err := repos.DB.Create(&model.TelegramBinding{TelegramUserID: 9103, ChatID: 9103, UserID: user.ID}).Error; err != nil {
		t.Fatal(err)
	}
	tracker := NewSessionTrackerService(zap.NewNop())
	now := time.Date(2026, 6, 21, 11, 0, 0, 0, time.UTC)
	tracker.now = func() time.Time { return now }
	tracker.RecordActivity(t.Context(), user.ID, user.Username, "dev-1", "Apple TV", "Yamby", "10.0.0.8")
	device := NewDeviceService(zap.NewNop(), repos)
	device.SetSessionTracker(tracker)
	bot.SetDeviceService(device)

	reply := bot.replyDevices(t.Context(), &TelegramMessage{
		From: TelegramUser{ID: 9103, Username: "viewer"},
		Chat: TelegramChat{ID: 9103, Type: "private"},
	})

	if !strings.Contains(reply.Text, "Apple TV / Yamby") || !strings.Contains(reply.Text, "在线") {
		t.Fatalf("reply should include realtime online device, got %q", reply.Text)
	}
	if !strings.Contains(reply.Text, "06-21 11:00") {
		t.Fatalf("reply should use realtime last seen time, got %q", reply.Text)
	}
}

func TestBotUserInfoUsesRealtimeLastLogin(t *testing.T) {
	repos, bot := newBotTestService(t)
	now := time.Date(2026, 6, 21, 13, 45, 0, 0, time.UTC)
	old := now.Add(-6 * time.Hour)
	user := model.User{Base: model.Base{ID: "u1"}, Username: "viewer", PasswordHash: "x", Role: "user", IsActive: true, LastLoginAt: &old}
	if err := repos.User.Create(t.Context(), &user); err != nil {
		t.Fatal(err)
	}
	tracker := NewSessionTrackerService(zap.NewNop())
	tracker.now = func() time.Time { return now }
	tracker.RecordActivity(t.Context(), user.ID, user.Username, "dev-1", "Apple TV", "Yamby", "10.0.0.8")
	device := NewDeviceService(zap.NewNop(), repos)
	device.SetSessionTracker(tracker)
	bot.SetDeviceService(device)

	reply := bot.cmdMgoUserInfo(t.Context(), []string{"viewer"})

	if !strings.Contains(reply.Text, "最后登录：<b>2026-06-21 13:45</b>") {
		t.Fatalf("reply should use realtime last login, got %q", reply.Text)
	}
	if !strings.Contains(reply.Text, "设备：<b>1</b>") {
		t.Fatalf("reply should count realtime device, got %q", reply.Text)
	}
}

func TestRealtimeRecentLoginProtectsCleanupCandidate(t *testing.T) {
	repos := newSessionTrackerTestRepos(t)
	now := time.Date(2026, 6, 21, 12, 0, 0, 0, time.UTC)
	old := now.Add(-30 * 24 * time.Hour)
	user := model.User{Base: model.Base{ID: "u1"}, Username: "viewer", PasswordHash: "x", Role: "user", IsActive: true, LastLoginAt: &old}
	if err := repos.User.Create(t.Context(), &user); err != nil {
		t.Fatal(err)
	}
	if err := repos.Setting.Set(t.Context(), SettingAccountCleanupEnabled, "true"); err != nil {
		t.Fatal(err)
	}
	if err := repos.Setting.Set(t.Context(), SettingAccountCleanupRules, `[{"id":"login_7d","name":"最近登录","type":"recent_login","enabled":true,"window_days_max":7}]`); err != nil {
		t.Fatal(err)
	}
	tracker := NewSessionTrackerService(zap.NewNop())
	tracker.now = func() time.Time { return now }
	tracker.RecordLogin(t.Context(), user.ID, user.Username, "web", "Web", "Browser", "127.0.0.1")
	device := NewDeviceService(zap.NewNop(), repos)
	device.SetSessionTracker(tracker)

	candidates, err := device.PreviewAccountCleanup(t.Context())
	if err != nil {
		t.Fatal(err)
	}
	if len(candidates) != 0 {
		t.Fatalf("recent realtime login should protect user, got candidates %#v", candidates)
	}
}

func newSessionTrackerTestRepos(t *testing.T) *repository.Container {
	t.Helper()
	db := newServiceTestDB(t, &model.User{}, &model.Setting{}, &model.UserDevice{}, &model.SignIn{}, &model.PlaybackHistory{})
	return repository.New(db)
}
