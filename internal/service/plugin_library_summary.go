package service

import (
	"context"
	"fmt"

	"github.com/ShukeBta/MediaStationGo/internal/model"
	"gorm.io/gorm"
)

type librarySummaryPlugin struct{ db *gorm.DB }

func (p *librarySummaryPlugin) Manifest() PluginManifest {
	return PluginManifest{
		ID: "library-summary", Name: "媒体库概览", Version: "1.0.0", Author: "MediaStationGo",
		Description:  "汇总媒体库与媒体文件数量，快速检查当前收藏规模。仅查询数据，不修改媒体。",
		Capabilities: []string{"library:read"},
		ConfigFields: []PluginConfigField{{Key: "include_disabled", Label: "包含已停用的媒体库",
			Description: "默认只统计已启用的媒体库；已删除的媒体库和媒体始终不计入。", Type: "boolean", Default: false}},
	}
}

func (p *librarySummaryPlugin) ValidateConfig(config map[string]any) error {
	if _, ok := config["include_disabled"].(bool); !ok || len(config) != 1 {
		return ErrPluginInvalidConfig
	}
	return nil
}

func (p *librarySummaryPlugin) Run(ctx context.Context, config map[string]any) (PluginRunResult, error) {
	var libraries, media int64
	includeDisabled := config["include_disabled"].(bool)
	err := p.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		libs := tx.Model(&model.Library{})
		if !includeDisabled {
			libs = libs.Where("enabled = ?", true)
		}
		if err := libs.Count(&libraries).Error; err != nil {
			return err
		}
		items := tx.Model(&model.Media{}).
			Joins("JOIN libraries ON libraries.id = media.library_id AND libraries.deleted_at IS NULL")
		if !includeDisabled {
			items = items.Where("libraries.enabled = ?", true)
		}
		return items.Count(&media).Error
	})
	if err != nil {
		return PluginRunResult{}, err
	}
	return PluginRunResult{Summary: fmt.Sprintf("共 %d 个媒体库，%d 个媒体文件。", libraries, media),
		Metrics: []PluginMetric{{Label: "媒体库", Value: libraries}, {Label: "媒体文件", Value: media}}}, nil
}
