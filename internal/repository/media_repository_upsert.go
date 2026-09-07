package repository

import (
	"context"
	"errors"
	"strings"

	"gorm.io/gorm"

	"github.com/ShukeBta/MediaStationGo/internal/model"
)

var ErrMediaHiddenByUser = errors.New("media hidden by user")

// Upsert inserts or updates a media row keyed by Path (unique index).
//
// 重要：当一条行已经存在时，scanner 重扫只应该刷新文件级元数据
// （时长、宽高、编码、容器、大小），不能把刮削器维护的字段（标题改写、
// 海报、TMDb/Bangumi ID、scrape_status 等）覆盖回零值。
//
// 之前用 Assign(*m).FirstOrCreate(m) 会把整张零值结构体写回，导致：
//  1. scrape_status 从 'matched' / 'no_match' 被清空成 ”；
//  2. 新建行使 GORM `default:pending` 也得不到应用（因为 zero value 被
//     显式写入）。这两个问题都让 EnrichLibrary(WHERE scrape_status='pending')
//     永远捞不到数据。
func (r *MediaRepository) Upsert(ctx context.Context, m *model.Media) error {
	return withSQLiteBusyRetry(ctx, func() error {
		return r.upsert(ctx, m)
	})
}

func (r *MediaRepository) upsert(ctx context.Context, m *model.Media) error {
	r.PrepareSeriesKey(m)
	r.PrepareVersionKey(m)
	r.PrepareEmbyKeys(m)
	prepareMediaSearchAliases(m)
	existing, created, err := r.findOrCreateMediaByPath(ctx, m)
	if err != nil {
		return err
	}
	if created {
		r.indexMediaBestEffort(ctx, *m)
		return nil
	}
	if mediaUpsertShouldKeepDeleted(existing, *m) {
		*m = existing
		return ErrMediaHiddenByUser
	}

	updates := mediaUpsertUpdates(existing, *m)
	seriesKeyInputsChanged := mediaSeriesKeyInputsChanged(updates)
	if r.seriesKeyFunc != nil && (seriesKeyInputsChanged || existing.SeriesKeyVersion != mediaSeriesKeyVersion || existing.SeriesKey == "") {
		keyMedia := existing
		for key, value := range updates {
			switch key {
			case "library_id":
				keyMedia.LibraryID, _ = value.(string)
			case "series_id":
				keyMedia.SeriesID, _ = value.(string)
			case "title":
				keyMedia.Title, _ = value.(string)
			case "original_name":
				keyMedia.OriginalName, _ = value.(string)
			case "season_num":
				keyMedia.SeasonNum, _ = value.(int)
			case "episode_num":
				keyMedia.EpisodeNum, _ = value.(int)
			case "scrape_status":
				keyMedia.ScrapeStatus, _ = value.(string)
			case "tm_db_id":
				keyMedia.TMDbID, _ = value.(int)
			case "bangumi_id":
				keyMedia.BangumiID, _ = value.(int)
			case "douban_id":
				keyMedia.DoubanID, _ = value.(string)
			case "thetvdb_id":
				keyMedia.TheTVDBID, _ = value.(string)
			case "path":
				keyMedia.Path, _ = value.(string)
			}
		}
		keyMedia.SeriesKey = ""
		keyMedia.SeriesKeyVersion = 0
		key := r.seriesKeyFunc(keyMedia)
		if key != "" {
			updates["series_key"] = key
			updates["series_key_version"] = mediaSeriesKeyVersion
		}
	}
	return r.applyMediaUpsertUpdates(ctx, m, existing, updates)
}

func mediaSeriesKeyInputsChanged(updates map[string]any) bool {
	for _, key := range []string{
		"library_id", "series_id", "title", "original_name", "path",
		"season_num", "episode_num", "scrape_status", "tm_db_id",
		"bangumi_id", "douban_id", "thetvdb_id",
	} {
		if _, changed := updates[key]; changed {
			return true
		}
	}
	return false
}

func mediaUpsertShouldKeepDeleted(existing, incoming model.Media) bool {
	if !existing.DeletedAt.Valid {
		return false
	}
	return strings.HasPrefix(strings.ToLower(strings.TrimSpace(existing.Path)), "cloud://") ||
		strings.HasPrefix(strings.ToLower(strings.TrimSpace(incoming.Path)), "cloud://")
}

