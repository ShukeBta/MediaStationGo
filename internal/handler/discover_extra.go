// Package handler — multi-section discover endpoints.
//
// The Vue DiscoverView paginates a configurable list of "sections"
// (trending day/week, popular movies, top rated, etc.) and asks the
// backend for a feed keyed by section name. We mirror that surface so
// the React DiscoverPage can render the same rails without a rewrite.
package handler

import (
	"context"
	"errors"
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/gin-gonic/gin"
	"go.uber.org/zap"

	"github.com/ShukeBta/MediaStationGo/internal/service"
)

type discoverSectionDef struct {
	Key      string
	Label    string
	Provider string
}

var discoverSectionCatalog = []discoverSectionDef{
	{Key: "tmdb_trending_day", Label: "TMDb 今日趋势", Provider: "tmdb"},
	{Key: "tmdb_trending_week", Label: "TMDb 本周热门", Provider: "tmdb"},
	{Key: "tmdb_latest_movie", Label: "TMDb 最新电影", Provider: "tmdb"},
	{Key: "tmdb_latest_tv", Label: "TMDb 最新剧集", Provider: "tmdb"},
	{Key: "tmdb_popular_movie", Label: "TMDb 热门电影", Provider: "tmdb"},
	{Key: "tmdb_popular_tv", Label: "TMDb 热门剧集", Provider: "tmdb"},
	{Key: "tmdb_top_rated_movie", Label: "TMDb 高分电影", Provider: "tmdb"},
	{Key: "tmdb_upcoming_movie", Label: "TMDb 即将上映", Provider: "tmdb"},
	{Key: "douban_hot_movie", Label: "豆瓣热门电影", Provider: "douban"},
	{Key: "douban_hot_tv", Label: "豆瓣热门剧集", Provider: "douban"},
	{Key: "douban_top_movie", Label: "豆瓣高分电影", Provider: "douban"},
	{Key: "bangumi_calendar", Label: "Bangumi 每日放送", Provider: "bangumi"},
}

const discoverFeedSectionTimeout = 20 * time.Second
const discoverFeedBangumiTimeout = 30 * time.Second
const discoverFeedSlowSectionThreshold = 2 * time.Second

// discoverSectionsHandler returns the catalog of sections the UI can
// pick from. The names match the upstream Vue UI so existing settings
// keep working.
func discoverSectionsHandler(svc *service.Container) gin.HandlerFunc {
	return func(c *gin.Context) {
		sections := make([]gin.H, 0, len(discoverSectionCatalog))
		for _, section := range enabledDiscoverSections(c.Request.Context(), svc) {
			sections = append(sections, gin.H{"key": section.Key, "label": section.Label, "provider": section.Provider})
		}
		c.JSON(http.StatusOK, gin.H{"sections": sections})
	}
}

// discoverFeedHandler resolves one or more section keys (?sections=a,b)
// to TMDb / Douban / Bangumi rails and returns the joined results keyed by
// section name. Unknown keys are silently dropped so URL typos don't break
// the page.
func discoverFeedHandler(svc *service.Container) gin.HandlerFunc {
	return func(c *gin.Context) {
		rawSections := c.Query("sections")
		if strings.TrimSpace(rawSections) == "" {
			rawSections = strings.Join(defaultDiscoverSectionKeys(c.Request.Context(), svc), ",")
		}
		page, _ := strconv.Atoi(c.DefaultQuery("page", "1"))
		if page < 1 {
			page = 1
		}
		keys := strings.Split(rawSections, ",")
		out := gin.H{}
		meta := gin.H{}
		artworkItems := []service.ExternalMediaResult{}
		for _, raw := range keys {
			k := strings.TrimSpace(raw)
			if k == "" {
				continue
			}
			if provider := discoverSectionProvider(k); provider != "" && !discoverProviderEnabled(c.Request.Context(), svc, provider) {
				out[k] = []service.ExternalMediaResult{}
				meta[k] = gin.H{"page": page, "has_next": false, "disabled": true}
				continue
			}
			sectionTimeout := discoverSectionTimeout(k)
			sectionCtx, cancel := context.WithTimeout(c.Request.Context(), sectionTimeout)
			started := time.Now()
			items, err := discoverSectionItems(sectionCtx, svc, k, page)
			elapsed := time.Since(started)
			cancel()
			metaEntry := gin.H{"page": page, "has_next": false, "duration_ms": elapsed.Milliseconds()}
			if err != nil {
				logDiscoverFetchFailed(svc, k, page, elapsed, sectionTimeout, err)
				if cached, ok := cachedDiscoverSection(svc, k, page); ok {
					items = cached
					metaEntry["stale"] = true
					metaEntry["warning"] = discoverFeedStaleMessage(err)
				} else if fallbackItems, fallbackKey, ok := fallbackDiscoverSectionItems(c.Request.Context(), svc, k, page); ok {
					items = fallbackItems
					metaEntry["fallback"] = fallbackKey
					metaEntry["warning"] = discoverFeedFallbackMessage(fallbackKey, err)
					rememberDiscoverSection(svc, k, page, items)
				} else {
					metaEntry["error"] = discoverFeedErrorMessage(err)
					items = nil
				}
			} else {
				logDiscoverFetchSlow(svc, k, page, elapsed, len(items))
				rememberDiscoverSection(svc, k, page, items)
			}
			artworkItems = append(artworkItems, items...)
			out[k] = items
			metaEntry["has_next"] = discoverSectionHasNext(k, len(items))
			meta[k] = metaEntry
		}
		out["_meta"] = meta
		if svc != nil && svc.Discover != nil {
			svc.Discover.WarmExternalArtwork(artworkItems)
		}
		c.JSON(http.StatusOK, out)
	}
}

