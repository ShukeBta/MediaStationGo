// Package service — duplicate grouping helpers.
package service

import (
	"context"
	"fmt"
	"sort"

	"go.uber.org/zap"

	"github.com/ShukeBta/MediaStationGo/internal/model"
)

func (d *DuplicateService) markDuplicateGroup(ctx context.Context, rep *Report, key string, group []model.Media, markedIDs map[string]struct{}) {
	primary := pickPrimary(group)
	dupes := make([]model.Media, 0, len(group)-1)
	for _, m := range group {
		if m.ID == primary.ID || m.DuplicateOf == primary.ID {
			continue
		}
		if _, ok := markedIDs[m.ID]; ok {
			continue
		}
		dupes = append(dupes, m)
		if err := d.repo.DB.WithContext(ctx).
			Model(&model.Media{}).
			Where("id = ?", m.ID).
			Updates(map[string]any{
				"is_duplicate": true,
				"duplicate_of": primary.ID,
			}).Error; err != nil {
			d.log.Warn("dup mark failed", zap.Error(err))
			continue
		}
		markedIDs[m.ID] = struct{}{}
		rep.ItemsMarked++
	}
	if len(dupes) == 0 {
		return
	}
	rep.Groups = append(rep.Groups, Group{
		Hash:       key,
		Primary:    primary,
		Duplicates: dupes,
	})
}

func groupByExternalIdentity(rows []model.Media) map[string][]model.Media {
	groups := map[string][]model.Media{}
	for _, row := range rows {
		key := mediaExternalIdentityKey(row)
		if key == "" {
			continue
		}
		groups[key] = append(groups[key], row)
	}
	return groups
}

func mediaExternalIdentityKey(row model.Media) string {
	var key string
	switch {
	case row.TMDbID > 0:
		key = fmt.Sprintf("tmdb:%d", row.TMDbID)
	case row.BangumiID > 0:
		key = fmt.Sprintf("bangumi:%d", row.BangumiID)
	case row.DoubanID != "":
		key = "douban:" + row.DoubanID
	case row.TheTVDBID != "":
		key = "thetvdb:" + row.TheTVDBID
	default:
		return ""
	}
	if row.SeasonNum > 0 || row.EpisodeNum > 0 {
		key += fmt.Sprintf(":s%d:e%d", row.SeasonNum, row.EpisodeNum)
	}
	return key
}

// pickPrimary picks the "best" media row to keep: prefer scraped > size > id.
func pickPrimary(group []model.Media) model.Media {
	sort.SliceStable(group, func(i, j int) bool {
		ai, aj := group[i].ScrapeStatus == "matched", group[j].ScrapeStatus == "matched"
		if ai != aj {
			return ai
		}
		if group[i].SizeBytes != group[j].SizeBytes {
			return group[i].SizeBytes > group[j].SizeBytes
		}
		return group[i].ID < group[j].ID
	})
	return group[0]
}
