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
	Group    string
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
	{Key: "adult_javdb_popular", Label: "JavDB 今日热门", Provider: "adult", Group: "adult"},
	{Key: "adult_fd2ppv", Label: "FC2 作品", Provider: "adult", Group: "adult"},
	{Key: "adult_followed_performers", Label: "关注女优", Provider: "adult", Group: "adult"},
	{Key: "adult_followed", Label: "关注女优新作", Provider: "adult", Group: "adult"},
	{Key: "adult_javdb_performers_new", Label: "JavDB 新人女优", Provider: "adult", Group: "adult"},
	{Key: "adult_javdb_performers_monthly", Label: "JavDB 月榜女优", Provider: "adult", Group: "adult"},
	{Key: "adult_javdb_performers_fanza", Label: "JavDB Fanza(DMM)推薦", Provider: "adult", Group: "adult"},
}

const discoverFeedSectionTimeout = 20 * time.Second
const discoverFeedBangumiTimeout = 30 * time.Second
const discoverFeedFD2PPVTimeout = 75 * time.Second
const discoverFeedSlowSectionThreshold = 2 * time.Second
const discoverWorkPageSize = 18

// discoverSectionsHandler returns the catalog of sections the UI can
// pick from. The names match the upstream Vue UI so existing settings
// keep working.
func discoverSectionsHandler(svc *service.Container) gin.HandlerFunc {
	return func(c *gin.Context) {
		sections := make([]gin.H, 0, len(discoverSectionCatalog))
		adultAllowed := discoverAdultAllowed(c, svc)
		for _, section := range enabledDiscoverSections(c.Request.Context(), svc) {
			if discoverSectionTemporarilyDisabled(section.Key) {
				continue
			}
			if section.Group == "adult" && !adultAllowed {
				continue
			}
			sections = append(sections, gin.H{
				"key": section.Key, "label": section.Label, "provider": section.Provider, "group": section.Group,
			})
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
		refresh := c.Query("refresh") == "true"
		fd2Sort, fd2SortOK := service.NormalizeFD2PPVDiscoverSort(c.Query("adult_fd2ppv_sort"))
		if !fd2SortOK {
			c.JSON(http.StatusBadRequest, gin.H{"error": "FC2 排序条件无效"})
			return
		}
		keys := strings.Split(rawSections, ",")
		out := gin.H{}
		meta := gin.H{}
		artworkItems := []service.ExternalMediaResult{}
		userID := currentUserID(c)
		adultAllowed := discoverAdultAllowed(c, svc)
		for _, raw := range keys {
			k := strings.TrimSpace(raw)
			if k == "" {
				continue
			}
			if discoverSectionTemporarilyDisabled(k) {
				out[k] = []service.ExternalMediaResult{}
				meta[k] = gin.H{"page": page, "has_next": false, "disabled": true}
				continue
			}
			if discoverSectionProvider(k) == "adult" && !adultAllowed {
				out[k] = []service.ExternalMediaResult{}
				meta[k] = gin.H{"page": page, "has_next": false, "disabled": true}
				continue
			}
			if provider := discoverSectionProvider(k); provider != "" && !discoverProviderEnabled(c.Request.Context(), svc, provider) {
				out[k] = []service.ExternalMediaResult{}
				meta[k] = gin.H{"page": page, "has_next": false, "disabled": true}
				continue
			}
			sectionCacheKey := k
			if k == "adult_fd2ppv" {
				sectionCacheKey += ":" + fd2Sort
			}
			cacheKey := discoverSectionCacheKeyForUser(sectionCacheKey, userID)
			metaEntry := gin.H{"page": page, "has_next": false, "duration_ms": int64(0)}
			items, cacheHit := cachedDiscoverSection(svc, cacheKey, page)
			lastGoodItems, lastGoodHit := items, cacheHit
			if !cacheHit && discoverAdultBackgroundSection(k) {
				lastGoodItems, lastGoodHit = cachedDiscoverSectionLastGood(svc, cacheKey, page)
			}
			if refresh && discoverAdultBackgroundSection(k) {
				if lastGoodHit {
					items = lastGoodItems
					metaEntry["cached"] = true
					if !cacheHit {
						metaEntry["stale"] = true
						metaEntry["warning"] = "成人源正在后台刷新，当前显示最近一次成功缓存"
					}
				} else {
					metaEntry["error"] = "成人源后台缓存尚未准备好，请稍后重试"
					items = nil
				}
			} else if !refresh && cacheHit {
				metaEntry["cached"] = true
			} else {
				sectionTimeout := discoverSectionTimeout(k)
				sectionCtx, cancel := context.WithTimeout(c.Request.Context(), sectionTimeout)
				started := time.Now()
				freshItems, err := discoverSectionItems(sectionCtx, svc, k, page, userID, fd2Sort)
				elapsed := time.Since(started)
				cancel()
				metaEntry["duration_ms"] = elapsed.Milliseconds()
				items = freshItems
				if err != nil {
					logDiscoverFetchFailed(svc, k, page, elapsed, sectionTimeout, err)
					if cacheHit {
						items, _ = cachedDiscoverSection(svc, cacheKey, page)
						metaEntry["stale"] = true
						metaEntry["warning"] = discoverFeedStaleMessage(err)
					} else if lastGoodHit && discoverAdultBackgroundSection(k) {
						items = lastGoodItems
						metaEntry["stale"] = true
						metaEntry["warning"] = discoverFeedStaleMessage(err)
					} else if fallbackItems, fallbackKey, ok := fallbackDiscoverSectionItems(c.Request.Context(), svc, k, page, userID); ok {
						items = fallbackItems
						metaEntry["fallback"] = fallbackKey
						metaEntry["warning"] = discoverFeedFallbackMessage(fallbackKey, err)
						rememberDiscoverSection(svc, cacheKey, page, items)
					} else {
						metaEntry["error"] = discoverFeedErrorMessage(err)
						items = nil
					}
				} else {
					logDiscoverFetchSlow(svc, k, page, elapsed, len(items))
					rememberDiscoverSection(svc, cacheKey, page, items)
				}
			}
			metaEntry["has_next"] = discoverSectionHasNext(k, len(items))
			visibleItems := discoverSectionVisibleItems(k, items)
			service.EnrichExternalMediaLibraryLinks(
				c.Request.Context(), svc.Repo, visibleItems, mediaVisibilityForRequest(c, svc),
			)
			warmItems := visibleItems
			if discoverSectionProvider(k) == "adult" && len(warmItems) > 12 {
				warmItems = warmItems[:12]
			}
			artworkItems = append(artworkItems, warmItems...)
			out[k] = visibleItems
			meta[k] = metaEntry
		}
		out["_meta"] = meta
		if svc != nil && svc.Discover != nil {
			svc.Discover.WarmExternalArtwork(artworkItems)
		}
		c.JSON(http.StatusOK, out)
	}
}

func discoverItemDetailHandler(svc *service.Container) gin.HandlerFunc {
	return func(c *gin.Context) {
		source := strings.ToLower(strings.TrimSpace(c.Param("source")))
		mediaType := strings.ToLower(strings.TrimSpace(c.Query("media_type")))
		if mediaType != "movie" && mediaType != "tv" {
			c.JSON(http.StatusBadRequest, gin.H{"error": "作品类型无效"})
			return
		}
		if svc == nil || svc.Discover == nil {
			c.JSON(http.StatusServiceUnavailable, gin.H{"error": "作品详情服务不可用"})
			return
		}

		ctx, cancel := context.WithTimeout(c.Request.Context(), discoverFeedSectionTimeout)
		defer cancel()
		var (
			item service.ExternalMediaResult
			err  error
		)
		switch source {
		case "tmdb":
			tmdbID, parseErr := strconv.Atoi(strings.TrimSpace(c.Param("provider_id")))
			if parseErr != nil || tmdbID <= 0 {
				c.JSON(http.StatusBadRequest, gin.H{"error": "TMDb ID 无效"})
				return
			}
			item, err = svc.Discover.TMDbItemDetail(ctx, mediaType, tmdbID)
		case "douban":
			doubanID := strings.TrimSpace(c.Param("provider_id"))
			if doubanID == "" || strings.Trim(doubanID, "0123456789") != "" {
				c.JSON(http.StatusBadRequest, gin.H{"error": "豆瓣 ID 无效"})
				return
			}
			item, err = svc.Discover.DoubanItemDetail(ctx, mediaType, doubanID)
		default:
			c.JSON(http.StatusBadRequest, gin.H{"error": "暂不支持该资料源的作品详情"})
			return
		}
		if err != nil {
			c.JSON(http.StatusBadGateway, gin.H{
				"error":  "作品详情暂时无法加载",
				"detail": err.Error(),
			})
			return
		}
		items := []service.ExternalMediaResult{item}
		service.EnrichExternalMediaLibraryLinks(
			c.Request.Context(), svc.Repo, items, mediaVisibilityForRequest(c, svc),
		)
		svc.Discover.WarmExternalArtwork(items)
		c.JSON(http.StatusOK, items[0])
	}
}

func cachedDiscoverSection(svc *service.Container, key string, page int) ([]service.ExternalMediaResult, bool) {
	if svc == nil || svc.Discover == nil {
		return nil, false
	}
	return svc.Discover.CachedSection(key, page)
}

func cachedDiscoverSectionLastGood(svc *service.Container, key string, page int) ([]service.ExternalMediaResult, bool) {
	if svc == nil || svc.Discover == nil {
		return nil, false
	}
	return svc.Discover.CachedSectionLastGood(key, page)
}

func discoverAdultBackgroundSection(key string) bool {
	return key == "adult_javdb_popular" || key == "adult_fd2ppv"
}

func discoverSectionTemporarilyDisabled(key string) bool {
	switch strings.TrimSpace(key) {
	case "adult_followed",
		"adult_javdb_performers_new",
		"adult_javdb_performers_monthly",
		"adult_javdb_performers_fanza":
		return true
	default:
		return false
	}
}

func rememberDiscoverSection(svc *service.Container, key string, page int, items []service.ExternalMediaResult) {
	if svc == nil || svc.Discover == nil {
		return
	}
	svc.Discover.RememberSection(key, page, items)
}

func fallbackDiscoverSectionItems(parent context.Context, svc *service.Container, key string, page int, userID string) ([]service.ExternalMediaResult, string, bool) {
	fallbackKey := fallbackDiscoverSectionKey(key)
	if fallbackKey == "" || svc == nil || svc.Discover == nil {
		return nil, "", false
	}
	ctx, cancel := context.WithTimeout(parent, discoverSectionTimeout(fallbackKey))
	defer cancel()
	items, err := discoverSectionItems(ctx, svc, fallbackKey, page, userID)
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
	if key == "adult_fd2ppv" {
		return discoverFeedFD2PPVTimeout
	}
	return discoverFeedSectionTimeout
}

func enabledDiscoverSections(ctx context.Context, svc *service.Container) []discoverSectionDef {
	sections := make([]discoverSectionDef, 0, len(discoverSectionCatalog))
	for _, section := range discoverSectionCatalog {
		if !discoverProviderEnabled(ctx, svc, section.Provider) {
			continue
		}
		if section.Key == "adult_fd2ppv" && (svc == nil || svc.Adult == nil || !svc.Adult.FD2PPVEnabled()) {
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
	case "adult_javdb_popular", "adult_javdb_performers", "adult_followed_performers", "adult_followed",
		"adult_javdb_performers_new", "adult_javdb_performers_monthly", "adult_javdb_performers_fanza", "adult_fd2ppv":
		return "adult"
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

func discoverSectionItems(ctx context.Context, svc *service.Container, k string, page int, userID string, fd2Sort ...string) ([]service.ExternalMediaResult, error) {
	switch k {
	case "tmdb_trending_day", "tmdb_trending_week", "tmdb_latest_movie", "tmdb_latest_tv", "tmdb_popular_movie", "tmdb_popular_tv", "tmdb_top_rated_movie", "tmdb_upcoming_movie",
		"trending_day", "trending_week", "latest_movie", "latest_tv", "popular_movie", "popular_tv", "top_rated_movie", "upcoming_movie":
		return svc.Discover.TMDbSectionWindow(ctx, k, page, discoverWorkPageSize)
	case "douban_hot_movie", "douban_hot_tv", "douban_top_movie":
		if svc.Douban == nil {
			return []service.ExternalMediaResult{}, nil
		}
		return svc.Douban.DiscoverWindow(ctx, k, page, discoverWorkPageSize)
	case "bangumi_calendar":
		if svc.Bangumi == nil {
			return []service.ExternalMediaResult{}, nil
		}
		items, err := svc.Bangumi.Calendar(ctx)
		if err == nil {
			rememberDiscoverStaticWindows(svc, k, items)
		}
		return discoverSectionWindow(items, page), err
	case "adult_javdb_popular":
		if svc.Adult == nil {
			return []service.ExternalMediaResult{}, nil
		}
		items, err := svc.Adult.DiscoverJavDBPopular(ctx)
		if err == nil {
			rememberDiscoverStaticWindows(svc, k, items)
		}
		return discoverSectionWindow(items, page), err
	case "adult_fd2ppv":
		if svc.Adult == nil {
			return []service.ExternalMediaResult{}, nil
		}
		sortKey := "release"
		if len(fd2Sort) > 0 {
			sortKey = fd2Sort[0]
		}
		return svc.Adult.DiscoverFD2PPVWindow(ctx, sortKey, page, discoverWorkPageSize)
	case "adult_javdb_performers", "adult_javdb_performers_monthly":
		if svc.Adult == nil || page > 1 {
			return []service.ExternalMediaResult{}, nil
		}
		items, err := svc.Adult.DiscoverJavDBPerformerSection(ctx, "monthly")
		if err != nil {
			return nil, err
		}
		if err := markAdultPerformerFollows(ctx, svc, userID, items); err != nil {
			return nil, err
		}
		return items, nil
	case "adult_javdb_performers_new", "adult_javdb_performers_fanza":
		if svc.Adult == nil || page > 1 {
			return []service.ExternalMediaResult{}, nil
		}
		section := "new"
		if k == "adult_javdb_performers_fanza" {
			section = "fanza"
		}
		items, err := svc.Adult.DiscoverJavDBPerformerSection(ctx, section)
		if err != nil {
			return nil, err
		}
		if err := markAdultPerformerFollows(ctx, svc, userID, items); err != nil {
			return nil, err
		}
		return items, nil
	case "adult_followed_performers":
		if svc.Repo == nil || svc.Repo.AdultFollow == nil || page > 1 {
			return []service.ExternalMediaResult{}, nil
		}
		follows, err := svc.Repo.AdultFollow.ListByUser(ctx, userID)
		if err != nil {
			return nil, err
		}
		return service.FollowedAdultPerformerItems(follows), nil
	case "adult_followed":
		if svc.Adult == nil || svc.Repo == nil || svc.Repo.AdultFollow == nil {
			return []service.ExternalMediaResult{}, nil
		}
		follows, err := svc.Repo.AdultFollow.ListByUser(ctx, userID)
		if err != nil {
			return nil, err
		}
		return svc.Adult.DiscoverFollowedPerformerWorksWindow(ctx, follows, page, discoverWorkPageSize)
	default:
		return []service.ExternalMediaResult{}, nil
	}
}

func discoverSectionHasNext(key string, itemCount int) bool {
	return discoverSectionUsesWorkPaging(key) && itemCount > discoverWorkPageSize
}

func discoverSectionVisibleItems(key string, items []service.ExternalMediaResult) []service.ExternalMediaResult {
	if !discoverSectionUsesWorkPaging(key) || len(items) <= discoverWorkPageSize {
		return items
	}
	return items[:discoverWorkPageSize]
}

func discoverSectionWindow(items []service.ExternalMediaResult, page int) []service.ExternalMediaResult {
	if page < 1 {
		page = 1
	}
	start := (page - 1) * discoverWorkPageSize
	if start >= len(items) {
		return []service.ExternalMediaResult{}
	}
	end := start + discoverWorkPageSize + 1
	if end > len(items) {
		end = len(items)
	}
	return items[start:end]
}

func rememberDiscoverStaticWindows(svc *service.Container, key string, items []service.ExternalMediaResult) {
	for page := 1; ; page++ {
		window := discoverSectionWindow(items, page)
		if len(window) == 0 {
			return
		}
		rememberDiscoverSection(svc, key, page, window)
		if len(window) <= discoverWorkPageSize {
			return
		}
	}
}

func discoverSectionUsesWorkPaging(key string) bool {
	switch key {
	case "tmdb_trending_day", "tmdb_trending_week", "tmdb_latest_movie", "tmdb_latest_tv", "tmdb_popular_movie", "tmdb_popular_tv", "tmdb_top_rated_movie", "tmdb_upcoming_movie",
		"trending_day", "trending_week", "latest_movie", "latest_tv", "popular_movie", "popular_tv", "top_rated_movie", "upcoming_movie",
		"douban_hot_movie", "douban_hot_tv", "douban_top_movie", "bangumi_calendar", "adult_javdb_popular", "adult_fd2ppv", "adult_followed":
		return true
	default:
		return false
	}
}

func discoverSectionCacheKeyForUser(key, userID string) string {
	switch key {
	case "adult_javdb_performers", "adult_followed_performers", "adult_followed",
		"adult_javdb_performers_new", "adult_javdb_performers_monthly", "adult_javdb_performers_fanza":
		return key + ":" + strings.TrimSpace(userID)
	default:
		return key
	}
}

func markAdultPerformerFollows(ctx context.Context, svc *service.Container, userID string, items []service.ExternalMediaResult) error {
	if svc == nil || svc.Repo == nil || svc.Repo.AdultFollow == nil || strings.TrimSpace(userID) == "" {
		return nil
	}
	follows, err := svc.Repo.AdultFollow.ListByUser(ctx, userID)
	if err != nil {
		return err
	}
	followed := make(map[string]struct{}, len(follows))
	for _, follow := range follows {
		key := strings.ToLower(strings.TrimSpace(follow.Source)) + "\x00" + strings.TrimSpace(follow.SourceID)
		followed[key] = struct{}{}
	}
	for index := range items {
		key := strings.ToLower(strings.TrimSpace(items[index].Source)) + "\x00" + strings.TrimSpace(items[index].ProviderID)
		_, items[index].Followed = followed[key]
	}
	return nil
}

func discoverAdultAllowed(c *gin.Context, svc *service.Container) bool {
	if c == nil || svc == nil || svc.Repo == nil || svc.Repo.Library == nil {
		return false
	}
	visibility := mediaVisibilityForRequest(c, svc)
	if !visibility.IncludeNSFW {
		return false
	}
	libraries, err := svc.Repo.Library.List(c.Request.Context())
	if err != nil {
		return false
	}
	for _, library := range libraries {
		if !service.LibraryIsAdult(library) {
			continue
		}
		if service.LibraryVisibleForUser(c.Request.Context(), svc.Repo, library, visibility) {
			return true
		}
	}
	return false
}
