package service

import (
	"context"
	"strings"

	"github.com/ShukeBta/MediaStationGo/internal/model"
	"gorm.io/gorm"
)

func (e *EmbyService) folderCoverSeriesSQL(ctx context.Context, libraryIDs []string, imageType string, limit int) ([]EmbyFolderCoverArtwork, error) {
	q := e.repo.DB.WithContext(ctx).Model(&model.Media{}).Where("library_id IN ?", libraryIDs).Where("poster_url <> '' OR backdrop_url <> ''")
	if err := e.ensureEmbyKeys(ctx, q); err != nil {
		return nil, err
	}
	base := q.Session(&gorm.Session{}).Select("media.id, media.library_id, media.emby_series_key AS group_key, media.tm_db_id, media.poster_url, media.backdrop_url, media.updated_at, media.created_at")
	const cte = `WITH inventory AS (?), ranked AS (
 SELECT inventory.*, ROW_NUMBER() OVER (PARTITION BY group_key ORDER BY updated_at DESC, created_at DESC, id DESC) AS rn FROM inventory
), representatives AS (
 SELECT group_key, MIN(CASE WHEN poster_url <> '' THEN rn ELSE NULL END) AS poster_rn,
 MIN(CASE WHEN backdrop_url <> '' THEN rn ELSE NULL END) AS backdrop_rn FROM ranked GROUP BY group_key
), artwork AS (
 SELECT sample.group_key, sample.id AS sample_id, sample.updated_at, sample.created_at,
 TRIM(COALESCE(NULLIF(TRIM(metadata.poster_url), ''), poster_sample.poster_url, '')) AS poster,
 CASE WHEN metadata.id IS NOT NULL THEN TRIM(COALESCE(metadata.backdrop_url, ''))
 ELSE TRIM(COALESCE(backdrop_sample.backdrop_url, '')) END AS backdrop
 FROM ranked sample JOIN representatives rep ON rep.group_key = sample.group_key
 LEFT JOIN ranked poster_sample ON poster_sample.group_key = rep.group_key AND poster_sample.rn = rep.poster_rn
 LEFT JOIN ranked backdrop_sample ON backdrop_sample.group_key = rep.group_key AND backdrop_sample.rn = rep.backdrop_rn
 LEFT JOIN series metadata ON metadata.id = COALESCE(
 (SELECT s.id FROM series s WHERE s.deleted_at IS NULL AND s.id = sample.group_key LIMIT 1),
 (SELECT s.id FROM series s WHERE s.deleted_at IS NULL AND sample.tm_db_id > 0 AND s.library_id = sample.library_id AND s.tm_db_id = sample.tm_db_id ORDER BY s.updated_at DESC, s.id DESC LIMIT 1)
 ) WHERE sample.rn = 1
), preferred AS (
 SELECT artwork.*, `
	seenGroups, seenURLs := []string{}, []string{}
	out := make([]EmbyFolderCoverArtwork, 0, limit)
	for _, preferred := range folderCoverImageTypePreference(imageType) {
		url, kind := "poster", "'Primary'"
		if preferred == "Backdrop" {
			url = "COALESCE(NULLIF(backdrop, ''), poster)"
			kind = "CASE WHEN backdrop <> '' THEN 'Backdrop' ELSE 'Primary' END"
		}
		query := cte + url + " AS url, " + kind + " AS image_type FROM artwork), unique_urls AS (SELECT preferred.*, ROW_NUMBER() OVER (PARTITION BY url ORDER BY updated_at DESC, created_at DESC, sample_id DESC) AS url_rank FROM preferred WHERE url <> ''"
		args := []any{base}
		if len(seenGroups) > 0 {
			query += " AND group_key NOT IN ?"
			args = append(args, seenGroups)
		}
		if len(seenURLs) > 0 {
			query += " AND url NOT IN ?"
			args = append(args, seenURLs)
		}
		query += ") SELECT group_key AS media_id, url, image_type FROM unique_urls WHERE url_rank = 1 ORDER BY updated_at DESC, created_at DESC, sample_id DESC LIMIT ?"
		args = append(args, limit-len(out))
		var page []EmbyFolderCoverArtwork
		if err := e.repo.DB.WithContext(ctx).Raw(query, args...).Scan(&page).Error; err != nil {
			return nil, err
		}
		for _, item := range page {
			item.Tag = item.MediaID
			if strings.EqualFold(item.ImageType, "Backdrop") {
				item.Tag += "-bd"
			}
			out = append(out, item)
			seenGroups = append(seenGroups, item.MediaID)
			seenURLs = append(seenURLs, item.URL)
		}
		if len(out) == limit {
			break
		}
	}
	return out, nil
}
