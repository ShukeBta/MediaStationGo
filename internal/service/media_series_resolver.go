package service

import (
	"fmt"
	"strings"

	"github.com/ShukeBta/MediaStationGo/internal/model"
)

type mediaSeriesKeyResolver struct {
	pathCounts     map[string]int
	externalCounts map[string]int
	titleCounts    map[string]int
}

func newMediaSeriesKeyResolver(items []model.Media) mediaSeriesKeyResolver {
	resolver := mediaSeriesKeyResolver{
		pathCounts:     make(map[string]int),
		externalCounts: make(map[string]int),
		titleCounts:    make(map[string]int),
	}
	for _, item := range items {
		if !mediaLooksEpisodicForGrouping(item) {
			continue
		}
		if key := mediaSeriesRawKey(item); strings.HasPrefix(key, "library-path") {
			resolver.pathCounts[key]++
		}
		if key := repeatedSeriesExternalKey(item); key != "" {
			resolver.externalCounts[key]++
		}
		if key := repeatedSeriesTitleKey(item); key != "" {
			resolver.titleCounts[key]++
		}
	}
	return resolver
}

func (r mediaSeriesKeyResolver) key(media model.Media) string {
	if mediaLooksEpisodicForGrouping(media) {
		if key := mediaSeriesRawKey(media); strings.HasPrefix(key, "library-path") && r.pathCounts[key] > 1 {
			return compactSeriesKey(key)
		}
		if key := repeatedSeriesExternalKey(media); key != "" && r.externalCounts[key] > 1 {
			return compactSeriesKey(key)
		}
		if key := repeatedSeriesTitleKey(media); key != "" && r.titleCounts[key] > 1 {
			return compactSeriesKey(key)
		}
	}
	return mediaSeriesKey(media)
}

func mediaLooksEpisodicForGrouping(media model.Media) bool {
	return media.SeasonNum > 0 || media.EpisodeNum > 0 ||
		episodicPathRE.MatchString(media.Path+" "+media.DisplayLibraryPath+" "+media.LibraryPath)
}

func repeatedSeriesExternalKey(media model.Media) string {
	identity := ""
	switch {
	case media.TMDbID > 0:
		identity = fmt.Sprintf("tmdb:%d", media.TMDbID)
	case media.BangumiID > 0:
		identity = fmt.Sprintf("bgm:%d", media.BangumiID)
	case strings.TrimSpace(media.DoubanID) != "":
		identity = "douban:" + strings.TrimSpace(media.DoubanID)
	case strings.TrimSpace(media.TheTVDBID) != "":
		identity = "thetvdb:" + strings.TrimSpace(media.TheTVDBID)
	}
	if identity == "" {
		return ""
	}
	return seriesFingerprint("library-external", mediaTargetLibraryID(media), identity)
}

func repeatedSeriesTitleKey(media model.Media) string {
	if !strings.EqualFold(strings.TrimSpace(media.ScrapeStatus), "matched") {
		return ""
	}
	title := strings.TrimSpace(firstNonEmpty(media.Title, media.OriginalName))
	if title == "" || unsafeAutomaticEpisodeQuery(title) || organizeMediaTitleLooksLikeRelease(title) {
		return ""
	}
	title = normalizeSeriesTitle(title)
	if title == "" {
		return ""
	}
	return seriesFingerprint("library-title-year", mediaTargetLibraryID(media), title, fmt.Sprint(media.Year))
}
