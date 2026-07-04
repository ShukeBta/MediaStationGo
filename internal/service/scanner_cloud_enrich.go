package service

import (
	"context"
	"strings"
	"time"

	"go.uber.org/zap"

	"github.com/ShukeBta/MediaStationGo/internal/model"
)

func (s *ScannerService) enrichCloudMetadataFromExternalIDs(ctx context.Context, lib *model.Library, path string, meta *LocalMetadata) *LocalMetadata {
	if s == nil || s.scraper == nil || meta == nil || !cloudMetadataNeedsExternalEnrich(meta) {
		return meta
	}
	localPoster, localBackdrop := cloudLocalArtworkURLs(meta)
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
		return meta
	}
	s.scraper.applyFanartArtwork(enrichCtx, match)
	mergeLocalMetadataIntoMatch(match, meta)

	enriched := cloneLocalMetadata(meta)
	if enriched == nil {
		enriched = &LocalMetadata{}
	}
	mergeMatchIntoLocalMetadata(enriched, match)
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

func cloudMetadataNeedsExternalEnrich(meta *LocalMetadata) bool {
	if meta == nil {
		return false
	}
	hasExternalID := meta.TMDbID > 0 || meta.BangumiID > 0 || strings.TrimSpace(meta.DoubanID) != "" || strings.TrimSpace(meta.TheTVDBID) != ""
	if !hasExternalID {
		return false
	}
	return meta.PosterURL == "" || meta.BackdropURL == "" || meta.Overview == "" || meta.Title == ""
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
