package service

import (
	"context"
	"fmt"
	"regexp"
	"sort"
	"strings"

	"github.com/ShukeBta/MediaStationGo/internal/model"
	"github.com/ShukeBta/MediaStationGo/internal/repository"
)

var availabilityNoiseRE = regexp.MustCompile(`(?i)(自动订阅|订阅|全集|合集|complete|batch|s\d{1,2}e\d{1,3}|season\s*\d+|s\d{1,2}|第\s*\d+\s*季|第\s*\d+\s*[集话話期]|\(\d{4}\)|\b\d{4}\b|2160p|1080p|720p|4k|uhd|bluray|blu-ray|web-?dl|hdtv|remux|x26[45]|h\.?26[45]|hevc|avc|hdr10?\+?|dovi|dv|atmos|aac|ddp?5\.1|truehd|flac)`)

type LocalAvailability struct {
	MediaID             string
	Media               *model.Media
	DownloadedEpisodes  int
	TotalEpisodes       int
	LocalMediaCount     int
	MissingEpisodes     []int
	InLibrary           bool
	HasSeriesPack       bool
	ExistingEpisodeKeys map[string]struct{}
	MissingEpisodeKeys  map[string]struct{}
}

func EnrichExternalMediaAvailability(ctx context.Context, repo *repository.Container, items []ExternalMediaResult) {
	for i := range items {
		availability := LookupLocalAvailability(ctx, repo, items[i].Title, items[i].SubscribeKeyword, items[i].MediaType, items[i].TotalEpisodes)
		items[i].DownloadedEpisodes = availability.DownloadedEpisodes
		items[i].LocalMediaCount = availability.LocalMediaCount
		items[i].MissingEpisodes = availability.MissingEpisodes
		items[i].InLibrary = availability.InLibrary
		if items[i].TotalEpisodes == 0 {
			items[i].TotalEpisodes = availability.TotalEpisodes
		}
	}
}

