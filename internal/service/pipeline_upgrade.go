package service

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/ShukeBta/MediaStationGo/internal/model"
	"gorm.io/gorm"
)

type PipelineWorkSourceCleanupResult struct {
	Status            string   `json:"status"`
	Reason            string   `json:"reason,omitempty"`
	WorkIdentity      string   `json:"work_identity"`
	Removed           int      `json:"removed"`
	Preserved         int      `json:"preserved"`
	RemovedMediaIDs   []string `json:"removed_media_ids"`
	PreservedMediaIDs []string `json:"preserved_media_ids"`
	NewOpenListPaths  []string `json:"new_openlist_paths"`
}

// ReplaceWorkSource moves active episodes covered by the newly scanned source
// into the recycle bin while preserving uncovered episodes and the new source.
// A stable external work identity and a distinct new source path are required;
// title-only matching is deliberately rejected because it is unsafe for bulk
// deletion.
func (s *PipelineMaintenanceService) ReplaceWorkSource(
	ctx context.Context,
	oldMediaID string,
	newMediaID string,
	target PipelineMaintenanceTarget,
	newOpenListPaths []string,
) (PipelineWorkSourceCleanupResult, error) {
	resolved, err := s.resolveTarget(ctx, target)
	if err != nil {
		return PipelineWorkSourceCleanupResult{}, err
	}
	if resolved.Category != "tv" && resolved.Category != "anime" {
		return PipelineWorkSourceCleanupResult{}, errors.New("整剧片源替换仅支持剧集和动漫媒体库")
	}
	newPaths, newCloudPaths, err := pipelineUpgradeNewSourcePaths(resolved, newOpenListPaths)
	if err != nil {
		return PipelineWorkSourceCleanupResult{}, err
	}

	result := PipelineWorkSourceCleanupResult{
		Status: "success", RemovedMediaIDs: []string{}, PreservedMediaIDs: []string{}, NewOpenListPaths: newPaths,
	}
	err = s.repos.DB.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		var oldMedia model.Media
		if err := tx.Unscoped().Where(
			"id = ? AND library_id = ? AND library_root_id = ?",
			strings.TrimSpace(oldMediaID), resolved.LibraryID, resolved.RootID,
		).First(&oldMedia).Error; err != nil {
			if errors.Is(err, gorm.ErrRecordNotFound) {
				return errors.New("整剧升级的旧片源锚点不存在")
			}
			return err
		}
		var newMedia model.Media
		if err := tx.Where(
			"id = ? AND library_id = ? AND library_root_id = ?",
			strings.TrimSpace(newMediaID), resolved.LibraryID, resolved.RootID,
		).First(&newMedia).Error; err != nil {
			if errors.Is(err, gorm.ErrRecordNotFound) {
				return errors.New("整剧升级的新片源锚点不存在")
			}
			return err
		}
		if !pipelineUpgradePathWithinAny(newMedia.Path, newCloudPaths) {
			return errors.New("新片源锚点不属于本次扫描目录")
		}
		if pipelineUpgradePathWithinAny(oldMedia.Path, newCloudPaths) {
			return errors.New("新旧片源位于同一目录，无法安全区分并替换旧片源")
		}

		oldIdentity := pipelineUpgradeWorkIdentity(oldMedia)
		newIdentity := pipelineUpgradeWorkIdentity(newMedia)
		if oldIdentity == "" || newIdentity == "" {
			return errors.New("整剧升级缺少稳定的 TMDb、Bangumi、豆瓣或 TVDB 作品标识")
		}
		if oldIdentity != newIdentity {
			return fmt.Errorf("新旧片源不属于同一作品：%s != %s", oldIdentity, newIdentity)
		}
		result.WorkIdentity = oldIdentity

		var rows []model.Media
		if err := tx.Where(
			"library_id = ? AND library_root_id = ? AND (season_num > 0 OR episode_num > 0)",
			resolved.LibraryID, resolved.RootID,
		).Order("path ASC, id ASC").Find(&rows).Error; err != nil {
			return err
		}
		newEpisodeKeys := make(map[string]bool)
		for _, row := range rows {
			if pipelineUpgradeWorkIdentity(row) != oldIdentity {
				continue
			}
			if pipelineUpgradePathWithinAny(row.Path, newCloudPaths) {
				newEpisodeKeys[pipelineUpgradeEpisodeKey(row)] = true
				result.PreservedMediaIDs = append(result.PreservedMediaIDs, row.ID)
			}
		}
		if len(result.PreservedMediaIDs) == 0 {
			return errors.New("本次扫描目录中没有可保留的新片源")
		}
		for _, row := range rows {
			if pipelineUpgradeWorkIdentity(row) != oldIdentity || pipelineUpgradePathWithinAny(row.Path, newCloudPaths) {
				continue
			}
			if newEpisodeKeys[pipelineUpgradeEpisodeKey(row)] {
				result.RemovedMediaIDs = append(result.RemovedMediaIDs, row.ID)
				continue
			}
			result.PreservedMediaIDs = append(result.PreservedMediaIDs, row.ID)
		}
		result.Preserved = len(result.PreservedMediaIDs)
		result.Removed = len(result.RemovedMediaIDs)
		if len(result.RemovedMediaIDs) == 0 {
			result.Status = "already_removed"
			result.Reason = "old_source_already_removed"
			return nil
		}

		now := time.Now()
		const batchSize = 400
		for start := 0; start < len(result.RemovedMediaIDs); start += batchSize {
			end := start + batchSize
			if end > len(result.RemovedMediaIDs) {
				end = len(result.RemovedMediaIDs)
			}
			if err := tx.Model(&model.Media{}).Where("id IN ?", result.RemovedMediaIDs[start:end]).Updates(map[string]any{
				"deleted_at":         now,
				"updated_at":         now,
				"deletion_kind":      "version",
				"deleted_by_user_id": "",
			}).Error; err != nil {
				return err
			}
		}
		result.Reason = "old_source_moved_to_recycle_bin"
		return nil
	})
	if err != nil {
		return PipelineWorkSourceCleanupResult{}, err
	}
	if s.cache != nil {
		s.cache.DeletePrefix(ctx, "media:")
		s.cache.DeletePrefix(ctx, "stats:")
	}
	return result, nil
}

