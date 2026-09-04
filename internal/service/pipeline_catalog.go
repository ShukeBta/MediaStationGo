package service

import (
	"context"
	"errors"
	"net/url"
	pathpkg "path"
	"regexp"
	"sort"
	"strings"
	"time"

	"github.com/ShukeBta/MediaStationGo/internal/model"
	"gorm.io/gorm"
	"gorm.io/gorm/clause"
)

type PipelineDeletedMediaHideCandidate struct {
	MediaID            string `json:"media_id"`
	LibraryID          string `json:"library_id"`
	LibraryRootID      string `json:"library_root_id"`
	Category           string `json:"category"`
	MediaPath          string `json:"media_path"`
	TargetOpenListPath string `json:"target_openlist_path"`
	TargetKind         string `json:"target_kind"`
	HidePath           string `json:"hide_path"`
	HidePattern        string `json:"hide_pattern"`
	DeletedAt          string `json:"deleted_at"`
}

type PipelineDeletedMediaHideCandidatesResult struct {
	Items []PipelineDeletedMediaHideCandidate `json:"items"`
}

type PipelineMigrationSearchRequest struct {
	Query string `json:"query"`
	Limit int    `json:"limit"`
}

type PipelineMigrationCandidate struct {
	Title              string `json:"title"`
	LibraryID          string `json:"library_id"`
	LibraryRootID      string `json:"library_root_id"`
	LibraryName        string `json:"library_name"`
	LibraryType        string `json:"library_type"`
	Category           string `json:"category"`
	SourceOpenListPath string `json:"source_openlist_path"`
	SourceKind         string `json:"source_kind"`
	MediaCount         int    `json:"media_count"`
	TotalSize          int64  `json:"total_size"`
	SamplePath         string `json:"sample_path"`
}

type PipelineMigrationSearchResult struct {
	Items []PipelineMigrationCandidate `json:"items"`
}

type PipelineMigrationSource struct {
	Category           string `json:"category,omitempty"`
	LibraryID          string `json:"library_id"`
	LibraryRootID      string `json:"library_root_id"`
	SourceOpenListPath string `json:"source_openlist_path"`
	SourceKind         string `json:"source_kind,omitempty"`
}

type PipelineMigrationRequest struct {
	Source             PipelineMigrationSource   `json:"source"`
	Target             PipelineMaintenanceTarget `json:"target"`
	TargetOpenListPath string                    `json:"target_openlist_path,omitempty"`
}

type PipelineMigrationResult struct {
	SourceOpenListPath string `json:"source_openlist_path"`
	TargetOpenListPath string `json:"target_openlist_path"`
	TargetCategory     string `json:"target_category"`
	MediaCount         int    `json:"media_count"`
	SeriesCount        int    `json:"series_count"`
	OpenListMoved      bool   `json:"openlist_moved,omitempty"`
	DedupeIndexCount   int    `json:"dedupe_index_count,omitempty"`
	DedupeIndexError   string `json:"dedupe_index_error,omitempty"`
}

type MediaMigrationPreview struct {
	Candidate PipelineMigrationCandidate `json:"candidate"`
	Result    PipelineMigrationResult    `json:"result"`
}

type pipelineMigrationSearchRow struct {
	ID            string
	LibraryID     string
	LibraryRootID string
	SeriesID      string
	Title         string
	OriginalName  string
	Path          string
	SizeBytes     int64
	LibraryName   string
	LibraryType   string
	RootPath      string
	SeasonNum     int
	EpisodeNum    int
}

type pipelineDeletedMediaRow struct {
	ID            string
	LibraryID     string
	LibraryRootID string
	Path          string
	DeletedAt     time.Time
	DeletionKind  string
	LibraryType   string
	RootPath      string
}

type pipelinePreparedMigration struct {
	SourcePath string
	SourceKind string
	TargetPath string
	Target     pipelineResolvedTarget
	Rows       []model.Media
	SeriesIDs  []string
}

