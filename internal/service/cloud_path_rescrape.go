package service

import (
	"context"
	"strings"

	"go.uber.org/zap"

	"github.com/ShukeBta/MediaStationGo/internal/model"
)

// RepairAndRescrapeResult 汇总一次「全库修复+重刮」的结果。
type RepairAndRescrapeResult struct {
	Repaired     int `json:"repaired"`     // 从路径占位符回填外部 ID 的媒体数
	Reclassified int `json:"reclassified"` // 按元数据纠偏到正确分类/媒体库的媒体数
	Libraries    int `json:"libraries"`    // 参与重刮的媒体库数
	Matched      int `json:"matched"`      // 重刮后成功匹配的媒体数
	Processed    int `json:"processed"`    // 实际完成刮削处理的媒体数
	Errors       int `json:"errors"`       // 单条媒体刮削失败数
	Reset        int `json:"reset"`        // 被重置为 pending 以便重刮的剧集行数
}

// resetEpisodicMatchedForRescrape 把剧集类(有季集号)且已 matched 的行重置为
// pending,使 EnrichLibrary(只处理 pending/no_match)能重新刮削它们。
//
// 背景: 历史版本(commit b44c7f8)曾把【单集 episode id】写进整剧 tm_db_id、把
// 单集名写进 original_name,污染了合集分组键 —— 同一部剧每集 id/原名各不相同,
// 被前端 / Emby 拆成 N 张单集卡。这些行 scrape_status 多为 matched,常规「修复+
// 重刮」会跳过,导致「无法修复」。源头已在 local_metadata.go 修正,这里把脏的
// matched 剧集行放回 pending,借重刮写回正确的整剧 ID / 原名。
//
// libraryIDs 为空时处理全库;非空时仅这些库。返回被重置的行数。
func (c *Container) resetEpisodicMatchedForRescrape(ctx context.Context, libraryIDs ...string) (int, error) {
	if c == nil || c.Repo == nil || c.Repo.DB == nil {
		return 0, nil
	}
	ids := compactLibraryIDs(libraryIDs...)
	q := c.Repo.DB.WithContext(ctx).Model(&model.Media{}).
		Where("(season_num > 0 OR episode_num > 0)").
		Where("LOWER(scrape_status) = ?", "matched")
	if len(ids) > 0 {
		q = q.Where("library_id IN ?", ids)
	}
	res := q.Update("scrape_status", "pending")
	if res.Error != nil {
		return 0, res.Error
	}
	reset := int(res.RowsAffected)
	if reset > 0 && c.Log != nil {
		c.Log.Info("episodic matched rows reset to pending for rescrape",
			zap.String("libraries", strings.Join(ids, ",")),
			zap.Int("reset", reset))
	}
	return reset, nil
}

// RepairAndRescrapeAllLibraries 修复并重刮所有媒体库:先从媒体路径中的
// {tmdb-123}/{bangumi-456} 等占位符回填缺失或错误的外部 ID(回填后会把相关
// 行的 scrape_status 重置为 pending),随后逐个媒体库重刮(含 no_match 重试),
// 让此前因空 ID / 脏 ID 无法刮削的媒体重新匹配到正确数据。
func repairRescrapeOptions(values ...ScrapeOptions) ScrapeOptions {
	options := ScrapeOptions{RetryNoMatch: true, IncludeMatched: true}
	if len(values) > 0 {
		options = values[0]
		options.RetryNoMatch = true
		options.IncludeMatched = true
	}
	if options.EpisodeArtwork == nil {
		episodeArtwork := false
		options.EpisodeArtwork = &episodeArtwork
	}
	return options
}

