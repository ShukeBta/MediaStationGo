package service

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"time"

	"go.uber.org/zap"

	"github.com/ShukeBta/MediaStationGo/internal/model"
	"github.com/ShukeBta/MediaStationGo/internal/repository"
)

const (
	PipelineScrapeModeApply = "apply"
	PipelineScrapeModeSmart = "smart"
)

type PipelineScrapeService struct {
	repos   *repository.Container
	scraper *ScraperService
}

func NewPipelineScrapeService(repos *repository.Container, scraper *ScraperService) *PipelineScrapeService {
	if repos != nil && repos.Media != nil {
		repos.Media.SetSeriesKeyFunc(MediaSeriesKey)
	}
	return &PipelineScrapeService{repos: repos, scraper: scraper}
}

type PipelineScrapeRequest struct {
	Category        string                          `json:"category,omitempty"`
	Title           string                          `json:"title,omitempty"`
	Queries         []string                        `json:"queries,omitempty"`
	Provider        string                          `json:"provider,omitempty"`
	MediaType       string                          `json:"media_type,omitempty"`
	MediaIDs        []string                        `json:"media_ids,omitempty"`
	EpisodeMappings map[string]ManualEpisodeMapping `json:"episode_mappings,omitempty"`
}

type PipelineScrapeResult struct {
	Mode         string               `json:"mode"`
	Query        string               `json:"query,omitempty"`
	MatchCount   int                  `json:"match_count,omitempty"`
	Match        *ExternalMediaResult `json:"match,omitempty"`
	MediaID      string               `json:"media_id"`
	MediaTitle   string               `json:"media_title,omitempty"`
	AppliedCount int                  `json:"applied_count,omitempty"`
	ScrapeStatus string               `json:"scrape_status,omitempty"`
}

func (s *PipelineScrapeService) Scrape(ctx context.Context, mediaID string, req PipelineScrapeRequest) (PipelineScrapeResult, error) {
	if s == nil || s.repos == nil || s.repos.Media == nil || s.scraper == nil {
		return PipelineScrapeResult{}, errors.New("pipeline scrape service unavailable")
	}
	mediaID = strings.TrimSpace(mediaID)
	if mediaID == "" {
		return PipelineScrapeResult{}, errors.New("media id is required")
	}
	media, err := s.repos.Media.FindByID(ctx, mediaID)
	if err != nil || media == nil {
		return PipelineScrapeResult{}, errors.New("media not found")
	}
	queries := pipelineCompactStrings(req.Queries)
	if len(queries) == 0 && strings.TrimSpace(req.Title) != "" {
		queries = []string{strings.TrimSpace(req.Title)}
	}
	provider := strings.TrimSpace(req.Provider)
	mediaType := strings.TrimSpace(req.MediaType)
	queries, enforceAdultCode := pipelineScrapeAdultExactQueries(req.Category, provider, mediaType, queries)
	options := pipelineScrapeOptions()

	if provider != "" && mediaType != "" {
		for _, query := range queries {
			matches, err := s.scraper.ManualSearch(ctx, media, query, provider, mediaType)
			if err != nil {
				return PipelineScrapeResult{}, err
			}
			matches = pipelineScrapeMatchesForMediaType(matches, mediaType)
			if enforceAdultCode {
				matches = pipelineScrapeMatchesForAdultCode(matches, query)
			}
			if len(matches) != 1 {
				continue
			}
			match := matches[0]
			if len(req.EpisodeMappings) > 0 || len(req.MediaIDs) > 0 {
				applied, err := s.applySelectedMatchBatch(ctx, mediaID, req, match)
				if err != nil {
					return PipelineScrapeResult{}, err
				}
				refreshed, _ := s.repos.Media.FindByID(ctx, mediaID)
				result := PipelineScrapeResult{
					Mode:         PipelineScrapeModeApply,
					Query:        query,
					MatchCount:   len(matches),
					Match:        &match,
					MediaID:      mediaID,
					AppliedCount: applied,
				}
				if refreshed != nil {
					result.MediaTitle = pipelineMediaDisplayTitle(*refreshed)
					result.ScrapeStatus = strings.TrimSpace(refreshed.ScrapeStatus)
				}
				return result, nil
			}
			refreshed, err := s.applySelectedMatch(ctx, media, match, options)
			if err != nil {
				return PipelineScrapeResult{}, err
			}
			result := PipelineScrapeResult{
				Mode:       PipelineScrapeModeApply,
				Query:      query,
				MatchCount: len(matches),
				Match:      &match,
				MediaID:    mediaID,
			}
			if refreshed != nil {
				result.MediaTitle = pipelineMediaDisplayTitle(*refreshed)
				result.ScrapeStatus = strings.TrimSpace(refreshed.ScrapeStatus)
				if result.ScrapeStatus == "matched" {
					result.AppliedCount = 1
					if pipelineShouldPropagateEpisodeMatch(req.Category, refreshed) {
						propagated, err := s.propagateEpisodeMatch(ctx, media, refreshed, req)
						if err != nil {
							return PipelineScrapeResult{}, err
						}
						result.AppliedCount += propagated
					}
				}
			}
			return result, nil
		}
	}

	if err := s.scraper.EnrichOneWithOptions(ctx, media, options); err != nil {
		return PipelineScrapeResult{}, err
	}
	refreshed, _ := s.repos.Media.FindByID(ctx, mediaID)
	result := PipelineScrapeResult{Mode: PipelineScrapeModeSmart, MediaID: mediaID}
	if refreshed != nil {
		result.MediaTitle = pipelineMediaDisplayTitle(*refreshed)
		result.ScrapeStatus = strings.TrimSpace(refreshed.ScrapeStatus)
		if result.ScrapeStatus == "matched" {
			result.AppliedCount = 1
			if pipelineShouldPropagateEpisodeMatch(req.Category, refreshed) {
				propagated, err := s.propagateEpisodeMatch(ctx, media, refreshed, req)
				if err != nil {
					return PipelineScrapeResult{}, err
				}
				result.AppliedCount += propagated
			}
		}
	}
	return result, nil
}

