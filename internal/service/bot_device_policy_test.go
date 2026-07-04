package service

import (
	"context"
	"testing"
	"time"

	"go.uber.org/zap"

	"github.com/ShukeBta/MediaStationGo/internal/model"
)

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

func TestTerminalDeviceLimitDeduplicatesAppsOnSameDevice(t *testing.T) {
	ctx := context.Background()
	repos, _ := newBotTestService(t)
	dev := NewDeviceService(zap.NewNop(), repos)
	now := time.Date(2026, 6, 25, 21, 30, 0, 0, time.UTC)
	tracker := NewSessionTrackerService(zap.NewNop())
	tracker.now = func() time.Time { return now }
	dev.SetSessionTracker(tracker)
	u := &model.User{Username: "device-user", PasswordHash: "x", Role: "user", IsActive: true}
	if err := repos.User.Create(ctx, u); err != nil {
		t.Fatal(err)
	}
	if err := repos.Setting.Set(ctx, SettingAntiShareEnabled, "true"); err != nil {
		t.Fatal(err)
	}
	if err := repos.Setting.Set(ctx, SettingMaxLoggedClients, "3"); err != nil {
		t.Fatal(err)
	}

	for _, login := range []struct {
		id     string
		name   string
		client string
	}{
		{id: "phone-infuse", name: "iPhone", client: "Infuse"},
		{id: "phone-emby", name: " iPhone ", client: "Emby"},
		{id: "phone-jellyfin", name: "IPHONE", client: "Jellyfin"},
	} {
		dev.RecordLogin(ctx, u.ID, login.id, login.name, login.client, "1.2.3.4")
		now = now.Add(time.Second)
	}
	count, err := repos.UserDevice.CountActiveClients(ctx, u.ID, now.Add(-24*time.Hour))
	if err != nil {
		t.Fatal(err)
	}
	if count != 1 {
		t.Fatalf("same terminal through multiple apps should count as 1, got %d", count)
	}
	devices, err := dev.ListDevices(ctx, u.ID)
	if err != nil {
		t.Fatal(err)
	}
	if len(devices) != 1 {
		t.Fatalf("same terminal should show as one device row, got %#v", devices)
	}
	rawRows, err := repos.UserDevice.ListByUser(ctx, u.ID)
	if err != nil {
		t.Fatal(err)
	}
	if len(rawRows) != 1 {
		t.Fatalf("same terminal should be persisted as one canonical row, got %#v", rawRows)
	}
	if devices[0].DeviceID != "phone-jellyfin" || devices[0].Client != "Jellyfin" {
		t.Fatalf("merged device row should keep latest login channel, got %#v", devices[0])
	}
	got, _ := repos.User.FindByID(ctx, u.ID)
	if !got.IsActive {
		t.Fatal("same terminal through multiple apps must not disable the account")
	}

	dev.RecordLogin(ctx, u.ID, "tablet", "iPad", "Infuse", "1.2.3.4")
	dev.RecordLogin(ctx, u.ID, "pc", "Windows PC", "Browser", "1.2.3.4")
	count, err = repos.UserDevice.CountActiveClients(ctx, u.ID, now.Add(-24*time.Hour))
	if err != nil {
		t.Fatal(err)
	}
	if count != 3 {
		t.Fatalf("three distinct terminal devices should count as 3, got %d", count)
	}
	got, _ = repos.User.FindByID(ctx, u.ID)
	if !got.IsActive {
		t.Fatal("device limit is inclusive; 3 of 3 terminals should stay active")
	}

	dev.RecordLogin(ctx, u.ID, "tv", "Apple TV", "Emby", "1.2.3.4")
	got, _ = repos.User.FindByID(ctx, u.ID)
	if got.IsActive {
		t.Fatal("fourth distinct terminal should disable the account")
	}
}

