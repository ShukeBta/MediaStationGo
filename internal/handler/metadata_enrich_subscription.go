package handler

import (
	"context"
	"strings"

	"go.uber.org/zap"

	"github.com/ShukeBta/MediaStationGo/internal/model"
	"github.com/ShukeBta/MediaStationGo/internal/service"
)

func enrichSubscriptionArtwork(ctx context.Context, svc *service.Container, sub *model.Subscription) {
	if svc == nil || sub == nil {
		return
	}
	// 已有图片且媒体展示字段也齐全时才跳过;否则仍需查一次补 年份/评分/类型 等
	// 富通知字段(老订阅只存了 poster/overview 的情况)。
	if strings.TrimSpace(sub.PosterURL) != "" && strings.TrimSpace(sub.BackdropURL) != "" &&
		sub.Year > 0 && sub.Rating > 0 && strings.TrimSpace(sub.Genres) != "" {
		return
	}
	meta := lookupDisplayMetadata(ctx, svc, sub.Name, sub.Filter, sub.MediaType)
	if meta.Title == "" && meta.PosterURL == "" && meta.BackdropURL == "" && meta.Overview == "" {
		return
	}
	if strings.TrimSpace(sub.Source) == "" {
		sub.Source = meta.Source
	}
	if strings.TrimSpace(sub.PosterURL) == "" {
		sub.PosterURL = meta.PosterURL
	}
	if strings.TrimSpace(sub.BackdropURL) == "" {
		sub.BackdropURL = meta.BackdropURL
	}
	if strings.TrimSpace(sub.Overview) == "" {
		sub.Overview = meta.Overview
	}
	if strings.TrimSpace(sub.OriginalName) == "" {
		sub.OriginalName = meta.OriginalName
	}
	if strings.TrimSpace(sub.OriginalLanguage) == "" {
		sub.OriginalLanguage = meta.OriginalLanguage
	}
	if sub.Year <= 0 && meta.Year > 0 {
		sub.Year = meta.Year
	}
	if sub.Rating <= 0 && meta.Rating > 0 {
		sub.Rating = meta.Rating
	}
	if strings.TrimSpace(sub.Genres) == "" {
		sub.Genres = meta.Genres
	}
}

func enrichAndPersistSubscriptions(ctx context.Context, svc *service.Container, items []model.Subscription) {
	for i := range items {
		before := items[i]
		enrichSubscriptionArtwork(ctx, svc, &items[i])
		updates := subscriptionMetadataUpdates(before, items[i])
		if len(updates) == 0 {
			continue
		}
		if err := svc.Repo.DB.WithContext(ctx).Model(&model.Subscription{}).Where("id = ?", items[i].ID).Updates(updates).Error; err != nil {
			svc.Log.Debug("subscription artwork backfill failed", zap.String("id", items[i].ID), zap.Error(err))
		}
	}
}

func subscriptionMetadataUpdates(before, after model.Subscription) map[string]any {
	updates := map[string]any{}
	if before.Source != after.Source {
		updates["source"] = after.Source
	}
	if before.PosterURL != after.PosterURL {
		updates["poster_url"] = after.PosterURL
	}
	if before.BackdropURL != after.BackdropURL {
		updates["backdrop_url"] = after.BackdropURL
	}
	if before.Overview != after.Overview {
		updates["overview"] = after.Overview
	}
	if before.OriginalName != after.OriginalName {
		updates["original_name"] = after.OriginalName
	}
	if before.OriginalLanguage != after.OriginalLanguage {
		updates["original_language"] = after.OriginalLanguage
	}
	if before.Year != after.Year {
		updates["year"] = after.Year
	}
	if before.Rating != after.Rating {
		updates["rating"] = after.Rating
	}
	if before.Genres != after.Genres {
		updates["genres"] = after.Genres
	}
	return updates
}