func (s *PipelineScrapeService) applySelectedMatchBatch(ctx context.Context, anchorID string, req PipelineScrapeRequest, match ExternalMediaResult) (int, error) {
	category := normalizePipelineCategory(req.Category)
	if category != "tv" && category != "anime" {
		return 0, errors.New("显式季集映射仅支持电视剧或动漫")
	}
	ids := pipelineScrapeBatchMediaIDs(anchorID, req.MediaIDs, req.EpisodeMappings)
	if len(ids) == 0 {
		return 0, errors.New("剧集批量刮削需要指定媒体和显式季集映射")
	}
	if len(req.EpisodeMappings) != len(ids) {
		return 0, errors.New("剧集批量刮削的季集映射必须覆盖每条媒体")
	}
	for _, id := range ids {
		if _, ok := req.EpisodeMappings[id]; !ok {
			return 0, fmt.Errorf("剧集批量刮削缺少媒体 %s 的季集映射", id)
		}
	}

	manual := pipelineManualScrapeRequest(match)
	manual.EpisodeMappings = cloneManualEpisodeMappings(req.EpisodeMappings)
	preview, applyOptions, resolvedMatch, err := s.scraper.previewManualMatchDetailsWithOptions(ctx, ids, manual, true, pipelineMatchFromExternalResult(match))
	if err != nil {
		return 0, fmt.Errorf("剧集批量刮削预览失败: %w", err)
	}
	revisions := make(map[string]string, len(preview.Rows))
	for _, row := range preview.Rows {
		if !row.Valid {
			if strings.TrimSpace(row.Error) == "" {
				return 0, fmt.Errorf("媒体 %s 的季集映射校验失败", row.MediaID)
			}
			return 0, fmt.Errorf("媒体 %s 的季集映射校验失败: %s", row.MediaID, row.Error)
		}
		if row.MediaID == "" || row.Revision == "" {
			return 0, errors.New("剧集批量刮削预览缺少媒体版本")
		}
		revisions[row.MediaID] = row.Revision
	}
	if len(revisions) != len(ids) {
		return 0, errors.New("剧集批量刮削预览未覆盖全部媒体")
	}
	manual.ExpectedRevisions = revisions
	applyOptions.manualMatch = resolvedMatch
	result, err := s.scraper.ApplyManualMatchBatchWithOptions(ctx, ids, manual, applyOptions)
	if err != nil {
		return 0, fmt.Errorf("剧集批量刮削应用失败: %w", err)
	}
	if len(result.Errors) > 0 {
		failures := make([]string, 0, len(result.Errors))
		for _, failure := range result.Errors {
			failures = append(failures, fmt.Sprintf("%s: %s", failure.MediaID, failure.Err))
		}
		return 0, fmt.Errorf("剧集批量刮削未完整应用（成功 %d/%d）: %s", len(result.AppliedIDs), len(ids), strings.Join(failures, "; "))
	}
	if len(result.AppliedIDs) != len(ids) {
		return 0, fmt.Errorf("剧集批量刮削未完整应用（成功 %d/%d）", len(result.AppliedIDs), len(ids))
	}
	s.enrichBatchTMDbMetadata(ctx, ids, anchorID, resolvedMatch)
	return len(result.AppliedIDs), nil
}

