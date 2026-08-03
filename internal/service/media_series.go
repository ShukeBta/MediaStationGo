package service

import (
	"context"
	"regexp"
	"sort"
	"strings"
	"time"

	"github.com/ShukeBta/MediaStationGo/internal/model"
)

type SeriesCard struct {
	Key       string      `json:"key"`
	Rep       model.Media `json:"rep"`
	LinkMedia model.Media `json:"linkMedia"`
	Count     int         `json:"count"`
}

type seriesCardGroup struct {
	card   SeriesCard
	latest time.Time
}

func (s *MediaService) ListLibrarySeriesCards(ctx context.Context, libraryID string, visibility MediaVisibility) ([]SeriesCard, int64, error) {
	rows, _, err := s.listAllMediaVisible(ctx, libraryID, visibility)
	if err != nil {
		return nil, 0, err
	}
	cards := groupMediaSeriesCards(rows)
	return cards, int64(len(cards)), nil
}

func (s *MediaService) ListRecentSeriesCards(ctx context.Context, limit int, visibility MediaVisibility) ([]SeriesCard, error) {
	if limit <= 0 {
		limit = 24
	} else if limit > 100 {
		limit = 100
	}
	rows, err := s.SearchMediaVisible(ctx, "", maxMediaSearchLimit, visibility)
	if err != nil {
		return nil, err
	}
	cards := groupMediaSeriesCards(rows)
	if len(cards) == 0 {
		return []SeriesCard{}, nil
	}
	if len(cards) > limit {
		cards = cards[:limit]
	}
	return cards, nil
}

func (s *MediaService) ListLibrarySeriesEpisodes(ctx context.Context, libraryID, key string, visibility MediaVisibility) ([]model.Media, error) {
	rows, _, err := s.listAllMediaVisible(ctx, libraryID, visibility)
	if err != nil {
		return nil, err
	}
	out := make([]model.Media, 0)
	resolver := newMediaSeriesKeyResolver(rows)
	for _, row := range rows {
		if resolver.key(row) == key {
			out = append(out, row)
		}
	}
	sort.SliceStable(out, func(i, j int) bool {
		if out[i].SeasonNum != out[j].SeasonNum {
			return out[i].SeasonNum < out[j].SeasonNum
		}
		if out[i].EpisodeNum != out[j].EpisodeNum {
			return out[i].EpisodeNum < out[j].EpisodeNum
		}
		return out[i].CreatedAt.Before(out[j].CreatedAt)
	})
	return out, nil
}

func (s *MediaService) listAllMediaVisible(ctx context.Context, libraryID string, visibility MediaVisibility) ([]model.Media, int64, error) {
	const pageSize = 2000
	var all []model.Media
	var total int64
	for page := 1; ; page++ {
		rows, n, err := s.ListMediaVisible(ctx, libraryID, page, pageSize, visibility)
		if err != nil {
			return nil, 0, err
		}
		if page == 1 {
			total = n
			all = make([]model.Media, 0, minInt64(n, pageSize))
		}
		all = append(all, rows...)
		if int64(len(all)) >= n || len(rows) < pageSize {
			break
		}
	}
	return all, total, nil
}

