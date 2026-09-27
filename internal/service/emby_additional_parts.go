package service

import (
	"context"
	"strings"

	"github.com/ShukeBta/MediaStationGo/internal/model"
)

// AdditionalParts exposes Emby's multipart-video contract without treating
// sequential files as selectable media versions.
func (e *EmbyService) AdditionalParts(ctx context.Context, mediaID, userID string) (map[string]any, error) {
	anchor, err := e.repo.Media.FindByID(ctx, strings.TrimSpace(mediaID))
	if err != nil || anchor == nil {
		return nil, err
	}
	if !UserDefaultMediaVisibility(ctx, e.repo, userID).Allows(anchor) {
		return nil, nil
	}
	rows, err := e.mediaPartRows(ctx, anchor, userID)
	if err != nil {
		return nil, err
	}
	startIndex := 0
	for index := range rows {
		if rows[index].ID == anchor.ID {
			startIndex = index
			break
		}
	}
	if startIndex+1 >= len(rows) {
		return map[string]any{"Items": []map[string]any{}, "TotalRecordCount": 0, "StartIndex": 0}, nil
	}
	items := make([]map[string]any, 0, len(rows)-startIndex-1)
	for index := startIndex + 1; index < len(rows); index++ {
		row := rows[index]
		row.PartCount = len(rows)
		item := e.itemPayload(ctx, &row, false, 0)
		item["Name"] = row.Title
		delete(item, "PartCount")
		items = append(items, item)
	}
	return map[string]any{"Items": items, "TotalRecordCount": len(items), "StartIndex": 0}, nil
}

func (e *EmbyService) mediaPartCount(ctx context.Context, anchor *model.Media) int {
	if e == nil || e.repo == nil || e.repo.DB == nil || anchor == nil {
		return 0
	}
	key := strings.TrimSpace(anchor.PartGroupKey)
	if key == "" {
		return 0
	}
	var count int64
	if err := e.repo.DB.WithContext(ctx).Model(&model.Media{}).
		Where("library_id = ? AND part_group_key = ? AND deleted_at IS NULL", anchor.LibraryID, key).
		Count(&count).Error; err != nil {
		return 0
	}
	return int(count)
}

func (e *EmbyService) mediaPartRows(ctx context.Context, anchor *model.Media, userID string) ([]model.Media, error) {
	if anchor == nil || strings.TrimSpace(anchor.PartGroupKey) == "" {
		return nil, nil
	}
	q := e.repo.DB.WithContext(ctx).Model(&model.Media{}).
		Where("library_id = ? AND part_group_key = ?", anchor.LibraryID, anchor.PartGroupKey)
	q = e.applyUserMediaVisibility(ctx, q, userID)
	var rows []model.Media
	err := q.Order("CASE WHEN part_index > 0 THEN part_index ELSE 2147483647 END ASC").
		Order("media.created_at ASC").
		Order("media.id ASC").
		Find(&rows).Error
	return rows, err
}
