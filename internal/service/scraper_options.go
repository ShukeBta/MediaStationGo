package service

type ScrapeOptions struct {
	manualEpisodeIdentity  *episodeRef
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
