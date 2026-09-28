package service

import (
	"context"
	"fmt"
	"sort"

	"github.com/ShukeBta/MediaStationGo/internal/model"
	"github.com/ShukeBta/MediaStationGo/internal/repository"
)

// expandPersistedSeriesGroups refines only physical groups whose public
// identity can change after attaching library metadata. It runs before public
// grouping, pagination and rating qualification so each resulting candidate
// carries the count, rating and latest time of its actual public members.
func (s *MediaService) expandPersistedSeriesGroups(ctx context.Context, candidates []repository.SeriesCardGroupCandidate, filter repository.MediaQueryFilter) ([]repository.SeriesCardGroupCandidate, error) {
	if len(candidates) == 0 {
		return candidates, nil
	}
	ctx, err := s.withMediaLibraryMetadata(ctx)
	if err != nil {
		return nil, err
	}
	snapshot, _ := ctx.Value(mediaLibraryMetadataContextKey{}).(*mediaLibraryMetadataSnapshot)
	pathDependentLibraries := seriesProjectionPathDependentLibraries(snapshot)
	projected := make([]model.Media, len(candidates))
	for i := range candidates {
		projected[i] = candidates[i].Media()
	}
	s.attachLibraryDisplayMetadata(ctx, projected)

	groups := make([]repository.SeriesCardGroupKey, 0)
	needsExpansion := make(map[repository.SeriesCardGroupKey]struct{})
	for i := range candidates {
		candidate := candidates[i]
		if candidate.ProjectionResolved || candidate.SeriesCount <= 1 {
			continue
		}
		if !persistedSeriesProjectionCanSplit(candidate.Media(), projected[i], pathDependentLibraries[candidate.LibraryID]) {
			continue
		}
		key := repository.SeriesCardGroupKey{LibraryID: candidate.LibraryID, SeriesKey: candidate.SeriesKey}
		if _, exists := needsExpansion[key]; !exists {
			groups = append(groups, key)
			needsExpansion[key] = struct{}{}
		}
	}
	if len(groups) == 0 {
		return candidates, nil
	}

	rows, err := s.repo.Media.ListAllMediaBySeriesCardGroupsFiltered(ctx, groups, filter)
	if err != nil {
		return nil, fmt.Errorf("expand persisted series groups: %w", err)
	}
	s.attachLibraryDisplayMetadata(ctx, rows)
	byPhysicalGroup := make(map[repository.SeriesCardGroupKey][]model.Media, len(groups))
	for _, row := range rows {
		key := repository.SeriesCardGroupKey{LibraryID: row.LibraryID, SeriesKey: row.SeriesKey}
		byPhysicalGroup[key] = append(byPhysicalGroup[key], row)
	}

	expanded := make([]repository.SeriesCardGroupCandidate, 0, len(candidates))
	for _, candidate := range candidates {
		physicalKey := repository.SeriesCardGroupKey{LibraryID: candidate.LibraryID, SeriesKey: candidate.SeriesKey}
		if _, needed := needsExpansion[physicalKey]; !needed {
			expanded = append(expanded, candidate)
			continue
		}
		members := byPhysicalGroup[physicalKey]
		if len(members) == 0 {
			return nil, fmt.Errorf("expand persisted series group %q/%q: no active media rows", physicalKey.LibraryID, physicalKey.SeriesKey)
		}
		byPublicKey := make(map[string]int)
		for _, member := range members {
			publicKey := mediaSeriesKey(member)
			if publicKey == "" {
				return nil, fmt.Errorf("expand persisted series group %q/%q: media %q has no public key", physicalKey.LibraryID, physicalKey.SeriesKey, member.ID)
			}
			index, exists := byPublicKey[publicKey]
			if !exists {
				resolved := candidate.WithMedia(member)
				resolved.SeriesCount = 0
				resolved.RatingSum = 0
				resolved.RatingCount = 0
				resolved.SeriesLatest = member.CreatedAt
				resolved.ProjectionResolved = true
				index = len(expanded)
				byPublicKey[publicKey] = index
				expanded = append(expanded, resolved)
			}
			resolved := &expanded[index]
			if betterPersistedSeriesRepresentative(member, resolved.Media()) {
				*resolved = resolved.WithMedia(member)
			}
			resolved.SeriesCount++
			if member.Rating > 0 {
				resolved.RatingSum += float64(member.Rating)
				resolved.RatingCount++
			}
			if member.CreatedAt.After(resolved.SeriesLatest) {
				resolved.SeriesLatest = member.CreatedAt
			}
		}
	}
	// Splitting a recent physical group can reveal an older public work. Sort
	// all candidates again, including untouched groups between those dates.
	sort.SliceStable(expanded, func(i, j int) bool {
		a, b := expanded[i], expanded[j]
		if !a.SeriesLatest.Equal(b.SeriesLatest) {
			return a.SeriesLatest.After(b.SeriesLatest)
		}
		if a.LibraryID != b.LibraryID {
			return a.LibraryID > b.LibraryID
		}
		if a.SeriesKey != b.SeriesKey {
			return a.SeriesKey > b.SeriesKey
		}
		return a.ID > b.ID
	})
	return expanded, nil
}

func persistedSeriesProjectionCanSplit(raw, projected model.Media, pathDependentLibrary bool) bool {
	if pathDependentLibrary {
		return true
	}
	pathTitle := seriesTitleFromMediaPath(raw.Path)
	if seriesTitleIsGenericContainer(pathTitle, raw) != seriesTitleIsGenericContainer(pathTitle, projected) {
		return true
	}
	return seriesProjectionIsEpisodic(raw) != seriesProjectionIsEpisodic(projected)
}

func seriesProjectionIsEpisodic(media model.Media) bool {
	return media.SeasonNum > 0 || media.EpisodeNum > 0 || episodicPathRE.MatchString(media.Path+" "+media.DisplayLibraryPath+" "+media.LibraryPath)
}

func seriesProjectionPathDependentLibraries(snapshot *mediaLibraryMetadataSnapshot) map[string]bool {
	dependent := make(map[string]bool)
	if snapshot == nil {
		return dependent
	}
	for id, own := range snapshot.byID {
		// An auto-category assignment always resolves from the owning library,
		// even when an individual media path points into another visible root.
		if CloudLibraryAutoCategory(own) {
			continue
		}
		for _, display := range snapshot.resolver.displayLibraries {
			if display.ID != id && display.Enabled && seriesLibraryPathCanShadow(own, display) {
				dependent[id] = true
				break
			}
		}
	}
	return dependent
}
