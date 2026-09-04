package service

import (
	"context"
	"errors"
	"net/url"
	"path"
	"regexp"
	"strconv"
	"strings"
	"time"

	"github.com/ShukeBta/MediaStationGo/internal/model"
	"github.com/ShukeBta/MediaStationGo/internal/repository"
	"go.uber.org/zap"
	"gorm.io/gorm"
)

const pipelineOpenListCloudPrefix = "cloud://openlist"

var (
	pipelineSxxExxPattern         = regexp.MustCompile(`(?i)(?:^|[^a-z0-9])S(\d{1,2})\s*E(\d{1,4})(?:[^a-z0-9]|$)`)
	pipelineChineseEpisodePattern = regexp.MustCompile(`第\s*(\d{1,4})\s*[集話话]`)
	pipelineEpisodeTokenPattern   = regexp.MustCompile(`(?i)(?:^|[^a-z0-9])(?:EP|E)\s*(\d{1,4})(?:[^a-z0-9]|$)`)
	pipelineBracketEpisodePattern = regexp.MustCompile(`(?i)(?:^|[\s._\-\[\(])(\d{1,3})(?:v\d+)?(?:[\s._\-\]\)]|$)`)
	pipelineExtraNormalizePattern = regexp.MustCompile(`[^0-9A-Za-z\p{Han}]+`)
)

var pipelineIgnoredEpisodeNumbers = map[int]bool{720: true, 1080: true, 2160: true, 264: true, 265: true}

type mediaMigrationPipelineClient interface {
	ValidateMediaMigration(context.Context, string, PipelineMigrationCandidate, string) (PipelineMigrationResult, error)
	ApplyMediaMigration(context.Context, string, PipelineMigrationCandidate, string) (PipelineMigrationResult, error)
}

type PipelineMaintenanceService struct {
	log             *zap.Logger
	repos           *repository.Container
	cache           *RuntimeCacheService
	migrationClient mediaMigrationPipelineClient
}

func NewPipelineMaintenanceService(log *zap.Logger, repos *repository.Container) *PipelineMaintenanceService {
	return &PipelineMaintenanceService{log: log, repos: repos}
}

func (s *PipelineMaintenanceService) SetRuntimeCache(cache *RuntimeCacheService) *PipelineMaintenanceService {
	s.cache = cache
	return s
}

func (s *PipelineMaintenanceService) SetMigrationClient(client mediaMigrationPipelineClient) *PipelineMaintenanceService {
	s.migrationClient = client
	return s
}

type PipelineMaintenanceTarget struct {
	Category         string `json:"category"`
	LibraryID        string `json:"library_id,omitempty"`
	RootID           string `json:"root_id,omitempty"`
	RootOpenListPath string `json:"root_openlist_path,omitempty"`
}

type PipelineRepairResult struct {
	Status               string   `json:"status"`
	Updated              int      `json:"updated"`
	MediaCount           int      `json:"media_count"`
	OpenListHidePath     string   `json:"openlist_hide_path,omitempty"`
	OpenListHidePatterns []string `json:"openlist_hide_patterns,omitempty"`
	OpenListHiddenCount  int      `json:"openlist_hidden_count,omitempty"`
	Reason               string   `json:"reason,omitempty"`
}

type PipelineDeletedMediaPruneResult struct {
	Status   string   `json:"status"`
	Deleted  int      `json:"deleted"`
	Reason   string   `json:"reason,omitempty"`
	MediaIDs []string `json:"media_ids"`
}

type pipelineResolvedTarget struct {
	Category         string
	LibraryID        string
	RootID           string
	RootOpenListPath string
}

type pipelineEpisodeUpdate struct {
	ID           string
	RelativePath string
	SeasonNum    int
	EpisodeNum   int
	EpisodeTitle string
}