func (s *PipelineScrapeService) enrichBatchTMDbMetadata(ctx context.Context, mediaIDs []string, anchorID string, match *Match) {
	if s == nil || s.repos == nil || s.repos.DB == nil || s.scraper == nil || match == nil || match.TMDbID <= 0 || normalizeMediaType(match.MediaType, "", "") != "tv" {
		return
	}
	if s.scraper.tmdb == nil || !s.scraper.tmdb.Enabled() {
		return
	}
	// The legacy pipeline enriched the selected anchor once and propagated its
	// optional series metadata to the sibling rows. Keep that behavior without
	// issuing one /tv/{id} request per episode in the explicit batch path.
	s.scraper.fetchAndSaveTMDbExtendedMetadata(ctx, anchorID, match.TMDbID, "tv")
	anchor, err := s.repos.Media.FindByID(ctx, anchorID)
	if err != nil || anchor == nil {
		return
	}
	updates := map[string]any{}
	if strings.TrimSpace(anchor.Languages) != "" {
		updates["languages"] = anchor.Languages
	}
	if strings.TrimSpace(anchor.Countries) != "" {
		updates["countries"] = anchor.Countries
	}
	if strings.TrimSpace(anchor.Genres) != "" {
		updates["genres"] = anchor.Genres
	}
	if strings.TrimSpace(anchor.Actors) != "" {
		updates["actors"] = anchor.Actors
	}
	if len(updates) == 0 {
		return
	}
	query := s.repos.DB.WithContext(ctx).Model(&model.Media{}).Where("id IN ?", mediaIDs)
	if err := query.Updates(updates).Error; err != nil {
		s.scraper.log.Warn("failed to propagate batch tmdb metadata", zap.Int("media_count", len(mediaIDs)), zap.Error(err))
	}
}

func pipelineScrapeBatchMediaIDs(anchorID string, mediaIDs []string, mappings map[string]ManualEpisodeMapping) []string {
	seen := make(map[string]struct{}, len(mediaIDs)+1)
	ids := make([]string, 0, len(mediaIDs)+1)
	add := func(value string) {
		value = strings.TrimSpace(value)
		if value == "" {
			return
		}
		if _, ok := seen[value]; ok {
			return
		}
		seen[value] = struct{}{}
		ids = append(ids, value)
	}
	for _, id := range mediaIDs {
		add(id)
	}
	if len(ids) == 0 {
		for id := range mappings {
			add(id)
		}
	}
	if len(ids) > 0 {
		anchorFound := false
		for _, id := range ids {
			if id == strings.TrimSpace(anchorID) {
				anchorFound = true
				break
			}
		}
		if !anchorFound {
			return nil
		}
	}
	return ids
}

func cloneManualEpisodeMappings(values map[string]ManualEpisodeMapping) map[string]ManualEpisodeMapping {
	if len(values) == 0 {
		return nil
	}
	out := make(map[string]ManualEpisodeMapping, len(values))
	for id, mapping := range values {
		out[strings.TrimSpace(id)] = mapping
	}
	return out
}

