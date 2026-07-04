package handler

import (
	"context"
	"net/url"
	"path"
	"strings"

	"go.uber.org/zap"

	"github.com/ShukeBta/MediaStationGo/internal/model"
	"github.com/ShukeBta/MediaStationGo/internal/service"
)

func enrichAndPersistDownloadRows(ctx context.Context, svc *service.Container, rows []model.DownloadTask) {
	for i := range rows {
		before := rows[i]
		enrichDownloadRow(ctx, svc, &rows[i])
		updates := downloadTaskMetadataUpdates(before, rows[i])
		if len(updates) == 0 {
			continue
		}
		if err := svc.Repo.DB.WithContext(ctx).Model(&model.DownloadTask{}).Where("id = ?", rows[i].ID).Updates(updates).Error; err != nil {
			svc.Log.Debug("download artwork backfill failed", zap.String("id", rows[i].ID), zap.Error(err))
		}
	}
}

func enrichDownloadRow(ctx context.Context, svc *service.Container, row *model.DownloadTask) {
	if strings.TrimSpace(row.Title) == "" {
		row.Title = downloadDisplayTitle(row.URL)
	}
	if !downloadTaskNeedsMetadata(*row) {
		return
	}
	meta := enrichDownloadTaskMeta(ctx, svc, service.DownloadTaskMeta{
		Title:            row.Title,
		PosterURL:        row.PosterURL,
		BackdropURL:      row.BackdropURL,
		Overview:         row.Overview,
		OriginalName:     row.OriginalName,
		OriginalLanguage: row.OriginalLanguage,
		Year:             row.Year,
		Rating:           row.Rating,
		Genres:           row.Genres,
	}, firstNonEmptyString(row.Title, row.URL), row.MediaType)
	row.Title = firstNonEmptyString(row.Title, meta.Title)
	row.PosterURL = meta.PosterURL
	row.BackdropURL = meta.BackdropURL
	row.Overview = meta.Overview
	row.OriginalName = meta.OriginalName
	row.OriginalLanguage = meta.OriginalLanguage
	row.Year = meta.Year
	row.Rating = meta.Rating
	row.Genres = meta.Genres
}

func downloadTaskNeedsMetadata(row model.DownloadTask) bool {
	return strings.TrimSpace(row.PosterURL) == "" || strings.TrimSpace(row.BackdropURL) == "" ||
		strings.TrimSpace(row.Overview) == "" || row.Year <= 0 || row.Rating <= 0 ||
		strings.TrimSpace(row.Genres) == "" || strings.TrimSpace(row.OriginalName) == "" ||
		strings.TrimSpace(row.OriginalLanguage) == ""
}

func downloadTaskMetadataUpdates(before, after model.DownloadTask) map[string]any {
	updates := map[string]any{}
	if before.Title != after.Title {
		updates["title"] = after.Title
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

func enrichDownloadTorrentViews(ctx context.Context, svc *service.Container, views []service.DownloadTorrentView) {
	cache := map[string]displayMetadata{}
	for i := range views {
		if strings.TrimSpace(views[i].PosterURL) != "" && strings.TrimSpace(views[i].BackdropURL) != "" {
			continue
		}
		query := firstNonEmptyString(views[i].Title, views[i].Name)
		cacheKey, _ := metadataSearchQuery(query)
		meta, ok := cache[cacheKey]
		if !ok {
			meta = lookupDisplayMetadata(ctx, svc, query, "", "")
			cache[cacheKey] = meta
		}
		if strings.TrimSpace(views[i].PosterURL) == "" {
			views[i].PosterURL = meta.PosterURL
		}
		if strings.TrimSpace(views[i].BackdropURL) == "" {
			views[i].BackdropURL = meta.BackdropURL
		}
		if strings.TrimSpace(views[i].Overview) == "" {
			views[i].Overview = meta.Overview
		}
		if (views[i].Title == "" || views[i].Title == views[i].Name) && meta.Title != "" {
			views[i].Title = meta.Title
		}
	}
}

func enrichDownloadTaskMeta(ctx context.Context, svc *service.Container, meta service.DownloadTaskMeta, fallbackTitle, mediaType string) service.DownloadTaskMeta {
	if strings.TrimSpace(meta.Title) == "" {
		meta.Title = strings.TrimSpace(downloadDisplayTitle(fallbackTitle))
	}
	// 仅当展示字段都齐时才跳过查询(含媒体富通知所需的年份/评分/类型/原始信息)。
	if strings.TrimSpace(meta.PosterURL) != "" && strings.TrimSpace(meta.BackdropURL) != "" &&
		strings.TrimSpace(meta.Overview) != "" && meta.Year > 0 && meta.Rating > 0 &&
		strings.TrimSpace(meta.Genres) != "" && strings.TrimSpace(meta.OriginalName) != "" &&
		strings.TrimSpace(meta.OriginalLanguage) != "" {
		return meta
	}
	found := lookupDisplayMetadata(ctx, svc, meta.Title, fallbackTitle, mediaType)
	if strings.TrimSpace(meta.Title) == "" {
		meta.Title = found.Title
	}
	if strings.TrimSpace(meta.PosterURL) == "" {
		meta.PosterURL = found.PosterURL
	}
	if strings.TrimSpace(meta.BackdropURL) == "" {
		meta.BackdropURL = found.BackdropURL
	}
	if strings.TrimSpace(meta.Overview) == "" {
		meta.Overview = found.Overview
	}
	if strings.TrimSpace(meta.OriginalName) == "" {
		meta.OriginalName = found.OriginalName
	}
	if strings.TrimSpace(meta.OriginalLanguage) == "" {
		meta.OriginalLanguage = found.OriginalLanguage
	}
	if meta.Year <= 0 && found.Year > 0 {
		meta.Year = found.Year
	}
	if meta.Rating <= 0 && found.Rating > 0 {
		meta.Rating = found.Rating
	}
	if strings.TrimSpace(meta.Genres) == "" {
		meta.Genres = found.Genres
	}
	return meta
}

func downloadDisplayTitle(raw string) string {
	raw = strings.TrimSpace(raw)
	if raw == "" {
		return ""
	}
	if u, err := url.Parse(raw); err == nil {
		if dn := strings.TrimSpace(u.Query().Get("dn")); dn != "" {
			if decoded, err := url.QueryUnescape(dn); err == nil && strings.TrimSpace(decoded) != "" {
				return strings.TrimSpace(decoded)
			}
			return dn
		}
		if u.Host != "" {
			base := path.Base(u.Path)
			if base != "." && base != "/" && base != "" {
				base = strings.TrimSuffix(base, path.Ext(base))
				return strings.TrimSpace(base)
			}
		}
	}
	return raw
}
