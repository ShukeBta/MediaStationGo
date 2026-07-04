package service

import (
	"context"
	"path/filepath"
	"strconv"
	"strings"

	"github.com/ShukeBta/MediaStationGo/internal/model"
)

func mediaYearHint(m *model.Media) int {
	if m == nil {
		return 0
	}
	if m.SeasonNum > 0 || m.EpisodeNum > 0 {
		return seriesPathYearHint(m.Path)
	}
	if season, episode := ParseEpisode(m.Path); season > 0 || episode > 0 {
		return seriesPathYearHint(m.Path)
	}
	if m.Year > 0 {
		return m.Year
	}
	if _, year := CleanQuery(filepath.Base(m.Path)); year > 0 {
		return year
	}
	return yearFromText(m.Path)
}

func seriesPathYearHint(path string) int {
	if showDir := showDirFromEpisodePath(path); showDir != "" {
		if _, year := CleanQuery(showDir); year > 0 {
			return year
		}
	}
	return 0
}

func yearFromText(raw string) int {
	if raw == "" {
		return 0
	}
	matches := yearPattern.FindStringSubmatch(strings.ToLower(raw))
	if len(matches) < 2 {
		return 0
	}
	year, _ := strconv.Atoi(matches[1])
	return year
}

func localAdultCode(local *LocalMetadata) string {
	if local == nil {
		return ""
	}
	return local.AdultCode
}

func mergeLocalMetadataIntoMatch(match *Match, local *LocalMetadata) {
	if match == nil || local == nil {
		return
	}
	if local.PathHint {
		mergePathHintIDsIntoMatch(match, local)
		return
	}
	if local.Title != "" {
		match.Title = local.Title
	}
	if local.OriginalName != "" {
		match.OriginalName = local.OriginalName
	}
	if local.AdultCode != "" {
		match.OriginalName = local.AdultCode
		match.NSFW = true
	}
	if local.Overview != "" {
		match.Overview = local.Overview
	}
	if local.PosterURL != "" {
		match.PosterURL = local.PosterURL
	}
	if local.BackdropURL != "" {
		match.BackdropURL = local.BackdropURL
	}
	if local.Rating > 0 {
		match.Rating = local.Rating
	}
	if local.Year > 0 {
		match.Year = local.Year
	}
	if local.ReleaseDate != "" {
		match.ReleaseDate = local.ReleaseDate
	}
	if local.TMDbID > 0 {
		match.TMDbID = local.TMDbID
	}
	if local.BangumiID > 0 {
		match.BangumiID = local.BangumiID
	}
	if local.DoubanID != "" {
		match.DoubanID = local.DoubanID
	}
	if local.TheTVDBID != "" {
		match.TheTVDBID = local.TheTVDBID
	}
	if local.Genres != "" {
		match.Genres = splitNFOList(local.Genres)
	}
	if local.Countries != "" {
		match.Countries = splitNFOList(local.Countries)
	}
	if local.Languages != "" {
		match.Languages = splitNFOList(local.Languages)
	}
	if local.NSFW {
		match.NSFW = true
	}
}

func mergePathHintIDsIntoMatch(match *Match, local *LocalMetadata) {
	if match == nil || local == nil {
		return
	}
	if local.TMDbID > 0 {
		match.TMDbID = local.TMDbID
	}
	if local.BangumiID > 0 {
		match.BangumiID = local.BangumiID
	}
	if local.DoubanID != "" {
		match.DoubanID = local.DoubanID
	}
	if local.TheTVDBID != "" {
		match.TheTVDBID = local.TheTVDBID
	}
	if match.Year <= 0 && local.Year > 0 {
		match.Year = local.Year
	}
}