func pipelineUpgradeNewSourcePaths(resolved pipelineResolvedTarget, values []string) ([]string, []string, error) {
	root := strings.TrimRight(pipelineNormalizeOpenListPath(resolved.RootOpenListPath), "/")
	if root == "" {
		return nil, nil, errors.New("整剧升级目标缺少 OpenList 根目录")
	}
	seen := make(map[string]bool)
	openListPaths := make([]string, 0, len(values))
	cloudPaths := make([]string, 0, len(values))
	for _, value := range values {
		item := pipelineNormalizeOpenListPath(value)
		if item == "" || item == root || !strings.HasPrefix(item, root+"/") {
			return nil, nil, fmt.Errorf("整剧升级的新片源目录无效：%q", value)
		}
		if seen[item] {
			continue
		}
		seen[item] = true
		openListPaths = append(openListPaths, item)
		cloudPaths = append(cloudPaths, pipelineOpenListPathToCloudPath(item))
	}
	if len(openListPaths) == 0 {
		return nil, nil, errors.New("整剧升级缺少本次扫描的新片源目录")
	}
	return openListPaths, cloudPaths, nil
}

func pipelineUpgradePathWithinAny(raw string, prefixes []string) bool {
	value := strings.TrimRight(strings.TrimSpace(raw), "/")
	for _, prefix := range prefixes {
		prefix = strings.TrimRight(strings.TrimSpace(prefix), "/")
		if value == prefix || strings.HasPrefix(value, prefix+"/") {
			return true
		}
	}
	return false
}

func pipelineUpgradeEpisodeKey(row model.Media) string {
	return fmt.Sprintf("s:%d:e:%d", row.SeasonNum, row.EpisodeNum)
}

func pipelineUpgradeWorkIdentity(row model.Media) string {
	switch {
	case row.TMDbID > 0:
		return fmt.Sprintf("tmdb:%d", row.TMDbID)
	case row.BangumiID > 0:
		return fmt.Sprintf("bangumi:%d", row.BangumiID)
	case strings.TrimSpace(row.DoubanID) != "":
		return "douban:" + strings.ToLower(strings.TrimSpace(row.DoubanID))
	case strings.TrimSpace(row.TheTVDBID) != "":
		return "thetvdb:" + strings.ToLower(strings.TrimSpace(row.TheTVDBID))
	}
	_, hints := pathHintMetadata(row.Path, true)
	switch {
	case hints.TMDbID > 0:
		return fmt.Sprintf("tmdb:%d", hints.TMDbID)
	case hints.BangumiID > 0:
		return fmt.Sprintf("bangumi:%d", hints.BangumiID)
	case strings.TrimSpace(hints.DoubanID) != "":
		return "douban:" + strings.ToLower(strings.TrimSpace(hints.DoubanID))
	case strings.TrimSpace(hints.TheTVDBID) != "":
		return "thetvdb:" + strings.ToLower(strings.TrimSpace(hints.TheTVDBID))
	default:
		return ""
	}
}
