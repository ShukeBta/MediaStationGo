package service

import (
	"context"
	"errors"
	"fmt"
	"sort"
	"strconv"
	"strings"
	"time"

	"github.com/ShukeBta/MediaStationGo/internal/model"
	"github.com/ShukeBta/MediaStationGo/internal/repository"
)

var ErrInvalidLibraryBrowseFilter = errors.New("unsupported library filter")

type LibraryBrowseOptions struct {
	Page                                                                              int
	Query, Sort, Category, Genre, Language, Actor, AdultType, SeriesKey, FocusMediaID string
	YearFrom, YearTo                                                                  int
	IncludeFacets                                                                     bool
}
type LibraryBrowseFacet struct {
	Name  string `json:"name"`
	Count int    `json:"count"`
}
type LibraryBrowseFacets struct {
	Categories []LibraryBrowseFacet `json:"categories"`
	Genres     []LibraryBrowseFacet `json:"genres"`
	Years      []LibraryBrowseFacet `json:"years"`
	Languages  []LibraryBrowseFacet `json:"languages"`
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
	cacheKey := s.libraryBrowseCacheKey(libraryID, options, visibility)
	if s.cache != nil {
		var cached LibraryBrowsePage
		if s.cache.GetJSON(ctx, cacheKey, &cached) {
			return cached, nil
		}
	}
	cacheResult := func(page LibraryBrowsePage) {
		if s.cache != nil {
			s.cache.SetJSON(ctx, cacheKey, page, time.Duration(s.mediaCacheTTLSeconds())*time.Second)
		}
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
	if options.Actor != "" && (lib.Type != "adult" || out.IsSeries) {
		return out, fmt.Errorf("%w: actor", ErrInvalidLibraryBrowseFilter)
	}
	if options.AdultType != "" && (lib.Type != "adult" || out.IsSeries || (options.AdultType != "AV" && options.AdultType != "FC2")) {
		return out, fmt.Errorf("%w: adult type", ErrInvalidLibraryBrowseFilter)
	}
	if !validLibraryBrowseSort(options.Sort) {
		return out, fmt.Errorf("%w: sort", ErrInvalidLibraryBrowseFilter)
	}
	if options.YearFrom < 0 || options.YearTo < 0 || (options.YearFrom > 0 && options.YearTo > 0 && options.YearFrom > options.YearTo) {
		return out, fmt.Errorf("%w: year", ErrInvalidLibraryBrowseFilter)
	}
	needsMetadata := options.IncludeFacets || options.Query != "" || options.Sort != "" || options.Category != "" || options.Genre != "" || options.YearFrom != 0 || options.YearTo != 0 || options.Language != "" || options.Actor != "" || options.AdultType != "" || options.FocusMediaID != ""
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
				sortLibraryBrowseSeries(selected, options.Sort)
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
				cacheResult(out)
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
		type selectedMovie struct {
			key string
			rep model.Media
		}
		selected := []selectedMovie{}
		for _, group := range groups {
			rep, ok := byID[group.RepresentativeID]
			if !ok {
				return out, fmt.Errorf("browse representative %q disappeared; retry the request", group.RepresentativeID)
			}
			if browseMatches(rep, options) {
				selected = append(selected, selectedMovie{key: group.Key, rep: rep})
			}
		}
		sort.SliceStable(selected, func(i, j int) bool {
			return browseMediaLess(selected[i].rep, selected[j].rep, options.Sort)
		})
		out.Total = int64(len(selected))
		focusKey := ""
		if options.FocusMediaID != "" {
			media, e := s.repo.Media.FindByID(ctx, options.FocusMediaID)
			if e != nil {
				return out, e
			}
			if browseFocusVisible(media, ids, visibility) {
				focusKey = media.MediaVersionKey
				for i, selectedItem := range selected {
					if selectedItem.key == media.MediaVersionKey {
						out.Page = i/48 + 1
						break
					}
				}
			}
		}
		start, end := browsePageBounds(&out, len(selected))
		selectedKeys := make([]string, end-start)
		for i := start; i < end; i++ {
			selectedKeys[i-start] = selected[i].key
		}
		out.Items, err = s.hydrateMediaVersionGroups(ctx, selectedKeys, ids, filter)
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
	cacheResult(out)
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
	if o.Query != "" {
		query := strings.ToLower(strings.TrimSpace(o.Query))
		searchable := strings.ToLower(strings.Join([]string{m.Title, m.OriginalName, m.DisplayTitle}, "\n"))
		if !strings.Contains(searchable, query) {
			return false
		}
	}
	if o.Category != "" && !strings.EqualFold(strings.TrimSpace(m.AutoCategory), o.Category) {
		return false
	}
	if o.Genre != "" && !browseCSVContains(m.Genres, o.Genre) {
		return false
	}
	if o.YearFrom != 0 && m.Year < o.YearFrom {
		return false
	}
	if o.YearTo != 0 && m.Year > o.YearTo {
		return false
	}
	if o.Language != "" && !browseLanguageCSVContains(m.Languages, o.Language) {
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
	categories, genres, years, languages := map[string]LibraryBrowseFacet{}, map[string]LibraryBrowseFacet{}, map[string]LibraryBrowseFacet{}, map[string]LibraryBrowseFacet{}
	types := map[string]LibraryBrowseFacet{}
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
		addBrowseCSVFacets(genres, row.Genres, add)
		if row.Year > 0 {
			add(years, strconv.Itoa(row.Year))
		}
		addBrowseLanguageFacets(languages, row.Languages, add)
		if adult {
			add(types, row.AdultType)
		}
	}
	list := func(values map[string]LibraryBrowseFacet, descending bool) []LibraryBrowseFacet {
		out := make([]LibraryBrowseFacet, 0, len(values))
		for _, v := range values {
			out = append(out, v)
		}
		sort.Slice(out, func(i, j int) bool {
			if descending {
				return out[i].Name > out[j].Name
			}
			return out[i].Name < out[j].Name
		})
		return out
	}
	return &LibraryBrowseFacets{
		Categories: list(categories, false),
		Genres:     list(genres, false),
		Years:      list(years, true),
		Languages:  list(languages, false),
		Actors:     []LibraryBrowseFacet{},
		AdultTypes: list(types, false),
	}
}

func validLibraryBrowseSort(value string) bool {
	switch value {
	case "", "rating", "year", "title":
		return true
	default:
		return false
	}
}

func sortLibraryBrowseSeries(cards []SeriesCard, sortBy string) {
	if sortBy == "" {
		return
	}
	sort.SliceStable(cards, func(i, j int) bool {
		return browseMediaLess(cards[i].Rep, cards[j].Rep, sortBy)
	})
}

func browseMediaLess(left, right model.Media, sortBy string) bool {
	switch sortBy {
	case "rating":
		if left.Rating != right.Rating {
			return left.Rating > right.Rating
		}
		if left.Year != right.Year {
			return left.Year > right.Year
		}
	case "year":
		if left.Year != right.Year {
			return left.Year > right.Year
		}
		if left.Rating != right.Rating {
			return left.Rating > right.Rating
		}
	case "title":
		leftTitle := strings.ToLower(strings.TrimSpace(firstNonEmpty(left.DisplayTitle, left.Title)))
		rightTitle := strings.ToLower(strings.TrimSpace(firstNonEmpty(right.DisplayTitle, right.Title)))
		if leftTitle != rightTitle {
			return leftTitle < rightTitle
		}
	default:
		return false
	}
	return strings.ToLower(left.Title) < strings.ToLower(right.Title)
}

func browseCSVContains(value, expected string) bool {
	expected = strings.TrimSpace(expected)
	if expected == "" {
		return true
	}
	for _, item := range strings.Split(value, ",") {
		if strings.EqualFold(strings.TrimSpace(item), expected) {
			return true
		}
	}
	return false
}

func browseLanguageCSVContains(value, expected string) bool {
	expected = canonicalBrowseLanguage(expected)
	if expected == "" {
		return true
	}
	for _, item := range strings.Split(value, ",") {
		if canonicalBrowseLanguage(item) == expected {
			return true
		}
	}
	return false
}

func canonicalBrowseLanguage(value string) string {
	value = strings.ToLower(strings.TrimSpace(value))
	switch value {
	case "cn", "zh-cn", "zh-tw", "zh-hans", "zh-hant":
		return "zh"
	case "jp":
		return "ja"
	case "kr":
		return "ko"
	default:
		return value
	}
}

func addBrowseCSVFacets(values map[string]LibraryBrowseFacet, raw string, add func(map[string]LibraryBrowseFacet, string)) {
	seen := map[string]bool{}
	for _, item := range strings.Split(raw, ",") {
		name := strings.TrimSpace(item)
		key := strings.ToLower(name)
		if name == "" || seen[key] {
			continue
		}
		seen[key] = true
		add(values, name)
	}
}

func addBrowseLanguageFacets(values map[string]LibraryBrowseFacet, raw string, add func(map[string]LibraryBrowseFacet, string)) {
	seen := map[string]bool{}
	for _, item := range strings.Split(raw, ",") {
		name := canonicalBrowseLanguage(item)
		if name == "" || seen[name] {
			continue
		}
		seen[name] = true
		add(values, name)
	}
}
