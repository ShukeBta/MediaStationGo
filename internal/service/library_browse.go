package service

import (
	"context"
	"errors"
	"fmt"
	"sort"
	"strings"

	"github.com/ShukeBta/MediaStationGo/internal/model"
	"github.com/ShukeBta/MediaStationGo/internal/repository"
)

var ErrInvalidLibraryBrowseFilter = errors.New("unsupported library filter")

type LibraryBrowseOptions struct {
	Page                                                int
	Category, Actor, AdultType, SeriesKey, FocusMediaID string
	IncludeFacets                                       bool
}
type LibraryBrowseFacet struct {
	Name  string `json:"name"`
	Count int    `json:"count"`
}
type LibraryBrowseFacets struct {
	Categories []LibraryBrowseFacet `json:"categories"`
	Actors     []LibraryBrowseFacet `json:"actors"`
	AdultTypes []LibraryBrowseFacet `json:"adult_types"`
}
type LibraryBrowsePage struct {
	Items          []MediaItem          `json:"items"`
	SeriesCards    []SeriesCard         `json:"series_cards"`
	IsSeries       bool                 `json:"is_series"`
	Total          int64                `json:"total"`
	Page           int                  `json:"page"`
	PageSize       int                  `json:"page_size"`
	Facets         *LibraryBrowseFacets `json:"facets,omitempty"`
	SelectedSeries *SeriesCard          `json:"selected_series,omitempty"`
	FocusedMediaID string               `json:"focused_media_id,omitempty"`
}

