package service

import (
	"context"
	"errors"
	"fmt"
	"net/url"
	pathpkg "path"
	"strings"
	"time"

	"go.uber.org/zap"
	"gorm.io/gorm"

	"github.com/ShukeBta/MediaStationGo/internal/model"
	"github.com/ShukeBta/MediaStationGo/internal/service/cloud"
)

const maxRecycleBinRecords = 200

type RecycleBinItem struct {
	model.Media
	DeletedAt    time.Time `json:"deleted_at"`
	DeletionKind string    `json:"deletion_kind"`
	CanManage    bool      `json:"can_manage"`
}

// SoftDelete moves a media row to the recycle bin (gorm soft delete).
// The source file is kept until an authorized user purges the record.
func (s *MediaService) SoftDelete(ctx context.Context, id string) error {
	return s.SoftDeleteBy(ctx, id, "", "media")
}

func (s *MediaService) SoftDeleteBy(ctx context.Context, id, userID, deletionKind string) error {
	deletionKind = strings.ToLower(strings.TrimSpace(deletionKind))
	if deletionKind != "media" && deletionKind != "version" {
		return fmt.Errorf("unsupported deletion kind %q", deletionKind)
	}
	err := s.repo.DB.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		updates := map[string]any{
			"deletion_kind":      deletionKind,
			"deleted_by_user_id": strings.TrimSpace(userID),
		}
		result := tx.Model(&model.Media{}).Where("id = ?", strings.TrimSpace(id)).Updates(updates)
		if result.Error != nil {
			return result.Error
		}
		if result.RowsAffected == 0 {
			return ErrMediaVersionNotFound
		}
		return tx.Where("id = ?", strings.TrimSpace(id)).Delete(&model.Media{}).Error
	})
	if err == nil {
		if pruneErr := pruneRecycleBinRows(ctx, s.repo.DB, maxRecycleBinRecords); pruneErr != nil {
			return pruneErr
		}
		s.invalidateMediaCache(ctx)
	}
	return err
}

// SoftDeleteManyBy moves several media rows to the recycle bin in one
// transaction. The caller is expected to have already authorized the batch.
func (s *MediaService) SoftDeleteManyBy(ctx context.Context, ids []string, userID, deletionKind string) (int, error) {
	deletionKind = strings.ToLower(strings.TrimSpace(deletionKind))
	if deletionKind != "media" && deletionKind != "version" {
		return 0, fmt.Errorf("unsupported deletion kind %q", deletionKind)
	}
	normalized := compactMediaIDs(ids)
	if len(normalized) == 0 {
		return 0, ErrMediaVersionNotFound
	}
	applied := 0
	err := s.repo.DB.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		result := tx.Model(&model.Media{}).
			Where("id IN ?", normalized).
			Updates(map[string]any{
				"deletion_kind":      deletionKind,
				"deleted_by_user_id": strings.TrimSpace(userID),
			})
		if result.Error != nil {
			return result.Error
		}
		if result.RowsAffected != int64(len(normalized)) {
			return ErrMediaVersionNotFound
		}
		result = tx.Where("id IN ?", normalized).Delete(&model.Media{})
		if result.Error != nil {
			return result.Error
		}
		if result.RowsAffected != int64(len(normalized)) {
			return ErrMediaVersionNotFound
		}
		applied = int(result.RowsAffected)
		return nil
	})
	if err != nil {
		return 0, err
	}
	if pruneErr := pruneRecycleBinRows(ctx, s.repo.DB, maxRecycleBinRecords); pruneErr != nil {
		return 0, pruneErr
	}
	s.invalidateMediaCache(ctx)
	return applied, nil
}

func compactMediaIDs(ids []string) []string {
	seen := make(map[string]struct{}, len(ids))
	result := make([]string, 0, len(ids))
	for _, id := range ids {
		id = strings.TrimSpace(id)
		if id == "" {
			continue
		}
		if _, ok := seen[id]; ok {
			continue
		}
		seen[id] = struct{}{}
		result = append(result, id)
	}
	return result
}

// RestoreDeleted unsets DeletedAt for a single media row.
func (s *MediaService) RestoreDeleted(ctx context.Context, id string) error {
	err := s.repo.DB.WithContext(ctx).Unscoped().Model(&model.Media{}).
		Where("id = ? AND deleted_at IS NOT NULL", strings.TrimSpace(id)).
		Updates(map[string]any{"deleted_at": nil, "deletion_kind": "", "deleted_by_user_id": ""}).Error
	if err == nil {
		s.invalidateMediaCache(ctx)
	}
	return err
}

