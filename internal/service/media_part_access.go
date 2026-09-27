package service

import (
	"context"
	"errors"
	"sort"
	"strings"

	"github.com/ShukeBta/MediaStationGo/internal/model"
)

type MediaPartItem struct {
	model.Media
	IsCurrent bool `json:"is_current"`
}

type MediaPartList struct {
	Items []MediaPartItem `json:"items"`
}

func (s *MediaService) ListMediaParts(ctx context.Context, mediaID string) (MediaPartList, error) {
	if s == nil || s.repo == nil || s.repo.Media == nil || s.repo.DB == nil {
		return MediaPartList{}, errors.New("media service unavailable")
	}
	anchor, err := s.repo.Media.FindByID(ctx, strings.TrimSpace(mediaID))
	if err != nil {
		return MediaPartList{}, err
	}
	if anchor == nil {
		return MediaPartList{}, ErrMediaVersionNotFound
	}

	rows := []model.Media{*anchor}
	if key := strings.TrimSpace(anchor.PartGroupKey); key != "" {
		if err := s.repo.DB.WithContext(ctx).
			Where("library_id = ? AND part_group_key = ?", anchor.LibraryID, key).
			Find(&rows).Error; err != nil {
			return MediaPartList{}, err
		}
	}
	s.attachLibraryMetadata(ctx, rows)
	sort.SliceStable(rows, func(i, j int) bool { return betterMediaPart(rows[i], rows[j]) })
	items := make([]MediaPartItem, 0, len(rows))
	for _, row := range rows {
		items = append(items, MediaPartItem{Media: row, IsCurrent: row.ID == anchor.ID})
	}
	return MediaPartList{Items: items}, nil
}
