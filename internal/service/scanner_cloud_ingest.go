package service

import (
	"context"
	"path/filepath"
	"strings"

	"go.uber.org/zap"

	"github.com/ShukeBta/MediaStationGo/internal/model"
)

func (s *ScannerService) ingestCloudFile(ctx context.Context, lib *model.Library, rootID, typ, ref, path, name string, size int64, localMeta *LocalMetadata, existingMedia map[string]existingCloudMedia, writeBatch *localMediaWriteBatch, probeBudget *int, res *ScanResult) {
	res.Visited++
	ext := strings.ToLower(filepath.Ext(name))
	title, year := CleanQueryWithRecognition(ctx, s.repo, name)
	if title == "" {
		title = strings.TrimSuffix(filepath.Base(name), ext)
	}
	if title == "" {
		title = ref
	}
	parsedSeason, parsedEpisode := ParseEpisode(path)
	if librarySupportsSeasons(lib) || parsedSeason > 0 || parsedEpisode > 0 {
		if seriesTitle, seriesYear := cloudSeriesTitleFromMediaPath(path); seriesTitle != "" {
			title = seriesTitle
			if seriesYear > 0 {
				year = seriesYear
			}
		}
	}
	expectedSTRMURL := BuildRelativeCloudPlayURL(typ, ref)
	m := &model.Media{
		LibraryID:     lib.ID,
		LibraryRootID: strings.TrimSpace(rootID),
		Title:         title,
		Year:          year,
		Path:          path,
		SizeBytes:     size,
		Container:     strings.TrimPrefix(ext, "."),
		STRMURL:       expectedSTRMURL,
		ScrapeStatus:  "pending",
		SeasonNum:     parsedSeason,
		EpisodeNum:    parsedEpisode,
	}
	if ext == ".strm" {
		if targetURL, err := s.resolveCloudSTRMTarget(ctx, typ, ref); err == nil && targetURL != "" {
			m.STRMURL = targetURL
		} else if err != nil {
			s.log.Debug("read cloud strm failed", zap.String("ref", ref), zap.Error(err))
		}
	}
	if localMeta != nil {
		applyLocalMetadata(m, localMeta)
		s.queueCloudArtworkPrefetch(localMeta.PosterURL)
		s.queueCloudArtworkPrefetch(localMeta.BackdropURL)
	}
	if _, hints := pathHintMetadata(path, librarySupportsSeasons(lib) || parsedSeason > 0 || parsedEpisode > 0); hints.useful() {
		if hints.TMDbID > 0 && m.TMDbID <= 0 {
			m.TMDbID = hints.TMDbID
		}
		if hints.BangumiID > 0 && m.BangumiID <= 0 {
			m.BangumiID = hints.BangumiID
		}
		if strings.TrimSpace(hints.DoubanID) != "" && strings.TrimSpace(m.DoubanID) == "" {
			m.DoubanID = strings.TrimSpace(hints.DoubanID)
		}
		if strings.TrimSpace(hints.TheTVDBID) != "" && strings.TrimSpace(m.TheTVDBID) == "" {
			m.TheTVDBID = strings.TrimSpace(hints.TheTVDBID)
		}
	}
	isNewMedia := false
	needsTrackProbe := true
	if existingMedia != nil {
		existing, exists := existingMedia[path]
		isNewMedia = !exists
		needsTrackProbe = !exists || cloudTrackMetadataMissing(existing)
		if exists && existing.LibraryID == lib.ID && existing.SizeBytes == size && existing.STRMURL == expectedSTRMURL && !cloudMetadataNeedsRefresh(existing, localMeta) && !cloudDerivedMetadataNeedsRefresh(existing, m) {
			if needsTrackProbe && ext != ".strm" {
				s.queueCloudMediaProbeWithBudget(typ, ref, path, probeBudget)
			}
			res.Skipped++
			return
		}
	} else {
		isNewMedia = !s.mediaPathExists(ctx, path)
	}
	if localMeta != nil {
		res.LocalMetadata++
	}
	if isNewMedia && writeBatch != nil {
		var after func()
		if needsTrackProbe && ext != ".strm" {
			after = func() {
				s.queueCloudMediaProbeWithBudget(typ, ref, path, probeBudget)
			}
		}
		writeBatch.AddWithAfter(path, m, after)
		return
	}
	if err := s.repo.Media.Upsert(ctx, m); err != nil {
		addScanError(res, path, err)
		s.log.Warn("upsert cloud media failed", zap.String("path", path), zap.Error(err))
		return
	}
	if needsTrackProbe && ext != ".strm" {
		s.queueCloudMediaProbeWithBudget(typ, ref, path, probeBudget)
	}
	if isNewMedia {
		res.Added++
	} else {
		res.Updated++
	}
	if s.hub != nil && (res.Visited == 1 || res.Visited%100 == 0) {
		s.hub.Publish("scan", map[string]any{
			"library_id": lib.ID,
			"path":       path,
			"visited":    res.Visited,
			"added":      res.Added,
			"updated":    res.Updated,
			"cloud":      true,
		})
	}
}
