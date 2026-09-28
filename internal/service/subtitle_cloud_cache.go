package service

import (
	"context"
	"fmt"
	"os"
	"strconv"
	"strings"
	"sync"
	"time"

	"go.uber.org/zap"

	"github.com/ShukeBta/MediaStationGo/internal/model"
)

const (
	defaultCloudSubtitleDiscoveryTTL = 30 * time.Hour
	maxCloudSubtitleDiscoveryHours   = 24 * 365
)

type cloudSubtitleLoader func(context.Context, *SubtitleService, model.Media) ([]SubtitleTrack, error)

type cloudSubtitleDiscoveryCache struct {
	mu         sync.Mutex
	ttl        time.Duration
	generation uint64
	entries    map[string]cloudSubtitleDiscoveryEntry
	flights    map[string]*cloudSubtitleDiscoveryFlight
	load       cloudSubtitleLoader
}

type cloudSubtitleDiscoveryEntry struct {
	provider    string
	fingerprint string
	tracks      []SubtitleTrack
	expiresAt   time.Time
}

type cloudSubtitleDiscoveryFlight struct {
	provider   string
	generation uint64
	done       chan struct{}
	tracks     []SubtitleTrack
	err        error
}

func newCloudSubtitleDiscoveryCache(ttl time.Duration) *cloudSubtitleDiscoveryCache {
	return &cloudSubtitleDiscoveryCache{
		ttl:     ttl,
		entries: make(map[string]cloudSubtitleDiscoveryEntry),
		flights: make(map[string]*cloudSubtitleDiscoveryFlight),
		load:    loadCloudSubtitles,
	}
}

func cloudSubtitleDiscoveryTTLFromEnv(log *zap.Logger) time.Duration {
	raw := strings.TrimSpace(os.Getenv("MEDIASTATION_SUBTITLE_CLOUD_CACHE_TTL_HOURS"))
	if raw == "" {
		return defaultCloudSubtitleDiscoveryTTL
	}
	hours, err := strconv.Atoi(raw)
	if err != nil || hours < 0 || hours > maxCloudSubtitleDiscoveryHours {
		if log != nil {
			log.Warn("invalid cloud subtitle cache ttl; using default",
				zap.String("value", raw),
				zap.Duration("default", defaultCloudSubtitleDiscoveryTTL))
		}
		return defaultCloudSubtitleDiscoveryTTL
	}
	return time.Duration(hours) * time.Hour
}

func (s *SubtitleService) discoverCloudSubtitlesCached(ctx context.Context, media model.Media) ([]SubtitleTrack, error) {
	if s == nil {
		return nil, fmt.Errorf("subtitle service unavailable")
	}
	provider, _, ok := cloudSubtitleMediaRef(media)
	if !ok {
		return []SubtitleTrack{}, nil
	}
	if s.cloudCache == nil {
		s.cloudCache = newCloudSubtitleDiscoveryCache(defaultCloudSubtitleDiscoveryTTL)
	}
	c := s.cloudCache
	if c.ttl <= 0 {
		return c.load(ctx, s, media)
	}
	key := strings.TrimSpace(media.ID)
	fingerprint := cloudSubtitleDiscoveryFingerprint(media)
	if key == "" {
		key = fingerprint
	}
	now := time.Now()

	c.mu.Lock()
	if entry, exists := c.entries[key]; exists && entry.provider == provider && entry.fingerprint == fingerprint && entry.expiresAt.After(now) {
		tracks := cloneSubtitleTracks(entry.tracks)
		c.mu.Unlock()
		return tracks, nil
	}
	if flight, exists := c.flights[key]; exists && flight.provider == provider {
		done := flight.done
		c.mu.Unlock()
		select {
		case <-done:
			return cloneSubtitleTracks(flight.tracks), flight.err
		case <-ctx.Done():
			return nil, ctx.Err()
		}
	}
	flight := &cloudSubtitleDiscoveryFlight{
		provider:   provider,
		generation: c.generation,
		done:       make(chan struct{}),
	}
	c.flights[key] = flight
	loader := c.load
	c.mu.Unlock()

	tracks, err := loader(ctx, s, media)
	tracks = cloneSubtitleTracks(tracks)

	c.mu.Lock()
	if err == nil && flight.generation == c.generation && c.flights[key] == flight {
		c.entries[key] = cloudSubtitleDiscoveryEntry{
			provider:    provider,
			fingerprint: fingerprint,
			tracks:      cloneSubtitleTracks(tracks),
			expiresAt:   time.Now().Add(c.ttl),
		}
	}
	flight.tracks = cloneSubtitleTracks(tracks)
	flight.err = err
	close(flight.done)
	if c.flights[key] == flight {
		delete(c.flights, key)
	}
	c.mu.Unlock()
	return tracks, err
}

func (s *SubtitleService) InvalidateCloudDiscovery(mediaID, provider string) int {
	if s == nil || s.cloudCache == nil {
		return 0
	}
	mediaID = strings.TrimSpace(mediaID)
	provider = strings.TrimSpace(provider)
	c := s.cloudCache
	c.mu.Lock()
	defer c.mu.Unlock()
	c.generation++
	invalidated := 0
	for key, entry := range c.entries {
		if mediaID != "" && key != mediaID {
			continue
		}
		if provider != "" && entry.provider != provider {
			continue
		}
		delete(c.entries, key)
		invalidated++
	}
	for key, flight := range c.flights {
		if mediaID != "" && key != mediaID {
			continue
		}
		if provider != "" && flight.provider != provider {
			continue
		}
		delete(c.flights, key)
	}
	return invalidated
}

func cloudSubtitleDiscoveryFingerprint(media model.Media) string {
	return strings.Join([]string{
		strings.TrimSpace(media.Path),
		strings.TrimSpace(media.STRMURL),
		media.UpdatedAt.UTC().Format(time.RFC3339Nano),
	}, "\x00")
}

func cloneSubtitleTracks(tracks []SubtitleTrack) []SubtitleTrack {
	if len(tracks) == 0 {
		return []SubtitleTrack{}
	}
	return append([]SubtitleTrack(nil), tracks...)
}