func (c *Container) RepairAndRescrapeAllLibraries(ctx context.Context, options ...ScrapeOptions) (RepairAndRescrapeResult, error) {
	var result RepairAndRescrapeResult
	if c == nil || c.Repo == nil || c.Repo.DB == nil {
		return result, nil
	}
	scrapeOptions := repairRescrapeOptions(options...)
	repaired, err := c.RepairCloudPathMetadata(ctx)
	if err != nil {
		return result, err
	}
	result.Repaired = repaired

	// 重置全库脏的 matched 剧集行(单集 id 污染整剧字段),让其下方重刮一并修正。
	if reset, err := c.resetEpisodicMatchedForRescrape(ctx); err != nil {
		return result, err
	} else {
		result.Reset = reset
	}

	if c.Scraper == nil || c.Repo.Library == nil {
		return result, nil
	}
	libraries, err := c.Repo.Library.List(ctx)
	if err != nil {
		return result, err
	}
	for i := range libraries {
		select {
		case <-ctx.Done():
			return result, ctx.Err()
		default:
		}
		lib := libraries[i]
		if !lib.Enabled {
			continue
		}
		result.Libraries++
		// retryNoMatch=true:连之前匹配失败的也再试一次,因为这次可能已回填到正确 ID。
		scrapeResult, err := c.Scraper.EnrichLibraryDetailedWithOptions(ctx, lib.ID, scrapeOptions)
		if err != nil {
			if c.Log != nil {
				c.Log.Warn("repair rescrape library failed", zap.String("library", lib.ID), zap.Error(err))
			}
			result.Errors++
			continue
		}
		result.Matched += scrapeResult.Matched
		result.Processed += scrapeResult.Processed
		result.Errors += scrapeResult.Failed
	}
	if c.Organizer != nil {
		reclassifyResult, err := c.Organizer.ReclassifyMisclassifiedMedia(ctx, MediaCategoryReclassifyOptions{})
		if err != nil {
			return result, err
		}
		if reclassifyResult != nil {
			result.Reclassified = reclassifyResult.Reclassified
			result.Errors += len(reclassifyResult.Errors)
		}
	}
	if c.Log != nil {
		c.Log.Info("repair and rescrape all libraries done",
			zap.Int("repaired", result.Repaired),
			zap.Int("reclassified", result.Reclassified),
			zap.Int("libraries", result.Libraries),
			zap.Int("matched", result.Matched),
			zap.Int("processed", result.Processed),
			zap.Int("errors", result.Errors))
	}
	return result, nil
}

// RepairAndRescrapeLibrary 修复并重刮单个媒体库:先从该库媒体路径中的占位符
// 回填缺失/错误的外部 ID(重置相关行 scrape_status=pending),再对该库重刮
// (含 no_match 重试)。用于「按媒体库」单独触发修复,不影响其它库。
func (c *Container) RepairAndRescrapeLibrary(ctx context.Context, libraryID string, options ...ScrapeOptions) (RepairAndRescrapeResult, error) {
	var result RepairAndRescrapeResult
	libraryID = strings.TrimSpace(libraryID)
	if c == nil || c.Repo == nil || c.Repo.DB == nil || libraryID == "" {
		return result, nil
	}
	scrapeOptions := repairRescrapeOptions(options...)
	libraryIDs, err := MergedLibraryIDsForLibrary(ctx, c.Repo, libraryID)
	if err != nil {
		return result, err
	}
	repaired, err := c.RepairCloudPathMetadata(ctx, libraryIDs...)
	if err != nil {
		return result, err
	}
	result.Repaired = repaired

	// 重置该库脏的 matched 剧集行,让下方重刮修正被单集 id 污染的整剧字段。
	if reset, err := c.resetEpisodicMatchedForRescrape(ctx, libraryIDs...); err != nil {
		return result, err
	} else {
		result.Reset = reset
	}

	if c.Scraper == nil {
		return result, nil
	}
	result.Libraries = 1
	// retryNoMatch=true:连之前匹配失败的也再试一次,因为这次可能已回填到正确 ID。
	scrapeResult, err := c.Scraper.EnrichLibraryDetailedWithOptions(ctx, libraryID, scrapeOptions)
	if err != nil {
		return result, err
	}
	result.Matched = scrapeResult.Matched
	result.Processed = scrapeResult.Processed
	result.Errors = scrapeResult.Failed
	if c.Organizer != nil {
		reclassifyResult, err := c.Organizer.ReclassifyMisclassifiedMedia(ctx, MediaCategoryReclassifyOptions{LibraryIDs: libraryIDs})
		if err != nil {
			return result, err
		}
		if reclassifyResult != nil {
			result.Reclassified = reclassifyResult.Reclassified
			result.Errors += len(reclassifyResult.Errors)
		}
	}
	if c.Log != nil {
		c.Log.Info("repair and rescrape library done",
			zap.String("library", libraryID),
			zap.Int("repaired", result.Repaired),
			zap.Int("reclassified", result.Reclassified),
			zap.Int("matched", result.Matched),
			zap.Int("processed", result.Processed),
			zap.Int("errors", result.Errors))
	}
	return result, nil
}