// ListRecycleBin returns every soft-deleted row, newest first.
func (s *MediaService) ListRecycleBin(ctx context.Context, limit int) ([]model.Media, error) {
	if err := pruneRecycleBinRows(ctx, s.repo.DB, maxRecycleBinRecords); err != nil {
		return nil, err
	}
	if limit <= 0 || limit > maxRecycleBinRecords {
		limit = maxRecycleBinRecords
	}
	var rows []model.Media
	err := s.repo.DB.Unscoped().
		Where("deleted_at IS NOT NULL").
		Order("deleted_at desc").
		Limit(limit).
		Find(&rows).Error
	return rows, err
}

func (s *MediaService) ListRecycleBinForUser(ctx context.Context, userID string, isAdmin bool, limit int) ([]RecycleBinItem, error) {
	if err := pruneRecycleBinRows(ctx, s.repo.DB, maxRecycleBinRecords); err != nil {
		return nil, err
	}
	if limit <= 0 || limit > maxRecycleBinRecords {
		limit = maxRecycleBinRecords
	}
	query := s.repo.DB.WithContext(ctx).Unscoped().
		Where("deleted_at IS NOT NULL")
	if !isAdmin {
		subquery := s.repo.DB.WithContext(ctx).Model(&model.ResourceImportJob{}).
			Select("DISTINCT media_id").
			Where("user_id = ? AND status IN ?", strings.TrimSpace(userID), successfulResourceImportStatuses)
		query = query.Where("id IN (?)", subquery)
	}
	var rows []model.Media
	if err := query.Order("deleted_at desc").Limit(limit).Find(&rows).Error; err != nil {
		return nil, err
	}
	items := make([]RecycleBinItem, 0, len(rows))
	for _, row := range rows {
		items = append(items, RecycleBinItem{
			Media: row, DeletedAt: row.DeletedAt.Time, DeletionKind: firstNonEmpty(row.DeletionKind, "media"), CanManage: true,
		})
	}
	return items, nil
}

func pruneRecycleBinRows(ctx context.Context, db *gorm.DB, keep int) error {
	if db == nil {
		return nil
	}
	if keep <= 0 {
		keep = maxRecycleBinRecords
	}
	var rows []struct {
		ID string
	}
	if err := db.WithContext(ctx).Unscoped().
		Model(&model.Media{}).
		Select("id").
		Where("deleted_at IS NOT NULL").
		Where("path NOT LIKE ?", "cloud://%").
		Order("deleted_at desc").
		Limit(100000).
		Offset(keep).
		Find(&rows).Error; err != nil {
		return err
	}
	if len(rows) == 0 {
		return nil
	}
	ids := make([]string, 0, len(rows))
	for _, row := range rows {
		if row.ID != "" {
			ids = append(ids, row.ID)
		}
	}
	if len(ids) == 0 {
		return nil
	}
	return db.WithContext(ctx).Unscoped().Where("id IN ?", ids).Delete(&model.Media{}).Error
}

func (s *MediaService) RestoreDeletedForUser(ctx context.Context, id, userID string, isAdmin bool) error {
	allowed, err := s.canManageDeletedMedia(ctx, id, userID, isAdmin)
	if err != nil {
		return err
	}
	if !allowed {
		return ErrMediaVersionForbidden
	}
	return s.RestoreDeleted(ctx, id)
}

