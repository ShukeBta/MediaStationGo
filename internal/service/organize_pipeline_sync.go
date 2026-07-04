package service

import (
	"context"
	"path/filepath"
	"strings"

	"go.uber.org/zap"

	"github.com/ShukeBta/MediaStationGo/internal/model"
)

func (p *OrganizePipelineService) syncScrapedOrganizedNames(ctx context.Context, opts OrganizeOptions, res *OrganizeResult, path string) {
	if p == nil || p.organizer == nil || p.repo == nil || p.repo.DB == nil || res == nil || res.DryRun {
		return
	}
	for _, target := range organizeNameSyncTargets(res, path) {
		media := p.mediaByPath(ctx, target)
		if media == nil {
			continue
		}
		before := media.Path
		syncOpts := opts
		syncOpts.SourcePath = ""
		syncOpts.TransferMode = TransferMove
		after, err := p.organizer.SyncMediaPathWithMetadata(ctx, media.ID, syncOpts)
		if err != nil {
			res.Errors = append(res.Errors, filepath.Base(before)+": "+err.Error())
			if p.log != nil {
				p.log.Warn("organize pipeline scrape rename failed",
					zap.String("media", media.ID),
					zap.String("path", before),
					zap.Error(err))
			}
			continue
		}
		if samePath(before, after) {
			continue
		}
		res.Items = append(res.Items, OrganizePreviewItem{
			Source: before,
			Target: after,
			Action: "organize",
			Reason: "scraped metadata rename",
			Title:  media.Title,
		})
		if p.log != nil {
			p.log.Info("organize pipeline synced scraped media name",
				zap.String("media", media.ID),
				zap.String("from", before),
				zap.String("to", after))
		}
	}
}

func organizeNameSyncTargets(res *OrganizeResult, path string) []string {
	seen := map[string]struct{}{}
	out := make([]string, 0, len(res.Items)+1)
	add := func(value string) {
		value = strings.TrimSpace(value)
		if value == "" {
			return
		}
		key := strings.ToLower(filepath.Clean(value))
		if _, ok := seen[key]; ok {
			return
		}
		seen[key] = struct{}{}
		out = append(out, value)
	}
	for _, item := range res.Items {
		switch item.Action {
		case "organize", "replace", "reclassify":
			add(item.Target)
		}
	}
	add(path)
	return out
}

func (p *OrganizePipelineService) mediaByPath(ctx context.Context, path string) *model.Media {
	path = filepath.Clean(strings.TrimSpace(path))
	if path == "" || path == "." {
		return nil
	}
	var media model.Media
	if err := p.repo.DB.WithContext(ctx).
		Where("path = ? AND deleted_at IS NULL", path).
		Limit(1).
		Take(&media).Error; err != nil {
		return nil
	}
	return &media
}
