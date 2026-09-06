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
	filter := repository.MediaQueryFilter{
		IncludeNSFW:       visibility.IncludeNSFW,
		AllowedLibraryIDs: visibility.AllowedLibraryIDs,
		HiddenLibraryIDs:  visibility.HiddenLibraryIDs,
	}
	if candidates, complete, err := s.repo.Media.ListPersistedSeriesCardGroups(ctx, libraryIDs, filter); err != nil {
		return nil, 0, err
	} else if complete {
		cards := s.persistedSeriesCards(ctx, candidates)
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
		pageCards, err := s.resolvePersistedSeriesCards(ctx, candidates, cards[start:end], filter)
		if err != nil {
			return nil, 0, err
		}
		pageCards, err = s.hydrateSeriesCards(ctx, pageCards)
		if err != nil {
			return nil, 0, err
		}
		return pageCards, total, nil
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
	ctx, err := s.withMediaLibraryMetadata(ctx)
	if err != nil {
		return nil, err
	}
	visibility = ExpandMediaVisibilityForMergedCloudLibraries(ctx, s.repo, visibility)
	filter := repository.MediaQueryFilter{
		IncludeNSFW:       visibility.IncludeNSFW,
		AllowedLibraryIDs: visibility.AllowedLibraryIDs,
		HiddenLibraryIDs:  visibility.HiddenLibraryIDs,
	}
	if persisted, complete, err := s.repo.Media.ListPersistedSeriesCardGroups(ctx, nil, filter); err != nil {
		return nil, err
	} else if complete {
		cards := s.persistedSeriesCards(ctx, persisted)
		if len(cards) > limit {
			cards = cards[:limit]
		}
		cards, err = s.resolvePersistedSeriesCards(ctx, persisted, cards, filter)
		if err != nil {
			return nil, err
		}
		return s.hydrateSeriesCards(ctx, cards)
	}
	rows, err := s.repo.Media.ListSeriesCardCandidatesFiltered(ctx, maxMediaSearchLimit, repository.MediaQueryFilter{
		IncludeNSFW:       visibility.IncludeNSFW,
		AllowedLibraryIDs: visibility.AllowedLibraryIDs,
		HiddenLibraryIDs:  visibility.HiddenLibraryIDs,
	})
	if err != nil {
		return nil, err
	}
	s.attachLibraryDisplayMetadata(ctx, rows)
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
	if candidates, complete, err := s.repo.Media.ListPersistedSeriesCardGroups(ctx, libraryIDs, repository.MediaQueryFilter{
		IncludeNSFW:       visibility.IncludeNSFW,
		AllowedLibraryIDs: visibility.AllowedLibraryIDs,
		HiddenLibraryIDs:  visibility.HiddenLibraryIDs,
	}); err != nil {
		return nil, err
	} else if complete {
		persistedKeys := s.persistedSeriesKeysForPublicKey(ctx, candidates, key)
		if len(persistedKeys) == 0 {
			return []model.Media{}, nil
		}
		rows, err := s.repo.Media.ListMediaBySeriesKeysFiltered(ctx, libraryIDs, persistedKeys, repository.MediaQueryFilter{
			IncludeNSFW:       visibility.IncludeNSFW,
			AllowedLibraryIDs: visibility.AllowedLibraryIDs,
			HiddenLibraryIDs:  visibility.HiddenLibraryIDs,
		})
		if err != nil {
			return nil, err
		}
		s.attachLibraryMetadata(ctx, rows)
		filtered := rows[:0]
		for i := range rows {
			if mediaSeriesKey(rows[i]) == key {
				filtered = append(filtered, rows[i])
			}
		}
		rows = filtered
		sort.SliceStable(rows, func(i, j int) bool {
			if rows[i].SeasonNum != rows[j].SeasonNum {
				return rows[i].SeasonNum < rows[j].SeasonNum
			}
			if rows[i].EpisodeNum != rows[j].EpisodeNum {
				return rows[i].EpisodeNum < rows[j].EpisodeNum
			}
			return rows[i].CreatedAt.Before(rows[j].CreatedAt)
		})
		return rows, nil
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

func (s *MediaService) persistedSeriesCards(ctx context.Context, candidates []repository.SeriesCardGroupCandidate) []SeriesCard {
	if len(candidates) == 0 {
		return []SeriesCard{}
	}
	items := make([]model.Media, len(candidates))
	for i := range candidates {
		items[i] = candidates[i].Media()
	}
	s.attachLibraryDisplayMetadata(ctx, items)

	// SQL first collapses each physical library so the database only returns a
	// small candidate set. A second, cheap pass recalculates the existing public
	// key after display-library resolution. This preserves historical URLs while
	// still merging physical libraries that represent the same logical library.
	cards := make([]SeriesCard, 0, len(candidates))
	byGroup := make(map[string]int, len(candidates))
	for i, candidate := range candidates {
		item := items[i]
		publicKey := mediaSeriesKey(item)
		if publicKey == "" {
			continue
		}
		if index, ok := byGroup[publicKey]; ok {
			card := &cards[index]
			card.Count += int(candidate.SeriesCount)
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
		byGroup[publicKey] = len(cards)
		cards = append(cards, SeriesCard{Key: publicKey, Rep: item, LinkMedia: item, Count: int(candidate.SeriesCount)})
	}
	return cards
}

// resolvePersistedSeriesCards loads episode-level candidate fields only for
// the logical cards already selected by pagination or Top-N ranking. The
// aggregate query remains the source of group membership and counts; this
// pass applies the same representative ordering as the former SQL window,
// without evaluating artwork rules over the whole library on every request.
func (s *MediaService) resolvePersistedSeriesCards(
	ctx context.Context,
	candidates []repository.SeriesCardGroupCandidate,
	selected []SeriesCard,
	filter repository.MediaQueryFilter,
) ([]SeriesCard, error) {
	if len(selected) == 0 {
		return []SeriesCard{}, nil
	}
	selectedKeys := make(map[string]struct{}, len(selected))
	for i := range selected {
		selectedKeys[selected[i].Key] = struct{}{}
	}

	samples := make([]model.Media, len(candidates))
	for i := range candidates {
		samples[i] = candidates[i].Media()
	}
	s.attachLibraryDisplayMetadata(ctx, samples)

	groupKeys := make([]repository.SeriesCardGroupKey, 0, len(selected))
	publicKeys := make([]string, len(candidates))
	for i := range candidates {
		publicKey := mediaSeriesKey(samples[i])
		publicKeys[i] = publicKey
		if _, wanted := selectedKeys[publicKey]; !wanted {
			continue
		}
		groupKeys = append(groupKeys, repository.SeriesCardGroupKey{
			LibraryID: candidates[i].LibraryID,
			SeriesKey: candidates[i].SeriesKey,
		})
	}
	rows, err := s.repo.Media.ListMediaBySeriesCardGroupsFiltered(ctx, groupKeys, filter)
	if err != nil {
		return nil, err
	}
	s.attachLibraryDisplayMetadata(ctx, rows)
	byPhysicalGroup := make(map[repository.SeriesCardGroupKey][]model.Media, len(groupKeys))
	for i := range rows {
		key := repository.SeriesCardGroupKey{LibraryID: rows[i].LibraryID, SeriesKey: rows[i].SeriesKey}
		byPhysicalGroup[key] = append(byPhysicalGroup[key], rows[i])
	}

	resolvedCandidates := make([]repository.SeriesCardGroupCandidate, 0, len(groupKeys))
	for i := range candidates {
		publicKey := publicKeys[i]
		if _, wanted := selectedKeys[publicKey]; !wanted {
			continue
		}
		physicalKey := repository.SeriesCardGroupKey{
			LibraryID: candidates[i].LibraryID,
			SeriesKey: candidates[i].SeriesKey,
		}
		groupRows := byPhysicalGroup[physicalKey]
		if len(groupRows) == 0 {
			return nil, fmt.Errorf("resolve persisted series group %q/%q: no active media rows", physicalKey.LibraryID, physicalKey.SeriesKey)
		}
		representative := groupRows[0]
		for j := 1; j < len(groupRows); j++ {
			if betterPersistedSeriesRepresentative(groupRows[j], representative) {
				representative = groupRows[j]
			}
		}
		if resolvedKey := mediaSeriesKey(representative); resolvedKey != publicKey {
			return nil, fmt.Errorf("resolve persisted series group %q/%q: public key changed from %q to %q", physicalKey.LibraryID, physicalKey.SeriesKey, publicKey, resolvedKey)
		}
		resolvedCandidates = append(resolvedCandidates, candidates[i].WithMedia(representative))
	}

	resolved := s.persistedSeriesCards(ctx, resolvedCandidates)
	byPublicKey := make(map[string]SeriesCard, len(resolved))
	for i := range resolved {
		byPublicKey[resolved[i].Key] = resolved[i]
	}
	out := make([]SeriesCard, 0, len(selected))
	for i := range selected {
		card, ok := byPublicKey[selected[i].Key]
		if !ok {
			return nil, fmt.Errorf("resolve persisted series card %q: selected group is missing", selected[i].Key)
		}
		out = append(out, card)
	}
	return out, nil
}

func betterPersistedSeriesRepresentative(candidate, current model.Media) bool {
	candidateScore := seriesArtworkScore(candidate)
	currentScore := seriesArtworkScore(current)
	if candidateScore != currentScore {
		return candidateScore > currentScore
	}
	candidateEpisode := candidate.SeasonNum*10000 + candidate.EpisodeNum
	currentEpisode := current.SeasonNum*10000 + current.EpisodeNum
	if candidateEpisode != currentEpisode {
		return candidateEpisode < currentEpisode
	}
	if !candidate.CreatedAt.Equal(current.CreatedAt) {
		return candidate.CreatedAt.After(current.CreatedAt)
	}
	return candidate.ID > current.ID
}

func (s *MediaService) persistedSeriesKeysForPublicKey(ctx context.Context, candidates []repository.SeriesCardGroupCandidate, publicKey string) []string {
	if len(candidates) == 0 || strings.TrimSpace(publicKey) == "" {
		return nil
	}
	items := make([]model.Media, len(candidates))
	for i := range candidates {
		items[i] = candidates[i].Media()
	}
	s.attachLibraryDisplayMetadata(ctx, items)
	keys := make([]string, 0, 1)
	seen := make(map[string]struct{}, 1)
	for i := range candidates {
		if mediaSeriesKey(items[i]) != publicKey {
			continue
		}
		key := candidates[i].SeriesKey
		if key == "" {
			continue
		}
		if _, exists := seen[key]; exists {
			continue
		}
		seen[key] = struct{}{}
		keys = append(keys, key)
	}
	return keys
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
