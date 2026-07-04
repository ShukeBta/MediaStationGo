package service

type ScrapeOptions struct {
	RetryNoMatch        bool
	IncludeMatched      bool
	RefreshWeakMatched  bool
	EpisodeArtwork      *bool
	DeferEpisodeDetails bool
}

func (o ScrapeOptions) episodeArtworkEnabled() bool {
	return o.EpisodeArtwork == nil || *o.EpisodeArtwork
}

func skipEpisodeArtworkOptions(retryNoMatch bool) ScrapeOptions {
	episodeArtwork := false
	return ScrapeOptions{RetryNoMatch: retryNoMatch, EpisodeArtwork: &episodeArtwork}
}