func (s *PipelineMaintenanceService) RepairMovieExtras(ctx context.Context, mediaID string, target PipelineMaintenanceTarget) (PipelineRepairResult, error) {
	category := normalizePipelineCategory(target.Category)
	if category != "movie" {
		return PipelineRepairResult{Status: "skipped", Updated: 0, Reason: "not_movie_library"}, nil
	}
	mediaID = strings.TrimSpace(mediaID)
	if mediaID == "" {
		return PipelineRepairResult{Status: "skipped", Updated: 0, Reason: "media_id_missing"}, nil
	}

	resolved, err := s.resolveTarget(ctx, target)
	if err != nil {
		return PipelineRepairResult{}, err
	}

	var result PipelineRepairResult
	err = s.repos.DB.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		var row model.Media
		if err := tx.Where("library_id = ? AND library_root_id = ? AND id = ?", resolved.LibraryID, resolved.RootID, mediaID).First(&row).Error; err != nil {
			if errors.Is(err, gorm.ErrRecordNotFound) {
				return errors.New("MediaStationGo movie extra cleanup target not found")
			}
			return err
		}

		sourcePath, sourceKind, err := pipelineMediaWorkItemPath(pipelineCloudPathToOpenListPath(row.Path), resolved.RootOpenListPath)
		if err != nil {
			return err
		}
		sourceCloudPath := pipelineOpenListPathToCloudPath(sourcePath)

		query := tx.Unscoped().Where("library_id = ? AND library_root_id = ?", resolved.LibraryID, resolved.RootID)
		if sourceKind == "file" {
			query = query.Where("path = ?", sourceCloudPath)
		} else {
			query = query.Where("(path = ? OR path LIKE ?)", sourceCloudPath, strings.TrimRight(sourceCloudPath, "/")+"/%")
		}

		var rows []model.Media
		if err := query.Order("path").Find(&rows).Error; err != nil {
			return err
		}

		extraRows := make([]model.Media, 0)
		extraIDs := make([]string, 0)
		for _, item := range rows {
			if item.ID == mediaID || !pipelineMovieMediaRowLooksLikeExtra(item, sourcePath) {
				continue
			}
			extraRows = append(extraRows, item)
			if !item.DeletedAt.Valid {
				extraIDs = append(extraIDs, item.ID)
			}
		}

		hidePatterns := pipelineMovieExtraHidePatterns(extraRows, sourcePath)
		if len(extraIDs) > 0 {
			now := time.Now()
			if err := tx.Model(&model.Media{}).Where("id IN ?", extraIDs).Updates(map[string]any{
				"deleted_at": now,
				"updated_at": now,
			}).Error; err != nil {
				return err
			}
		}

		reason := "already_clean"
		if len(extraIDs) > 0 {
			reason = "extras_hidden"
		} else if len(hidePatterns) > 0 {
			reason = "extras_already_hidden"
		}
		hidePath := ""
		if len(hidePatterns) > 0 {
			hidePath = sourcePath
		}
		result = PipelineRepairResult{
			Status:               "success",
			Updated:              len(extraIDs),
			MediaCount:           len(rows),
			OpenListHidePath:     hidePath,
			OpenListHidePatterns: hidePatterns,
			OpenListHiddenCount:  len(hidePatterns),
			Reason:               reason,
		}
		return nil
	})
	return result, err
}