func (s *PipelineMaintenanceService) ListDeletedMediaHideCandidates(ctx context.Context, limit int) (PipelineDeletedMediaHideCandidatesResult, error) {
	limit = pipelineBoundedLimit(limit, 100, 1000)
	var rows []pipelineDeletedMediaRow
	err := s.repos.DB.WithContext(ctx).Unscoped().Table("media AS m").
		Select("m.id, m.library_id, m.library_root_id, m.path, m.deleted_at, m.deletion_kind, l.type AS library_type, r.path AS root_path").
		Joins("LEFT JOIN libraries AS l ON l.id = m.library_id").
		Joins("LEFT JOIN library_roots AS r ON r.id = m.library_root_id").
		Where("m.deleted_at IS NOT NULL").
		Where("COALESCE(m.path, '') LIKE ?", pipelineOpenListCloudPrefix+"/%").
		Order("m.deleted_at ASC, m.updated_at ASC").
		Limit(limit).
		Scan(&rows).Error
	if err != nil {
		return PipelineDeletedMediaHideCandidatesResult{}, err
	}

	items := make([]PipelineDeletedMediaHideCandidate, 0, len(rows))
	for _, row := range rows {
		mediaPath := pipelineCloudPathToOpenListPath(row.Path)
		rootPath := pipelineCloudPathToOpenListPath(row.RootPath)
		if mediaPath == "" || rootPath == "" || mediaPath == rootPath || !pipelinePathIsSameOrChild(mediaPath, rootPath) {
			continue
		}
		category := normalizePipelineCategory(row.LibraryType)
		targetPath := mediaPath
		targetKind := "file"
		if !strings.EqualFold(strings.TrimSpace(row.DeletionKind), "version") && category != "tv" && category != "anime" {
			var workErr error
			targetPath, targetKind, workErr = pipelineMediaWorkItemPath(mediaPath, rootPath)
			if workErr != nil {
				continue
			}
		}
		hideName := pathpkg.Base(strings.TrimRight(targetPath, "/"))
		if hideName == "" || hideName == "." || hideName == "/" {
			continue
		}
		items = append(items, PipelineDeletedMediaHideCandidate{
			MediaID:            row.ID,
			LibraryID:          row.LibraryID,
			LibraryRootID:      row.LibraryRootID,
			Category:           category,
			MediaPath:          mediaPath,
			TargetOpenListPath: targetPath,
			TargetKind:         targetKind,
			HidePath:           pipelineNormalizeOpenListPath(pathpkg.Dir(strings.TrimRight(targetPath, "/"))),
			HidePattern:        "^" + regexp.QuoteMeta(hideName) + "$",
			DeletedAt:          row.DeletedAt.UTC().Format(time.RFC3339Nano),
		})
	}
	return PipelineDeletedMediaHideCandidatesResult{Items: items}, nil
}

func (s *PipelineMaintenanceService) SearchMigrationCandidates(ctx context.Context, req PipelineMigrationSearchRequest) (PipelineMigrationSearchResult, error) {
	query := strings.TrimSpace(req.Query)
	if query == "" {
		return PipelineMigrationSearchResult{}, errors.New("migration query must not be empty")
	}
	limit := pipelineBoundedLimit(req.Limit, 20, 100)
	pattern := "%" + strings.ToLower(query) + "%"
	var rows []pipelineMigrationSearchRow
	err := s.repos.DB.WithContext(ctx).Table("media AS m").
		Select("m.id, m.library_id, m.library_root_id, m.series_id, m.title, m.original_name, m.path, m.size_bytes, m.season_num, m.episode_num, l.name AS library_name, l.type AS library_type, r.path AS root_path").
		Joins("LEFT JOIN libraries AS l ON l.id = m.library_id").
		Joins("LEFT JOIN library_roots AS r ON r.id = m.library_root_id").
		Where("m.deleted_at IS NULL").
		Where("LOWER(COALESCE(m.title, '')) LIKE ? OR LOWER(COALESCE(m.original_name, '')) LIKE ? OR LOWER(COALESCE(m.path, '')) LIKE ?", pattern, pattern, pattern).
		Order("m.updated_at DESC, m.created_at DESC").
		Limit(limit * 20).
		Scan(&rows).Error
	if err != nil {
		return PipelineMigrationSearchResult{}, err
	}

	items := make([]PipelineMigrationCandidate, 0, limit)
	indexes := make(map[string]int)
	for _, row := range rows {
		mediaPath := pipelineCloudPathToOpenListPath(row.Path)
		rootPath := pipelineCloudPathToOpenListPath(row.RootPath)
		sourcePath, sourceKind, err := pipelineMediaWorkItemPath(mediaPath, rootPath)
		if err != nil {
			continue
		}
		key := row.LibraryID + "\x00" + row.LibraryRootID + "\x00" + sourcePath
		if index, ok := indexes[key]; ok {
			items[index].MediaCount++
			items[index].TotalSize += row.SizeBytes
			continue
		}
		if len(items) >= limit {
			continue
		}
		title := strings.TrimSpace(row.Title)
		if title == "" {
			title = strings.TrimSpace(row.OriginalName)
		}
		if title == "" {
			title = pathpkg.Base(sourcePath)
		}
		libraryName := strings.TrimSpace(row.LibraryName)
		if libraryName == "" {
			libraryName = "-"
		}
		libraryType := normalizePipelineCategory(row.LibraryType)
		indexes[key] = len(items)
		items = append(items, PipelineMigrationCandidate{
			Title:              title,
			LibraryID:          row.LibraryID,
			LibraryRootID:      row.LibraryRootID,
			LibraryName:        libraryName,
			LibraryType:        libraryType,
			Category:           libraryType,
			SourceOpenListPath: sourcePath,
			SourceKind:         sourceKind,
			MediaCount:         1,
			TotalSize:          row.SizeBytes,
			SamplePath:         mediaPath,
		})
	}
	return PipelineMigrationSearchResult{Items: items}, nil
}