// PurgeDeleted permanently removes a soft-deleted row. Cloud-backed media is
// deleted from the provider first; the tombstone is retained when that fails.
func (s *MediaService) PurgeDeleted(ctx context.Context, id string) error {
	if s == nil || s.repo == nil || s.repo.DB == nil {
		return errors.New("media service unavailable")
	}
	s.purgeMu.Lock()
	defer s.purgeMu.Unlock()

	var row model.Media
	err := s.repo.DB.WithContext(ctx).Unscoped().
		Where("id = ? AND deleted_at IS NOT NULL", strings.TrimSpace(id)).First(&row).Error
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return ErrMediaVersionNotFound
	}
	if err != nil {
		return err
	}
	provider, ref, isCloud, parseErr := cloudMediaFileTarget(row.Path)
	if parseErr != nil {
		return parseErr
	}
	if isCloud {
		if s.cloudDeleter == nil {
			return errors.New("cloud file deletion is unavailable")
		}
		rootRef := ""
		if provider == cloud.TypeOpenList {
			rootRef, err = s.cloudLibraryRootRef(ctx, row, provider)
			if err != nil {
				return err
			}
		}
		if err := s.cloudDeleter.DeleteCloudFile(ctx, provider, ref); err != nil {
			return fmt.Errorf("delete cloud file %s: %w", row.Path, err)
		}
		// OpenList paths have a stable directory namespace, so only this
		// provider gets empty-parent cleanup. Other cloud providers retain the
		// existing exact-file purge behavior.
		if provider == cloud.TypeOpenList && rootRef != "" {
			if err := s.cloudDeleter.PruneEmptyCloudParents(ctx, provider, ref, rootRef); err != nil {
				if s.log != nil {
					s.log.Warn("purge deleted media: prune empty cloud parents failed; continuing",
						zap.String("media_id", row.ID),
						zap.String("path", row.Path),
						zap.Error(err))
				}
			}
		}
	}
	result := s.repo.DB.WithContext(ctx).Unscoped().
		Where("id = ? AND deleted_at IS NOT NULL", row.ID).Delete(&model.Media{})
	err = result.Error
	if err == nil && result.RowsAffected == 0 {
		err = ErrMediaVersionNotFound
	}
	if err == nil {
		s.invalidateMediaCache(ctx)
	}
	return err
}

func (s *MediaService) cloudLibraryRootRef(ctx context.Context, row model.Media, provider string) (string, error) {
	if strings.TrimSpace(row.LibraryRootID) == "" {
		return "", nil
	}
	var root model.LibraryRoot
	if err := s.repo.DB.WithContext(ctx).
		Where("id = ? AND library_id = ?", row.LibraryRootID, row.LibraryID).
		First(&root).Error; err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return "", fmt.Errorf("cloud media library root not found: %s", row.LibraryRootID)
		}
		return "", err
	}
	rootProvider, rootRef, isCloud, err := cloudMediaDirectoryTarget(root.Path)
	if err != nil {
		return "", err
	}
	if !isCloud || rootProvider != provider {
		return "", fmt.Errorf("cloud media library root provider mismatch: %s", row.LibraryRootID)
	}
	return rootRef, nil
}

func (s *MediaService) PurgeDeletedForUser(ctx context.Context, id, userID string, isAdmin bool) error {
	allowed, err := s.canManageDeletedMedia(ctx, id, userID, isAdmin)
	if err != nil {
		return err
	}
	if !allowed {
		return ErrMediaVersionForbidden
	}
	return s.PurgeDeleted(ctx, id)
}

func (s *MediaService) canManageDeletedMedia(ctx context.Context, id, userID string, isAdmin bool) (bool, error) {
	var count int64
	if err := s.repo.DB.WithContext(ctx).Unscoped().Model(&model.Media{}).
		Where("id = ? AND deleted_at IS NOT NULL", strings.TrimSpace(id)).Count(&count).Error; err != nil {
		return false, err
	}
	if count == 0 {
		return false, ErrMediaVersionNotFound
	}
	return userCanManageMediaVersion(ctx, s.repo, userID, isAdmin, id)
}

func cloudMediaFileTarget(raw string) (provider, ref string, isCloud bool, err error) {
	provider, ref, isCloud, err = cloudMediaDirectoryTarget(raw)
	if err != nil || !isCloud {
		return provider, ref, isCloud, err
	}
	ext := strings.ToLower(pathpkg.Ext(ref))
	if _, ok := videoExtensions[ext]; !ok {
		return "", "", true, fmt.Errorf("refusing to delete non-video cloud path %q", ref)
	}
	return provider, ref, true, nil
}

func cloudMediaDirectoryTarget(raw string) (provider, ref string, isCloud bool, err error) {
	raw = strings.TrimSpace(raw)
	if !strings.HasPrefix(strings.ToLower(raw), "cloud://") {
		return "", "", false, nil
	}
	isCloud = true
	rest := raw[len("cloud://"):]
	parts := strings.SplitN(rest, "/", 2)
	if len(parts) != 2 || strings.TrimSpace(parts[0]) == "" {
		return "", "", true, errors.New("invalid cloud media path")
	}
	provider = strings.TrimSpace(parts[0])
	ref = strings.Trim(strings.TrimSpace(parts[1]), "/")
	if decoded, decodeErr := url.PathUnescape(ref); decodeErr == nil {
		ref = decoded
	}
	if ref == "" || ref == "." || ref == ".." {
		return "", "", true, errors.New("refusing to delete cloud provider root")
	}
	return provider, ref, true, nil
}