func (r *MediaRepository) findOrCreateMediaByPath(ctx context.Context, m *model.Media) (model.Media, bool, error) {
	var existing model.Media
	err := r.db.WithContext(ctx).Unscoped().Where("path = ?", m.Path).First(&existing).Error
	if errors.Is(err, gorm.ErrRecordNotFound) {
		// 新行：保证 scrape_status 走 GORM default:pending（即留空让数据库填）。
		if m.ScrapeStatus == "" {
			m.ScrapeStatus = "pending"
		}
		if createErr := r.db.WithContext(ctx).Create(m).Error; createErr == nil {
			return *m, true, nil
		} else if retryErr := r.db.WithContext(ctx).Unscoped().Where("path = ?", m.Path).First(&existing).Error; retryErr != nil {
			return model.Media{}, false, createErr
		}
	}
	if err != nil {
		return model.Media{}, false, err
	}
	return existing, false, nil
}

func mediaUpsertUpdates(existing, incoming model.Media) map[string]any {
	updates := map[string]any{}
	addMediaFileScanUpdates(updates, existing, incoming)
	addMediaTitleUpdates(updates, existing, incoming)
	addMediaExternalIDUpdates(updates, existing, incoming)
	addMatchedMediaMetadataUpdates(updates, existing, incoming)
	addMediaArtworkUpdates(updates, existing, incoming)
	addMediaPlacementUpdates(updates, existing, incoming)
	addMediaSTRMUpdate(updates, existing, incoming)
	return updates
}

func addMediaFileScanUpdates(updates map[string]any, existing, incoming model.Media) {
	// 已存在：仅刷新文件层面的字段。
	sourceChanged := incoming.SizeBytes > 0 && existing.SizeBytes > 0 && existing.SizeBytes != incoming.SizeBytes
	setIfChanged(updates, "size_bytes", existing.SizeBytes, incoming.SizeBytes)
	setIfChanged(updates, "duration_sec", existing.DurationSec, incoming.DurationSec)
	setIfChanged(updates, "width", existing.Width, incoming.Width)
	setIfChanged(updates, "height", existing.Height, incoming.Height)
	setIfChanged(updates, "video_codec", existing.VideoCodec, incoming.VideoCodec)
	setIfChanged(updates, "audio_codec", existing.AudioCodec, incoming.AudioCodec)
	setIfChanged(updates, "container", existing.Container, incoming.Container)
	setIfChanged(updates, "bit_rate", existing.BitRate, incoming.BitRate)
	setIfChanged(updates, "video_bit_rate", existing.VideoBitRate, incoming.VideoBitRate)
	setIfChanged(updates, "frame_rate", existing.FrameRate, incoming.FrameRate)
	setIfChanged(updates, "video_profile", existing.VideoProfile, incoming.VideoProfile)
	setIfChanged(updates, "video_range", existing.VideoRange, incoming.VideoRange)
	setIfChanged(updates, "video_bit_depth", existing.VideoBitDepth, incoming.VideoBitDepth)
	setIfChanged(updates, "audio_bit_rate", existing.AudioBitRate, incoming.AudioBitRate)
	setIfChanged(updates, "audio_channels", existing.AudioChannels, incoming.AudioChannels)
	setIfChanged(updates, "audio_channel_layout", existing.AudioChannelLayout, incoming.AudioChannelLayout)
	setIfChanged(updates, "audio_sample_rate", existing.AudioSampleRate, incoming.AudioSampleRate)
	setIfChanged(updates, "media_probe_version", existing.MediaProbeVersion, incoming.MediaProbeVersion)
	if incoming.NSFW && !existing.NSFW {
		updates["nsfw"] = true
	}
	if existing.DeletedAt.Valid {
		updates["deleted_at"] = nil
	}
	// 回填硬链接身份标识，便于后续扫描去重（避免重复识别/多倍占用）。
	if incoming.FileID != "" && incoming.FileID != existing.FileID {
		updates["file_id"] = incoming.FileID
	}
	if sourceChanged {
		updates["generated_poster_url"] = ""
		updates["generated_backdrop_url"] = ""
		updates["generated_artwork_hash"] = ""
		updates["generated_artwork_status"] = ""
		updates["generated_artwork_error"] = ""
		updates["generated_artwork_attempts"] = 0
	}
}

