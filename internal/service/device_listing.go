package service

import (
	"context"
	"fmt"
	"sort"
	"strings"
	"time"

	"github.com/ShukeBta/MediaStationGo/internal/model"
)

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
	if fp := strings.TrimSpace(d.Fingerprint); fp != "" {
		return s.repo.UserDevice.SetKickedByFingerprint(ctx, userID, fp, true)
	}
	return s.repo.UserDevice.SetKicked(ctx, d.ID, true)
}

// KickAllDevices marks all devices for a user as kicked.
func (s *DeviceService) KickAllDevices(ctx context.Context, userID string) error {
	return s.repo.UserDevice.SetKickedByUser(ctx, userID, true)
}

// ListDevices returns the device sessions for a user.
func (s *DeviceService) ListDevices(ctx context.Context, userID string) ([]model.UserDevice, error) {
	rows, err := s.repo.UserDevice.ListByUser(ctx, userID)
	if err != nil {
		return nil, err
	}
	rows = collapseUserDeviceRows(rows)
	if s.sessions == nil {
		return rows, nil
	}
	now := s.sessions.now()
	byDevice := make(map[string]int, len(rows))
	byTerminal := make(map[string]int, len(rows))
	for i := range rows {
		byDevice[rows[i].DeviceID] = i
		byTerminal[userDeviceTerminalKey(rows[i])] = i
	}
	for _, sess := range s.sessions.ListByUser(ctx, userID) {
		online := sess.LastActivityAt.After(now.Add(-realtimeSessionOnlineTTL))
		idx, ok := byDevice[sess.DeviceID]
		if !ok {
			idx, ok = byTerminal[sessionDeviceKey(sess)]
		}
		if ok {
			if sess.LastActivityAt.After(rows[idx].LastSeenAt) {
				rows[idx].LastSeenAt = sess.LastActivityAt
			}
			if sess.DeviceName != "" {
				rows[idx].DeviceName = sess.DeviceName
			}
			if sess.Client != "" {
				rows[idx].Client = sess.Client
			}
			if sess.RemoteEndPoint != "" {
				rows[idx].LastIP = sess.RemoteEndPoint
			}
			if sess.LastPlaybackAt != nil {
				rows[idx].LastPlayAt = sess.LastPlaybackAt
			}
			rows[idx].Realtime = true
			rows[idx].Online = online
			rows[idx].Playing = sess.IsPlaying && online
			continue
		}
		row := model.UserDevice{
			UserID:      userID,
			DeviceID:    sess.DeviceID,
			DeviceName:  sess.DeviceName,
			Client:      sess.Client,
			Fingerprint: fingerprint(sess.Client, sess.DeviceName),
			LastIP:      sess.RemoteEndPoint,
			FirstSeenAt: sess.LastActivityAt,
			LastSeenAt:  sess.LastActivityAt,
			LastPlayAt:  sess.LastPlaybackAt,
			Realtime:    true,
			Online:      online,
			Playing:     sess.IsPlaying && online,
		}
		row.ID = "rt:" + sess.ID
		rows = append(rows, row)
		byDevice[row.DeviceID] = len(rows) - 1
		byTerminal[userDeviceTerminalKey(row)] = len(rows) - 1
	}
	sort.SliceStable(rows, func(i, j int) bool {
		return rows[i].LastSeenAt.After(rows[j].LastSeenAt)
	})
	return rows, nil
}

func collapseUserDeviceRows(rows []model.UserDevice) []model.UserDevice {
	if len(rows) < 2 {
		return rows
	}
	out := make([]model.UserDevice, 0, len(rows))
	byTerminal := map[string]int{}
	for _, row := range rows {
		key := userDeviceTerminalKey(row)
		if idx, ok := byTerminal[key]; ok {
			out[idx] = mergeUserDeviceRows(out[idx], row)
			continue
		}
		byTerminal[key] = len(out)
		out = append(out, row)
	}
	return out
}

func mergeUserDeviceRows(a, b model.UserDevice) model.UserDevice {
	if b.LastSeenAt.After(a.LastSeenAt) {
		a.ID = b.ID
		a.DeviceID = b.DeviceID
		a.DeviceName = b.DeviceName
		a.Client = b.Client
		a.LastIP = b.LastIP
		a.FirstSeenAt = earlierTime(a.FirstSeenAt, b.FirstSeenAt)
		a.LastSeenAt = b.LastSeenAt
		a.LastPlayAt = latestOptionalTime(a.LastPlayAt, b.LastPlayAt)
		a.Fingerprint = firstNonEmptyString(b.Fingerprint, a.Fingerprint)
		a.Kicked = b.Kicked
		return a
	}
	a.FirstSeenAt = earlierTime(a.FirstSeenAt, b.FirstSeenAt)
	a.LastPlayAt = latestOptionalTime(a.LastPlayAt, b.LastPlayAt)
	a.Fingerprint = firstNonEmptyString(a.Fingerprint, b.Fingerprint)
	return a
}

func userDeviceTerminalKey(row model.UserDevice) string {
	if key := strings.TrimSpace(row.Fingerprint); key != "" {
		return key
	}
	if strings.TrimSpace(row.DeviceName) != "" {
		return "fp-" + fingerprint(row.Client, row.DeviceName)
	}
	if id := strings.TrimSpace(row.DeviceID); id != "" {
		return id
	}
	return "unknown"
}

func earlierTime(a, b time.Time) time.Time {
	if a.IsZero() || (!b.IsZero() && b.Before(a)) {
		return b
	}
	return a
}

func latestOptionalTime(a, b *time.Time) *time.Time {
	if a == nil {
		return b
	}
	if b != nil && b.After(*a) {
		return b
	}
	return a
}

// IsDeviceKicked reports whether a (user, device) pair was kicked and should be
// forced to re-authenticate.
func (s *DeviceService) IsDeviceKicked(ctx context.Context, userID, deviceID string) bool {
	return s.IsTerminalKicked(ctx, userID, deviceID, "", "")
}

// IsTerminalKicked reports whether a request belongs to a kicked terminal.
func (s *DeviceService) IsTerminalKicked(ctx context.Context, userID, deviceID, deviceName, client string) bool {
	if userID == "" {
		return false
	}
	if strings.TrimSpace(deviceID) != "" {
		d, err := s.repo.UserDevice.Find(ctx, userID, deviceID)
		if err == nil && d != nil {
			return d.Kicked
		}
	}
	fp := ""
	if strings.TrimSpace(deviceName) != "" {
		fp = fingerprint(client, deviceName)
	}
	if fp == "" {
		return false
	}
	d, err := s.repo.UserDevice.FindByFingerprint(ctx, userID, fp)
	return err == nil && d != nil && d.Kicked
}
