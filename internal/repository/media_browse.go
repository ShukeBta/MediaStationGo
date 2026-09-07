package repository

import (
	"context"
	"github.com/ShukeBta/MediaStationGo/internal/model"
)

func (r *MediaRepository) LibraryHasEpisodes(ctx context.Context, libraryIDs []string, filter MediaQueryFilter) (bool, error) {
	var ids []string
	q := r.db.WithContext(ctx).Model(&model.Media{}).Where("library_id IN ? AND deleted_at IS NULL AND (season_num > 0 OR episode_num > 0)", libraryIDs)
	err := applyMediaQueryFilter(q, filter).Limit(1).Pluck("id", &ids).Error
	return len(ids) > 0, err
}

// ListMediaBrowseMetadata reads only representative rows, never every version
// or episode. Full payloads are hydrated separately after filtering/pagination.
func (r *MediaRepository) ListMediaBrowseMetadata(ctx context.Context, ids, libraryIDs []string, filter MediaQueryFilter) ([]model.Media, error) {
	rows := []model.Media{}
	for start := 0; start < len(ids); start += 400 {
		end := start + 400
		if end > len(ids) {
			end = len(ids)
		}
		var batch []model.Media
		q := r.db.WithContext(ctx).Model(&model.Media{}).Select("id", "created_at", "library_id", "library_root_id", "title", "original_name", "path", "relative_path", "part_group_key", "part_group_title", "season_num", "episode_num", "languages", "countries", "genres", "actors", "nsfw", "media_version_key").Where("id IN ? AND library_id IN ? AND deleted_at IS NULL", ids[start:end], libraryIDs)
		if err := applyMediaQueryFilter(q, filter).Find(&batch).Error; err != nil {
			return nil, err
		}
		rows = append(rows, batch...)
	}
	return rows, nil
}
