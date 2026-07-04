package service

import (
	"context"
	"crypto/sha1"
	"encoding/hex"
	"fmt"
	"sort"
	"strings"

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

func (s *MediaService) mediaCacheTTLSeconds() int {
	if s == nil || s.cfg == nil || s.cfg.Cache.MediaTTLSeconds < 1 {
		return 15
	}
	return s.cfg.Cache.MediaTTLSeconds
}

func (s *MediaService) invalidateMediaCache(ctx context.Context) {
	if s != nil && s.cache != nil {
		s.cache.DeletePrefix(ctx, "media:")
		s.cache.DeletePrefix(ctx, "stats:")
	}
}
