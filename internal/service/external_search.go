package service

import (
	"context"
	"fmt"
	"strconv"
	"strings"
)

// ExternalMediaResult is a metadata candidate from an online catalog. It is
// intentionally separate from model.Media because the item may not exist in
// the local library yet.
type ExternalMediaResult struct {
	Source             string   `json:"source"`
	MediaType          string   `json:"media_type,omitempty"`
	Title              string   `json:"title"`
	OriginalTitle      string   `json:"original_title,omitempty"`
	OriginalName       string   `json:"original_name,omitempty"`
	OriginalLanguage   string   `json:"original_language,omitempty"`
	Overview           string   `json:"overview,omitempty"`
	PosterURL          string   `json:"poster_url,omitempty"`
	BackdropURL        string   `json:"backdrop_url,omitempty"`
	Year               int      `json:"year,omitempty"`
	ReleaseDate        string   `json:"release_date,omitempty"`
	Rating             float32  `json:"rating,omitempty"`
	Genres             string   `json:"genres,omitempty"`
	TMDbID             int      `json:"tmdb_id,omitempty"`
	BangumiID          int      `json:"bangumi_id,omitempty"`
	DoubanID           string   `json:"douban_id,omitempty"`
	TheTVDBID          string   `json:"thetvdb_id,omitempty"`
	SubscribeKeyword   string   `json:"subscribe_keyword"`
	SubscribeAliases   []string `json:"subscribe_aliases,omitempty"`
	TotalEpisodes      int      `json:"total_episodes,omitempty"`
	DownloadedEpisodes int      `json:"downloaded_episodes,omitempty"`
	LocalMediaCount    int      `json:"local_media_count,omitempty"`
	MissingEpisodes    []int    `json:"missing_episodes,omitempty"`
	InLibrary          bool     `json:"in_library"`
	Languages          []string `json:"languages,omitempty"`
	Countries          []string `json:"countries,omitempty"`
	NSFW               bool     `json:"nsfw,omitempty"`
}

// SearchExternalMedia fans out one normalized search intent to TMDb, Douban
// and Bangumi. This keeps a clean separation of "metadata discovery"
// from later tracker searching/downloading, but keeps our Go service small.
func SearchExternalMedia(ctx context.Context, query string, year int, mediaType string, tmdb *TMDbProvider, douban *DoubanProvider, bangumi *BangumiProvider) []ExternalMediaResult {
	query = strings.TrimSpace(query)
	if query == "" {
		return []ExternalMediaResult{}
	}

	results := make([]ExternalMediaResult, 0, 6)
	addMatch := func(source, typ string, m *Match) {
		if m == nil || strings.TrimSpace(m.Title) == "" {
			return
		}
		totalEpisodes := 0
		if source == "tmdb" && typ == "tv" && m.TMDbID > 0 && tmdb != nil {
			totalEpisodes, _ = tmdb.GetTVEpisodeCount(ctx, m.TMDbID)
		}
		results = append(results, ExternalMediaResult{
			Source:           source,
			MediaType:        typ,
			Title:            m.Title,
			OriginalTitle:    m.OriginalName,
			OriginalName:     m.OriginalName,
			OriginalLanguage: strings.Join(m.Languages, ","),
			Overview:         m.Overview,
			PosterURL:        m.PosterURL,
			BackdropURL:      m.BackdropURL,
			Year:             m.Year,
			ReleaseDate:      m.ReleaseDate,
			Rating:           m.Rating,
			Genres:           strings.Join(m.Genres, ","),
			TMDbID:           m.TMDbID,
			BangumiID:        m.BangumiID,
			SubscribeKeyword: buildSubscribeKeyword(m.Title, m.Year),
			SubscribeAliases: buildSubscribeAliases(m.Title, m.OriginalName, m.Year),
			TotalEpisodes:    totalEpisodes,
			Languages:        m.Languages,
			Countries:        m.Countries,
			NSFW:             m.NSFW,
		})
	}

	if tmdb != nil {
		if mediaType == "" || mediaType == "movie" {
			if m, err := tmdb.SearchMovie(ctx, query, year); err == nil {
				addMatch("tmdb", "movie", m)
			}
		}
		if mediaType == "" || mediaType == "tv" || mediaType == "anime" {
			if m, err := tmdb.SearchTV(ctx, query, year); err == nil {
				addMatch("tmdb", "tv", m)
			}
		}
	}

	if bangumi != nil && (mediaType == "" || mediaType == "anime") {
		if m, err := bangumi.Search(ctx, query); err == nil {
			addMatch("bangumi", "anime", m)
		}
	}

	if douban != nil {
		if m, err := douban.Search(ctx, query); err == nil && m != nil {
			yearValue, _ := strconv.Atoi(m.Year)
			typ := normalizeDoubanType(m.Type, mediaType)
			results = append(results, ExternalMediaResult{
				Source:           "douban",
				MediaType:        typ,
				Title:            m.Title,
				PosterURL:        m.Img,
				Year:             yearValue,
				Rating:           m.Rating,
				DoubanID:         m.DoubanID,
				SubscribeKeyword: buildSubscribeKeyword(m.Title, yearValue),
				SubscribeAliases: buildSubscribeAliases(m.Title, "", yearValue),
			})
		}
	}

	return dedupeExternalMedia(results)
}

func buildSubscribeKeyword(title string, year int) string {
	title = strings.TrimSpace(title)
	if title == "" {
		return ""
	}
	if year > 0 {
		return fmt.Sprintf("%s %d", title, year)
	}
	return title
}

func buildSubscribeAliases(title, originalName string, year int) []string {
	values := []string{
		title,
		originalName,
		buildSubscribeKeyword(title, year),
		buildSubscribeKeyword(originalName, year),
	}
	return compactUniqueStrings(values...)
}

func normalizeDoubanType(doubanType, fallback string) string {
	doubanType = strings.ToLower(strings.TrimSpace(doubanType))
	switch doubanType {
	case "movie":
		return "movie"
	case "tv", "tvshow", "drama":
		return "tv"
	}
	if fallback != "" {
		return fallback
	}
	return "movie"
}

func dedupeExternalMedia(in []ExternalMediaResult) []ExternalMediaResult {
	seen := map[string]struct{}{}
	out := make([]ExternalMediaResult, 0, len(in))
	for _, item := range in {
		key := item.Source + ":" + strings.ToLower(item.Title)
		if item.TMDbID > 0 {
			key = fmt.Sprintf("tmdb:%d:%s", item.TMDbID, item.MediaType)
		} else if item.BangumiID > 0 {
			key = fmt.Sprintf("bangumi:%d", item.BangumiID)
		} else if item.DoubanID != "" {
			key = "douban:" + item.DoubanID
		}
		if _, ok := seen[key]; ok {
			continue
		}
		seen[key] = struct{}{}
		out = append(out, item)
	}
	return out
}
