package service

import (
	"fmt"
	"strconv"
	"strings"
)

func mergeManualRequestIntoMatch(match *Match, req ManualScrapeRequest) *Match {
	if match == nil {
		match = &Match{}
	}
	if req.Title != "" {
		match.Title = req.Title
	}
	if mediaType := normalizeOrganizeMediaType(req.MediaType); mediaType != "" {
		match.MediaType = mediaType
	}
	if req.OriginalName != "" {
		match.OriginalName = req.OriginalName
	}
	if req.Overview != "" {
		match.Overview = req.Overview
	}
	if req.PosterURL != "" {
		match.PosterURL = req.PosterURL
	}
	if req.BackdropURL != "" {
		match.BackdropURL = req.BackdropURL
	}
	if req.Year > 0 {
		match.Year = req.Year
	}
	if value := normalizeReleaseDate(req.ReleaseDate); value != "" {
		match.ReleaseDate = value
	}
	if req.Rating > 0 {
		match.Rating = req.Rating
	}
	if req.TMDbID > 0 {
		match.TMDbID = req.TMDbID
	}
	if req.BangumiID > 0 {
		match.BangumiID = req.BangumiID
	}
	if req.DoubanID != "" {
		match.DoubanID = req.DoubanID
	}
	if req.TheTVDBID != "" {
		match.TheTVDBID = req.TheTVDBID
	}
	if len(req.Genres) > 0 {
		match.Genres = req.Genres
	}
	if len(req.Countries) > 0 {
		match.Countries = req.Countries
	}
	if len(req.Languages) > 0 {
		match.Languages = req.Languages
	}
	if req.NSFW {
		match.NSFW = true
	}
	return match
}

func isTVLikeTMDbMatch(match *Match, mediaType string) bool {
	return mediaType == "tv" || mediaType == "anime" || mediaType == "variety"
}

func parsePositiveInt(value string) (int, bool) {
	idString, ok := parsePositiveIDString(value)
	if !ok {
		return 0, false
	}
	id, err := strconv.Atoi(idString)
	return id, err == nil && id > 0
}

func parsePositiveIDString(value string) (string, bool) {
	for _, provider := range []string{"tmdb", "bangumi", "douban", "thetvdb"} {
		if id, ok := parseProviderIDString(value, provider); ok {
			return id, true
		}
	}
	return parseBarePositiveIDString(value)
}

func parseProviderIDInt(value, provider string) (int, bool) {
	idString, ok := parseProviderIDString(value, provider)
	if !ok {
		return 0, false
	}
	id, err := strconv.Atoi(idString)
	return id, err == nil && id > 0
}

func parseProviderIDString(value, provider string) (string, bool) {
	hints := externalIDHintsFromText(value)
	switch strings.ToLower(strings.TrimSpace(provider)) {
	case "tmdb":
		if hints.TMDbID > 0 {
			return strconv.Itoa(hints.TMDbID), true
		}
	case "bangumi", "bgm":
		if hints.BangumiID > 0 {
			return strconv.Itoa(hints.BangumiID), true
		}
	case "douban", "db":
		if strings.TrimSpace(hints.DoubanID) != "" {
			return strings.TrimSpace(hints.DoubanID), true
		}
	case "thetvdb", "tvdb":
		if strings.TrimSpace(hints.TheTVDBID) != "" {
			return strings.TrimSpace(hints.TheTVDBID), true
		}
	}
	if hints.useful() {
		return "", false
	}
	return parseBarePositiveIDString(value)
}

func providerIDHintMismatched(value, provider string) bool {
	hints := externalIDHintsFromText(value)
	if !hints.useful() {
		return false
	}
	switch strings.ToLower(strings.TrimSpace(provider)) {
	case "tmdb":
		return hints.TMDbID <= 0
	case "bangumi", "bgm":
		return hints.BangumiID <= 0
	case "douban", "db":
		return strings.TrimSpace(hints.DoubanID) == ""
	case "thetvdb", "tvdb":
		return strings.TrimSpace(hints.TheTVDBID) == ""
	default:
		return false
	}
}

func parseBarePositiveIDString(value string) (string, bool) {
	value = strings.Trim(strings.TrimSpace(value), "[]{}()")
	if strings.Contains(value, ":") {
		value = value[strings.LastIndex(value, ":")+1:]
	}
	id, err := strconv.Atoi(strings.TrimSpace(value))
	if err != nil || id <= 0 {
		return "", false
	}
	return strconv.Itoa(id), true
}

func manualScrapeBatchName(ids []string) string {
	if len(ids) == 1 {
		return ids[0]
	}
	return fmt.Sprintf("%d 个媒体", len(ids))
}