func pipelineManualScrapeRequest(match ExternalMediaResult) ManualScrapeRequest {
	return ManualScrapeRequest{
		Source:       match.Source,
		MediaType:    match.MediaType,
		Title:        match.Title,
		OriginalName: match.OriginalName,
		Overview:     match.Overview,
		PosterURL:    match.PosterURL,
		BackdropURL:  match.BackdropURL,
		Year:         match.Year,
		ReleaseDate:  match.ReleaseDate,
		Rating:       match.Rating,
		TMDbID:       match.TMDbID,
		BangumiID:    match.BangumiID,
		DoubanID:     match.DoubanID,
		TheTVDBID:    match.TheTVDBID,
		Languages:    append([]string(nil), match.Languages...),
		Countries:    append([]string(nil), match.Countries...),
		Genres:       append([]string(nil), match.Genres...),
		Actors:       append([]string(nil), match.Actors...),
		People:       append([]PersonMetadata(nil), match.People...),
		NSFW:         match.NSFW,
	}
}

func pipelineShouldPropagateEpisodeMatch(category string, refreshed *model.Media) bool {
	category = normalizePipelineCategory(category)
	if category != "tv" && category != "anime" {
		return false
	}
	// A movie match explicitly resets the refreshed row's episode numbers.
	// Use that persisted result as the authority instead of the library type or
	// an SxxExx-looking source filename, both of which can describe a mixed pack.
	return refreshed != nil && (refreshed.SeasonNum > 0 || refreshed.EpisodeNum > 0)
}

func pipelineScrapeMatchesForMediaType(matches []ExternalMediaResult, mediaType string) []ExternalMediaResult {
	mediaType = strings.TrimSpace(mediaType)
	if mediaType == "" {
		return matches
	}
	filtered := make([]ExternalMediaResult, 0, len(matches))
	for _, match := range matches {
		if strings.EqualFold(strings.TrimSpace(match.MediaType), mediaType) {
			filtered = append(filtered, match)
		}
	}
	return filtered
}

// pipelineScrapeAdultExactQueries keeps an adult ingest task on the explicit
// number supplied by the pipeline. Generic task labels must not make a manual
// search fall back to numbers embedded in the original cloud path.
func pipelineScrapeAdultExactQueries(category, provider, mediaType string, queries []string) ([]string, bool) {
	if normalizePipelineCategory(category) != "adult" || !strings.EqualFold(strings.TrimSpace(provider), "adult") || !strings.EqualFold(strings.TrimSpace(mediaType), "adult") {
		return queries, false
	}

	codes := make([]string, 0, len(queries))
	seen := make(map[string]struct{}, len(queries))
	for _, query := range queries {
		code := normalizeAdultCode(query)
		key := adultCodeKey(code)
		if key == "" {
			continue
		}
		if _, ok := seen[key]; ok {
			continue
		}
		seen[key] = struct{}{}
		codes = append(codes, code)
	}
	if len(codes) == 0 {
		return queries, false
	}
	return codes, true
}

func pipelineScrapeMatchesForAdultCode(matches []ExternalMediaResult, code string) []ExternalMediaResult {
	expected := adultCodeKey(code)
	if expected == "" {
		return nil
	}
	filtered := make([]ExternalMediaResult, 0, len(matches))
	for _, match := range matches {
		if adultCodeKey(match.OriginalName) == expected {
			filtered = append(filtered, match)
		}
	}
	return filtered
}

func (s *PipelineScrapeService) applySelectedMatch(
	ctx context.Context,
	media *model.Media,
	match ExternalMediaResult,
	options ScrapeOptions,
) (*model.Media, error) {
	lib, _ := s.repos.Library.FindByID(ctx, media.LibraryID)
	if err := s.scraper.applyProviderMatchWithOptions(ctx, media, lib, pipelineMatchFromExternalResult(match), options); err != nil {
		return nil, err
	}
	return s.repos.Media.FindByID(ctx, media.ID)
}

func pipelineMatchFromExternalResult(match ExternalMediaResult) *Match {
	return &Match{
		TMDbID:          match.TMDbID,
		BangumiID:       match.BangumiID,
		DoubanID:        match.DoubanID,
		TheTVDBID:       match.TheTVDBID,
		MediaType:       match.MediaType,
		Title:           match.Title,
		OriginalName:    match.OriginalName,
		Overview:        match.Overview,
		PosterURL:       match.PosterURL,
		BackdropURL:     match.BackdropURL,
		PreviewImages:   append([]string(nil), match.PreviewImages...),
		Year:            match.Year,
		ReleaseDate:     match.ReleaseDate,
		Rating:          match.Rating,
		DurationMinutes: match.DurationMinutes,
		Maker:           match.Maker,
		Languages:       append([]string(nil), match.Languages...),
		Countries:       append([]string(nil), match.Countries...),
		Genres:          append([]string(nil), match.Genres...),
		Actors:          append([]string(nil), match.Actors...),
		People:          append([]PersonMetadata(nil), match.People...),
		NSFW:            match.NSFW,
	}
}