func groupMediaSeriesCards(items []model.Media) []SeriesCard {
	if len(items) == 0 {
		return nil
	}
	groups := make([]seriesCardGroup, 0)
	byKey := make(map[string]int, len(items))
	resolver := newMediaSeriesKeyResolver(items)
	for _, item := range items {
		key := resolver.key(item)
		if key == "" {
			continue
		}
		if idx, ok := byKey[key]; ok {
			group := &groups[idx]
			if latest := seriesMediaTime(item); latest.After(group.latest) {
				group.latest = latest
			}
			card := &group.card
			// A shared external ID means duplicate encodes/locations for movies,
			// not multiple episodes. Keep a single movie card without presenting
			// its versions as an "N episodes" collection.
			if mediaLooksEpisodicForGrouping(item) || mediaLooksEpisodicForGrouping(card.LinkMedia) {
				card.Count++
			}
			if betterSeriesLinkMedia(item, card.LinkMedia) {
				card.LinkMedia = item
			}
			currentArtwork := seriesArtworkScore(item)
			representativeArtwork := seriesArtworkScore(card.Rep)
			if currentArtwork > representativeArtwork {
				card.Rep = item
			} else if currentArtwork == representativeArtwork {
				cur := item.SeasonNum*10000 + item.EpisodeNum
				rep := card.Rep.SeasonNum*10000 + card.Rep.EpisodeNum
				if cur > 0 && (rep == 0 || cur < rep) {
					card.Rep = item
				}
			}
			continue
		}
		byKey[key] = len(groups)
		groups = append(groups, seriesCardGroup{
			card:   SeriesCard{Key: key, Rep: item, LinkMedia: item, Count: 1},
			latest: seriesMediaTime(item),
		})
	}
	sort.SliceStable(groups, func(i, j int) bool {
		return groups[i].latest.After(groups[j].latest)
	})
	cards := make([]SeriesCard, 0, len(groups))
	for _, group := range groups {
		cards = append(cards, group.card)
	}
	return cards
}

func seriesMediaTime(media model.Media) time.Time {
	if releaseDate := strings.TrimSpace(media.ReleaseDate); releaseDate != "" {
		if parsed, err := time.Parse("2006-01-02", releaseDate); err == nil {
			return parsed
		}
	}
	if media.Year > 0 {
		return time.Date(media.Year, time.December, 31, 0, 0, 0, 0, time.UTC)
	}
	if media.UpdatedAt.After(media.CreatedAt) {
		return media.UpdatedAt
	}
	return media.CreatedAt
}

func betterSeriesLinkMedia(candidate, current model.Media) bool {
	candidateScore := librarySpecificityScore(candidate)
	currentScore := librarySpecificityScore(current)
	if candidateScore != currentScore {
		return candidateScore > currentScore
	}
	return seriesArtworkScore(candidate) > seriesArtworkScore(current)
}

func librarySpecificityScore(media model.Media) int {
	rawPath := strings.TrimSpace(firstNonEmpty(media.DisplayLibraryPath, media.LibraryPath))
	if rawPath == "" {
		return 0
	}
	normalized := strings.TrimRight(strings.ReplaceAll(rawPath, "\\", "/"), "/")
	lower := strings.ToLower(normalized)
	if strings.HasPrefix(lower, "cloud://") {
		rest := normalized[len("cloud://"):]
		slash := strings.Index(rest, "/")
		if slash < 0 || slash == len(rest)-1 {
			return 0
		}
		return 100 + len(nonEmptySlashParts(rest[slash+1:]))
	}
	return 200 + len(nonEmptySlashParts(normalized))
}

func nonEmptySlashParts(value string) []string {
	parts := strings.Split(value, "/")
	out := parts[:0]
	for _, part := range parts {
		if strings.TrimSpace(part) != "" {
			out = append(out, part)
		}
	}
	return out
}

var (
	posterArtworkRE = regexp.MustCompile(`(poster|folder|cover|movie|show|pl)(?:[._-]|\.[a-z0-9]+$|$)`)
	badArtworkRE    = regexp.MustCompile(`(actor|actress|cast|avatar|sample|screenshot|screen|still|scene|fanart|backdrop|background|landscape|banner|logo|disc)`)
)

func seriesArtworkScore(media model.Media) int {
	poster := strings.ToLower(media.PosterURL)
	backdrop := strings.ToLower(media.BackdropURL)
	if poster == "" {
		if backdrop != "" {
			return 5
		}
		return 0
	}
	if posterArtworkRE.MatchString(poster) {
		return 40
	}
	if badArtworkRE.MatchString(poster) {
		return 10
	}
	if strings.Contains(poster, "thumb") {
		return 20
	}
	return 30
}

func minInt64(a int64, b int) int {
	if a <= 0 {
		return 0
	}
	if a > int64(b) {
		return b
	}
	return int(a)
}