func addMediaTitleUpdates(updates map[string]any, existing, incoming model.Media) {
	if incoming.Title != "" {
		if incoming.PreserveSourceTitle {
			if strings.EqualFold(strings.TrimSpace(existing.ScrapeStatus), "title_cleaned") {
				return
			}
			setIfChanged(updates, "title", existing.Title, incoming.Title)
			return
		}
		// scanner 给出的标题只是从路径推导，刮削后 title 已被替换为
		// 真实剧名。仅在 existing 还停留在 'pending'/'' 时回填扫描标题，
		// 避免覆盖刮削结果。
		if incoming.ScrapeStatus == "matched" || existing.ScrapeStatus == "pending" || existing.ScrapeStatus == "" || existing.ScrapeStatus == "no_match" {
			titleChanged := !strings.EqualFold(strings.TrimSpace(existing.Title), strings.TrimSpace(incoming.Title))
			yearChanged := incoming.Year > 0 && existing.Year != incoming.Year
			setIfChanged(updates, "title", existing.Title, incoming.Title)
			if incoming.Year > 0 {
				setIfChanged(updates, "year", existing.Year, incoming.Year)
			}
			if incoming.ReleaseDate != "" {
				setIfChanged(updates, "release_date", existing.ReleaseDate, incoming.ReleaseDate)
			}
			if strings.TrimSpace(existing.ScrapeStatus) == "no_match" && incoming.ScrapeStatus != "matched" && (titleChanged || yearChanged) {
				updates["scrape_status"] = "pending"
			}
		}
	}
}

func addMediaExternalIDUpdates(updates map[string]any, existing, incoming model.Media) {
	status := strings.TrimSpace(existing.ScrapeStatus)
	if !mediaCanRefreshExternalIDs(status, incoming) {
		return
	}
	changedExternalID := addIncomingMediaProviderIDs(updates, existing, incoming)
	if incoming.Year > 0 && existing.Year <= 0 {
		updates["year"] = incoming.Year
	}
	if incoming.ReleaseDate != "" && existing.ReleaseDate == "" {
		updates["release_date"] = incoming.ReleaseDate
	}
	if changedExternalID && (status == "no_match" || status == "matched") && incoming.ScrapeStatus != "matched" {
		updates["scrape_status"] = "pending"
	}
}

func mediaCanRefreshExternalIDs(existingStatus string, incoming model.Media) bool {
	return existingStatus == "pending" || existingStatus == "" || existingStatus == "no_match" ||
		incoming.ScrapeStatus == "matched" || strings.HasPrefix(strings.ToLower(strings.TrimSpace(incoming.Path)), "cloud://")
}

func addMatchedMediaMetadataUpdates(updates map[string]any, existing, incoming model.Media) {
	if incoming.ScrapeStatus == "matched" {
		setIfChanged(updates, "scrape_status", existing.ScrapeStatus, incoming.ScrapeStatus)
		addMatchedMediaDetailUpdates(updates, existing, incoming)
		addIncomingMediaProviderIDs(updates, existing, incoming)
	}
}

func addMatchedMediaDetailUpdates(updates map[string]any, existing, incoming model.Media) {
	setNonEmptyMediaString(updates, "original_name", existing.OriginalName, incoming.OriginalName)
	setNonEmptyMediaString(updates, "episode_title", existing.EpisodeTitle, incoming.EpisodeTitle)
	setNonEmptyMediaString(updates, "poster_url", existing.PosterURL, incoming.PosterURL)
	setNonEmptyMediaString(updates, "backdrop_url", existing.BackdropURL, incoming.BackdropURL)
	setNonEmptyMediaString(updates, "overview", existing.Overview, incoming.Overview)
	setNonEmptyMediaString(updates, "languages", existing.Languages, incoming.Languages)
	setNonEmptyMediaString(updates, "countries", existing.Countries, incoming.Countries)
	setNonEmptyMediaString(updates, "genres", existing.Genres, incoming.Genres)
	setNonEmptyMediaString(updates, "actors", existing.Actors, incoming.Actors)
	if incoming.Rating > 0 {
		setIfChanged(updates, "rating", existing.Rating, incoming.Rating)
	}
	if incoming.Year > 0 {
		setIfChanged(updates, "year", existing.Year, incoming.Year)
	}
	if incoming.ReleaseDate != "" {
		setIfChanged(updates, "release_date", existing.ReleaseDate, incoming.ReleaseDate)
	}
}