// EnrichExternalMediaLibraryLinks resolves discover cards to one visible local
// media row in a single query. Provider IDs are authoritative; normalized
// titles are only used when the external item has no matching provider ID.
func EnrichExternalMediaLibraryLinks(
	ctx context.Context,
	repo *repository.Container,
	items []ExternalMediaResult,
	visibility MediaVisibility,
) {
	if repo == nil || repo.DB == nil || len(items) == 0 {
		return
	}
	tmdbIDs := make([]int, 0)
	bangumiIDs := make([]int, 0)
	doubanIDs := make([]string, 0)
	fd2PPVNumbers := make([]string, 0)
	titleKeys := make([]string, 0)
	seenTMDb, seenBangumi := map[int]struct{}{}, map[int]struct{}{}
	seenDouban, seenFD2PPV, seenTitle := map[string]struct{}{}, map[string]struct{}{}, map[string]struct{}{}
	for _, item := range items {
		if strings.EqualFold(strings.TrimSpace(item.MediaType), "person") {
			continue
		}
		if number := fd2PPVExternalNumber(item); number != "" {
			if _, ok := seenFD2PPV[number]; !ok {
				seenFD2PPV[number] = struct{}{}
				fd2PPVNumbers = append(fd2PPVNumbers, number)
			}
		}
		if item.TMDbID > 0 {
			if _, ok := seenTMDb[item.TMDbID]; !ok {
				seenTMDb[item.TMDbID] = struct{}{}
				tmdbIDs = append(tmdbIDs, item.TMDbID)
			}
		}
		if item.BangumiID > 0 {
			if _, ok := seenBangumi[item.BangumiID]; !ok {
				seenBangumi[item.BangumiID] = struct{}{}
				bangumiIDs = append(bangumiIDs, item.BangumiID)
			}
		}
		if id := strings.TrimSpace(item.DoubanID); id != "" {
			if _, ok := seenDouban[id]; !ok {
				seenDouban[id] = struct{}{}
				doubanIDs = append(doubanIDs, id)
			}
		}
		for _, title := range []string{item.Title, item.OriginalName} {
			if key := localMediaTitleKey(title); key != "" {
				if _, ok := seenTitle[key]; !ok {
					seenTitle[key] = struct{}{}
					titleKeys = append(titleKeys, key)
				}
			}
		}
	}
	clauses := make([]string, 0, 4+len(fd2PPVNumbers))
	args := make([]any, 0, 6)
	if len(tmdbIDs) > 0 {
		clauses = append(clauses, "tm_db_id IN ?")
		args = append(args, tmdbIDs)
	}
	if len(bangumiIDs) > 0 {
		clauses = append(clauses, "bangumi_id IN ?")
		args = append(args, bangumiIDs)
	}
	if len(doubanIDs) > 0 {
		clauses = append(clauses, "douban_id IN ?")
		args = append(args, doubanIDs)
	}
	for _, number := range fd2PPVNumbers {
		pattern := "%" + number + "%"
		clauses = append(clauses, "nsfw = ? AND (LOWER(original_name) LIKE ? OR LOWER(title) LIKE ? OR LOWER(path) LIKE ?)")
		args = append(args, true, pattern, pattern, pattern)
	}
	if len(titleKeys) > 0 {
		clauses = append(clauses, "LOWER(TRIM(title)) IN ? OR LOWER(TRIM(original_name)) IN ?")
		args = append(args, titleKeys, titleKeys)
	}
	if len(clauses) == 0 {
		return
	}
	query := repo.DB.WithContext(ctx).Model(&model.Media{}).
		Where("deleted_at IS NULL").
		Where("("+strings.Join(clauses, ") OR (")+")", args...)
	if !visibility.IncludeNSFW {
		query = query.Where("nsfw = ?", false)
	}
	if len(visibility.HiddenLibraryIDs) > 0 {
		query = query.Where("library_id NOT IN ?", visibility.HiddenLibraryIDs)
	}
	if len(visibility.AllowedLibraryIDs) > 0 {
		query = query.Where("library_id IN ?", visibility.AllowedLibraryIDs)
	}
	var rows []model.Media
	if err := query.Order("updated_at DESC, created_at DESC").Limit(5000).Find(&rows).Error; err != nil {
		return
	}
	for index := range items {
		if strings.EqualFold(strings.TrimSpace(items[index].MediaType), "person") {
			continue
		}
		if row := bestLocalMediaLink(items[index], rows); row != nil {
			items[index].InLibrary = true
			items[index].LocalMediaCount = maxInt(items[index].LocalMediaCount, 1)
			items[index].LocalMediaID = row.ID
			items[index].LocalLibraryID = row.LibraryID
		}
	}
}

func bestLocalMediaLink(item ExternalMediaResult, rows []model.Media) *model.Media {
	if number := fd2PPVExternalNumber(item); number != "" {
		for index := range rows {
			if rows[index].NSFW && fd2PPVLocalNumber(rows[index]) == number {
				return &rows[index]
			}
		}
	}
	providerMatch := func(row model.Media) bool {
		return (item.TMDbID > 0 && row.TMDbID == item.TMDbID && tmdbMediaTypeMatches(item.MediaType, row)) ||
			(item.BangumiID > 0 && row.BangumiID == item.BangumiID) ||
			(strings.TrimSpace(item.DoubanID) != "" && strings.TrimSpace(row.DoubanID) == strings.TrimSpace(item.DoubanID))
	}
	for index := range rows {
		if providerMatch(rows[index]) && localMediaYearsMatch(item.Year, rows[index].Year) {
			return &rows[index]
		}
	}
	for index := range rows {
		if providerMatch(rows[index]) && (item.Year <= 0 || rows[index].Year <= 0) {
			return &rows[index]
		}
	}
	keys := map[string]struct{}{}
	for _, title := range []string{item.Title, item.OriginalName} {
		if key := localMediaTitleKey(title); key != "" {
			keys[key] = struct{}{}
		}
	}
	for index := range rows {
		_, titleMatch := keys[localMediaTitleKey(rows[index].Title)]
		_, originalMatch := keys[localMediaTitleKey(rows[index].OriginalName)]
		if (titleMatch || originalMatch) && (item.Year <= 0 || rows[index].Year <= 0 || rows[index].Year == item.Year) {
			return &rows[index]
		}
	}
	return nil
}