// BrowseLibrary retains the existing SQL group order and authoritative category
// rules. Global facets inspect group representatives; only the selected page is
// hydrated. Plain page changes keep the existing indexed SQL pagination path.
func (s *MediaService) BrowseLibrary(ctx context.Context, libraryID string, options LibraryBrowseOptions, visibility MediaVisibility) (LibraryBrowsePage, error) {
	out := LibraryBrowsePage{Items: []MediaItem{}, SeriesCards: []SeriesCard{}, Page: options.Page, PageSize: 48}
	if out.Page < 1 {
		out.Page = 1
	}
	lib, err := s.repo.Library.FindByID(ctx, libraryID)
	if err != nil {
		return out, err
	}
	if lib == nil || !LibraryVisibleForUser(ctx, s.repo, *lib, visibility) {
		return out, fmt.Errorf("library not found")
	}
	ctx, err = s.withMediaLibraryMetadata(ctx)
	if err != nil {
		return out, err
	}
	visibility = ExpandMediaVisibilityForMergedCloudLibraries(ctx, s.repo, visibility)
	ids, err := MergedLibraryIDsForLibrary(ctx, s.repo, libraryID)
	if err != nil {
		return out, err
	}
	filter := repository.MediaQueryFilter{IncludeNSFW: visibility.IncludeNSFW, AllowedLibraryIDs: visibility.AllowedLibraryIDs, HiddenLibraryIDs: visibility.HiddenLibraryIDs}
	out.IsSeries = lib.Type == "tv" || lib.Type == "anime" || lib.Type == "variety"
	if !out.IsSeries {
		out.IsSeries, err = s.repo.Media.LibraryHasEpisodes(ctx, ids, filter)
		if err != nil {
			return out, err
		}
	}
	if options.Actor != "" && (lib.Type != "adult" || out.IsSeries) {
		return out, fmt.Errorf("%w: actor", ErrInvalidLibraryBrowseFilter)
	}
	if options.AdultType != "" && (lib.Type != "adult" || out.IsSeries || (options.AdultType != "AV" && options.AdultType != "FC2")) {
		return out, fmt.Errorf("%w: adult type", ErrInvalidLibraryBrowseFilter)
	}
	needsMetadata := options.IncludeFacets || options.Category != "" || options.Actor != "" || options.AdultType != "" || options.FocusMediaID != ""
	if out.IsSeries {
		// A direct URL is resolved independently of the visible page and filters.
		if options.SeriesKey != "" || needsMetadata {
			candidates, e := s.listPersistedSeriesCardGroups(ctx, ids, filter)
			if e != nil {
				return out, e
			}
			cards := s.persistedSeriesCards(ctx, candidates)
			if options.SeriesKey != "" {
				for _, card := range cards {
					if card.Key == options.SeriesKey {
						resolved, e := s.resolvePersistedSeriesCards(ctx, candidates, []SeriesCard{card}, filter)
						if e != nil {
							return out, e
						}
						decorated := s.decorateSeriesCards(ctx, resolved)
						out.SelectedSeries = &decorated[0]
						break
					}
				}
			}
			if needsMetadata {
				metadata, e := s.resolvePersistedSeriesCardProjection(ctx, candidates, cards, filter, true)
				if e != nil {
					return out, e
				}
				metadata = s.decorateSeriesCards(ctx, metadata)
				reps := make([]model.Media, len(metadata))
				for i := range metadata {
					reps[i] = metadata[i].Rep
				}
				if options.IncludeFacets {
					out.Facets = buildLibraryBrowseFacets(reps, false)
				}
				selected := make([]SeriesCard, 0, len(metadata))
				for _, card := range metadata {
					if browseMatches(card.Rep, options) {
						selected = append(selected, card)
					}
				}
				out.Total = int64(len(selected))
				focusKey := ""
				if options.FocusMediaID != "" {
					media, e := s.repo.Media.FindByID(ctx, options.FocusMediaID)
					if e != nil {
						return out, e
					}
					if browseFocusVisible(media, ids, visibility) {
						rows := []model.Media{*media}
						s.attachLibraryDisplayMetadata(ctx, rows)
						focusKey = mediaSeriesKey(rows[0])
					}
				}
				for i := range selected {
					if focusKey != "" && selected[i].Key == focusKey {
						out.Page = i/48 + 1
						break
					}
				}
				start, end := browsePageBounds(&out, len(selected))
				resolved, e := s.resolvePersistedSeriesCards(ctx, candidates, selected[start:end], filter)
				if e != nil {
					return out, e
				}
				out.SeriesCards = s.decorateSeriesCards(ctx, resolved)
				for _, card := range out.SeriesCards {
					if card.Key == focusKey {
						out.FocusedMediaID = card.Rep.ID
					}
				}
				return out, nil
			}
		}
		out.SeriesCards, out.Total, err = s.ListLibrarySeriesCards(ctx, libraryID, out.Page, 48, visibility)
	} else if needsMetadata {
		complete, e := s.repo.Media.MediaVersionKeysComplete(ctx, ids, filter)
		if e != nil {
			return out, e
		}
		if !complete {
			_, err = s.ensureMediaVersionKeys(ctx, ids, filter)
		}
		if err != nil {
			return out, err
		}
		groups, _, e := s.repo.Media.ListMediaVersionGroupPage(ctx, ids, filter, 0, int(^uint(0)>>1))
		if e != nil {
			return out, e
		}
		repIDs := make([]string, len(groups))
		for i := range groups {
			repIDs[i] = groups[i].RepresentativeID
		}
		reps, e := s.repo.Media.ListMediaBrowseMetadata(ctx, repIDs, ids, filter)
		if e != nil {
			return out, e
		}
		for i := range reps {
			if reps[i].PartGroupKey != "" {
				reps[i].Title = firstNonEmpty(reps[i].PartGroupTitle, reps[i].Title)
			}
		}
		s.attachLibraryMetadata(ctx, reps)
		if options.IncludeFacets {
			out.Facets = buildLibraryBrowseFacets(reps, lib.Type == "adult")
		}
		byID := map[string]model.Media{}
		for _, rep := range reps {
			byID[rep.ID] = rep
		}
		selected := []string{}
		for _, group := range groups {
			rep, ok := byID[group.RepresentativeID]
			if !ok {
				return out, fmt.Errorf("browse representative %q disappeared; retry the request", group.RepresentativeID)
			}
			if browseMatches(rep, options) {
				selected = append(selected, group.Key)
			}
		}
		out.Total = int64(len(selected))
		focusKey := ""
		if options.FocusMediaID != "" {
			media, e := s.repo.Media.FindByID(ctx, options.FocusMediaID)
			if e != nil {
				return out, e
			}
			if browseFocusVisible(media, ids, visibility) {
				focusKey = media.MediaVersionKey
				for i, key := range selected {
					if key == media.MediaVersionKey {
						out.Page = i/48 + 1
						break
					}
				}
			}
		}
		start, end := browsePageBounds(&out, len(selected))
		out.Items, err = s.hydrateMediaVersionGroups(ctx, selected[start:end], ids, filter)
		for _, item := range out.Items {
			if item.MediaVersionKey == focusKey {
				out.FocusedMediaID = item.ID
			}
		}
	} else {
		out.Items, out.Total, err = s.ListMediaVisibleGrouped(ctx, libraryID, out.Page, 48, visibility)
	}
	if err != nil {
		return out, err
	}
	// Deletes can remove the last page. Return the actual last page explicitly.
	maxPage := max(1, int((out.Total+47)/48))
	if out.Page > maxPage {
		out.Page = maxPage
		if out.IsSeries {
			out.SeriesCards, out.Total, err = s.ListLibrarySeriesCards(ctx, libraryID, out.Page, 48, visibility)
		} else {
			out.Items, out.Total, err = s.ListMediaVisibleGrouped(ctx, libraryID, out.Page, 48, visibility)
		}
		if err != nil {
			return out, err
		}
		if out.Page > max(1, int((out.Total+47)/48)) {
			return out, fmt.Errorf("library changed during pagination; retry the request")
		}
	}
	return out, nil
}