func addMediaArtworkUpdates(updates map[string]any, existing, incoming model.Media) {
	if incoming.PosterURL != "" {
		setIfChanged(updates, "poster_url", existing.PosterURL, incoming.PosterURL)
	}
	if incoming.BackdropURL != "" {
		setIfChanged(updates, "backdrop_url", existing.BackdropURL, incoming.BackdropURL)
	}
}

func addMediaPlacementUpdates(updates map[string]any, existing, incoming model.Media) {
	// 云盘媒体：同一 cloud:// 文件可能先被父目录库扫描入库，之后用户按二级
	// 分类重新挂载/扫描到更精确的分类库。此时让 library_id 迁移到当前扫描库，
	// 否则媒体被钉死在旧库、新分类库里看不到(表现为"媒体部分消失")。
	// 本地媒体物理位置固定：仅在原 library_id 为空时回填，不迁移。
	if isCloudMediaPath := strings.HasPrefix(strings.ToLower(strings.TrimSpace(incoming.Path)), "cloud://"); incoming.LibraryID != "" && incoming.LibraryID != existing.LibraryID {
		if isCloudMediaPath || existing.LibraryID == "" {
			updates["library_id"] = incoming.LibraryID
		}
	}
	if incoming.LibraryRootID != "" && incoming.LibraryRootID != existing.LibraryRootID {
		updates["library_root_id"] = incoming.LibraryRootID
	}
	if incoming.RelativePath != "" && incoming.RelativePath != existing.RelativePath {
		updates["relative_path"] = incoming.RelativePath
	}
	if incoming.PreserveSourceTitle {
		setIfChanged(updates, "season_num", existing.SeasonNum, 0)
		setIfChanged(updates, "episode_num", existing.EpisodeNum, 0)
		setIfChanged(updates, "series_id", existing.SeriesID, "")
		setIfChanged(updates, "episode_title", existing.EpisodeTitle, "")
		return
	}
	seasonChanged := (incoming.SeasonNum > 0 || incoming.EpisodeNum > 0) && existing.SeasonNum != incoming.SeasonNum
	episodeChanged := incoming.EpisodeNum > 0 && existing.EpisodeNum != incoming.EpisodeNum
	if seasonChanged {
		updates["season_num"] = incoming.SeasonNum
	}
	if episodeChanged {
		updates["episode_num"] = incoming.EpisodeNum
	}
	if strings.TrimSpace(existing.ScrapeStatus) == "no_match" && incoming.ScrapeStatus != "matched" && (seasonChanged || episodeChanged) {
		updates["scrape_status"] = "pending"
	}
}

func addMediaSTRMUpdate(updates map[string]any, existing, incoming model.Media) {
	if incoming.STRMURL != "" {
		setIfChanged(updates, "strm_url", existing.STRMURL, incoming.STRMURL)
	}
}

func addIncomingMediaProviderIDs(updates map[string]any, existing, incoming model.Media) bool {
	changed := false
	if incoming.TMDbID > 0 && existing.TMDbID != incoming.TMDbID {
		updates["tm_db_id"] = incoming.TMDbID
		changed = true
	}
	if incoming.BangumiID > 0 && existing.BangumiID != incoming.BangumiID {
		updates["bangumi_id"] = incoming.BangumiID
		changed = true
	}
	if incoming.DoubanID != "" && strings.TrimSpace(existing.DoubanID) != strings.TrimSpace(incoming.DoubanID) {
		updates["douban_id"] = incoming.DoubanID
		changed = true
	}
	if incoming.TheTVDBID != "" && strings.TrimSpace(existing.TheTVDBID) != strings.TrimSpace(incoming.TheTVDBID) {
		updates["thetvdb_id"] = incoming.TheTVDBID
		changed = true
	}
	return changed
}

func setNonEmptyMediaString(updates map[string]any, key, current, next string) {
	if next != "" {
		setIfChanged(updates, key, current, next)
	}
}

