// Package service — one-time cleanup for episode metadata polluted by an older
// scraper bug that wrote per-episode TMDB ids / episode names into the
// series-level fields (tm_db_id / original_name).
package service

import (
	"context"
	"regexp"
	"strings"

	"go.uber.org/zap"
	"gorm.io/gorm"

	"github.com/ShukeBta/MediaStationGo/internal/model"
)

// pollutedEpisodeCleanupSettingKey marks that the one-time normalization has run.
const pollutedEpisodeCleanupSettingKey = "media.polluted_episode_cleanup_v2_done"

// seasonFolderTailRE 去掉路径末尾的「季文件夹 + 文件名」,得到整剧目录(show_dir)。
// 例: /tv/国漫/遮天 (2023)/Season 01/遮天 - S01E01.mkv → /tv/国漫/遮天 (2023)
var seasonFolderTailRE = regexp.MustCompile(`(?i)[\\/](?:season[\s._-]*\d+|s\d{1,2}|specials?|sp|ova|oad|extra|extras|第\s*[0-9一二三四五六七八九十百零两]+\s*季|特别篇|特別篇|番外|特典)[\\/][^\\/]*$`)

// showDirFromEpisodePath 从单集路径推出整剧目录, 作为「同一部剧」的聚合键。
// 若没有季文件夹, 则退而去掉文件名取其父目录。
func showDirFromEpisodePath(path string) string {
	path = strings.TrimSpace(path)
	if path == "" {
		return ""
	}
	if loc := seasonFolderTailRE.FindStringIndex(path); loc != nil {
		return path[:loc[0]]
	}
	// 无季文件夹: 取父目录(去掉最后一段文件名)。
	sep := strings.LastIndexAny(path, `/\`)
	if sep <= 0 {
		return ""
	}
	return path[:sep]
}

// NormalizePollutedEpisodeMetadata 一次性清洗历史脏数据:老版本刮削曾把
// 【单集 episode id】写进整剧 tm_db_id、把单集名写进 original_name, 导致同一部剧
// 每集 id/原名各不相同, 被前端 / Emby 拆成 N 张单集卡(实测「遮天」90 集 = 89 个
// 不同 tm_db_id)。
//
// 处理方式: 按「library_id + 整剧目录(show_dir)」聚合有季集号的行;当一组内出现
// 「多个不同的非零 tm_db_id」或「多个不同的 original_name」时, 判定该组被单集级
// 数据污染, 将该组所有行的 tm_db_id 清零、original_name 清空, 并把 scrape_status
// 重置为 pending —— 让其借「修复+重刮」写回正确的整剧 id/原名。即便在重刮前,
// 前端 / Emby 也已改为「路径剧名优先」分组, 清空脏字段后即可正确合并成一张卡。
//
// 幂等: 完成后写入 setting 标记;已标记则直接跳过。仅处理剧集类(有季集号)行,
// 不触碰电影。
func (c *Container) NormalizePollutedEpisodeMetadata(ctx context.Context) (int, error) {
	if c == nil || c.Repo == nil || c.Repo.DB == nil {
		return 0, nil
	}
	if c.Repo.Setting != nil {
		if v, err := c.Repo.Setting.Get(ctx, pollutedEpisodeCleanupSettingKey); err == nil && strings.TrimSpace(v) == "true" {
			return 0, nil
		}
	}

	db := c.Repo.DB.WithContext(ctx)

	// 聚合状态: 每个 show_dir 收集 行 id、非零 tmdb 集合、original_name 集合。
	type agg struct {
		ids     []string
		tmdbSet map[int]struct{}
		origSet map[string]struct{}
	}
	groups := map[string]*agg{}

	var rows []model.Media
	err := db.Model(&model.Media{}).
		Select("id, library_id, original_name, season_num, episode_num, tm_db_id, path").
		Where("season_num > 0 OR episode_num > 0").
		FindInBatches(&rows, 1000, func(_ *gorm.DB, _ int) error {
			for i := range rows {
				row := rows[i]
				showDir := showDirFromEpisodePath(row.Path)
				if showDir == "" {
					continue
				}
				key := strings.ToLower(strings.TrimSpace(row.LibraryID)) + "|" + strings.ToLower(showDir)
				g := groups[key]
				if g == nil {
					g = &agg{tmdbSet: map[int]struct{}{}, origSet: map[string]struct{}{}}
					groups[key] = g
				}
				g.ids = append(g.ids, row.ID)
				if row.TMDbID > 0 {
					g.tmdbSet[row.TMDbID] = struct{}{}
				}
				if name := strings.TrimSpace(row.OriginalName); name != "" {
					g.origSet[name] = struct{}{}
				}
			}
			return nil
		}).Error
	if err != nil {
		return 0, err
	}

	cleaned := 0
	for _, g := range groups {
		// 被单集数据污染的判据: 同一部剧出现多个不同的非零 tmdb id,
		// 或多个不同的 original_name(整剧原名本应全组一致)。
		polluted := len(g.tmdbSet) > 1 || len(g.origSet) > 1
		if !polluted || len(g.ids) == 0 {
			continue
		}
		// 分批更新(避免 IN 列表过长)。
		const chunk = 400
		for start := 0; start < len(g.ids); start += chunk {
			end := start + chunk
			if end > len(g.ids) {
				end = len(g.ids)
			}
			res := db.Model(&model.Media{}).
				Where("id IN ?", g.ids[start:end]).
				Updates(map[string]any{
					"tm_db_id":      0,
					"original_name": "",
					"scrape_status": "pending",
				})
			if res.Error != nil {
				return cleaned, res.Error
			}
			cleaned += int(res.RowsAffected)
		}
	}

	if c.Repo.Setting != nil {
		if err := c.Repo.Setting.Set(ctx, pollutedEpisodeCleanupSettingKey, "true"); err != nil && c.Log != nil {
			c.Log.Warn("mark polluted episode cleanup done failed", zap.Error(err))
		}
	}
	if cleaned > 0 && c.Log != nil {
		c.Log.Info("polluted episode metadata normalized",
			zap.Int("rows", cleaned),
			zap.Int("groups", len(groups)))
	}
	return cleaned, nil
}
