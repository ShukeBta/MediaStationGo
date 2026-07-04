package service

import (
	"context"
	"sort"
	"strings"
	"sync"
	"time"

	"go.uber.org/zap"
)

const (
	realtimeSessionTTL       = 30 * time.Minute
	realtimeSessionOnlineTTL = 5 * time.Minute
)

func RealtimeDeletionGuardWindow() time.Duration {
	return realtimeSessionTTL
}

// RealtimeSession is an in-memory Emby-compatible session view. It mirrors the
// information reported by Emby clients through AuthenticateByName and
// /Sessions/Playing/* without requiring Playback Reporting persistence.
type RealtimeSession struct {
	ID             string     `json:"id"`
	UserID         string     `json:"user_id"`
	UserName       string     `json:"user_name,omitempty"`
	DeviceID       string     `json:"device_id"`
	DeviceName     string     `json:"device_name,omitempty"`
	Client         string     `json:"client,omitempty"`
	RemoteEndPoint string     `json:"remote_end_point,omitempty"`
	LastActivityAt time.Time  `json:"last_activity_at"`
	ItemID         string     `json:"item_id,omitempty"`
	PositionTicks  int64      `json:"position_ticks,omitempty"`
	RuntimeTicks   int64      `json:"runtime_ticks,omitempty"`
	IsPlaying      bool       `json:"is_playing"`
	IsPaused       bool       `json:"is_paused"`
	LastPlaybackAt *time.Time `json:"last_playback_at,omitempty"`
}

type realtimeSessionInput struct {
	UserID         string
	UserName       string
	DeviceID       string
	DeviceName     string
	Client         string
	RemoteEndPoint string
	ItemID         string
	PositionTicks  int64
	RuntimeTicks   int64
	IsPlaying      bool
	IsPaused       bool
	PlaybackUpdate bool
}

type userRealtimeActivity struct {
	LastActivityAt    *time.Time
	ActiveDeviceCount int
	Online            bool
}

// SessionTrackerService keeps recent client state in memory. It is intentionally
// not durable: process restart clears transient online status, while normal
// login/playback requests repopulate it immediately.
type SessionTrackerService struct {
	log *zap.Logger

	mu       sync.RWMutex
	sessions map[string]RealtimeSession
	activity map[string]time.Time
	now      func() time.Time
}

func NewSessionTrackerService(log *zap.Logger) *SessionTrackerService {
	return &SessionTrackerService{
		log:      log,
		sessions: make(map[string]RealtimeSession),
		activity: make(map[string]time.Time),
		now:      time.Now,
	}
}

func (s *SessionTrackerService) RecordLogin(ctx context.Context, userID, userName, deviceID, deviceName, client, remoteEndPoint string) {
	s.RecordActivity(ctx, userID, userName, deviceID, deviceName, client, remoteEndPoint)
}

func (s *SessionTrackerService) RecordActivity(ctx context.Context, userID, userName, deviceID, deviceName, client, remoteEndPoint string) {
	if s == nil {
		return
	}
	s.upsert(ctx, realtimeSessionInput{
		UserID:         userID,
		UserName:       userName,
		DeviceID:       deviceID,
		DeviceName:     deviceName,
		Client:         client,
		RemoteEndPoint: remoteEndPoint,
	})
}

func (s *SessionTrackerService) RecordPlayback(ctx context.Context, userID, userName, deviceID, deviceName, client, remoteEndPoint, itemID string, positionTicks, runtimeTicks int64, stopped bool) {
	if s == nil {
		return
	}
	s.upsert(ctx, realtimeSessionInput{
		UserID:         userID,
		UserName:       userName,
		DeviceID:       deviceID,
		DeviceName:     deviceName,
		Client:         client,
		RemoteEndPoint: remoteEndPoint,
		ItemID:         itemID,
		PositionTicks:  positionTicks,
		RuntimeTicks:   runtimeTicks,
		IsPlaying:      !stopped,
		PlaybackUpdate: true,
	})
}

func (s *SessionTrackerService) Logout(ctx context.Context, userID, deviceID, remoteEndPoint string) {
	if s == nil {
		return
	}
	userID = strings.TrimSpace(userID)
	if userID == "" {
		return
	}
	deviceID = strings.TrimSpace(deviceID)
	remoteEndPoint = strings.TrimSpace(remoteEndPoint)
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.activity == nil {
		s.activity = make(map[string]time.Time)
	}
	s.activity[userID] = s.now()
	for key, sess := range s.sessions {
		if sess.UserID != userID {
			continue
		}
		if deviceID != "" && sess.DeviceID != deviceID {
			continue
		}
		if deviceID == "" && remoteEndPoint != "" && sess.RemoteEndPoint != remoteEndPoint {
			continue
		}
		delete(s.sessions, key)
	}
}

func (s *SessionTrackerService) List(ctx context.Context) []RealtimeSession {
	if s == nil {
		return nil
	}
	now := s.now()
	s.mu.Lock()
	defer s.mu.Unlock()
	s.pruneLocked(now)
	out := make([]RealtimeSession, 0, len(s.sessions))
	for _, sess := range s.sessions {
		out = append(out, sess)
	}
	sort.SliceStable(out, func(i, j int) bool {
		return out[i].LastActivityAt.After(out[j].LastActivityAt)
	})
	return out
}

func (s *SessionTrackerService) ListByUser(ctx context.Context, userID string) []RealtimeSession {
	userID = strings.TrimSpace(userID)
	if userID == "" {
		return nil
	}
	all := s.List(ctx)
	out := make([]RealtimeSession, 0, len(all))
	for _, sess := range all {
		if sess.UserID == userID {
			out = append(out, sess)
		}
	}
	return out
}