func (s *PipelineMaintenanceService) RepairEpisodeVisibility(ctx context.Context, mediaID string, target PipelineMaintenanceTarget) (PipelineRepairResult, error) {
	category := normalizePipelineCategory(target.Category)
	if category != "tv" && category != "anime" {
		return PipelineRepairResult{Status: "skipped", Updated: 0, Reason: "not_episode_library"}, nil
	}

	resolved, err := s.resolveTarget(ctx, target)
	if err != nil {
		return PipelineRepairResult{}, err
	}
	rootCloudPath := pipelineOpenListPathToCloudPath(resolved.RootOpenListPath)

	var result PipelineRepairResult
	err = s.repos.DB.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		rows, err := pipelineLoadEpisodeVisibilityRowsForUpdate(tx, resolved, strings.TrimSpace(mediaID))
		if err != nil {
			return err
		}
		if len(rows) == 0 {
			return errors.New("MediaStationGo episode visibility target not found")
		}

		updates := pipelineBuildEpisodeVisibilityUpdates(rows, rootCloudPath)
		if len(updates) > 0 {
			if err := pipelineAllowCloudMediaMaintenanceUpdates(tx); err != nil {
				return err
			}
		}
		for _, update := range updates {
			if err := tx.Model(&model.Media{}).Where("id = ?", update.ID).Updates(map[string]any{
				"relative_path": update.RelativePath,
				"season_num":    update.SeasonNum,
				"episode_num":   update.EpisodeNum,
				"episode_title": update.EpisodeTitle,
				"updated_at":    time.Now(),
			}).Error; err != nil {
				return err
			}
		}

		ids := make([]string, 0, len(rows))
		for _, row := range rows {
			ids = append(ids, row.ID)
		}
		var badCount int64
		if err := tx.Model(&model.Media{}).
			Where("id IN ?", ids).
			Where("COALESCE(relative_path, '') = '' OR COALESCE(season_num, 0) <= 0 OR COALESCE(episode_num, 0) <= 0").
			Count(&badCount).Error; err != nil {
			return err
		}
		if badCount != 0 {
			return errors.New("MediaStationGo episode visibility validation failed")
		}

		reason := "already_valid"
		if len(updates) > 0 {
			reason = "repaired"
		}
		result = PipelineRepairResult{Status: "success", Updated: len(updates), MediaCount: len(rows), Reason: reason}
		return nil
	})
	return result, err
}

func (s *PipelineMaintenanceService) PruneDeletedMedia(ctx context.Context, target PipelineMaintenanceTarget, openListPaths []string) (PipelineDeletedMediaPruneResult, error) {
	resolved, err := s.resolveTarget(ctx, target)
	if err != nil {
		return PipelineDeletedMediaPruneResult{}, err
	}

	cloudPaths := make([]string, 0)
	seen := map[string]bool{}
	for _, item := range openListPaths {
		cloudPath := pipelineOpenListPathToCloudPath(item)
		if cloudPath == pipelineOpenListCloudPrefix || seen[cloudPath] {
			continue
		}
		seen[cloudPath] = true
		cloudPaths = append(cloudPaths, cloudPath)
	}
	if len(cloudPaths) == 0 {
		return PipelineDeletedMediaPruneResult{Status: "skipped", Deleted: 0, Reason: "target_missing", MediaIDs: []string{}}, nil
	}

	var mediaIDs []string
	err = s.repos.DB.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		query := tx.Unscoped().Model(&model.Media{}).
			Where("deleted_at IS NOT NULL").
			Where("library_id = ?", resolved.LibraryID)

		parts := make([]string, 0, len(cloudPaths))
		args := make([]any, 0, len(cloudPaths)*2)
		for _, cloudPath := range cloudPaths {
			parts = append(parts, "(path = ? OR path LIKE ?)")
			args = append(args, cloudPath, strings.TrimRight(cloudPath, "/")+"/%")
		}
		query = query.Where("("+strings.Join(parts, " OR ")+")", args...)

		var rows []model.Media
		if err := query.Order("deleted_at ASC, updated_at ASC").Find(&rows).Error; err != nil {
			return err
		}
		for _, row := range rows {
			mediaIDs = append(mediaIDs, row.ID)
		}
		if len(mediaIDs) == 0 {
			return nil
		}
		return tx.Unscoped().Where("id IN ?", mediaIDs).Delete(&model.Media{}).Error
	})
	if err != nil {
		return PipelineDeletedMediaPruneResult{}, err
	}
	if len(mediaIDs) == 0 {
		return PipelineDeletedMediaPruneResult{Status: "skipped", Deleted: 0, Reason: "no_deleted_media", MediaIDs: []string{}}, nil
	}
	return PipelineDeletedMediaPruneResult{Status: "success", Deleted: len(mediaIDs), Reason: "deleted_media_pruned", MediaIDs: mediaIDs}, nil
}

