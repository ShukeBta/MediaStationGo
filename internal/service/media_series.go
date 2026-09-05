package service

import (
	"context"
	"fmt"
	"regexp"
	"sort"
	"strings"
	"time"

	"github.com/ShukeBta/MediaStationGo/internal/model"
	"github.com/ShukeBta/MediaStationGo/internal/repository"
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

func (s *MediaService) ListLibrarySeriesCards(ctx context.Context, libraryID string, page, pageSize int, visibility MediaVisibility) ([]SeriesCard, int64, error) {
	if page < 1 {
		page = 1
	}
	if pageSize <= 0 || pageSize > 1000 {
		pageSize = 500
	}
	ctx, err := s.withMediaLibraryMetadata(ctx)
	if err != nil {
		return nil, 0, err
	}
	visibility = ExpandMediaVisibilityForMergedCloudLibraries(ctx, s.repo, visibility)
	libraryIDs, err := MergedLibraryIDsForLibrary(ctx, s.repo, libraryID)
	if err != nil {
		return nil, 0, err
	}
	rows, err := s.repo.Media.ListSeriesCardCandidatesByLibrariesFiltered(ctx, libraryIDs, repository.MediaQueryFilter{
		IncludeNSFW:       visibility.IncludeNSFW,
		AllowedLibraryIDs: visibility.AllowedLibraryIDs,
		HiddenLibraryIDs:  visibility.HiddenLibraryIDs,
	})
	if err != nil {
		return nil, 0, err
	}
	s.attachLibraryDisplayMetadata(ctx, rows)
	cards := groupMediaSeriesCards(rows)
	total := int64(len(cards))
	start := len(cards)
	pageIndex := page - 1
	if pageIndex <= len(cards)/pageSize {
		start = pageIndex * pageSize
		if start > len(cards) {
			start = len(cards)
		}
	}
	end := start + pageSize
	if end > len(cards) {
		end = len(cards)
	}
	pageCards, err := s.hydrateSeriesCards(ctx, cards[start:end])
	if err != nil {
		return nil, 0, err
	}
	return pageCards, total, nil
}

func (s *MediaService) ListRecentSeriesCards(ctx context.Context, limit int, visibility MediaVisibility) ([]SeriesCard, error) {
	if limit <= 0 {
		limit = 24
	} else if limit > 100 {
		limit = 100
	}
	ctx, rows, err := s.listVisibleSeriesCardCandidates(ctx, visibility)
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
	return s.hydrateSeriesCards(ctx, cards)
}

func (s *MediaService) listVisibleSeriesCardCandidates(ctx context.Context, visibility MediaVisibility) (context.Context, []model.Media, error) {
	ctx, err := s.withMediaLibraryMetadata(ctx)
	if err != nil {
		return nil, nil, err
	}
	visibility = ExpandMediaVisibilityForMergedCloudLibraries(ctx, s.repo, visibility)
	rows, err := s.repo.Media.ListSeriesCardCandidatesFiltered(ctx, maxMediaSearchLimit, repository.MediaQueryFilter{
		IncludeNSFW:       visibility.IncludeNSFW,
		AllowedLibraryIDs: visibility.AllowedLibraryIDs,
		HiddenLibraryIDs:  visibility.HiddenLibraryIDs,
	})
	if err != nil {
		return nil, nil, err
	}
	s.attachLibraryDisplayMetadata(ctx, rows)
	return ctx, rows, nil
}

func (s *MediaService) hydrateSeriesCards(ctx context.Context, cards []SeriesCard) ([]SeriesCard, error) {
	if len(cards) == 0 {
		return []SeriesCard{}, nil
	}
	ids := make([]string, 0, len(cards)*2)
	seen := make(map[string]struct{}, len(cards)*2)
	for i := range cards {
		for _, id := range []string{cards[i].Rep.ID, cards[i].LinkMedia.ID} {
			if id == "" {
				return nil, fmt.Errorf("hydrate series card %q: media id is empty", cards[i].Key)
			}
			if _, exists := seen[id]; exists {
				continue
			}
			seen[id] = struct{}{}
			ids = append(ids, id)
		}
	}
	rows, err := s.repo.Media.FindByIDs(ctx, ids)
	if err != nil {
		return nil, err
	}
	s.attachLibraryMetadata(ctx, rows)
	byID := make(map[string]model.Media, len(rows))
	for i := range rows {
		byID[rows[i].ID] = rows[i]
	}
	for i := range cards {
		rep, ok := byID[cards[i].Rep.ID]
		if !ok {
			return nil, fmt.Errorf("hydrate series card %q: representative media %q not found", cards[i].Key, cards[i].Rep.ID)
		}
		link, ok := byID[cards[i].LinkMedia.ID]
		if !ok {
			return nil, fmt.Errorf("hydrate series card %q: link media %q not found", cards[i].Key, cards[i].LinkMedia.ID)
		}
		cards[i].Rep = rep
		cards[i].LinkMedia = link
	}
	return cards, nil
}

func (s *MediaService) ListLibrarySeriesEpisodes(ctx context.Context, libraryID, key string, visibility MediaVisibility) ([]model.Media, error) {
	if strings.TrimSpace(key) == "" {
		return []model.Media{}, nil
	}
	ctx, err := s.withMediaLibraryMetadata(ctx)
	if err != nil {
		return nil, err
	}
	visibility = ExpandMediaVisibilityForMergedCloudLibraries(ctx, s.repo, visibility)
	libraryIDs, err := MergedLibraryIDsForLibrary(ctx, s.repo, libraryID)
	if err != nil {
		return nil, err
	}
	// The grouping projection contains every field needed to calculate the
	// authoritative key, but skips the large metadata columns. Hydrate only the
	// episodes that belong to the requested series before returning them.
	candidates, err := s.repo.Media.ListSeriesCardCandidatesByLibrariesFiltered(ctx, libraryIDs, repository.MediaQueryFilter{
		IncludeNSFW:       visibility.IncludeNSFW,
		AllowedLibraryIDs: visibility.AllowedLibraryIDs,
		HiddenLibraryIDs:  visibility.HiddenLibraryIDs,
	})
	if err != nil {
		return nil, err
	}
	s.attachLibraryDisplayMetadata(ctx, candidates)
	ids := make([]string, 0)
	for _, row := range candidates {
		if mediaSeriesKey(row) == key {
			ids = append(ids, row.ID)
		}
	}
	if len(ids) == 0 {
		return []model.Media{}, nil
	}
	rows, err := s.repo.Media.FindByIDs(ctx, ids)
	if err != nil {
		return nil, err
	}
	s.attachLibraryMetadata(ctx, rows)
	// FindByIDs does not promise ordering. Rebuild the candidate order before
	// the existing season/episode stable sort so equal keys keep their prior
	// deterministic mediaLibraryListOrder tie-breaker.
	byID := make(map[string]model.Media, len(rows))
	for _, row := range rows {
		byID[row.ID] = row
	}
	out := make([]model.Media, 0, len(ids))
	for _, id := range ids {
		if row, ok := byID[id]; ok {
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
	return groupMediaSeriesCardsWithOrder(items, true)
}

func groupMediaSearchCards(items []model.Media) []SeriesCard {
	return groupMediaSeriesCardsWithOrder(items, false)
}

func groupMediaSeriesCardsWithOrder(items []model.Media, sortByLatest bool) []SeriesCard {
	if len(items) == 0 {
		return nil
	}
	groups := make([]seriesCardGroup, 0)
	byKey := make(map[string]int, len(items))
	for _, item := range items {
		key := mediaSeriesKey(item)
		if key == "" {
			continue
		}
		if idx, ok := byKey[key]; ok {
			group := &groups[idx]
			if latest := seriesMediaTime(item); latest.After(group.latest) {
				group.latest = latest
			}
			card := &group.card
			card.Count++
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
	if sortByLatest {
		sort.SliceStable(groups, func(i, j int) bool {
			return groups[i].latest.After(groups[j].latest)
		})
	}
	cards := make([]SeriesCard, 0, len(groups))
	for _, group := range groups {
		cards = append(cards, group.card)
	}
	return cards
}

func seriesMediaTime(media model.Media) time.Time {
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