func cachedDiscoverSection(svc *service.Container, key string, page int) ([]service.ExternalMediaResult, bool) {
	if svc == nil || svc.Discover == nil {
		return nil, false
	}
	return svc.Discover.CachedSection(key, page)
}

func rememberDiscoverSection(svc *service.Container, key string, page int, items []service.ExternalMediaResult) {
	if svc == nil || svc.Discover == nil {
		return
	}
	svc.Discover.RememberSection(key, page, items)
}

func fallbackDiscoverSectionItems(parent context.Context, svc *service.Container, key string, page int) ([]service.ExternalMediaResult, string, bool) {
	fallbackKey := fallbackDiscoverSectionKey(key)
	if fallbackKey == "" || svc == nil || svc.Discover == nil {
		return nil, "", false
	}
	ctx, cancel := context.WithTimeout(parent, discoverSectionTimeout(fallbackKey))
	defer cancel()
	items, err := discoverSectionItems(ctx, svc, fallbackKey, page)
	if err != nil || len(items) == 0 {
		return nil, fallbackKey, false
	}
	if svc.Log != nil {
		svc.Log.Info("discover section fallback used",
			zap.String("section", key),
			zap.String("fallback_section", fallbackKey),
			zap.Int("page", page),
			zap.Int("items", len(items)))
	}
	return items, fallbackKey, true
}

func fallbackDiscoverSectionKey(key string) string {
	switch key {
	case "douban_hot_movie":
		return "tmdb_popular_movie"
	case "douban_hot_tv":
		return "tmdb_popular_tv"
	case "douban_top_movie":
		return "tmdb_top_rated_movie"
	default:
		return ""
	}
}

func logDiscoverFetchFailed(svc *service.Container, key string, page int, elapsed, timeout time.Duration, err error) {
	if svc == nil || svc.Log == nil || err == nil {
		return
	}
	svc.Log.Warn("discover section fetch failed",
		zap.String("section", key),
		zap.String("provider", discoverSectionProvider(key)),
		zap.Int("page", page),
		zap.Duration("duration", elapsed),
		zap.Int64("duration_ms", elapsed.Milliseconds()),
		zap.Duration("timeout", timeout),
		zap.Error(err))
}

func logDiscoverFetchSlow(svc *service.Container, key string, page int, elapsed time.Duration, itemCount int) {
	if svc == nil || svc.Log == nil || elapsed < discoverFeedSlowSectionThreshold {
		return
	}
	svc.Log.Info("discover section fetch slow",
		zap.String("section", key),
		zap.String("provider", discoverSectionProvider(key)),
		zap.Int("page", page),
		zap.Int("items", itemCount),
		zap.Duration("duration", elapsed),
		zap.Int64("duration_ms", elapsed.Milliseconds()),
		zap.Duration("slow_threshold", discoverFeedSlowSectionThreshold))
}

func discoverFeedErrorMessage(err error) string {
	if err == nil {
		return ""
	}
	if errors.Is(err, context.DeadlineExceeded) {
		return "推荐源响应超时，已跳过本次加载"
	}
	var timeout interface{ Timeout() bool }
	if errors.As(err, &timeout) && timeout.Timeout() {
		return "推荐源响应超时，已跳过本次加载"
	}
	msg := strings.ToLower(err.Error())
	if strings.Contains(msg, "timeout") || strings.Contains(msg, "deadline exceeded") || strings.Contains(msg, "context deadline exceeded") {
		return "推荐源响应超时，已跳过本次加载"
	}
	return "推荐源暂时不可用，已跳过本次加载"
}

