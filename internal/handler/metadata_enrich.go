package handler

import (
	"context"
	"regexp"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/ShukeBta/MediaStationGo/internal/service"
)

var metadataNoiseRE = regexp.MustCompile(`(?i)(自动订阅|订阅|全集|合集|complete|batch|season\s*\d+|s\d{1,2}|s\d{1,2}e\d{1,3}|第\s*\d+\s*季|第\s*\d+\s*[集话話期]|2160p|1080p|720p|4k|uhd|bluray|blu-ray|web-?dl|hdtv|remux|x26[45]|h\.?26[45]|hevc|avc|hdr10?\+?|dovi|dv|atmos|aac|ddp?5\.1|truehd|flac)`)
var metadataYearRE = regexp.MustCompile(`(?:19|20)\d{2}`)

var displayMetadataCache sync.Map

type cachedDisplayMetadata struct {
	value     displayMetadata
	expiresAt time.Time
}

type displayMetadata struct {
	Source           string
	Title            string
	OriginalName     string
	OriginalLanguage string
	PosterURL        string
	BackdropURL      string
	Overview         string
	Year             int
	Rating           float32
	Genres           string
}

func lookupDisplayMetadata(ctx context.Context, svc *service.Container, title, fallback, mediaType string) displayMetadata {
	if svc == nil {
		return displayMetadata{}
	}
	query, year := metadataSearchQuery(title, fallback)
	if query == "" {
		return displayMetadata{}
	}
	searchType := normalizeMetadataMediaType(mediaType)
	cacheKey := searchType + ":" + strconv.Itoa(year) + ":" + strings.ToLower(query)
	if cached, ok := displayMetadataCache.Load(cacheKey); ok {
		entry := cached.(cachedDisplayMetadata)
		if time.Now().Before(entry.expiresAt) {
			return entry.value
		}
		displayMetadataCache.Delete(cacheKey)
	}
	searchCtx, cancel := context.WithTimeout(ctx, 12*time.Second)
	defer cancel()
	results := service.SearchExternalMedia(searchCtx, query, year, searchType, svc.TMDb, svc.Douban, svc.Bangumi)
	if len(results) == 0 {
		displayMetadataCache.Store(cacheKey, cachedDisplayMetadata{expiresAt: time.Now().Add(30 * time.Minute)})
		return displayMetadata{}
	}
	best := results[0]
	for _, item := range results {
		if searchType != "" && item.MediaType != "" && item.MediaType != searchType && !(searchType == "tv" && item.MediaType == "anime") {
			continue
		}
		if strings.TrimSpace(item.PosterURL) != "" {
			best = item
			break
		}
	}
	originalLanguage := ""
	if len(best.Languages) > 0 {
		originalLanguage = strings.TrimSpace(best.Languages[0])
	}
	meta := displayMetadata{
		Source:           best.Source,
		Title:            best.Title,
		OriginalName:     best.OriginalName,
		OriginalLanguage: originalLanguage,
		PosterURL:        best.PosterURL,
		BackdropURL:      best.BackdropURL,
		Overview:         best.Overview,
		Year:             best.Year,
		Rating:           best.Rating,
		Genres:           strings.Join(best.Genres, ","),
	}
	displayMetadataCache.Store(cacheKey, cachedDisplayMetadata{value: meta, expiresAt: time.Now().Add(12 * time.Hour)})
	return meta
}

func metadataSearchQuery(values ...string) (string, int) {
	for _, value := range values {
		query := strings.TrimSpace(value)
		if query == "" || strings.Contains(query, "://") {
			continue
		}
		year := 0
		if yearLoc := metadataYearRE.FindStringIndex(query); len(yearLoc) == 2 {
			yearText := query[yearLoc[0]:yearLoc[1]]
			year, _ = strconv.Atoi(yearText)
			query = query[:yearLoc[1]]
		}
		query = stripBracketNoise(query)
		query = metadataNoiseRE.ReplaceAllString(query, " ")
		query = strings.NewReplacer("_", " ", ".", " ", "-", " ", "|", " ", "/", " ").Replace(query)
		query = strings.Join(strings.Fields(query), " ")
		query = strings.TrimSpace(query)
		if query != "" {
			return query, year
		}
	}
	return "", 0
}

func stripBracketNoise(value string) string {
	replacer := strings.NewReplacer("[", " ", "]", " ", "【", " ", "】", " ", "(", " ", ")", " ")
	return replacer.Replace(value)
}

func normalizeMetadataMediaType(mediaType string) string {
	switch strings.ToLower(strings.TrimSpace(mediaType)) {
	case "movie":
		return "movie"
	case "anime":
		return "anime"
	case "tv", "series", "show", "variety", "综艺":
		return "tv"
	default:
		return ""
	}
}

func firstNonEmptyString(values ...string) string {
	for _, value := range values {
		if strings.TrimSpace(value) != "" {
			return strings.TrimSpace(value)
		}
	}
	return ""
}
