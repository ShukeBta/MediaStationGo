package service

import (
	"context"
	"os"
	"path/filepath"
	"strings"

	"go.uber.org/zap"

	"github.com/ShukeBta/MediaStationGo/internal/model"
)

// RemovePath deletes the media row for a path that has disappeared from disk
// (incremental delete used by the watcher on Remove/Rename events).
func (s *ScannerService) RemovePath(ctx context.Context, path string) (int64, error) {
	if _, err := os.Stat(path); err == nil {
		return 0, nil // still exists; nothing to remove
	}
	res := s.repo.DB.WithContext(ctx).
		Where("path = ?", path).
		Delete(&model.Media{})
	if res.Error == nil && res.RowsAffected > 0 {
		s.invalidateMediaCache(ctx)
	}
	return res.RowsAffected, res.Error
}

func (s *ScannerService) pruneMissingMedia(ctx context.Context, libraryID string, seen map[string]struct{}) (int64, error) {
	// 只取 id/path，并把删除按批提交：此前整表载入完整 Media 结构体、
	// 每行一条 DELETE，大库 prune 既费内存又长期占用写锁。
	var rows []struct {
		ID   string
		Path string
	}
	if err := s.repo.DB.WithContext(ctx).
		Model(&model.Media{}).
		Select("id, path").
		Where("library_id = ?", libraryID).
		Find(&rows).Error; err != nil {
		return 0, err
	}
	stale := make([]string, 0)
	for _, row := range rows {
		if row.Path == "" {
			continue
		}
		if _, ok := seen[filepath.Clean(row.Path)]; ok {
			continue
		}
		if _, err := os.Stat(row.Path); err == nil {
			continue
		} else if !os.IsNotExist(err) {
			continue
		}
		stale = append(stale, row.ID)
	}
	return s.deleteMediaByIDs(ctx, stale, false)
}

func (s *ScannerService) pruneMissingMediaForRoot(ctx context.Context, libraryID, rootID, rootPath string, seen map[string]struct{}) (int64, error) {
	var rows []struct {
		ID            string
		Path          string
		LibraryRootID string
	}
	q := s.repo.DB.WithContext(ctx).
		Model(&model.Media{}).
		Select("id, path, library_root_id").
		Where("library_id = ? AND path NOT LIKE ?", libraryID, "cloud://%")
	if strings.TrimSpace(rootID) != "" {
		q = q.Where("library_root_id = ? OR library_root_id = '' OR library_root_id IS NULL", rootID)
	}
	if err := q.Find(&rows).Error; err != nil {
		return 0, err
	}
	stale := make([]string, 0)
	for _, row := range rows {
		if row.Path == "" {
			continue
		}
		if row.LibraryRootID == "" && !pathBelongsToRoot(row.Path, rootPath) {
			continue
		}
		if _, ok := seen[filepath.Clean(row.Path)]; ok {
			continue
		}
		if _, err := os.Stat(row.Path); err == nil {
			continue
		} else if !os.IsNotExist(err) {
			continue
		}
		stale = append(stale, row.ID)
	}
	return s.deleteMediaByIDs(ctx, stale, false)
}

func pathBelongsToRoot(pathValue, rootPath string) bool {
	pathValue = filepath.Clean(strings.TrimSpace(pathValue))
	rootPath = filepath.Clean(strings.TrimSpace(rootPath))
	if pathValue == "" || rootPath == "" || pathValue == "." || rootPath == "." {
		return false
	}
	return strings.EqualFold(pathValue, rootPath) || pathWithin(pathValue, rootPath)
}

// deleteMediaByIDs removes media rows in fixed-size batches so each write
// transaction stays short and the global write gate is released frequently.
func (s *ScannerService) deleteMediaByIDs(ctx context.Context, ids []string, hard bool) (int64, error) {
	const batch = 500
	var removed int64
	for i := 0; i < len(ids); i += batch {
		end := i + batch
		if end > len(ids) {
			end = len(ids)
		}
		q := s.repo.DB.WithContext(ctx)
		if hard {
			q = q.Unscoped()
		}
		res := q.Where("id IN ?", ids[i:end]).Delete(&model.Media{})
		if res.Error != nil {
			return removed, res.Error
		}
		removed += res.RowsAffected
	}
	return removed, nil
}

func (s *ScannerService) pruneMissingCloudMedia(ctx context.Context, libraryID string, seen map[string]struct{}) (int64, error) {
	return s.pruneMissingCloudMediaForLibraries(ctx, []string{libraryID}, seen)
}

func (s *ScannerService) pruneMissingCloudMediaForRoot(ctx context.Context, libraryID, rootID string, seen map[string]struct{}) (int64, error) {
	if strings.TrimSpace(libraryID) == "" || strings.TrimSpace(rootID) == "" {
		return 0, nil
	}
	var rows []struct {
		ID   string
		Path string
	}
	if err := s.repo.DB.WithContext(ctx).
		Model(&model.Media{}).
		Select("id, path").
		Where("library_id = ? AND library_root_id = ? AND path LIKE ?", libraryID, rootID, "cloud://%").
		Find(&rows).Error; err != nil {
		return 0, err
	}
	stale := make([]string, 0)
	for _, row := range rows {
		if _, ok := seen[row.Path]; ok {
			continue
		}
		stale = append(stale, row.ID)
	}
	return s.deleteMediaByIDs(ctx, stale, true)
}

func (s *ScannerService) pruneMissingCloudMediaForLibraries(ctx context.Context, libraryIDs []string, seen map[string]struct{}) (int64, error) {
	if len(libraryIDs) == 0 {
		return 0, nil
	}
	var rows []struct {
		ID   string
		Path string
	}
	if err := s.repo.DB.WithContext(ctx).
		Model(&model.Media{}).
		Select("id, path").
		Where("library_id IN ? AND path LIKE ?", libraryIDs, "cloud://%").
		Find(&rows).Error; err != nil {
		return 0, err
	}
	stale := make([]string, 0)
	for _, row := range rows {
		if _, ok := seen[row.Path]; ok {
			continue
		}
		stale = append(stale, row.ID)
	}
	return s.deleteMediaByIDs(ctx, stale, true)
}

func (s *ScannerService) autoScrapeEnabled(ctx context.Context) bool {
	if s.repo == nil || s.repo.Setting == nil {
		return false
	}
	value, err := s.repo.Setting.Get(ctx, "scrape.auto_on_scan")
	if err != nil {
		s.log.Warn("read scrape.auto_on_scan failed", zap.Error(err))
		return false
	}
	switch strings.ToLower(strings.TrimSpace(value)) {
	case "1", "true", "yes", "on", "enabled":
		return true
	default:
		return false
	}
}