func TestDeviceKickAppliesToMergedTerminal(t *testing.T) {
	ctx := context.Background()
	repos, _ := newBotTestService(t)
	dev := NewDeviceService(zap.NewNop(), repos)
	u := &model.User{Username: "kick-merged", PasswordHash: "x", Role: "user", IsActive: true}
	if err := repos.User.Create(ctx, u); err != nil {
		t.Fatal(err)
	}

	dev.RecordLogin(ctx, u.ID, "phone-infuse", "iPhone", "Infuse", "1.2.3.4")
	dev.RecordLogin(ctx, u.ID, "phone-emby", " iPhone ", "Emby", "1.2.3.4")
	if err := dev.KickDevice(ctx, u.ID, "phone-emby"); err != nil {
		t.Fatal(err)
	}
	if !dev.IsTerminalKicked(ctx, u.ID, "phone-jellyfin", "IPHONE", "Jellyfin") {
		t.Fatal("same terminal with a new app/device id should still be kicked")
	}

	dev.RecordLogin(ctx, u.ID, "phone-jellyfin", "IPHONE", "Jellyfin", "1.2.3.4")
	if dev.IsTerminalKicked(ctx, u.ID, "phone-jellyfin", "IPHONE", "Jellyfin") {
		t.Fatal("re-login should clear kicked state for the merged terminal")
	}
	rawRows, err := repos.UserDevice.ListByUser(ctx, u.ID)
	if err != nil {
		t.Fatal(err)
	}
	if len(rawRows) != 1 || rawRows[0].DeviceID != "phone-jellyfin" {
		t.Fatalf("merged terminal should keep one latest row, got %#v", rawRows)
	}
}

func TestRecordPlaybackMergesChangingDeviceIDOnSameTerminal(t *testing.T) {
	ctx := context.Background()
	repos, _ := newBotTestService(t)
	dev := NewDeviceService(zap.NewNop(), repos)
	u := &model.User{Username: "play-merged", PasswordHash: "x", Role: "user", IsActive: true}
	if err := repos.User.Create(ctx, u); err != nil {
		t.Fatal(err)
	}

	dev.RecordPlayback(ctx, u.ID, "tv-emby", "Living Room TV", "Emby")
	dev.RecordPlayback(ctx, u.ID, "tv-infuse", " living room tv ", "Infuse")

	rawRows, err := repos.UserDevice.ListByUser(ctx, u.ID)
	if err != nil {
		t.Fatal(err)
	}
	if len(rawRows) != 1 {
		t.Fatalf("same playback terminal should persist one row, got %#v", rawRows)
	}
	if rawRows[0].DeviceID != "tv-infuse" || rawRows[0].Client != "Infuse" || rawRows[0].LastPlayAt == nil {
		t.Fatalf("merged playback row should keep latest playback channel, got %#v", rawRows[0])
	}
	count, err := repos.UserDevice.CountConcurrentPlaying(ctx, u.ID, time.Now().Add(-time.Minute))
	if err != nil {
		t.Fatal(err)
	}
	if count != 1 {
		t.Fatalf("same terminal playback should count once, got %d", count)
	}
}

func TestConcurrentPlaybackDeduplicatesAppsOnSameDevice(t *testing.T) {
	ctx := context.Background()
	repos, _ := newBotTestService(t)
	u := &model.User{Username: "play-user", PasswordHash: "x", Role: "user", IsActive: true}
	if err := repos.User.Create(ctx, u); err != nil {
		t.Fatal(err)
	}
	now := time.Now()
	fp := fingerprint("Infuse", "Living Room TV")
	for _, row := range []model.UserDevice{
		{UserID: u.ID, DeviceID: "tv-emby", DeviceName: "Living Room TV", Client: "Emby", Fingerprint: fp, FirstSeenAt: now, LastSeenAt: now, LastPlayAt: &now},
		{UserID: u.ID, DeviceID: "tv-jellyfin", DeviceName: "living room tv", Client: "Jellyfin", Fingerprint: fp, FirstSeenAt: now, LastSeenAt: now, LastPlayAt: &now},
		{UserID: u.ID, DeviceID: "phone", DeviceName: "iPhone", Client: "Infuse", Fingerprint: fingerprint("Infuse", "iPhone"), FirstSeenAt: now, LastSeenAt: now, LastPlayAt: &now},
	} {
		if err := repos.UserDevice.Create(ctx, &row); err != nil {
			t.Fatal(err)
		}
	}
	count, err := repos.UserDevice.CountConcurrentPlaying(ctx, u.ID, now.Add(-time.Minute))
	if err != nil {
		t.Fatal(err)
	}
	if count != 2 {
		t.Fatalf("same terminal playback through multiple apps should count as 1 terminal, got %d", count)
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
	// 多次违规也不应删除/警告/禁用管理员
	for i := 0; i < 5; i++ {
		dev.registerFingerprintWarning(ctx, admin.ID, "test", cfg)
	}
	got, _ := repos.User.FindByID(ctx, admin.ID)
	if got == nil {
		t.Fatal("admin must never be auto-deleted")
	}
	if !got.IsActive {
		t.Fatal("admin must never be auto-disabled")
	}
	if got.ShareWarnings != 0 {
		t.Fatalf("admin should accrue no warnings, got %d", got.ShareWarnings)
	}
}
