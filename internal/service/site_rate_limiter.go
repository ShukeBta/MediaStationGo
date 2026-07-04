package service

import (
	"context"
	"crypto/sha1"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"sort"
	"strings"
	"sync"
	"time"

	"github.com/ShukeBta/MediaStationGo/internal/repository"
)

const (
	mteamAPIEndpointSearch   = "torrent_search"
	mteamAPIEndpointDetail   = "torrent_detail"
	mteamAPIEndpointDownload = "torrent_download"

	mteamAPISearchDailyLimit = 1500
)

type siteAPIRateLimit struct {
	Bucket string
	Limit  int
	Window time.Duration
}

type siteAPIRateLimiter interface {
	Allow(ctx context.Context, siteKey string, limits ...siteAPIRateLimit) error
}

type siteAPIRateLimitError struct {
	SiteKey    string
	Bucket     string
	Limit      int
	Window     time.Duration
	RetryAfter time.Duration
}

func (e *siteAPIRateLimitError) Error() string {
	if e == nil {
		return ""
	}
	return fmt.Sprintf("M-Team API rate limit reached for %s: %d requests per %s, retry after %s",
		e.Bucket, e.Limit, formatRateLimitDuration(e.Window), formatRateLimitDuration(e.RetryAfter))
}

type persistentSiteAPIRateLimiter struct {
	repo     *repository.Container
	fallback *memorySiteAPIRateLimiter
	now      func() time.Time
	mu       sync.Mutex
}

func newPersistentSiteAPIRateLimiter(repo *repository.Container) *persistentSiteAPIRateLimiter {
	return &persistentSiteAPIRateLimiter{
		repo:     repo,
		fallback: newMemorySiteAPIRateLimiter(time.Now),
		now:      time.Now,
	}
}

func (l *persistentSiteAPIRateLimiter) Allow(ctx context.Context, siteKey string, limits ...siteAPIRateLimit) error {
	if err := ctx.Err(); err != nil {
		return err
	}
	if l == nil || l.repo == nil || l.repo.Setting == nil {
		if l != nil && l.fallback != nil {
			return l.fallback.Allow(ctx, siteKey, limits...)
		}
		return defaultMemorySiteAPIRateLimiter.Allow(ctx, siteKey, limits...)
	}
	l.mu.Lock()
	defer l.mu.Unlock()

	now := time.Now()
	if l.now != nil {
		now = l.now()
	}
	records := make([]siteAPIRateLimitRecord, 0, len(limits))
	for _, limit := range normalizeSiteAPIRateLimits(limits) {
		key := siteAPIRateLimitSettingKey(siteKey, limit.Bucket)
		raw, err := l.repo.Setting.Get(ctx, key)
		if err != nil {
			return err
		}
		timestamps := pruneSiteAPIRateTimestamps(parseSiteAPIRateTimestamps(raw), now, limit.Window)
		if err := checkSiteAPIRateLimit(siteKey, limit, timestamps, now); err != nil {
			return err
		}
		records = append(records, siteAPIRateLimitRecord{key: key, timestamps: timestamps})
	}
	nowUnix := now.Unix()
	for _, record := range records {
		next := append(record.timestamps, nowUnix)
		if err := l.repo.Setting.Set(ctx, record.key, encodeSiteAPIRateTimestamps(next)); err != nil {
			return err
		}
	}
	return nil
}

type memorySiteAPIRateLimiter struct {
	now     func() time.Time
	mu      sync.Mutex
	buckets map[string][]int64
}

var defaultMemorySiteAPIRateLimiter = newMemorySiteAPIRateLimiter(time.Now)

func newMemorySiteAPIRateLimiter(now func() time.Time) *memorySiteAPIRateLimiter {
	if now == nil {
		now = time.Now
	}
	return &memorySiteAPIRateLimiter{now: now, buckets: map[string][]int64{}}
}

func (l *memorySiteAPIRateLimiter) Allow(ctx context.Context, siteKey string, limits ...siteAPIRateLimit) error {
	if err := ctx.Err(); err != nil {
		return err
	}
	if l == nil {
		return nil
	}
	l.mu.Lock()
	defer l.mu.Unlock()

	now := l.now()
	records := make([]siteAPIRateLimitRecord, 0, len(limits))
	for _, limit := range normalizeSiteAPIRateLimits(limits) {
		key := siteAPIRateLimitSettingKey(siteKey, limit.Bucket)
		timestamps := pruneSiteAPIRateTimestamps(l.buckets[key], now, limit.Window)
		if err := checkSiteAPIRateLimit(siteKey, limit, timestamps, now); err != nil {
			return err
		}
		records = append(records, siteAPIRateLimitRecord{key: key, timestamps: timestamps})
	}
	nowUnix := now.Unix()
	for _, record := range records {
		l.buckets[record.key] = append(record.timestamps, nowUnix)
	}
	return nil
}