func browseFocusVisible(media *model.Media, libraryIDs []string, visibility MediaVisibility) bool {
	if media == nil || !visibility.Allows(media) {
		return false
	}
	for _, id := range libraryIDs {
		if media.LibraryID == id {
			return true
		}
	}
	return false
}

func browsePageBounds(out *LibraryBrowsePage, n int) (int, int) {
	out.Page = min(out.Page, max(1, (n+47)/48))
	start := (out.Page - 1) * 48
	return start, min(start+48, n)
}
func browseMatches(m model.Media, o LibraryBrowseOptions) bool {
	if o.Category != "" && !strings.EqualFold(strings.TrimSpace(m.AutoCategory), o.Category) {
		return false
	}
	if o.AdultType != "" && !strings.EqualFold(m.AdultType, o.AdultType) {
		return false
	}
	if o.Actor != "" {
		for _, a := range strings.Split(m.Actors, ",") {
			if strings.EqualFold(strings.TrimSpace(a), o.Actor) {
				return true
			}
		}
		return false
	}
	return true
}
func buildLibraryBrowseFacets(rows []model.Media, adult bool) *LibraryBrowseFacets {
	categories, actors, types := map[string]LibraryBrowseFacet{}, map[string]LibraryBrowseFacet{}, map[string]LibraryBrowseFacet{}
	add := func(values map[string]LibraryBrowseFacet, name string) {
		name = strings.TrimSpace(name)
		if name == "" {
			return
		}
		key := strings.ToLower(name)
		v := values[key]
		v.Name = name
		v.Count++
		values[key] = v
	}
	for _, row := range rows {
		add(categories, row.AutoCategory)
		if adult {
			add(types, row.AdultType)
			seen := map[string]bool{}
			for _, a := range strings.Split(row.Actors, ",") {
				key := strings.ToLower(strings.TrimSpace(a))
				if !seen[key] {
					add(actors, a)
					seen[key] = true
				}
			}
		}
	}
	list := func(values map[string]LibraryBrowseFacet) []LibraryBrowseFacet {
		out := make([]LibraryBrowseFacet, 0, len(values))
		for _, v := range values {
			out = append(out, v)
		}
		sort.Slice(out, func(i, j int) bool { return out[i].Name < out[j].Name })
		return out
	}
	return &LibraryBrowseFacets{Categories: list(categories), Actors: list(actors), AdultTypes: list(types)}
}