func (s *PipelineMaintenanceService) resolveTarget(ctx context.Context, target PipelineMaintenanceTarget) (pipelineResolvedTarget, error) {
	category := normalizePipelineCategory(target.Category)
	libraryID := strings.TrimSpace(target.LibraryID)
	rootID := strings.TrimSpace(target.RootID)
	rootOpenListPath := pipelineNormalizeOpenListPath(target.RootOpenListPath)

	db := s.repos.DB.WithContext(ctx)
	if libraryID != "" || rootID != "" {
		var root model.LibraryRoot
		query := db
		if rootID != "" {
			query = query.Where("id = ?", rootID)
		}
		if libraryID != "" {
			query = query.Where("library_id = ?", libraryID)
		}
		if err := query.First(&root).Error; err != nil {
			if errors.Is(err, gorm.ErrRecordNotFound) {
				return pipelineResolvedTarget{}, errors.New("MediaStationGo target root not found")
			}
			return pipelineResolvedTarget{}, err
		}
		var library model.Library
		if err := db.Where("id = ?", root.LibraryID).First(&library).Error; err != nil {
			if errors.Is(err, gorm.ErrRecordNotFound) {
				return pipelineResolvedTarget{}, errors.New("MediaStationGo target library not found")
			}
			return pipelineResolvedTarget{}, err
		}
		if !pipelineCategoryCompatible(category, library.Type) {
			return pipelineResolvedTarget{}, errors.New("MediaStationGo target library type does not match category")
		}
		if rootOpenListPath == "" {
			rootOpenListPath = pipelineCloudPathToOpenListPath(root.Path)
		}
		return pipelineResolvedTarget{
			Category:         category,
			LibraryID:        root.LibraryID,
			RootID:           root.ID,
			RootOpenListPath: rootOpenListPath,
		}, nil
	}

	var roots []model.LibraryRoot
	if err := db.Table("library_roots").
		Select("library_roots.*").
		Joins("JOIN libraries ON libraries.id = library_roots.library_id").
		Where("libraries.deleted_at IS NULL").
		Where("library_roots.deleted_at IS NULL").
		Where("libraries.type = ?", category).
		Where("libraries.enabled = ? AND library_roots.enabled = ?", true, true).
		Order("library_roots.sort_order ASC, library_roots.created_at ASC").
		Find(&roots).Error; err != nil {
		return pipelineResolvedTarget{}, err
	}
	if len(roots) == 0 {
		return pipelineResolvedTarget{}, errors.New("MediaStationGo target root not found")
	}
	if len(roots) > 1 {
		return pipelineResolvedTarget{}, errors.New("MediaStationGo target root is ambiguous; provide library_id/root_id")
	}
	root := roots[0]
	if rootOpenListPath == "" {
		rootOpenListPath = pipelineCloudPathToOpenListPath(root.Path)
	}
	return pipelineResolvedTarget{Category: category, LibraryID: root.LibraryID, RootID: root.ID, RootOpenListPath: rootOpenListPath}, nil
}

func pipelineAllowCloudMediaMaintenanceUpdates(tx *gorm.DB) error {
	if tx == nil || tx.Dialector == nil || tx.Dialector.Name() != "postgres" {
		return nil
	}
	return tx.Exec("SELECT set_config('media_pipeline.allow_cloud_media_migration', 'on', true)").Error
}

