package service

type ScrapeOptions struct {
	RetryNoMatch           bool
	IncludeMatched         bool
	RefreshWeakMatched     bool
	DeferEpisodeDetails    bool
	deferTMDbDetails       bool
	deferPeople            bool
	deferCacheInvalidation bool
}
