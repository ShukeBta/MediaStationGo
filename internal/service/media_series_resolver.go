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
	pathTitles     map[string]string
}

func newMediaSeriesKeyResolver(items []model.Media) mediaSeriesKeyResolver {
	resolver := mediaSeriesKeyResolver{
		pathCounts:     make(map[string]int),
		externalCounts: make(map[string]int),
		titleCounts:    make(map[string]int),
		pathTitles:     make(map[string]string),
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
	// A scraper can normalize the same show to one title while the source
	// release folders still contain different tags (1080p/2160p, uploader
	// names, etc.). Remember an unambiguous title alias for each path group so
	// those folders are bridged instead of rendered as separate collections.
	pathTitleCandidates := make(map[string]map[string]struct{})
	for _, item := range items {
		if !mediaLooksEpisodicForGrouping(item) {
			continue
		}
		pathKey := mediaSeriesRawKey(item)
		titleKey := repeatedSeriesTitleKey(item)
		if !strings.HasPrefix(pathKey, "library-path") || titleKey == "" || resolver.titleCounts[titleKey] < 2 {
			continue
		}
		if pathTitleCandidates[pathKey] == nil {
			pathTitleCandidates[pathKey] = make(map[string]struct{})
		}
		pathTitleCandidates[pathKey][titleKey] = struct{}{}
	}
	for pathKey, candidates := range pathTitleCandidates {
		if len(candidates) != 1 {
			continue
		}
		for titleKey := range candidates {
			resolver.pathTitles[pathKey] = titleKey
		}
	}
	return resolver
}

func (r mediaSeriesKeyResolver) key(media model.Media) string {
	if mediaLooksEpisodicForGrouping(media) {
		if key := repeatedSeriesTitleKey(media); key != "" && r.titleCounts[key] > 1 {
			return compactSeriesKey(key)
		}
		if pathKey := mediaSeriesRawKey(media); strings.HasPrefix(pathKey, "library-path") {
			if titleKey := r.pathTitles[pathKey]; titleKey != "" {
				return compactSeriesKey(titleKey)
			}
			if r.pathCounts[pathKey] > 1 {
				return compactSeriesKey(pathKey)
			}
		}
		if key := repeatedSeriesExternalKey(media); key != "" && r.externalCounts[key] > 1 {
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