type siteAPIRateLimitRecord struct {
	key        string
	timestamps []int64
}

func reserveMTeamAPIQuota(ctx context.Context, cfg SiteConfig, endpoint string) error {
	limits := mteamAPIRateLimits(endpoint)
	if len(limits) == 0 {
		return nil
	}
	// M-Team's published API quotas are upstream hard limits, so protect them
	// regardless of the generic per-site RateLimit toggle.
	limiter := cfg.rateLimiter
	if limiter == nil {
		limiter = defaultMemorySiteAPIRateLimiter
	}
	return limiter.Allow(ctx, mteamAPIRateSiteKey(cfg), limits...)
}

func mteamAPIRateLimits(endpoint string) []siteAPIRateLimit {
	switch endpoint {
	case mteamAPIEndpointSearch:
		return []siteAPIRateLimit{{Bucket: "torrent_search_24h", Limit: mteamAPISearchDailyLimit, Window: 24 * time.Hour}}
	case mteamAPIEndpointDetail:
		return []siteAPIRateLimit{{Bucket: "torrent_detail_1h", Limit: 100, Window: time.Hour}}
	case mteamAPIEndpointDownload:
		return []siteAPIRateLimit{
			{Bucket: "torrent_download_1h", Limit: 100, Window: time.Hour},
			{Bucket: "torrent_download_24h", Limit: 1000, Window: 24 * time.Hour},
		}
	default:
		return nil
	}
}

func mteamAPIRateSiteKey(cfg SiteConfig) string {
	base := strings.TrimRight(strings.ToLower(strings.TrimSpace(cfg.URL)), "/")
	if base == "" {
		base = "mteam"
	}
	if apiKey := strings.TrimSpace(cfg.APIKey); apiKey != "" {
		sum := sha1.Sum([]byte(apiKey))
		return base + "|api:" + hex.EncodeToString(sum[:])
	}
	if siteID := strings.TrimSpace(cfg.SiteID); siteID != "" {
		return base + "|site:" + siteID
	}
	if name := strings.TrimSpace(cfg.Name); name != "" {
		return base + "|name:" + strings.ToLower(name)
	}
	return base
}

func normalizeSiteAPIRateLimits(limits []siteAPIRateLimit) []siteAPIRateLimit {
	out := make([]siteAPIRateLimit, 0, len(limits))
	for _, limit := range limits {
		limit.Bucket = strings.TrimSpace(limit.Bucket)
		if limit.Bucket == "" || limit.Limit <= 0 || limit.Window <= 0 {
			continue
		}
		out = append(out, limit)
	}
	return out
}

func checkSiteAPIRateLimit(siteKey string, limit siteAPIRateLimit, timestamps []int64, now time.Time) error {
	if len(timestamps) < limit.Limit {
		return nil
	}
	oldest := time.Unix(timestamps[0], 0)
	retryAfter := oldest.Add(limit.Window).Sub(now)
	if retryAfter < time.Second {
		retryAfter = time.Second
	}
	return &siteAPIRateLimitError{
		SiteKey:    siteKey,
		Bucket:     limit.Bucket,
		Limit:      limit.Limit,
		Window:     limit.Window,
		RetryAfter: retryAfter,
	}
}

func siteAPIRateLimitSettingKey(siteKey, bucket string) string {
	sum := sha1.Sum([]byte(siteKey))
	return "site.api_rate." + hex.EncodeToString(sum[:])[:20] + "." + bucket
}

func parseSiteAPIRateTimestamps(raw string) []int64 {
	raw = strings.TrimSpace(raw)
	if raw == "" {
		return nil
	}
	var out []int64
	if err := json.Unmarshal([]byte(raw), &out); err != nil {
		return nil
	}
	return out
}

func encodeSiteAPIRateTimestamps(values []int64) string {
	data, _ := json.Marshal(values)
	return string(data)
}

func pruneSiteAPIRateTimestamps(values []int64, now time.Time, window time.Duration) []int64 {
	if len(values) == 0 {
		return nil
	}
	cutoff := now.Add(-window).Unix()
	out := make([]int64, 0, len(values))
	for _, value := range values {
		if value > cutoff && value <= now.Add(time.Minute).Unix() {
			out = append(out, value)
		}
	}
	sort.Slice(out, func(i, j int) bool { return out[i] < out[j] })
	return out
}

func formatRateLimitDuration(value time.Duration) string {
	if value < 0 {
		value = 0
	}
	value = value.Round(time.Second)
	if value%time.Hour == 0 && value >= time.Hour {
		return fmt.Sprintf("%dh", int(value/time.Hour))
	}
	if value%time.Minute == 0 && value >= time.Minute {
		return fmt.Sprintf("%dm", int(value/time.Minute))
	}
	return value.String()
}