func mergeScrapePathHintMetadata(dst, src *LocalMetadata) *LocalMetadata {
	if src == nil {
		return dst
	}
	if dst == nil {
		return cloneLocalMetadata(src)
	}
	hasLocalMetadata := localMetadataMarksMatched(dst)
	if dst.Title == "" && src.Title != "" {
		dst.Title = src.Title
	}
	if dst.Year <= 0 && src.Year > 0 {
		dst.Year = src.Year
	}
	if dst.ReleaseDate == "" && src.ReleaseDate != "" {
		dst.ReleaseDate = src.ReleaseDate
	}
	if src.TMDbID > 0 {
		dst.TMDbID = src.TMDbID
	}
	if src.BangumiID > 0 {
		dst.BangumiID = src.BangumiID
	}
	if strings.TrimSpace(src.DoubanID) != "" {
		dst.DoubanID = strings.TrimSpace(src.DoubanID)
	}
	if strings.TrimSpace(src.TheTVDBID) != "" {
		dst.TheTVDBID = strings.TrimSpace(src.TheTVDBID)
	}
	if !hasLocalMetadata {
		dst.PathHint = dst.PathHint || src.PathHint
	}
	return dst
}

func isCloudMediaPath(value string) bool {
	return strings.HasPrefix(strings.ToLower(strings.TrimSpace(value)), "cloud://")
}

func (s *ScraperService) applyLocalMetadataMatch(ctx context.Context, m *model.Media, local *LocalMetadata) error {
	next := *m
	applyLocalMetadata(&next, local)
	status := "matched"
	if !localMetadataMarksMatched(local) {
		status = "pending"
	}
	updates := map[string]any{
		"title":         next.Title,
		"scrape_status": status,
	}
	if next.OriginalName != "" {
		updates["original_name"] = next.OriginalName
	}
	if next.EpisodeTitle != "" {
		updates["episode_title"] = next.EpisodeTitle
	}
	if next.Overview != "" {
		updates["overview"] = next.Overview
	}
	if next.PosterURL != "" {
		updates["poster_url"] = next.PosterURL
	}
	if next.BackdropURL != "" {
		updates["backdrop_url"] = next.BackdropURL
	}
	if next.Rating > 0 {
		updates["rating"] = next.Rating
	}
	if next.Year > 0 {
		updates["year"] = next.Year
	}
	if next.ReleaseDate != "" {
		updates["release_date"] = next.ReleaseDate
	}
	if next.TMDbID > 0 {
		updates["tm_db_id"] = next.TMDbID
	}
	if next.BangumiID > 0 {
		updates["bangumi_id"] = next.BangumiID
	}
	if next.DoubanID != "" {
		updates["douban_id"] = next.DoubanID
	}
	if next.TheTVDBID != "" {
		updates["thetvdb_id"] = next.TheTVDBID
	}
	if next.SeasonNum > 0 || next.EpisodeNum > 0 {
		updates["season_num"] = next.SeasonNum
	}
	if next.EpisodeNum > 0 {
		updates["episode_num"] = next.EpisodeNum
	}
	if next.Genres != "" {
		updates["genres"] = next.Genres
	}
	if next.Countries != "" {
		updates["countries"] = next.Countries
	}
	if next.Languages != "" {
		updates["languages"] = next.Languages
	}
	if next.NSFW {
		updates["nsfw"] = true
	}
	if err := s.repo.DB.WithContext(ctx).Model(&model.Media{}).
		Where("id = ?", m.ID).Updates(updates).Error; err != nil {
		return err
	}
	s.invalidateMediaCache(ctx)
	s.hub.Publish("scrape", map[string]any{
		"media_id":  m.ID,
		"title":     next.Title,
		"tmdb_id":   next.TMDbID,
		"douban_id": next.DoubanID,
		"source":    "local_nfo",
	})
	return nil
}

func (s *ScraperService) invalidateMediaCache(ctx context.Context) {
	if s != nil && s.cache != nil {
		s.cache.DeletePrefix(ctx, "media:")
		s.cache.DeletePrefix(ctx, "stats:")
	}
}
