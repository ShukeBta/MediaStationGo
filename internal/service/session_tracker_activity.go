package service

import (
	"context"
	"strings"
	"time"

	"github.com/ShukeBta/MediaStationGo/internal/model"
)

func (s *SessionTrackerService) ApplyToUsers(ctx context.Context, users []model.User) {
	if s == nil || len(users) == 0 {
		return
	}
	activity := s.activityByUser(ctx)
	for i := range users {
		a, ok := activity[users[i].ID]
		if !ok {
			continue
		}
		if a.LastActivityAt != nil && (users[i].LastLoginAt == nil || a.LastActivityAt.After(*users[i].LastLoginAt)) {
			t := *a.LastActivityAt
			users[i].LastLoginAt = &t
		}
		users[i].RealtimeOnline = a.Online
		users[i].RealtimeDeviceCount = a.ActiveDeviceCount
	}
}

func (s *SessionTrackerService) UserRecentlyActive(ctx context.Context, userID string, within time.Duration) bool {
	if s == nil || within <= 0 {
		return false
	}
	activity := s.activityByUser(ctx)[strings.TrimSpace(userID)]
	return activity.LastActivityAt != nil && activity.LastActivityAt.After(s.now().Add(-within))
}

func (s *SessionTrackerService) activityByUser(ctx context.Context) map[string]userRealtimeActivity {
	sessions := s.List(ctx)
	now := s.now()
	out := make(map[string]userRealtimeActivity)
	for userID, lastActivity := range s.userActivitySnapshot() {
		if strings.TrimSpace(userID) == "" {
			continue
		}
		a := out[userID]
		if a.LastActivityAt == nil || lastActivity.After(*a.LastActivityAt) {
			t := lastActivity
			a.LastActivityAt = &t
		}
		out[userID] = a
	}
	seenDevices := make(map[string]map[string]struct{})
	for _, sess := range sessions {
		if strings.TrimSpace(sess.UserID) == "" {
			continue
		}
		a := out[sess.UserID]
		if a.LastActivityAt == nil || sess.LastActivityAt.After(*a.LastActivityAt) {
			t := sess.LastActivityAt
			a.LastActivityAt = &t
		}
		if sess.LastActivityAt.After(now.Add(-realtimeSessionOnlineTTL)) {
			a.Online = true
		}
		if seenDevices[sess.UserID] == nil {
			seenDevices[sess.UserID] = map[string]struct{}{}
		}
		seenDevices[sess.UserID][sessionDeviceKey(sess)] = struct{}{}
		a.ActiveDeviceCount = len(seenDevices[sess.UserID])
		out[sess.UserID] = a
	}
	return out
}

func (s *SessionTrackerService) userActivitySnapshot() map[string]time.Time {
	s.mu.RLock()
	defer s.mu.RUnlock()
	out := make(map[string]time.Time, len(s.activity))
	for userID, lastActivity := range s.activity {
		out[userID] = lastActivity
	}
	return out
}
