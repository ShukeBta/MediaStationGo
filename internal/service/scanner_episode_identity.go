package service

import "github.com/ShukeBta/MediaStationGo/internal/model"

// scannedMediaEpisodeIdentity keeps permissive filename parsing for series
// libraries, but requires an explicit season and episode marker elsewhere.
// This prevents movie ordinals such as 【01】 from becoming S01E01 while still
// accepting unambiguous SxxEyy paths in legacy mixed libraries.
func scannedMediaEpisodeIdentity(lib *model.Library, path string) (season, episode int, trusted bool) {
	evidence := ParseEpisodeEvidence(path)
	if evidence.Episode <= 0 {
		return 0, 0, false
	}
	if librarySupportsSeasons(lib) || evidence.SeasonExplicit && evidence.EpisodeExplicit {
		return evidence.Season, evidence.Episode, true
	}
	return 0, 0, false
}

func clearUntrustedEpisodeMetadata(media *model.Media) {
	if media == nil {
		return
	}
	media.SeasonNum = 0
	media.EpisodeNum = 0
	media.EpisodeTitle = ""
}