func (s *PipelineScrapeService) propagateEpisodeMatch(ctx context.Context, media *model.Media, refreshed *model.Media, req PipelineScrapeRequest) (int, error) {
	category := normalizePipelineCategory(req.Category)
	if category != "tv" && category != "anime" {
		return 0, nil
	}
	if s == nil || s.repos == nil || s.repos.DB == nil || media == nil || refreshed == nil || refreshed.ScrapeStatus != "matched" {
		return 0, nil
	}
	folder := pipelineScrapeParentPath(media.Path)
	if folder == "" || folder == "/" {
		return 0, nil
	}
	options := pipelineScrapeOptions()
	if refreshed.TMDbID > 0 {
		// Validate every target before propagating the anchor's identity. A pack
		// directory can contain unrelated shows; directory membership is not proof.
		var targets []model.Media
		q := s.repos.DB.WithContext(ctx).Where("library_id = ? AND path LIKE ?", media.LibraryID, folder+"/%")
		if media.LibraryRootID != "" {
			q = q.Where("library_root_id = ?", media.LibraryRootID)
		}
		if err := q.Find(&targets).Error; err != nil {
			return 0, err
		}
		match := &Match{TMDbID: refreshed.TMDbID, MediaType: "tv", Title: refreshed.Title, OriginalName: refreshed.OriginalName}
		for i := range targets {
			if _, err := s.scraper.validateEpisodeMatch(ctx, &targets[i], nil, match, options); err != nil {
				return 0, fmt.Errorf("episode group validation: %w", err)
			}
		}
		// Keep the whole episode group visibly retryable until the season batch
		// has been validated and committed. A process interruption must not
		// leave a series-level match looking like complete episode metadata.
		if err := s.resetEpisodeGroupScrapeStatus(ctx, media, folder, "pending"); err != nil {
			return 0, fmt.Errorf("mark episode group pending: %w", err)
		}
		if s.scraper == nil {
			return 0, errors.New("pipeline tmdb episode detail service unavailable")
		}
	}
	updates := map[string]any{
		"title":         refreshed.Title,
		"overview":      refreshed.Overview,
		"poster_url":    refreshed.PosterURL,
		"rating":        refreshed.Rating,
		"year":          refreshed.Year,
		"scrape_status": refreshed.ScrapeStatus,
		"updated_at":    time.Now(),
	}
	// TMDb episode backdrops are per-episode stills. Do not copy the anchor's
	// backdrop to every sibling; the strict season batch below fills each row.
	if refreshed.TMDbID <= 0 {
		updates["backdrop_url"] = refreshed.BackdropURL
	} else {
		updates["scrape_status"] = "pending"
	}
	if refreshed.OriginalName != "" {
		updates["original_name"] = refreshed.OriginalName
	}
	if refreshed.ReleaseDate != "" {
		updates["release_date"] = refreshed.ReleaseDate
	}
	if refreshed.TMDbID > 0 {
		updates["tm_db_id"] = refreshed.TMDbID
	}
	if refreshed.BangumiID > 0 {
		updates["bangumi_id"] = refreshed.BangumiID
	}
	if refreshed.DoubanID != "" {
		updates["douban_id"] = refreshed.DoubanID
	}
	if refreshed.TheTVDBID != "" {
		updates["thetvdb_id"] = refreshed.TheTVDBID
	}
	if refreshed.Languages != "" {
		updates["languages"] = refreshed.Languages
	}
	if refreshed.Countries != "" {
		updates["countries"] = refreshed.Countries
	}
	if refreshed.Genres != "" {
		updates["genres"] = refreshed.Genres
	}
	if refreshed.Actors != "" {
		updates["actors"] = refreshed.Actors
	}
	if refreshed.NSFW {
		updates["nsfw"] = refreshed.NSFW
	}
	query := s.repos.DB.WithContext(ctx).Model(&model.Media{}).
		Where("library_id = ? AND id <> ?", media.LibraryID, media.ID).
		Where("path LIKE ?", folder+"/%")
	if strings.TrimSpace(media.LibraryRootID) != "" {
		query = query.Where("library_root_id = ?", media.LibraryRootID)
	}
	var siblingIDs []string
	if err := query.Pluck("id", &siblingIDs).Error; err != nil {
		return 0, err
	}
	updated, err := s.repos.Media.UpdateManyWithCurrentSeriesKeys(ctx, nil, siblingIDs, updates)
	if err != nil {
		return 0, err
	}
	if refreshed.TMDbID > 0 {
		var rows []model.Media
		rowsQuery := s.repos.DB.WithContext(ctx).Model(&model.Media{}).
			Where("library_id = ?", media.LibraryID).
			Where("path LIKE ?", folder+"/%")
		if strings.TrimSpace(media.LibraryRootID) != "" {
			rowsQuery = rowsQuery.Where("library_root_id = ?", media.LibraryRootID)
		}
		if err := rowsQuery.Order("season_num ASC, episode_num ASC, created_at ASC").Find(&rows).Error; err != nil {
			return 0, err
		}
		episodes := make([]*model.Media, 0, len(rows))
		for i := range rows {
			if rows[i].EpisodeNum > 0 {
				episodes = append(episodes, &rows[i])
			}
		}
		applied, err := s.scraper.applyTMDbEpisodeDetailsBatch(ctx, episodes, refreshed.TMDbID, refreshed.Year, true, options.episodeValidation)
		if err != nil {
			return 0, err
		}
		if applied != len(episodes) {
			return 0, fmt.Errorf("tmdb episode details applied %d of %d rows", applied, len(episodes))
		}
		if strings.TrimSpace(refreshed.BackdropURL) != "" {
			clearQuery := s.repos.DB.WithContext(ctx).Model(&model.Media{}).
				Where("library_id = ?", media.LibraryID).
				Where("path LIKE ?", folder+"/%").
				Where("backdrop_url = ?", refreshed.BackdropURL)
			if strings.TrimSpace(media.LibraryRootID) != "" {
				clearQuery = clearQuery.Where("library_root_id = ?", media.LibraryRootID)
			}
			if err := clearQuery.Update("backdrop_url", "").Error; err != nil {
				return 0, fmt.Errorf("clear shared series backdrop: %w", err)
			}
		}
		if err := s.resetEpisodeGroupScrapeStatus(ctx, media, folder, "matched"); err != nil {
			return 0, fmt.Errorf("mark episode group matched: %w", err)
		}
		lib, _ := s.repos.Library.FindByID(ctx, media.LibraryID)
		s.scraper.writeMediaNFOAfterScrape(ctx, media, lib)
		s.scraper.invalidateMediaCache(ctx)
	}
	return int(updated), nil
}