func fd2PPVExternalNumber(item ExternalMediaResult) string {
	if !strings.EqualFold(strings.TrimSpace(item.Source), "fd2ppv") ||
		!strings.EqualFold(strings.TrimSpace(item.MediaType), "adult") {
		return ""
	}
	if number := strings.TrimSpace(item.ProviderID); adultFD2RawNumberPattern.MatchString(number) {
		return number
	}
	for _, value := range []string{item.OriginalName, item.SubscribeKeyword, item.Title} {
		if number := adultFC2Number(value); number != "" {
			return number
		}
	}
	return ""
}

func fd2PPVLocalNumber(row model.Media) string {
	for _, value := range []string{row.OriginalName, row.Title, row.Path} {
		if number := adultFC2Number(value); number != "" {
			return number
		}
	}
	return ""
}

func tmdbMediaTypeMatches(externalType string, row model.Media) bool {
	externalSeries, known := externalMediaTypeIsSeries(externalType)
	if !known {
		return true
	}
	localSeries := strings.TrimSpace(row.SeriesID) != "" || row.SeasonNum > 0 || row.EpisodeNum > 0
	return externalSeries == localSeries
}

func externalMediaTypeIsSeries(value string) (bool, bool) {
	switch strings.ToLower(strings.TrimSpace(value)) {
	case "tv", "series", "episode", "anime", "variety", "show":
		return true, true
	case "movie", "film", "adult":
		return false, true
	default:
		return false, false
	}
}

func localMediaYearsMatch(externalYear, localYear int) bool {
	return externalYear <= 0 || localYear <= 0 || externalYear == localYear
}

func localMediaTitleKey(value string) string {
	return strings.ToLower(strings.TrimSpace(strings.Join(strings.Fields(value), " ")))
}

func EnrichSubscriptionProgress(ctx context.Context, repo *repository.Container, items []model.Subscription) {
	for i := range items {
		availability := SubscriptionLocalAvailability(ctx, repo, &items[i])
		items[i].DownloadedEpisodes = availability.DownloadedEpisodes
		items[i].LocalMediaCount = availability.LocalMediaCount
		items[i].MissingEpisodes = availability.MissingEpisodes
		items[i].InLibrary = availability.InLibrary
		if items[i].TotalEpisodes == 0 {
			items[i].TotalEpisodes = availability.TotalEpisodes
		}
	}
}

func SubscriptionLocalAvailability(ctx context.Context, repo *repository.Container, sub *model.Subscription) LocalAvailability {
	if sub == nil {
		return LocalAvailability{}
	}
	expected := sub.TotalEpisodes
	return LookupLocalAvailability(ctx, repo, sub.Name, sub.Filter, sub.MediaType, expected)
}

func LookupLocalAvailability(ctx context.Context, repo *repository.Container, title, keyword, mediaType string, expectedTotal int) LocalAvailability {
	out := LocalAvailability{
		TotalEpisodes:       expectedTotal,
		ExistingEpisodeKeys: map[string]struct{}{},
		MissingEpisodeKeys:  map[string]struct{}{},
	}
	if repo == nil || repo.DB == nil {
		return out
	}
	query := availabilityQuery(title, keyword)
	if query == "" {
		return out
	}
	like := "%" + query + "%"
	var rows []model.Media
	if err := repo.DB.WithContext(ctx).
		Where("title LIKE ? OR original_name LIKE ? OR path LIKE ?", like, like, like).
		Order("season_num asc, episode_num asc, created_at desc").
		Limit(2000).
		Find(&rows).Error; err != nil {
		return out
	}
	out.LocalMediaCount = len(rows)
	out.InLibrary = len(rows) > 0
	if len(rows) == 0 {
		return out
	}

	seriesLike := isSubscriptionSeriesType(mediaType)
	for _, row := range rows {
		if row.EpisodeNum <= 0 {
			if seriesLike {
				out.HasSeriesPack = true
			}
			continue
		}
		season := row.SeasonNum
		if season <= 0 {
			season = 1
		}
		key := episodeKey(season, row.EpisodeNum)
		out.ExistingEpisodeKeys[key] = struct{}{}
	}
	if seriesLike || len(out.ExistingEpisodeKeys) > 0 {
		out.DownloadedEpisodes = len(out.ExistingEpisodeKeys)
		out.MissingEpisodes = missingEpisodes(out.ExistingEpisodeKeys, out.TotalEpisodes)
		for _, episode := range out.MissingEpisodes {
			out.MissingEpisodeKeys[episodeKey(1, episode)] = struct{}{}
		}
		return out
	}
	out.DownloadedEpisodes = 1
	if out.TotalEpisodes == 0 {
		out.TotalEpisodes = 1
	}
	return out
}

