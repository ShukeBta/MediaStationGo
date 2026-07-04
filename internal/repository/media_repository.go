package repository

import (
	"context"
	"errors"
	"sync"

	"gorm.io/gorm"

	"github.com/ShukeBta/MediaStationGo/internal/model"
)

// MediaRepository persists model.Media records.
type MediaRepository struct {
	db *gorm.DB

	searchIndexOnce      sync.Once
	searchIndexAvailable bool
	searchBackend        MediaSearchBackend
}

type MediaSearchBackend interface {
	SearchMediaIDs(ctx context.Context, query string, offset, limit int, filter MediaQueryFilter) ([]string, int64, error)
}

type MediaSearchSyncBackend interface {
	MediaSearchBackend
	EnsureIndex(ctx context.Context) error
	IndexMedia(ctx context.Context, rows []model.Media) error
}

func (r *MediaRepository) SetSearchBackend(backend MediaSearchBackend) {
	if r != nil {
		r.searchBackend = backend
	}
}

// MediaQueryFilter is applied to user-facing media queries so NSFW items and
// profile-restricted libraries are filtered in SQL instead of only in React.
type MediaQueryFilter struct {
	IncludeNSFW       bool
	AllowedLibraryIDs []string
	HiddenLibraryIDs  []string
}

func applyMediaQueryFilter(q *gorm.DB, filter MediaQueryFilter) *gorm.DB {
	if !filter.IncludeNSFW {
		q = q.Where("nsfw = ?", false)
	}
	if len(filter.HiddenLibraryIDs) > 0 {
		q = q.Where("library_id NOT IN ?", filter.HiddenLibraryIDs)
	}
	if len(filter.AllowedLibraryIDs) > 0 {
		q = q.Where("library_id IN ?", filter.AllowedLibraryIDs)
	}
	return q
}

func (r *MediaRepository) indexMediaBestEffort(ctx context.Context, media model.Media) {
	backend, ok := r.searchBackend.(MediaSearchSyncBackend)
	if !ok {
		return
	}
	_ = backend.IndexMedia(ctx, []model.Media{media})
}

// FindByID returns the media row or (nil, nil).
func (r *MediaRepository) FindByID(ctx context.Context, id string) (*model.Media, error) {
	var m model.Media
	err := r.db.WithContext(ctx).Where("id = ?", id).First(&m).Error
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	return &m, nil
}

// ListByLibrary returns paginated media items for a library.
func (r *MediaRepository) ListByLibrary(ctx context.Context, libraryID string, offset, limit int) ([]model.Media, int64, error) {
	return r.ListByLibraryFiltered(ctx, libraryID, offset, limit, MediaQueryFilter{IncludeNSFW: true})
}

func (r *MediaRepository) ListByLibraryFiltered(ctx context.Context, libraryID string, offset, limit int, filter MediaQueryFilter) ([]model.Media, int64, error) {
	return r.ListByLibrariesFiltered(ctx, []string{libraryID}, offset, limit, filter)
}

func (r *MediaRepository) ListByLibrariesFiltered(ctx context.Context, libraryIDs []string, offset, limit int, filter MediaQueryFilter) ([]model.Media, int64, error) {
	var items []model.Media
	var total int64
	if len(libraryIDs) == 0 {
		return items, 0, nil
	}
	q := r.db.WithContext(ctx).Model(&model.Media{})
	if len(libraryIDs) == 1 {
		q = q.Where("library_id = ?", libraryIDs[0])
	} else {
		q = q.Where("library_id IN ?", libraryIDs)
	}
	q = applyMediaQueryFilter(q, filter)
	if err := q.Count(&total).Error; err != nil {
		return nil, 0, err
	}
	// 多级排序消除"随机"观感:
	//  1. release_date desc — 精确上映/首播日期新→旧
	//  2. year desc         — 老数据没有完整日期时仍按年份新→旧
	//  3. updated_at desc   — 同日期/同年按最近更新兜底
	//  4. created_at desc   — 再按入库时间
	//  5. id desc           — 稳定 tie-breaker:云盘批量扫描同批 created_at 相同时,
	//                        没有它 DB 返回顺序不确定,正是"随机排序"的根因。
	err := q.Order("release_date DESC, year DESC, updated_at DESC, created_at DESC, id DESC").
		Offset(offset).Limit(limit).Find(&items).Error
	return items, total, err
}

// DeleteByLibrary purges all media tied to a library.
func (r *MediaRepository) DeleteByLibrary(ctx context.Context, libraryID string) error {
	// FTS 行由 media 表上的触发器同步清理（软删/硬删都覆盖）。
	return r.db.WithContext(ctx).Where("library_id = ?", libraryID).Delete(&model.Media{}).Error
}

func (r *MediaRepository) DeleteByLibraryRoot(ctx context.Context, libraryID, rootID string) error {
	return r.db.WithContext(ctx).
		Where("library_id = ? AND library_root_id = ?", libraryID, rootID).
		Delete(&model.Media{}).Error
}

// PurgeByLibrary permanently removes media tied to a library. Used for virtual
// cloud mounts where "remove mount" must not populate the recycle bin.
func (r *MediaRepository) PurgeByLibrary(ctx context.Context, libraryID string) error {
	return r.db.WithContext(ctx).Unscoped().Where("library_id = ?", libraryID).Delete(&model.Media{}).Error
}