func (s *PipelineMaintenanceService) MigrationCandidateForMedia(ctx context.Context, mediaID string) (PipelineMigrationCandidate, error) {
	mediaID = strings.TrimSpace(mediaID)
	if mediaID == "" {
		return PipelineMigrationCandidate{}, errors.New("media id is required")
	}
	var row pipelineMigrationSearchRow
	err := s.repos.DB.WithContext(ctx).Table("media AS m").
		Select("m.id, m.library_id, m.library_root_id, m.series_id, m.title, m.original_name, m.path, m.size_bytes, m.season_num, m.episode_num, l.name AS library_name, l.type AS library_type, r.path AS root_path").
		Joins("LEFT JOIN libraries AS l ON l.id = m.library_id").
		Joins("LEFT JOIN library_roots AS r ON r.id = m.library_root_id").
		Where("m.id = ? AND m.deleted_at IS NULL", mediaID).
		First(&row).Error
	if err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return PipelineMigrationCandidate{}, errors.New("media not found")
		}
		return PipelineMigrationCandidate{}, err
	}
	mediaPath := pipelineCloudPathToOpenListPath(row.Path)
	rootPath := pipelineCloudPathToOpenListPath(row.RootPath)
	var sourcePath, sourceKind string
	if row.SeasonNum > 0 || row.EpisodeNum > 0 {
		sourcePath, sourceKind, err = pipelineMediaWorkItemPath(mediaPath, rootPath)
	} else {
		sourcePath, sourceKind, err = pipelineMediaDetailWorkItemPath(mediaPath, rootPath)
	}
	if err != nil {
		return PipelineMigrationCandidate{}, err
	}
	sourceCloudPath := pipelineOpenListPathToCloudPath(sourcePath)
	query := s.repos.DB.WithContext(ctx).Model(&model.Media{}).
		Where("library_id = ? AND library_root_id = ?", row.LibraryID, row.LibraryRootID)
	if sourceKind == "file" {
		query = query.Where("path = ?", sourceCloudPath)
	} else {
		query = query.Where("path = ? OR path LIKE ?", sourceCloudPath, strings.TrimRight(sourceCloudPath, "/")+"/%")
	}
	var aggregate struct {
		MediaCount int
		TotalSize  int64
	}
	if err := query.Select("COUNT(*) AS media_count, COALESCE(SUM(size_bytes), 0) AS total_size").Scan(&aggregate).Error; err != nil {
		return PipelineMigrationCandidate{}, err
	}
	title := pipelineFirstNonEmpty(row.Title, row.OriginalName, pathpkg.Base(sourcePath))
	libraryName := strings.TrimSpace(row.LibraryName)
	if libraryName == "" {
		libraryName = "-"
	}
	category := normalizePipelineCategory(row.LibraryType)
	return PipelineMigrationCandidate{
		Title:              title,
		LibraryID:          row.LibraryID,
		LibraryRootID:      row.LibraryRootID,
		LibraryName:        libraryName,
		LibraryType:        category,
		Category:           category,
		SourceOpenListPath: sourcePath,
		SourceKind:         sourceKind,
		MediaCount:         aggregate.MediaCount,
		TotalSize:          aggregate.TotalSize,
		SamplePath:         mediaPath,
	}, nil
}

