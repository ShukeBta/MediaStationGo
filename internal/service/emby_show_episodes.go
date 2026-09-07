package service

import (
	"context"
	"strings"

	"github.com/ShukeBta/MediaStationGo/internal/model"
	"gorm.io/gorm"
)

// Resolve the requested season using narrow identity columns. Full media rows
// are loaded only after the season restriction, not once per season for a show.
func (e *EmbyService) showEpisodeItems(ctx context.Context, p ItemsParams) (map[string]any, error) {
	scope := e.repo.DB.WithContext(ctx).Model(&model.Media{}).
		Where("season_num > 0 OR episode_num > 0 OR COALESCE(part_group_key,'')<>''")
	scope = e.applyUserMediaVisibility(ctx, scope, p.UserID)
	// Missing keys cannot be located by their future identity. Retain bounded
	// repair before filtering, including direct inserts and moves.
	if err := e.ensureEmbyKeys(ctx, scope); err != nil {
		return nil, err
	}
	scope = scope.Where("emby_series_key = ? OR emby_list_key = ?", p.ShowID, p.ShowID)
	if p.ParentID != p.ShowID {
		if !strings.HasPrefix(p.ParentID, embyVirtualSeasonPrefix) {
			return emptyItemsEnvelope(p.StartIndex), nil
		}
		// Multipart list identities expose season 1 regardless of physical
		// season metadata. Ordinary identities retain specials and negatives.
		const seasonExpr = "CASE WHEN COALESCE(part_group_key,'')<>'' AND emby_list_key = ? THEN 1 WHEN season_num < 0 THEN 1 ELSE COALESCE(season_num,0) END"
		var seasons []struct{ Number int }
		if err := scope.Session(&gorm.Session{}).Distinct().
			Select(seasonExpr+" AS number", p.ShowID).Find(&seasons).Error; err != nil {
			return nil, err
		}
		found := false
		for _, season := range seasons {
			number := season.Number
			if seasonID(p.ShowID, number) == p.ParentID {
				scope = scope.Where("("+seasonExpr+") = ?", p.ShowID, number)
				found = true
				break
			}
		}
		if !found {
			return emptyItemsEnvelope(p.StartIndex), nil
		}
	}
	var rows []model.Media
	if err := scope.Order("media.season_num asc, media.episode_num asc, media.created_at asc, media.id asc").Find(&rows).Error; err != nil {
		return nil, err
	}
	if len(rows) > 0 && strings.TrimSpace(rows[0].PartGroupKey) != "" &&
		multipartSeriesID(rows[0].LibraryID, rows[0].PartGroupKey) == p.ShowID {
		for _, group := range e.multipartSeriesGroupsFromMedia(rows) {
			if group.ID == p.ShowID {
				return e.episodeItems(ctx, group.Episodes, p)
			}
		}
		return emptyItemsEnvelope(p.StartIndex), nil
	}
	return e.episodeItems(ctx, rows, p)
}
