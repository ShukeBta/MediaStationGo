package service

import (
	"context"
	"path/filepath"
	"strings"

	"github.com/ShukeBta/MediaStationGo/internal/model"
)

func (s *ScannerService) existingCloudMediaSnapshot(ctx context.Context, libraryID string) (map[string]existingCloudMedia, error) {
	return s.existingCloudMediaSnapshotForLibraries(ctx, []string{libraryID})
}

func (s *ScannerService) existingCloudMediaSnapshotForLibraries(ctx context.Context, libraryIDs []string) (map[string]existingCloudMedia, error) {
	if len(libraryIDs) == 0 {
		return map[string]existingCloudMedia{}, nil
	}
	var rows []model.Media
	if err := s.repo.DB.WithContext(ctx).
		Model(&model.Media{}).
		Select("library_id", "path", "title", "original_name", "episode_title", "size_bytes", "duration_sec", "width", "height", "video_codec", "audio_codec", "container", "poster_url", "backdrop_url", "strm_url", "overview", "year", "release_date", "rating", "tm_db_id", "bangumi_id", "douban_id", "thetvdb_id", "season_num", "episode_num", "genres", "countries", "languages", "nsfw", "scrape_status").
		Where("library_id IN ? AND path LIKE ?", libraryIDs, "cloud://%").
		Find(&rows).Error; err != nil {
		return nil, err
	}
	snapshot := make(map[string]existingCloudMedia, len(rows))
	for _, row := range rows {
		if strings.TrimSpace(row.Path) == "" {
			continue
		}
		snapshot[row.Path] = existingCloudMedia{
			LibraryID:    row.LibraryID,
			Title:        row.Title,
			OriginalName: row.OriginalName,
			EpisodeTitle: row.EpisodeTitle,
			SizeBytes:    row.SizeBytes,
			DurationSec:  row.DurationSec,
			Width:        row.Width,
			Height:       row.Height,
			VideoCodec:   row.VideoCodec,
			AudioCodec:   row.AudioCodec,
			Container:    row.Container,
			PosterURL:    row.PosterURL,
			BackdropURL:  row.BackdropURL,
			STRMURL:      row.STRMURL,
			Overview:     row.Overview,
			Year:         row.Year,
			ReleaseDate:  row.ReleaseDate,
			Rating:       row.Rating,
			TMDbID:       row.TMDbID,
			BangumiID:    row.BangumiID,
			DoubanID:     row.DoubanID,
			TheTVDBID:    row.TheTVDBID,
			SeasonNum:    row.SeasonNum,
			EpisodeNum:   row.EpisodeNum,
			Genres:       row.Genres,
			Countries:    row.Countries,
			Languages:    row.Languages,
			NSFW:         row.NSFW,
			ScrapeStatus: row.ScrapeStatus,
		}
	}
	return snapshot, nil
}

func (s *ScannerService) existingLocalMediaSnapshot(ctx context.Context, libraryID string) (map[string]existingLocalMedia, error) {
	var rows []model.Media
	if err := s.repo.DB.WithContext(ctx).
		Model(&model.Media{}).
		Select("path", "library_root_id", "relative_path", "title", "original_name", "episode_title", "size_bytes", "duration_sec", "width", "height", "video_codec", "audio_codec", "container", "strm_url", "file_id", "poster_url", "backdrop_url", "overview", "year", "release_date", "rating", "tm_db_id", "bangumi_id", "douban_id", "thetvdb_id", "season_num", "episode_num", "genres", "countries", "languages", "nsfw", "scrape_status").
		Where("library_id = ? AND path NOT LIKE ?", libraryID, "cloud://%").
		Find(&rows).Error; err != nil {
		return nil, err
	}
	snapshot := make(map[string]existingLocalMedia, len(rows))
	for _, row := range rows {
		if strings.TrimSpace(row.Path) == "" {
			continue
		}
		snapshot[filepath.Clean(row.Path)] = existingLocalMedia{
			LibraryRootID: row.LibraryRootID,
			RelativePath:  row.RelativePath,
			Title:         row.Title,
			OriginalName:  row.OriginalName,
			EpisodeTitle:  row.EpisodeTitle,
			SizeBytes:     row.SizeBytes,
			DurationSec:   row.DurationSec,
			Width:         row.Width,
			Height:        row.Height,
			VideoCodec:    row.VideoCodec,
			AudioCodec:    row.AudioCodec,
			Container:     row.Container,
			STRMURL:       row.STRMURL,
			FileID:        row.FileID,
			PosterURL:     row.PosterURL,
			BackdropURL:   row.BackdropURL,
			Overview:      row.Overview,
			Year:          row.Year,
			ReleaseDate:   row.ReleaseDate,
			Rating:        row.Rating,
			TMDbID:        row.TMDbID,
			BangumiID:     row.BangumiID,
			DoubanID:      row.DoubanID,
			TheTVDBID:     row.TheTVDBID,
			SeasonNum:     row.SeasonNum,
			EpisodeNum:    row.EpisodeNum,
			Genres:        row.Genres,
			Countries:     row.Countries,
			Languages:     row.Languages,
			NSFW:          row.NSFW,
			ScrapeStatus:  row.ScrapeStatus,
		}
	}
	return snapshot, nil
}