func pipelineLoadEpisodeVisibilityRowsForUpdate(tx *gorm.DB, target pipelineResolvedTarget, mediaID string) ([]model.Media, error) {
	queryRows := func(sourceCloudPath string, sourceKind string) ([]model.Media, error) {
		query := tx.Where("library_id = ? AND library_root_id = ?", target.LibraryID, target.RootID)
		if sourceKind == "file" {
			query = query.Where("path = ?", sourceCloudPath)
		} else {
			query = query.Where("(path = ? OR path LIKE ?)", sourceCloudPath, strings.TrimRight(sourceCloudPath, "/")+"/%")
		}
		var rows []model.Media
		err := query.Order("path").Find(&rows).Error
		return rows, err
	}

	if mediaID != "" {
		var row model.Media
		if err := tx.Where("library_id = ? AND library_root_id = ? AND id = ?", target.LibraryID, target.RootID, mediaID).First(&row).Error; err != nil {
			if errors.Is(err, gorm.ErrRecordNotFound) {
				return nil, nil
			}
			return nil, err
		}
		sourcePath, sourceKind, err := pipelineMediaWorkItemPath(pipelineCloudPathToOpenListPath(row.Path), target.RootOpenListPath)
		if err != nil {
			return nil, err
		}
		return queryRows(pipelineOpenListPathToCloudPath(sourcePath), sourceKind)
	}

	var rows []model.Media
	err := tx.Where("library_id = ? AND library_root_id = ?", target.LibraryID, target.RootID).Order("path").Find(&rows).Error
	return rows, err
}

func pipelineBuildEpisodeVisibilityUpdates(rows []model.Media, rootCloudPath string) []pipelineEpisodeUpdate {
	usedEpisodeNumbers := map[int]bool{}
	for _, row := range rows {
		if row.EpisodeNum > 0 {
			usedEpisodeNumbers[row.EpisodeNum] = true
		}
	}

	nextEpisode := 1
	updates := make([]pipelineEpisodeUpdate, 0)
	for _, row := range rows {
		relativePath := strings.TrimSpace(row.RelativePath)
		inferredRelativePath := pipelineCloudRelativePath(row.Path, rootCloudPath)
		if relativePath == "" {
			relativePath = inferredRelativePath
		}

		seasonNum := row.SeasonNum
		episodeNum := row.EpisodeNum
		parsedSeason, parsedEpisode := pipelineSeasonEpisodeFromPath(pipelineFirstNonEmpty(relativePath, row.Path))
		if seasonNum <= 0 {
			seasonNum = 1
			if parsedSeason > 0 {
				seasonNum = parsedSeason
			}
		}
		if episodeNum <= 0 {
			candidateEpisode := 0
			if parsedEpisode > 0 && !usedEpisodeNumbers[parsedEpisode] {
				candidateEpisode = parsedEpisode
			}
			if candidateEpisode <= 0 {
				for usedEpisodeNumbers[nextEpisode] {
					nextEpisode++
				}
				candidateEpisode = nextEpisode
			}
			episodeNum = candidateEpisode
			usedEpisodeNumbers[episodeNum] = true
		}

		episodeTitle := strings.TrimSpace(row.EpisodeTitle)
		if episodeTitle == "" {
			episodeTitle = pipelineEpisodeTitleFromRelativePath(relativePath, episodeNum)
		}

		update := pipelineEpisodeUpdate{
			ID:           row.ID,
			RelativePath: relativePath,
			SeasonNum:    seasonNum,
			EpisodeNum:   episodeNum,
			EpisodeTitle: episodeTitle,
		}
		if pipelineEpisodeVisibilityUpdateNeeded(row, update) {
			updates = append(updates, update)
		}
	}
	return updates
}

func pipelineEpisodeVisibilityUpdateNeeded(row model.Media, update pipelineEpisodeUpdate) bool {
	return strings.TrimSpace(row.RelativePath) != update.RelativePath ||
		row.SeasonNum != update.SeasonNum ||
		row.EpisodeNum != update.EpisodeNum ||
		strings.TrimSpace(row.EpisodeTitle) != update.EpisodeTitle
}

func pipelineSeasonEpisodeFromPath(value string) (int, int) {
	if match := pipelineSxxExxPattern.FindStringSubmatch(value); len(match) == 3 {
		return atoi(match[1]), atoi(match[2])
	}
	for _, pattern := range []*regexp.Regexp{pipelineChineseEpisodePattern, pipelineEpisodeTokenPattern, pipelineBracketEpisodePattern} {
		match := pattern.FindStringSubmatch(value)
		if len(match) < 2 {
			continue
		}
		episode := atoi(match[1])
		if pipelineIgnoredEpisodeNumbers[episode] {
			continue
		}
		return 0, episode
	}
	return 0, 0
}

