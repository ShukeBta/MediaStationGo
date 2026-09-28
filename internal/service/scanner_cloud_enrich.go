package service

import (
	"context"
	"fmt"
	"strings"
	"sync"
	"time"

	"go.uber.org/zap"

	"github.com/ShukeBta/MediaStationGo/internal/model"
)

func (s *ScannerService) enrichCloudMetadataFromExternalIDs(ctx context.Context, lib *model.Library, path string, meta *LocalMetadata) *LocalMetadata {
	if s == nil || s.scraper == nil || meta == nil || !cloudMetadataNeedsExternalEnrich(meta) {
		return meta
	}
	match := s.lookupCloudMetadataFromExternalIDs(ctx, lib, path, meta)
	return s.mergeCloudMetadataFromExternalIDs(ctx, meta, match)
}

func (s *ScannerService) enrichCloudMetadataFromExternalIDsCached(ctx context.Context, lib *model.Library, path string, meta *LocalMetadata, cache *cloudExternalMetadataCache) *LocalMetadata {
	if s == nil || s.scraper == nil || meta == nil || !cloudMetadataNeedsExternalEnrich(meta) {
		return meta
	}
	if cache == nil {
		return s.enrichCloudMetadataFromExternalIDs(ctx, lib, path, meta)
	}
	match := cache.lookup(ctx, cloudExternalMetadataCacheKey(lib, meta), func() *Match {
		return s.lookupCloudMetadataFromExternalIDs(ctx, lib, path, meta)
	})
	return s.mergeCloudMetadataFromExternalIDs(ctx, meta, match)
}

func (s *ScannerService) lookupCloudMetadataFromExternalIDs(ctx context.Context, lib *model.Library, path string, meta *LocalMetadata) *Match {
	media := &model.Media{
		LibraryID:   "",
		Title:       firstNonEmpty(meta.Title, pathBaseSlash(path)),
		Path:        path,
		Year:        meta.Year,
		TMDbID:      meta.TMDbID,
		BangumiID:   meta.BangumiID,
		DoubanID:    meta.DoubanID,
		TheTVDBID:   meta.TheTVDBID,
		SeasonNum:   meta.SeasonNum,
		EpisodeNum:  meta.EpisodeNum,
		PosterURL:   meta.PosterURL,
		BackdropURL: meta.BackdropURL,
	}
	if lib != nil {
		media.LibraryID = lib.ID
	}
	enrichCtx, cancel := context.WithTimeout(ctx, 10*time.Second)
	defer cancel()
	match := s.scraper.matchFromMediaExternalIDs(enrichCtx, media, lib)
	if match == nil {
		return nil
	}
	s.scraper.applyFanartArtwork(enrichCtx, match)
	return match
}

func (s *ScannerService) mergeCloudMetadataFromExternalIDs(ctx context.Context, meta *LocalMetadata, match *Match) *LocalMetadata {
	if meta == nil || match == nil {
		return meta
	}
	localPoster, localBackdrop := cloudLocalArtworkURLs(meta)
	mergedMatch := cloneCloudMetadataMatch(match)
	mergeLocalMetadataIntoMatch(mergedMatch, meta)

	enriched := cloneLocalMetadata(meta)
	if enriched == nil {
		enriched = &LocalMetadata{}
	}
	mergeMatchIntoLocalMetadata(enriched, mergedMatch)
	if localPoster != "" {
		enriched.PosterURL = localPoster
		enriched.HasArtwork = true
	}
	if localBackdrop != "" {
		enriched.BackdropURL = localBackdrop
		enriched.HasArtwork = true
	}
	enriched.PathHint = false
	enriched.HasNFO = true
	if enriched.PosterURL != "" || enriched.BackdropURL != "" {
		enriched.HasArtwork = true
	}
	s.prefetchRemoteArtworkFromScan(ctx, enriched.PosterURL)
	s.prefetchRemoteArtworkFromScan(ctx, enriched.BackdropURL)
	return enriched
}

type cloudExternalMetadataCache struct {
	mu      sync.Mutex
	entries map[string]*cloudExternalMetadataCacheEntry
}

type cloudExternalMetadataCacheEntry struct {
	ready chan struct{}
	match *Match
}

func newCloudExternalMetadataCache() *cloudExternalMetadataCache {
	return &cloudExternalMetadataCache{entries: make(map[string]*cloudExternalMetadataCacheEntry)}
}

func (c *cloudExternalMetadataCache) lookup(ctx context.Context, key string, load func() *Match) *Match {
	if c == nil || key == "" {
		return load()
	}
	c.mu.Lock()
	if entry, ok := c.entries[key]; ok {
		c.mu.Unlock()
		select {
		case <-entry.ready:
			return entry.match
		case <-ctx.Done():
			return nil
		}
	}
	entry := &cloudExternalMetadataCacheEntry{ready: make(chan struct{})}
	c.entries[key] = entry
	c.mu.Unlock()

	match := load()
	c.mu.Lock()
	entry.match = match
	close(entry.ready)
	c.mu.Unlock()
	return match
}

func cloudExternalMetadataCacheKey(lib *model.Library, meta *LocalMetadata) string {
	if meta == nil {
		return ""
	}
	mediaType := ""
	if lib != nil {
		mediaType = strings.ToLower(strings.TrimSpace(lib.Type))
	}
	return fmt.Sprintf("%s|tmdb:%d|bangumi:%d|douban:%s|thetvdb:%s",
		mediaType,
		meta.TMDbID,
		meta.BangumiID,
		strings.TrimSpace(meta.DoubanID),
		strings.TrimSpace(meta.TheTVDBID),
	)
}