func (r *MediaRepository) applyMediaUpsertUpdates(ctx context.Context, m *model.Media, existing model.Media, updates map[string]any) error {
	if len(updates) == 0 {
		if r.embyProjectionStale(existing) {
			if err := r.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
				if err := r.RefreshEmbyKeys(ctx, tx, []string{existing.ID}); err != nil {
					return err
				}
				return tx.Unscoped().Where("id = ?", existing.ID).First(&existing).Error
			}); err != nil {
				return err
			}
		}
		*m = existing
		return nil
	}
	versionKeyNeedsRefresh := r.versionKeyFunc != nil && (mediaVersionKeyInputsChanged(updates) || existing.MediaVersionKeyVersion != mediaVersionKeyVersion || existing.MediaVersionKey == "")
	writeUpdates := func(tx *gorm.DB) error {
		if err := tx.WithContext(ctx).Unscoped().Model(&model.Media{}).
			Where("id = ?", existing.ID).Updates(updates).Error; err != nil {
			return err
		}
		// Direct SQL metadata and migration writes are intentionally invalidated
		// by a database trigger. Upsert has already recomputed the authoritative
		// key in Go, so restore that current value after the trigger in the same
		// transaction instead of leaving a newly scanned row on the fallback path.
		if mediaSeriesKeyInputsChanged(updates) {
			key, hasKey := updates["series_key"]
			version, hasVersion := updates["series_key_version"]
			if hasKey && hasVersion {
				if err := tx.WithContext(ctx).Unscoped().Model(&model.Media{}).
					Where("id = ?", existing.ID).
					UpdateColumns(map[string]any{"series_key": key, "series_key_version": version}).Error; err != nil {
					return err
				}
			}
		}
		if versionKeyNeedsRefresh {
			var updated model.Media
			if err := tx.WithContext(ctx).Where("id = ?", existing.ID).First(&updated).Error; err != nil {
				return err
			}
			r.PrepareVersionKey(&updated)
			if updated.MediaVersionKey == "" {
				return errors.New("updated media has no current version key")
			}
			if err := tx.WithContext(ctx).Model(&model.Media{}).Where("id = ?", existing.ID).
				UpdateColumns(map[string]any{
					"media_version_key":         updated.MediaVersionKey,
					"media_version_key_version": updated.MediaVersionKeyVersion,
				}).Error; err != nil {
				return err
			}
		}
		if r.embyKeyFunc != nil && (mediaEmbyKeyInputsChanged(updates) || r.embyProjectionStale(existing)) {
			return r.RefreshEmbyKeys(ctx, tx, []string{existing.ID})
		}
		return nil
	}
	var err error
	if mediaSeriesKeyInputsChanged(updates) || versionKeyNeedsRefresh || (r.embyKeyFunc != nil && (mediaEmbyKeyInputsChanged(updates) || r.embyProjectionStale(existing))) {
		err = r.db.WithContext(ctx).Transaction(writeUpdates)
	} else {
		err = writeUpdates(r.db)
	}
	if err != nil {
		return err
	}
	// 回写 ID / 不可变字段，让 caller 拿到完整的现有行。
	*m = existing
	fresh, err := r.FindByID(ctx, existing.ID)
	if err != nil {
		return err
	}
	if fresh == nil {
		return errors.New("updated media not found")
	}
	*m = *fresh
	if err := r.RefreshSearchAliases(ctx, fresh.ID); err != nil {
		return err
	}
	indexed, err := r.FindByID(ctx, fresh.ID)
	if err != nil {
		return err
	}
	if indexed == nil {
		return errors.New("updated media search aliases not found")
	}
	*m = *indexed
	return nil
}

func mediaVersionKeyInputsChanged(updates map[string]any) bool {
	for _, key := range []string{
		"library_id", "title", "original_name", "path", "part_group_key", "part_index",
		"version_group_key", "title_cleanup_version", "season_num", "episode_num", "year",
		"tm_db_id", "bangumi_id", "douban_id", "thetvdb_id",
	} {
		if _, changed := updates[key]; changed {
			return true
		}
	}
	return false
}

func setIfChanged[T comparable](updates map[string]any, key string, current, next T) {
	if current != next {
		updates[key] = next
	}
}
