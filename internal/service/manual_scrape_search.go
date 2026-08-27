package service

import (
	"context"
	"errors"
	"strings"

	"github.com/ShukeBta/MediaStationGo/internal/model"
)

func (s *ScraperService) ManualSearch(ctx context.Context, media *model.Media, query, provider, mediaType string) ([]ExternalMediaResult, error) {
	if s == nil || media == nil {
		return nil, errors.New("media required")
	}
	lib, _ := s.repo.Library.FindByID(ctx, media.LibraryID)
	queries := s.manualSearchQueries(ctx, media, lib, query)
	if len(queries) == 0 {
		return nil, errors.New("search query required")
	}
	if mediaType == "" {
		if mediaIsEpisodic(media, lib) {
			mediaType = "tv"
		} else if lib != nil {
			mediaType = lib.Type
		}
	}
	mediaType = normalizeMediaType(mediaType, queries[0], "")
	providers := manualSearchProviderSet(provider)
	year := mediaYearHint(media)
	if year <= 0 {
		_, year = CleanQueryWithRecognition(ctx, s.repo, queries[0])
	}

	out := make([]ExternalMediaResult, 0, 6)
	add := func(source, typ string, match *Match) {
		if match == nil || strings.TrimSpace(match.Title) == "" {
			return
		}
		out = append(out, ExternalMediaResult{
			Source:           source,
			MediaType:        typ,
			Title:            match.Title,
			OriginalName:     match.OriginalName,
			Overview:         match.Overview,
			PosterURL:        match.PosterURL,
			BackdropURL:      match.BackdropURL,
			Year:             match.Year,
			ReleaseDate:      match.ReleaseDate,
			Rating:           match.Rating,
			TMDbID:           match.TMDbID,
			BangumiID:        match.BangumiID,
			DoubanID:         match.DoubanID,
			TheTVDBID:        match.TheTVDBID,
			SubscribeKeyword: buildSubscribeKeyword(match.Title, match.Year),
			SubscribeAliases: buildSubscribeAliases(match.Title, match.OriginalName, match.Year),
			Languages:        match.Languages,
			Countries:        match.Countries,
			Genres:           match.Genres,
			Actors:           match.Actors,
			People:           match.People,
			NSFW:             match.NSFW,
		})
	}

	if providers.want("adult") {
		adultQueries := make([]string, 0, len(queries))
		adultMedia := media
		if code := normalizeAdultCode(query); code != "" {
			adultQueries = append(adultQueries, code)
			adultMedia = nil
		} else {
			for _, candidateQuery := range queries {
				if externalIDHintsFromText(candidateQuery).useful() {
					continue
				}
				adultQueries = append(adultQueries, candidateQuery)
			}
		}
		for _, match := range s.manualAdultMatches(ctx, adultMedia, adultQueries) {
			add("adult", "adult", match)
		}
	}
	if providers.want("tmdb") {
		for _, candidateQuery := range queries {
			for _, candidate := range s.manualTMDbCandidates(ctx, candidateQuery, year, mediaType) {
				add("tmdb", candidate.MediaType, candidate.Match)
			}
		}
	}
	if providers.want("douban") {
		for _, candidateQuery := range queries {
			if match := s.manualDoubanMatch(ctx, candidateQuery); match != nil {
				add("douban", normalizeMediaType(mediaType, candidateQuery, ""), match)
			}
		}
	}
	if providers.want("bangumi") {
		for _, candidateQuery := range queries {
			if match := s.manualBangumiMatch(ctx, candidateQuery); match != nil {
				add("bangumi", "anime", match)
			}
		}
	}
	if providers.want("thetvdb") {
		for _, candidateQuery := range queries {
			if match := s.manualTheTVDBMatch(ctx, candidateQuery); match != nil {
				add("thetvdb", "tv", match)
			}
		}
	}
	return dedupeExternalMedia(out), nil
}

type manualSearchProviders map[string]struct{}

func manualSearchProviderSet(provider string) manualSearchProviders {
	out := manualSearchProviders{}
	for _, field := range strings.FieldsFunc(provider, func(r rune) bool {
		return r == ',' || r == ';' || r == '|' || r == ' '
	}) {
		field = strings.ToLower(strings.TrimSpace(field))
		if field == "" || field == "all" {
			return nil
		}
		out[field] = struct{}{}
	}
	if len(out) == 0 {
		return nil
	}
	return out
}

func (p manualSearchProviders) want(provider string) bool {
	if len(p) == 0 {
		return true
	}
	_, ok := p[provider]
	return ok
}

func (s *ScraperService) manualSearchQueries(ctx context.Context, media *model.Media, lib *model.Library, query string) []string {
	seen := map[string]struct{}{}
	out := make([]string, 0, 4)
	add := func(value string) {
		value = strings.Join(strings.Fields(strings.TrimSpace(value)), " ")
		if value == "" {
			return
		}
		key := strings.ToLower(value)
		if _, ok := seen[key]; ok {
			return
		}
		seen[key] = struct{}{}
		out = append(out, value)
	}

	recognizedQuery := ApplyRecognitionWords(ctx, s.repo, query)
	add(recognizedQuery)
	if externalIDHintsFromText(recognizedQuery).useful() {
		return out
	}
	if strings.TrimSpace(query) == "" && media != nil {
		add(firstText(media.Title, media.OriginalName))
	}
	if media != nil {
		for _, candidate := range scrapeQueryCandidatesWithRecognition(ctx, s.repo, media, lib) {
			add(candidate)
		}
		if len(out) == 0 {
			title, _ := CleanQueryWithRecognition(ctx, s.repo, media.Path)
			add(title)
		}
	}
	return out
}