func cloneCloudMetadataMatch(match *Match) *Match {
	if match == nil {
		return nil
	}
	cloned := *match
	cloned.PreviewImages = append([]string(nil), match.PreviewImages...)
	cloned.Languages = append([]string(nil), match.Languages...)
	cloned.Countries = append([]string(nil), match.Countries...)
	cloned.Genres = append([]string(nil), match.Genres...)
	cloned.Actors = append([]string(nil), match.Actors...)
	cloned.Directors = append([]string(nil), match.Directors...)
	cloned.Writers = append([]string(nil), match.Writers...)
	cloned.Aliases = append([]string(nil), match.Aliases...)
	cloned.People = append([]PersonMetadata(nil), match.People...)
	return &cloned
}

func cloudExistingMetadataSatisfiesExternalEnrich(existingMedia map[string]existingCloudMedia, path string, size int64, provider, ref string, meta *LocalMetadata) bool {
	if existingMedia == nil || meta == nil || !cloudMetadataNeedsExternalEnrich(meta) || !meta.PathHint || meta.HasNFO || meta.HasArtwork {
		return false
	}
	existing, ok := existingMedia[path]
	if !ok || existing.SizeBytes != size || existing.STRMURL != BuildRelativeCloudPlayURL(provider, ref) {
		return false
	}
	if meta.TMDbID > 0 && existing.TMDbID != meta.TMDbID {
		return false
	}
	if meta.BangumiID > 0 && existing.BangumiID != meta.BangumiID {
		return false
	}
	if strings.TrimSpace(meta.DoubanID) != "" && strings.TrimSpace(existing.DoubanID) != strings.TrimSpace(meta.DoubanID) {
		return false
	}
	if strings.TrimSpace(meta.TheTVDBID) != "" && strings.TrimSpace(existing.TheTVDBID) != strings.TrimSpace(meta.TheTVDBID) {
		return false
	}
	return strings.TrimSpace(existing.Title) != "" &&
		strings.TrimSpace(existing.PosterURL) != "" &&
		strings.TrimSpace(existing.BackdropURL) != "" &&
		strings.TrimSpace(existing.Overview) != ""
}

func cloudMetadataNeedsExternalEnrich(meta *LocalMetadata) bool {
	if meta == nil {
		return false
	}
	hasExternalID := meta.TMDbID > 0 || meta.BangumiID > 0 || strings.TrimSpace(meta.DoubanID) != "" || strings.TrimSpace(meta.TheTVDBID) != ""
	if !hasExternalID {
		return false
	}
	return meta.PosterURL == "" || meta.BackdropURL == "" || meta.Overview == "" || meta.Title == "" || meta.Rating <= 0
}

func cloudLocalArtworkURLs(meta *LocalMetadata) (poster, backdrop string) {
	if meta == nil || !meta.HasArtwork {
		return "", ""
	}
	if _, _, ok := ParseCloudArtworkURL(meta.PosterURL); ok {
		poster = meta.PosterURL
	}
	if _, _, ok := ParseCloudArtworkURL(meta.BackdropURL); ok {
		backdrop = meta.BackdropURL
	}
	return poster, backdrop
}

func mergeMatchIntoLocalMetadata(meta *LocalMetadata, match *Match) {
	if meta == nil || match == nil {
		return
	}
	if match.Title != "" {
		meta.Title = match.Title
	}
	if match.OriginalName != "" {
		meta.OriginalName = match.OriginalName
	}
	if match.Year > 0 {
		meta.Year = match.Year
	}
	if match.ReleaseDate != "" {
		meta.ReleaseDate = match.ReleaseDate
	}
	if match.Overview != "" {
		meta.Overview = match.Overview
	}
	if match.Rating > 0 {
		meta.Rating = match.Rating
	}
	if match.PosterURL != "" {
		meta.PosterURL = match.PosterURL
	}
	if match.BackdropURL != "" {
		meta.BackdropURL = match.BackdropURL
	}
	if match.TMDbID > 0 {
		meta.TMDbID = match.TMDbID
	}
	if match.BangumiID > 0 {
		meta.BangumiID = match.BangumiID
	}
	if match.DoubanID != "" {
		meta.DoubanID = match.DoubanID
	}
	if match.TheTVDBID != "" {
		meta.TheTVDBID = match.TheTVDBID
	}
	if len(match.Genres) > 0 {
		meta.Genres = strings.Join(match.Genres, ",")
	}
	if len(match.Actors) > 0 {
		meta.Actors = strings.Join(match.Actors, ",")
	}
	if len(match.Countries) > 0 {
		meta.Countries = strings.Join(match.Countries, ",")
	}
	if len(match.Languages) > 0 {
		meta.Languages = strings.Join(match.Languages, ",")
	}
	if match.NSFW {
		meta.NSFW = true
	}
}

func (s *ScannerService) prefetchRemoteArtworkFromScan(ctx context.Context, raw string) {
	if s == nil || s.imageProxy == nil || !isHTTPish(raw) {
		return
	}
	fetchCtx, cancel := context.WithTimeout(context.WithoutCancel(ctx), 15*time.Second)
	err := s.imageProxy.PrefetchRemote(fetchCtx, raw)
	cancel()
	if err != nil && s.log != nil {
		s.log.Debug("scan remote artwork prefetch failed", zap.String("url", raw), zap.Error(err))
	}
}