func (s *PipelineMaintenanceService) ValidateMediaMigration(ctx context.Context, ownerID, mediaID, targetCategory string) (MediaMigrationPreview, error) {
	candidate, err := s.MigrationCandidateForMedia(ctx, mediaID)
	if err != nil {
		return MediaMigrationPreview{}, err
	}
	if s.migrationClient == nil {
		return MediaMigrationPreview{}, errors.New("media-pipeline migration service unavailable")
	}
	result, err := s.migrationClient.ValidateMediaMigration(ctx, ownerID, candidate, targetCategory)
	if err != nil {
		return MediaMigrationPreview{}, err
	}
	if err := validatePipelineMigrationBridgeResult(candidate, targetCategory, result, false); err != nil {
		return MediaMigrationPreview{}, err
	}
	return MediaMigrationPreview{Candidate: candidate, Result: result}, nil
}

func (s *PipelineMaintenanceService) ApplyMediaMigration(ctx context.Context, ownerID, mediaID, targetCategory string) (MediaMigrationPreview, error) {
	candidate, err := s.MigrationCandidateForMedia(ctx, mediaID)
	if err != nil {
		return MediaMigrationPreview{}, err
	}
	if s.migrationClient == nil {
		return MediaMigrationPreview{}, errors.New("media-pipeline migration service unavailable")
	}
	result, err := s.migrationClient.ApplyMediaMigration(ctx, ownerID, candidate, targetCategory)
	if err != nil {
		return MediaMigrationPreview{}, err
	}
	if err := validatePipelineMigrationBridgeResult(candidate, targetCategory, result, true); err != nil {
		return MediaMigrationPreview{}, err
	}
	if s.cache != nil {
		s.cache.DeletePrefix(ctx, "media:")
		s.cache.DeletePrefix(ctx, "stats:")
	}
	return MediaMigrationPreview{Candidate: candidate, Result: result}, nil
}

func validatePipelineMigrationBridgeResult(candidate PipelineMigrationCandidate, targetCategory string, result PipelineMigrationResult, applied bool) error {
	targetCategory = normalizePipelineCategory(targetCategory)
	if result.SourceOpenListPath != candidate.SourceOpenListPath || result.TargetCategory != targetCategory || strings.TrimSpace(result.TargetOpenListPath) == "" {
		return errors.New("media-pipeline migration returned an invalid result")
	}
	if applied && !result.OpenListMoved {
		return errors.New("media-pipeline migration did not confirm the OpenList move")
	}
	return nil
}

func (s *PipelineMaintenanceService) ValidateMigration(ctx context.Context, req PipelineMigrationRequest) (PipelineMigrationResult, error) {
	target, err := s.resolveTarget(ctx, req.Target)
	if err != nil {
		return PipelineMigrationResult{}, err
	}
	var result PipelineMigrationResult
	err = s.repos.DB.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		prepared, err := pipelinePrepareMigration(tx, req.Source, target, req.TargetOpenListPath, false)
		if err != nil {
			return err
		}
		result = pipelineMigrationResult(prepared)
		return nil
	})
	return result, err
}

func (s *PipelineMaintenanceService) ApplyMigration(ctx context.Context, req PipelineMigrationRequest) (PipelineMigrationResult, error) {
	target, err := s.resolveTarget(ctx, req.Target)
	if err != nil {
		return PipelineMigrationResult{}, err
	}
	var result PipelineMigrationResult
	err = s.repos.DB.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		prepared, err := pipelinePrepareMigration(tx, req.Source, target, req.TargetOpenListPath, true)
		if err != nil {
			return err
		}
		if err := pipelineAllowCloudMediaMaintenanceUpdates(tx); err != nil {
			return err
		}

		sourceCloudPath := pipelineOpenListPathToCloudPath(prepared.SourcePath)
		targetCloudPath := pipelineOpenListPathToCloudPath(prepared.TargetPath)
		targetRootCloudPath := pipelineOpenListPathToCloudPath(prepared.Target.RootOpenListPath)
		mediaIDs := make([]string, 0, len(prepared.Rows))
		now := time.Now()
		for _, row := range prepared.Rows {
			newPath, err := pipelineReplacePathPrefix(row.Path, sourceCloudPath, targetCloudPath)
			if err != nil {
				return err
			}
			mediaIDs = append(mediaIDs, row.ID)
			if err := tx.Model(&model.Media{}).Where("id = ?", row.ID).Updates(map[string]any{
				"library_id":      prepared.Target.LibraryID,
				"library_root_id": prepared.Target.RootID,
				"path":            newPath,
				"relative_path":   pipelineCloudRelativePath(newPath, targetRootCloudPath),
				"strm_url":        pipelineReplaceOpenListReferences(row.STRMURL, prepared.SourcePath, prepared.TargetPath),
				"updated_at":      now,
			}).Error; err != nil {
				return err
			}
		}

		if len(prepared.SeriesIDs) > 0 {
			if err := tx.Model(&model.Series{}).Where("id IN ?", prepared.SeriesIDs).Updates(map[string]any{
				"library_id": prepared.Target.LibraryID,
				"updated_at": now,
			}).Error; err != nil {
				return err
			}
		}
		if err := pipelineMigrateSTRMRecords(tx, mediaIDs, prepared.SourcePath, prepared.TargetPath); err != nil {
			return err
		}
		if err := pipelineValidateMigratedMedia(tx, mediaIDs, prepared); err != nil {
			return err
		}
		result = pipelineMigrationResult(prepared)
		return nil
	})
	return result, err
}