func pipelineEpisodeTitleFromRelativePath(relativePath string, episodeNum int) string {
	base := path.Base(strings.TrimRight(relativePath, "/"))
	ext := path.Ext(base)
	title := strings.TrimSpace(strings.TrimSuffix(base, ext))
	if title != "" {
		return title
	}
	if episodeNum <= 0 {
		episodeNum = 1
	}
	return "Episode " + strconv.Itoa(episodeNum)
}

func pipelineMovieMediaRowLooksLikeExtra(row model.Media, sourceOpenListPath string) bool {
	openListPath := pipelineCloudPathToOpenListPath(row.Path)
	if !pipelinePathIsSameOrChild(openListPath, sourceOpenListPath) {
		return false
	}
	sourcePath := pipelineNormalizeOpenListPath(sourceOpenListPath)
	relative := strings.Trim(strings.TrimPrefix(openListPath, sourcePath), "/")
	parts := pipelineSplitNonEmpty(relative, "/")
	if len(parts) <= 1 {
		return false
	}

	last := parts[len(parts)-1]
	last = strings.TrimSuffix(last, path.Ext(last))
	text := strings.Join(append(parts[:len(parts)-1], last), "/")
	normalized := strings.ToLower(pipelineExtraNormalizePattern.ReplaceAllString(text, ""))
	for _, token := range []string{
		"bonus", "extra", "extras", "gallery", "images", "menu", "pv", "specials", "tokuten",
		"予告", "图集", "映像特典", "特典", "特典映像", "特报", "花絮", "菜单", "预告",
	} {
		cleanToken := strings.ToLower(pipelineExtraNormalizePattern.ReplaceAllString(token, ""))
		if cleanToken != "" && strings.Contains(normalized, cleanToken) {
			return true
		}
	}
	return false
}

func pipelineMovieExtraHidePatterns(rows []model.Media, sourceOpenListPath string) []string {
	sourcePath := pipelineNormalizeOpenListPath(sourceOpenListPath)
	patterns := make([]string, 0)
	seen := map[string]bool{}
	for _, row := range rows {
		openListPath := pipelineCloudPathToOpenListPath(row.Path)
		if !pipelinePathIsSameOrChild(openListPath, sourcePath) || openListPath == sourcePath {
			continue
		}
		relative := strings.Trim(strings.TrimPrefix(openListPath, sourcePath), "/")
		firstPart := strings.TrimSpace(strings.SplitN(relative, "/", 2)[0])
		if firstPart == "" {
			continue
		}
		pattern := "^" + regexp.QuoteMeta(firstPart) + "$"
		if seen[pattern] {
			continue
		}
		seen[pattern] = true
		patterns = append(patterns, pattern)
	}
	return patterns
}

func pipelineMediaWorkItemPath(mediaPath, rootPath string) (string, string, error) {
	mediaPath = pipelineNormalizeOpenListPath(mediaPath)
	rootPath = pipelineNormalizeOpenListPath(rootPath)
	if mediaPath == rootPath {
		return "", "", errors.New("media path equals root path")
	}
	if !pipelinePathIsSameOrChild(mediaPath, rootPath) {
		return "", "", errors.New("media path is outside library root")
	}
	relative := strings.TrimPrefix(mediaPath, strings.TrimRight(rootPath, "/")+"/")
	parts := pipelineSplitNonEmpty(relative, "/")
	if len(parts) == 0 {
		return "", "", errors.New("media relative path missing")
	}
	sourcePath := path.Join(rootPath, parts[0])
	sourceKind := "file"
	if len(parts) > 1 {
		sourceKind = "folder"
	}
	return pipelineNormalizeOpenListPath(sourcePath), sourceKind, nil
}

