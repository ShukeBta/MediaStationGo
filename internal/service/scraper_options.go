package service

import "github.com/ShukeBta/MediaStationGo/internal/model"

type ScrapeOptions struct {
	manualEpisodeEnd       int
	manualEpisodePart      int
	sourceSnapshot         *model.Media
	tvLookup               map[tvLookupKey]tvLookupResult
	episodeFailures        map[[2]int]error
	manualEpisodeIdentity  *episodeRef
	manualMatch            *Match
	automaticSelection     bool
	episodeValidation      map[[2]int]map[int]*TMDbEpisodeDetails
	RetryNoMatch           bool
	IncludeMatched         bool
	RefreshWeakMatched     bool
	DeferEpisodeDetails    bool
	deferTMDbDetails       bool
	deferPeople            bool
	deferCacheInvalidation bool
}