func (s *PipelineScrapeService) resetEpisodeGroupScrapeStatus(ctx context.Context, media *model.Media, folder string, status string) error {
	query := s.repos.DB.WithContext(ctx).Model(&model.Media{}).
		Where("library_id = ?", media.LibraryID).
		Where("path LIKE ?", folder+"/%")
	if strings.TrimSpace(media.LibraryRootID) != "" {
		query = query.Where("library_root_id = ?", media.LibraryRootID)
	}
	var mediaIDs []string
	if err := query.Pluck("id", &mediaIDs).Error; err != nil {
		return err
	}
	_, err := s.repos.Media.UpdateManyWithCurrentSeriesKeys(ctx, nil, mediaIDs, map[string]any{"scrape_status": status})
	return err
}

func pipelineScrapeParentPath(value string) string {
	value = strings.TrimRight(strings.TrimSpace(value), "/")
	idx := strings.LastIndex(value, "/")
	if idx <= 0 {
		return ""
	}
	return value[:idx]
}

func pipelineScrapeOptions() ScrapeOptions {
	return ScrapeOptions{RetryNoMatch: true, IncludeMatched: true, DeferEpisodeDetails: true, automaticSelection: true, episodeValidation: make(map[[2]int]map[int]*TMDbEpisodeDetails)}
}