// pipelineMediaDetailWorkItemPath keeps a non-episodic detail-page migration
// scoped to the selected playable item's containing directory. Imported
// collection trees can contain several independent shows and movies below one
// top-level task folder; promoting a nested movie to that task folder would
// migrate unrelated media.
func pipelineMediaDetailWorkItemPath(mediaPath, rootPath string) (string, string, error) {
	mediaPath = pipelineNormalizeOpenListPath(mediaPath)
	rootPath = pipelineNormalizeOpenListPath(rootPath)
	if mediaPath == rootPath {
		return "", "", errors.New("media path equals root path")
	}
	if !pipelinePathIsSameOrChild(mediaPath, rootPath) {
		return "", "", errors.New("media path is outside library root")
	}
	parentPath := pipelineNormalizeOpenListPath(path.Dir(mediaPath))
	if parentPath == rootPath {
		return mediaPath, "file", nil
	}
	if parentPath == "" || !pipelinePathIsSameOrChild(parentPath, rootPath) {
		return "", "", errors.New("media parent path is outside library root")
	}
	return parentPath, "folder", nil
}

func pipelineCloudPathToOpenListPath(value string) string {
	raw := strings.TrimSpace(value)
	if raw == "" {
		return ""
	}
	if strings.HasPrefix(raw, pipelineOpenListCloudPrefix+"/") {
		raw = strings.TrimPrefix(raw, pipelineOpenListCloudPrefix)
	} else if strings.HasPrefix(raw, pipelineOpenListCloudPrefix) {
		raw = strings.TrimPrefix(raw, pipelineOpenListCloudPrefix)
	}
	if decoded, err := url.PathUnescape(raw); err == nil {
		raw = decoded
	}
	return pipelineNormalizeOpenListPath(raw)
}

func pipelineOpenListPathToCloudPath(openListPath string) string {
	return pipelineOpenListCloudPrefix + pipelineNormalizeOpenListPath(openListPath)
}

func pipelineNormalizeOpenListPath(value string) string {
	raw := strings.TrimSpace(strings.ReplaceAll(value, "\\", "/"))
	if raw == "" {
		return ""
	}
	if !strings.HasPrefix(raw, "/") {
		raw = "/" + raw
	}
	normalized := path.Clean(raw)
	if normalized == "." {
		return ""
	}
	return normalized
}

func pipelinePathIsSameOrChild(itemPath, parentPath string) bool {
	itemPath = pipelineNormalizeOpenListPath(itemPath)
	parentPath = pipelineNormalizeOpenListPath(parentPath)
	return itemPath == parentPath || strings.HasPrefix(itemPath, strings.TrimRight(parentPath, "/")+"/")
}

func pipelineCloudRelativePath(itemPath, rootCloudPath string) string {
	rootCloudPath = strings.TrimRight(rootCloudPath, "/")
	if itemPath == rootCloudPath {
		return ""
	}
	if strings.HasPrefix(itemPath, rootCloudPath+"/") {
		return strings.TrimPrefix(itemPath, rootCloudPath+"/")
	}
	return ""
}

func normalizePipelineCategory(category string) string {
	return strings.ToLower(strings.TrimSpace(category))
}

func pipelineCategoryCompatible(category, libraryType string) bool {
	category = normalizePipelineCategory(category)
	libraryType = normalizePipelineCategory(libraryType)
	return category == "" || category == "other" || category == libraryType
}

func pipelineSplitNonEmpty(value, sep string) []string {
	raw := strings.Split(value, sep)
	out := make([]string, 0, len(raw))
	for _, item := range raw {
		item = strings.TrimSpace(item)
		if item != "" {
			out = append(out, item)
		}
	}
	return out
}

func pipelineFirstNonEmpty(values ...string) string {
	for _, value := range values {
		if strings.TrimSpace(value) != "" {
			return value
		}
	}
	return ""
}

func atoi(value string) int {
	parsed, _ := strconv.Atoi(strings.TrimSpace(value))
	return parsed
}
