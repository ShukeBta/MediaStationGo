package service

import (
	"context"
	"fmt"
	"strings"
	"time"

	"github.com/ShukeBta/MediaStationGo/internal/model"
)

func (s *SubscriptionService) notifySubscriptionHit(sub *model.Subscription, queued int, resources []string) {
	if s == nil || s.notify == nil || sub == nil || queued <= 0 {
		return
	}
	body := fmt.Sprintf("订阅：%s\n新增资源：%d", sub.Name, queued)
	if len(resources) > 0 {
		body += "\n资源：\n- " + strings.Join(resources, "\n- ")
	}
	go func() {
		ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
		defer cancel()
		data := map[string]interface{}{}
		if strings.TrimSpace(sub.PosterURL) != "" {
			data["poster_url"] = sub.PosterURL
		}
		if strings.TrimSpace(sub.BackdropURL) != "" {
			data["backdrop_url"] = sub.BackdropURL
		}
		if strings.TrimSpace(sub.MediaType) != "" {
			data["media_type"] = sub.MediaType
		}
		if strings.TrimSpace(sub.MediaCategory) != "" {
			data["media_category"] = sub.MediaCategory
		}
		// 补充媒体通知模板(formatTelegramMediaNotification)所需字段:片名 / 原名 /
		// 语言 / 年份 / 评分 / 类型 / 简介 / 外链 / 资源标题(供模板提取季集 + 版本)。
		// 仅填现成可用的,缺失项模板会自动略过。
		if strings.TrimSpace(sub.Name) != "" {
			data["title"] = sub.Name
		}
		if strings.TrimSpace(sub.OriginalName) != "" {
			data["original_title"] = sub.OriginalName
		}
		if strings.TrimSpace(sub.OriginalLanguage) != "" {
			data["original_language"] = sub.OriginalLanguage
		}
		if sub.Year > 0 {
			data["year"] = sub.Year
		}
		if sub.Rating > 0 {
			data["rating"] = sub.Rating
		}
		if strings.TrimSpace(sub.Genres) != "" {
			data["genres"] = sub.Genres
		}
		if strings.TrimSpace(sub.Overview) != "" {
			data["overview"] = sub.Overview
		}
		if id := strings.TrimSpace(sub.IMDBID); id != "" {
			data["imdb_url"] = "https://www.imdb.com/title/" + id + "/"
		}
		if len(resources) > 0 {
			data["resource_title"] = resources[0]
		}
		s.notify.BroadcastEvent(ctx, NotifyEvent{
			Type:    EventSubscriptionHit,
			Title:   "MediaStationGo 订阅命中新资源",
			Message: body,
			Data:    data,
		})
	}()
}