func pipelinePrepareMigration(tx *gorm.DB, source PipelineMigrationSource, target pipelineResolvedTarget, requestedTargetPath string, lock bool) (pipelinePreparedMigration, error) {
	source.LibraryID = strings.TrimSpace(source.LibraryID)
	source.LibraryRootID = strings.TrimSpace(source.LibraryRootID)
	source.SourceOpenListPath = pipelineNormalizeOpenListPath(source.SourceOpenListPath)
	if source.LibraryID == "" || source.LibraryRootID == "" || source.SourceOpenListPath == "" {
		return pipelinePreparedMigration{}, errors.New("migration source library_id, library_root_id and source_openlist_path are required")
	}
	if source.LibraryID == target.LibraryID && source.LibraryRootID == target.RootID {
		return pipelinePreparedMigration{}, errors.New("migration target must be different from source")
	}

	var sourceRoot model.LibraryRoot
	if err := tx.Where("id = ? AND library_id = ?", source.LibraryRootID, source.LibraryID).First(&sourceRoot).Error; err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return pipelinePreparedMigration{}, errors.New("MediaStationGo source root not found")
		}
		return pipelinePreparedMigration{}, err
	}
	sourceRootPath := pipelineCloudPathToOpenListPath(sourceRoot.Path)
	if sourceRootPath == "" {
		return pipelinePreparedMigration{}, errors.New("MediaStationGo source root path is invalid")
	}
	var targetRoot model.LibraryRoot
	if err := tx.Where("id = ? AND library_id = ?", target.RootID, target.LibraryID).First(&targetRoot).Error; err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return pipelinePreparedMigration{}, errors.New("MediaStationGo target root not found")
		}
		return pipelinePreparedMigration{}, err
	}
	if actualTargetRootPath := pipelineCloudPathToOpenListPath(targetRoot.Path); actualTargetRootPath == "" || actualTargetRootPath != target.RootOpenListPath {
		return pipelinePreparedMigration{}, errors.New("MediaStationGo target root path does not match request")
	}
	if source.SourceOpenListPath == sourceRootPath || !pipelinePathIsSameOrChild(source.SourceOpenListPath, sourceRootPath) {
		return pipelinePreparedMigration{}, errors.New("migration source path is not a direct library work item")
	}
	relativeSourcePath := strings.Trim(strings.TrimPrefix(source.SourceOpenListPath, strings.TrimRight(sourceRootPath, "/")), "/")
	if relativeSourcePath == "" {
		return pipelinePreparedMigration{}, errors.New("migration source path is not inside the library root")
	}
	sourceKind := strings.TrimSpace(source.SourceKind)
	if sourceKind != "" && sourceKind != "file" && sourceKind != "folder" {
		return pipelinePreparedMigration{}, errors.New("migration source kind is invalid")
	}

	sourceCloudPath := pipelineOpenListPathToCloudPath(source.SourceOpenListPath)
	query := tx.Where("library_id = ? AND library_root_id = ?", source.LibraryID, source.LibraryRootID)
	if sourceKind == "file" {
		query = query.Where("path = ?", sourceCloudPath)
	} else {
		query = query.Where("path = ? OR path LIKE ?", sourceCloudPath, strings.TrimRight(sourceCloudPath, "/")+"/%")
	}
	if lock {
		query = query.Clauses(clause.Locking{Strength: "UPDATE"})
	}
	var rows []model.Media
	if err := query.Order("path").Find(&rows).Error; err != nil {
		return pipelinePreparedMigration{}, err
	}
	if len(rows) == 0 {
		return pipelinePreparedMigration{}, errors.New("MediaStationGo source media not found: " + source.SourceOpenListPath)
	}
	inferredSourceKind := "file"
	for _, row := range rows {
		if row.Path != sourceCloudPath {
			inferredSourceKind = "folder"
			break
		}
	}
	if sourceKind == "" {
		sourceKind = inferredSourceKind
	} else if sourceKind != inferredSourceKind {
		return pipelinePreparedMigration{}, errors.New("migration source kind does not match stored media paths")
	}

	targetPath, err := pipelineMigrationTargetPath(source.SourceOpenListPath, sourceKind, target.RootOpenListPath, requestedTargetPath)
	if err != nil {
		return pipelinePreparedMigration{}, err
	}
	targetCloudPath := pipelineOpenListPathToCloudPath(targetPath)
	targetConflictPath := targetCloudPath
	if sourceKind == "file" {
		targetConflictPath = pipelineOpenListPathToCloudPath(pathpkg.Dir(targetPath))
	}
	var targetCount int64
	if err := tx.Unscoped().Model(&model.Media{}).
		Where("path = ? OR path LIKE ?", targetConflictPath, strings.TrimRight(targetConflictPath, "/")+"/%").
		Count(&targetCount).Error; err != nil {
		return pipelinePreparedMigration{}, err
	}
	if targetCount > 0 {
		return pipelinePreparedMigration{}, errors.New("MediaStationGo target already exists: " + targetPath)
	}

	mediaIDs := make([]string, 0, len(rows))
	seriesSet := make(map[string]bool)
	for _, row := range rows {
		mediaIDs = append(mediaIDs, row.ID)
		if row.SeriesID != "" {
			seriesSet[row.SeriesID] = true
		}
	}
	seriesIDs := make([]string, 0, len(seriesSet))
	for seriesID := range seriesSet {
		seriesIDs = append(seriesIDs, seriesID)
	}
	sort.Strings(seriesIDs)
	if len(seriesIDs) > 0 {
		var outside model.Media
		err := tx.Where("series_id IN ?", seriesIDs).Where("id NOT IN ?", mediaIDs).Order("series_id").First(&outside).Error
		if err == nil {
			return pipelinePreparedMigration{}, errors.New("refuse partial series migration: " + outside.SeriesID)
		}
		if !errors.Is(err, gorm.ErrRecordNotFound) {
			return pipelinePreparedMigration{}, err
		}
	}

	return pipelinePreparedMigration{
		SourcePath: source.SourceOpenListPath,
		SourceKind: sourceKind,
		TargetPath: targetPath,
		Target:     target,
		Rows:       rows,
		SeriesIDs:  seriesIDs,
	}, nil
}