func discoverFeedStaleMessage(err error) string {
	if discoverFeedErrorMessage(err) == "推荐源响应超时，已跳过本次加载" {
		return "推荐源响应超时，已显示上次成功结果"
	}
	return "推荐源暂时不可用，已显示上次成功结果"
}

func discoverFeedFallbackMessage(fallbackKey string, err error) string {
	if strings.TrimSpace(fallbackKey) == "" {
		return discoverFeedErrorMessage(err)
	}
	return "推荐源暂时不可用，已显示同类备用榜单"
}

func discoverSectionTimeout(key string) time.Duration {
	if key == "bangumi_calendar" {
		return discoverFeedBangumiTimeout
	}
	return discoverFeedSectionTimeout
}

func enabledDiscoverSections(ctx context.Context, svc *service.Container) []discoverSectionDef {
	sections := make([]discoverSectionDef, 0, len(discoverSectionCatalog))
	for _, section := range discoverSectionCatalog {
		if !discoverProviderEnabled(ctx, svc, section.Provider) {
			continue
		}
		sections = append(sections, section)
	}
	return sections
}

func defaultDiscoverSectionKeys(ctx context.Context, svc *service.Container) []string {
	preferred := []string{"tmdb_trending_day", "tmdb_latest_movie", "tmdb_latest_tv", "douban_hot_movie", "douban_hot_tv", "bangumi_calendar"}
	enabled := map[string]struct{}{}
	for _, section := range enabledDiscoverSections(ctx, svc) {
		enabled[section.Key] = struct{}{}
	}
	out := make([]string, 0, len(preferred))
	for _, key := range preferred {
		if _, ok := enabled[key]; ok {
			out = append(out, key)
		}
	}
	if len(out) > 0 {
		return out
	}
	for _, section := range enabledDiscoverSections(ctx, svc) {
		out = append(out, section.Key)
		if len(out) >= 4 {
			break
		}
	}
	return out
}

func discoverSectionProvider(key string) string {
	for _, section := range discoverSectionCatalog {
		if section.Key == key {
			return section.Provider
		}
	}
	switch key {
	case "trending_day", "trending_week", "latest_movie", "latest_tv", "popular_movie", "popular_tv", "top_rated_movie", "upcoming_movie":
		return "tmdb"
	default:
		return ""
	}
}

func discoverProviderEnabled(ctx context.Context, svc *service.Container, provider string) bool {
	if svc == nil || svc.APIConfig == nil || strings.TrimSpace(provider) == "" {
		return true
	}
	cfg, err := svc.APIConfig.Get(ctx, provider)
	if err != nil || cfg == nil {
		return true
	}
	return cfg.Enabled
}

func discoverSectionItems(ctx context.Context, svc *service.Container, k string, page int) ([]service.ExternalMediaResult, error) {
	switch k {
	case "tmdb_trending_day", "tmdb_trending_week", "tmdb_latest_movie", "tmdb_latest_tv", "tmdb_popular_movie", "tmdb_popular_tv", "tmdb_top_rated_movie", "tmdb_upcoming_movie",
		"trending_day", "trending_week", "latest_movie", "latest_tv", "popular_movie", "popular_tv", "top_rated_movie", "upcoming_movie":
		return svc.Discover.TMDbSection(ctx, k, page)
	case "douban_hot_movie", "douban_hot_tv", "douban_top_movie":
		if svc.Douban == nil {
			return []service.ExternalMediaResult{}, nil
		}
		return svc.Douban.Discover(ctx, k, page)
	case "bangumi_calendar":
		if svc.Bangumi == nil {
			return []service.ExternalMediaResult{}, nil
		}
		if page > 1 {
			return []service.ExternalMediaResult{}, nil
		}
		return svc.Bangumi.Calendar(ctx)
	default:
		return []service.ExternalMediaResult{}, nil
	}
}

func discoverSectionHasNext(key string, itemCount int) bool {
	if itemCount <= 0 {
		return false
	}
	switch discoverSectionProvider(key) {
	case "tmdb":
		return itemCount >= 20
	case "douban":
		return itemCount >= 24
	default:
		return false
	}
}
