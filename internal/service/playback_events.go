package service

import (
	"context"
	"crypto/sha256"
	"fmt"
	"strings"
	"time"

	"github.com/ShukeBta/MediaStationGo/internal/model"
	"github.com/google/uuid"
	"gorm.io/gorm/clause"
)

type playbackEventSession struct {
	ID        string
	UpdatedAt time.Time
	Stopped   bool
}

// PlaybackEventSession reuses the existing session tracker to assign an ID to
// clients without PlaySessionId. A stop or thirty minutes of inactivity starts
// a new session; repeated heartbeats stay in the same session.
func (s *SessionTrackerService) PlaybackEventSession(userID, deviceID, mediaID, provided string, stopped bool) string {
	if provided = strings.TrimSpace(provided); provided != "" {
		return fmt.Sprintf("%x", sha256.Sum256([]byte(provided)))
	}
	if s == nil {
		return ""
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	now := s.now()
	if s.playbackEvents == nil {
		s.playbackEvents = map[string]playbackEventSession{}
	}
	for key, value := range s.playbackEvents {
		if now.Sub(value.UpdatedAt) > realtimeSessionTTL {
			delete(s.playbackEvents, key)
		}
	}
	key := userID + "\x00" + deviceID + "\x00" + mediaID
	entry := s.playbackEvents[key]
	if entry.ID == "" || (entry.Stopped && !stopped) {
		entry.ID = uuid.NewString()
	}
	entry.UpdatedAt, entry.Stopped = now, stopped
	s.playbackEvents[key] = entry
	return entry.ID
}

func (c *Container) RecordPlaybackEvent(ctx context.Context, userID, mediaID, sessionID, client string, positionMS, durationMS int64) error {
	if positionMS < 20_000 || userID == "" || sessionID == "" {
		return nil
	}
	if c.Repo == nil || !c.Repo.DB.Migrator().HasTable(&model.PlaybackEvent{}) {
		return nil
	}
	media, err := c.Repo.Media.FindByID(ctx, mediaID)
	if err != nil || media == nil {
		return err
	}
	if durationMS <= 0 {
		durationMS = int64(media.DurationSec) * 1000
	}
	threshold := int64(20_000)
	if durationMS > 10*60_000 {
		threshold = 60_000
	}
	if positionMS < threshold || (durationMS > 0 && positionMS > durationMS) {
		return nil
	}
	kind, key := "movie", media.ID
	if media.EpisodeNum > 0 || media.SeasonNum > 0 {
		kind = "tv"
		key = fmt.Sprintf("%s/s%d", firstNonEmptyString(media.SeriesKey, media.SeriesID, media.Title), media.SeasonNum)
		if media.TMDbID > 0 {
			key = fmt.Sprintf("tv:%d/s%d", media.TMDbID, media.SeasonNum)
		}
	} else if media.TMDbID > 0 {
		key = fmt.Sprintf("movie:%d", media.TMDbID)
	}
	key = fmt.Sprintf("%x", sha256.Sum256([]byte(key)))
	// Episode number remains in the event identity; rankings group by season.
	eventKey := fmt.Sprintf("%x", sha256.Sum256([]byte(fmt.Sprintf("%s\x00%s\x00%s\x00%d", userID, sessionID, key, media.EpisodeNum))))
	event := model.PlaybackEvent{EventKey: eventKey, UserID: userID, MediaID: media.ID, LibraryID: media.LibraryID, PlayedAt: time.Now().UTC(), MediaType: kind, WorkKey: key, Title: media.Title, PosterURL: media.PosterURL, SeasonNum: media.SeasonNum, EpisodeNum: media.EpisodeNum, Client: client}
	if len(event.Client) > 128 {
		event.Client = event.Client[:128]
	}
	return c.Repo.DB.WithContext(ctx).Clauses(clause.OnConflict{Columns: []clause.Column{{Name: "event_key"}}, DoNothing: true}).Create(&event).Error
}