func pipelineMigrationTargetPath(sourcePath, sourceKind, targetRootPath, requestedTargetPath string) (string, error) {
	targetRootPath = pipelineNormalizeOpenListPath(targetRootPath)
	sourceName := pathpkg.Base(pipelineNormalizeOpenListPath(sourcePath))
	requestedTargetPath = pipelineNormalizeOpenListPath(requestedTargetPath)
	defaultTargetPath := pipelineNormalizeOpenListPath(pathpkg.Join(targetRootPath, sourceName))
	if sourceKind != "file" {
		if requestedTargetPath == "" {
			return defaultTargetPath, nil
		}
		if requestedTargetPath != defaultTargetPath {
			return "", errors.New("folder migration target must preserve the source directory name")
		}
		return requestedTargetPath, nil
	}
	if requestedTargetPath == "" {
		return "", errors.New("file migration target_openlist_path is required")
	}
	if pathpkg.Base(requestedTargetPath) != sourceName {
		return "", errors.New("file migration target must preserve the source file name")
	}
	targetParent := pipelineNormalizeOpenListPath(pathpkg.Dir(requestedTargetPath))
	if targetParent == targetRootPath || pipelineNormalizeOpenListPath(pathpkg.Dir(targetParent)) != targetRootPath {
		return "", errors.New("file migration target must be inside one work directory under the library root")
	}
	return requestedTargetPath, nil
}