func missingEpisodes(existing map[string]struct{}, total int) []int {
	if total <= 0 {
		return nil
	}
	missing := make([]int, 0)
	for episode := 1; episode <= total; episode++ {
		if _, ok := existing[episodeKey(1, episode)]; ok {
			continue
		}
		missing = append(missing, episode)
	}
	return missing
}

func availabilityQuery(title, keyword string) string {
	for _, candidate := range []string{keyword, title} {
		cleaned := cleanAvailabilityTitle(candidate)
		if cleaned != "" {
			return cleaned
		}
	}
	return ""
}

func cleanAvailabilityTitle(value string) string {
	value = availabilityNoiseRE.ReplaceAllString(value, " ")
	value = strings.TrimSpace(strings.Join(strings.Fields(value), " "))
	value = strings.TrimSuffix(value, "-")
	value = strings.TrimSpace(value)
	return value
}

func episodeKey(season, episode int) string {
	if season <= 0 {
		season = 1
	}
	return fmt.Sprintf("%02dE%03d", season, episode)
}

func missingEpisodeSet(availability LocalAvailability) map[int]struct{} {
	out := make(map[int]struct{}, len(availability.MissingEpisodes))
	for _, episode := range availability.MissingEpisodes {
		out[episode] = struct{}{}
	}
	return out
}

func sortedEpisodeCandidates(candidates []siteSearchCandidate) []siteSearchCandidate {
	selected := make([]siteSearchCandidate, 0, len(candidates))
	covered := make(map[string]struct{}, len(candidates))
	for _, candidate := range candidates {
		keys := candidateEpisodeKeys(candidate)
		if len(keys) == 0 {
			continue
		}
		if episodeKeysOverlap(covered, keys) {
			continue
		}
		selected = append(selected, candidate)
		for _, key := range keys {
			covered[key] = struct{}{}
		}
	}
	sort.SliceStable(selected, func(i, j int) bool {
		return candidateFirstEpisodeKey(selected[i]) < candidateFirstEpisodeKey(selected[j])
	})
	return selected
}

func candidateEpisodeKeys(candidate siteSearchCandidate) []string {
	episodes := candidateEpisodeNumbers(candidate)
	if len(episodes) == 0 {
		return nil
	}
	season := candidate.Season
	if season <= 0 {
		season = 1
	}
	keys := make([]string, 0, len(episodes))
	seen := make(map[string]struct{}, len(episodes))
	for _, episode := range episodes {
		if episode <= 0 {
			continue
		}
		key := episodeKey(season, episode)
		if _, ok := seen[key]; ok {
			continue
		}
		seen[key] = struct{}{}
		keys = append(keys, key)
	}
	sort.Strings(keys)
	return keys
}

func candidateFirstEpisodeKey(candidate siteSearchCandidate) string {
	keys := candidateEpisodeKeys(candidate)
	if len(keys) == 0 {
		return ""
	}
	return keys[0]
}

func episodeKeysOverlap(covered map[string]struct{}, keys []string) bool {
	for _, key := range keys {
		if _, ok := covered[key]; ok {
			return true
		}
	}
	return false
}
