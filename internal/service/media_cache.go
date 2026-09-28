package service

import (
	"context"
	"crypto/sha1"
	"encoding/hex"
	"fmt"
	"sort"
	"strconv"
	"strings"
	"time"

	"github.com/ShukeBta/MediaStationGo/internal/model"
	"github.com/ShukeBta/MediaStationGo/internal/repository"
)

type mediaListCacheValue struct {
	Items []model.Media `json:"items"`
	Total int64         `json:"total"`
}

func (s *MediaService) mediaListCacheKey(libraryID string, libraryIDs []string, page, pageSize int, filter repository.MediaQueryFilter) string {
	allowed := append([]string(nil), filter.AllowedLibraryIDs...)
	hidden := append([]string(nil), filter.HiddenLibraryIDs...)
	libs := append([]string(nil), libraryIDs...)
	sort.Strings(allowed)
	sort.Strings(hidden)
	sort.Strings(libs)
	sum := sha1.Sum([]byte(strings.Join([]string{
		libraryID,
		strings.Join(libs, ","),
		fmt.Sprintf("%d:%d:%t", page, pageSize, filter.IncludeNSFW),
		strings.Join(allowed, ","),
		strings.Join(hidden, ","),
	}, "|")))
	return "media:list:" + hex.EncodeToString(sum[:])
}

func (s *MediaService) libraryBrowseCacheKey(ctx context.Context, libraryID string, options LibraryBrowseOptions, visibility MediaVisibility) string {
	allowed := append([]string(nil), visibility.AllowedLibraryIDs...)
	hidden := append([]string(nil), visibility.HiddenLibraryIDs...)
	sort.Strings(allowed)
	sort.Strings(hidden)
	sum := sha1.Sum([]byte(strings.Join([]string{
		"browse-v2",
		libraryID,
		strconv.FormatUint(s.mediaCacheRevision(ctx), 10),
		fmt.Sprintf("%d:%t:%t:%t", options.Page, options.IncludeFacets, options.FacetsOnly, visibility.IncludeNSFW),
		options.Query, options.Sort, options.Category, options.Genre,
		fmt.Sprintf("%d:%d", options.YearFrom, options.YearTo),
		options.Language, options.Actor, options.AdultType, options.SeriesKey, options.FocusMediaID,
		strings.Join(allowed, ","), strings.Join(hidden, ","),
	}, "|")))
	return "media:browse:" + hex.EncodeToString(sum[:])
}

func (s *MediaService) libraryFacetCacheKey(ctx context.Context, libraryID string, libraryIDs []string, visibility MediaVisibility) string {
	allowed := append([]string(nil), visibility.AllowedLibraryIDs...)
	hidden := append([]string(nil), visibility.HiddenLibraryIDs...)
	ids := append([]string(nil), libraryIDs...)
	sort.Strings(allowed)
	sort.Strings(hidden)
	sort.Strings(ids)
	sum := sha1.Sum([]byte(strings.Join([]string{
		"facets-v2", libraryID, strconv.FormatUint(s.mediaCacheRevision(ctx), 10), fmt.Sprintf("%t", visibility.IncludeNSFW),
		strings.Join(ids, ","), strings.Join(allowed, ","), strings.Join(hidden, ","),
	}, "|")))
	return "media:facets:" + hex.EncodeToString(sum[:])
}

func (s *MediaService) mediaCacheRevision(ctx context.Context) uint64 {
	if s == nil || s.cache == nil {
		return 0
	}
	return s.cache.Revision(ctx, "media")
}

func (s *MediaService) mediaCacheTTLSeconds() int {
	if s == nil || s.cfg == nil || s.cfg.Cache.MediaTTLSeconds < 1 {
		return 15
	}
	return s.cfg.Cache.MediaTTLSeconds
}

func (s *MediaService) libraryFacetCacheTTL() time.Duration {
	if s == nil || s.cfg == nil || s.cfg.Cache.LibraryFacetTTLSeconds < 1 {
		return 24 * time.Hour
	}
	return time.Duration(s.cfg.Cache.LibraryFacetTTLSeconds) * time.Second
}

func (s *MediaService) libraryBrowseCacheTTL(_ LibraryBrowseOptions) time.Duration {
	if s == nil || s.cfg == nil || s.cfg.Cache.LibraryBrowseTTLSeconds < 1 {
		return time.Hour
	}
	return time.Duration(s.cfg.Cache.LibraryBrowseTTLSeconds) * time.Second
}

func (s *MediaService) invalidateMediaCache(ctx context.Context) {
	if s != nil && s.cache != nil {
		s.cache.DeletePrefix(ctx, "media:")
		s.cache.DeletePrefix(ctx, "stats:")
	}
}