func pipelineMigrateSTRMRecords(tx *gorm.DB, mediaIDs []string, sourcePath, targetPath string) error {
	if len(mediaIDs) == 0 {
		return nil
	}
	var rows []model.STRMRecord
	if err := tx.Where("media_id IN ?", mediaIDs).Find(&rows).Error; err != nil {
		return err
	}
	for _, row := range rows {
		filePath := pipelineReplaceOpenListReferences(row.FilePath, sourcePath, targetPath)
		strmURL := pipelineReplaceOpenListReferences(row.URL, sourcePath, targetPath)
		if filePath == row.FilePath && strmURL == row.URL {
			continue
		}
		if err := tx.Model(&model.STRMRecord{}).Where("id = ?", row.ID).Updates(map[string]any{
			"file_path":  filePath,
			"url":        strmURL,
			"updated_at": time.Now(),
		}).Error; err != nil {
			return err
		}
	}
	return nil
}

func pipelineValidateMigratedMedia(tx *gorm.DB, mediaIDs []string, prepared pipelinePreparedMigration) error {
	var rows []model.Media
	if err := tx.Where("id IN ?", mediaIDs).Find(&rows).Error; err != nil {
		return err
	}
	if len(rows) != len(mediaIDs) {
		return errors.New("MediaStationGo migration read-back validation failed")
	}
	targetCloudPath := pipelineOpenListPathToCloudPath(prepared.TargetPath)
	for _, row := range rows {
		if row.LibraryID != prepared.Target.LibraryID || row.LibraryRootID != prepared.Target.RootID ||
			(row.Path != targetCloudPath && !strings.HasPrefix(row.Path, strings.TrimRight(targetCloudPath, "/")+"/")) ||
			pipelineContainsOpenListReference(row.STRMURL, prepared.SourcePath) {
			return errors.New("MediaStationGo migration read-back validation failed")
		}
	}
	var strmRows []model.STRMRecord
	if err := tx.Where("media_id IN ?", mediaIDs).Find(&strmRows).Error; err != nil {
		return err
	}
	for _, row := range strmRows {
		if pipelineContainsOpenListReference(row.URL, prepared.SourcePath) || pipelineContainsOpenListReference(row.FilePath, prepared.SourcePath) {
			return errors.New("MediaStationGo migration STRM read-back validation failed")
		}
	}
	return nil
}

func pipelineMigrationResult(prepared pipelinePreparedMigration) PipelineMigrationResult {
	return PipelineMigrationResult{
		SourceOpenListPath: prepared.SourcePath,
		TargetOpenListPath: prepared.TargetPath,
		TargetCategory:     prepared.Target.Category,
		MediaCount:         len(prepared.Rows),
		SeriesCount:        len(prepared.SeriesIDs),
	}
}

func pipelineReplacePathPrefix(value, sourcePrefix, targetPrefix string) (string, error) {
	if value == sourcePrefix {
		return targetPrefix, nil
	}
	trimmed := strings.TrimRight(sourcePrefix, "/")
	if strings.HasPrefix(value, trimmed+"/") {
		return strings.TrimRight(targetPrefix, "/") + value[len(trimmed):], nil
	}
	return "", errors.New("path does not start with migration source prefix")
}

func pipelineReplaceOpenListReferences(value, sourcePath, targetPath string) string {
	sourcePath = pipelineNormalizeOpenListPath(sourcePath)
	targetPath = pipelineNormalizeOpenListPath(targetPath)
	pairs := [][2]string{
		{url.QueryEscape(sourcePath), url.QueryEscape(targetPath)},
		{url.PathEscape(sourcePath), url.PathEscape(targetPath)},
		{sourcePath, targetPath},
	}
	for _, pair := range pairs {
		value = strings.ReplaceAll(value, pair[0], pair[1])
	}
	return value
}

func pipelineContainsOpenListReference(value, sourcePath string) bool {
	sourcePath = pipelineNormalizeOpenListPath(sourcePath)
	return strings.Contains(value, sourcePath) ||
		strings.Contains(value, url.QueryEscape(sourcePath)) ||
		strings.Contains(value, url.PathEscape(sourcePath))
}

func pipelineBoundedLimit(value, fallback, maximum int) int {
	if value <= 0 {
		value = fallback
	}
	if value > maximum {
		value = maximum
	}
	return value
}
