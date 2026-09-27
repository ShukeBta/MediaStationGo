package service

import (
	"context"
	"strings"
	"time"

	"github.com/ShukeBta/MediaStationGo/internal/model"
)

// ImageURL returns artwork for a media/series/season item id.
func (e *EmbyService) ImageURL(ctx context.Context, id, imageType string) (string, error) {
	pick := func(primary, backdrop string) string {
		switch strings.ToLower(imageType) {
		case "backdrop", "art":
			if backdrop != "" {
				return backdrop
			}
		}
		if primary != "" {
			return primary
		}
		return backdrop
	}
	if personName, ok := embyPersonName(id); ok {
		if strings.ToLower(strings.TrimSpace(imageType)) != "primary" {
			return "", nil
		}
		snapshot, err := e.personMetadataSnapshot(ctx)
		if err != nil {
			return "", err
		}
		return snapshot[normalizePersonNameKey(personName)].ImageURL, nil
	}
	if strings.HasPrefix(id, embyVirtualSeasonPrefix) {
		if raw, ok := e.cachedArtworkURL(id, imageType); ok {
			return raw, nil
		}
		if season, ok, err := e.findSeasonGroup(ctx, id, ""); err != nil {
			return "", err
		} else if ok {
			if raw := pick(season.Series.PosterURL, season.Series.BackdropURL); raw != "" {
				return raw, nil
			}
			if embyWantsPrimaryImage(imageType) {
				return e.localThumbnailFromMediaRows(ctx, season.Episodes)
			}
		}
		return "", nil
	}
	if strings.HasPrefix(id, embyVirtualSeriesPrefix) {
		if raw, ok := e.cachedArtworkURL(id, imageType); ok {
			return raw, nil
		}
		if series, ok, err := e.findSeriesGroup(ctx, id, ""); err != nil {
			return "", err
		} else if ok {
			if raw := pick(series.PosterURL, series.BackdropURL); raw != "" {
				return raw, nil
			}
			if embyWantsPrimaryImage(imageType) {
				return e.localThumbnailFromMediaRows(ctx, series.Episodes)
			}
		}
		return "", nil
	}
	m, err := e.repo.Media.FindByID(ctx, id)
	if err == nil && m != nil {
		if e.mediaShouldBeEpisode(ctx, m) {
			switch strings.ToLower(imageType) {
			case "backdrop", "art":
				return "", nil
			}
		}
		if raw := pick(e.mediaPrimaryArtwork(ctx, m), e.mediaBackdropArtwork(ctx, m)); raw != "" {
			return raw, nil
		}
		if embyWantsPrimaryImage(imageType) {
			return e.localVideoThumbnail(ctx, m)
		}
		return "", nil
	}
	if err != nil {
		return "", err
	}
	if series, ok, err := e.findSeriesGroup(ctx, id, ""); err != nil {
		return "", err
	} else if ok {
		if raw := pick(series.PosterURL, series.BackdropURL); raw != "" {
			return raw, nil
		}
		if embyWantsPrimaryImage(imageType) {
			return e.localThumbnailFromMediaRows(ctx, series.Episodes)
		}
	}
	return "", nil
}

func (e *EmbyService) mediaPrimaryArtwork(ctx context.Context, m *model.Media) string {
	if m == nil {
		return ""
	}
	if raw := mediaPrimaryArtworkForEpisode(m, e.mediaShouldBeEpisode(ctx, m)); raw != "" && raw != m.GeneratedPosterURL {
		return raw
	}
	// 本地同目录海报 / NFO 图片(yebuwudong)优先于自动生成的预览图。
	if local := e.localMediaArtwork(ctx, m, "primary"); local != "" {
		return local
	}
	return m.GeneratedPosterURL
}

func mediaPrimaryArtworkForEpisode(m *model.Media, isEpisode bool) string {
	if m == nil {
		return ""
	}
	if isEpisode && strings.TrimSpace(m.BackdropURL) != "" {
		return m.BackdropURL
	}
	if poster := strings.TrimSpace(m.PosterURL); poster != "" {
		return poster
	}
	return m.GeneratedPosterURL
}

func (e *EmbyService) mediaBackdropArtwork(ctx context.Context, m *model.Media) string {
	if m == nil {
		return ""
	}
	if strings.TrimSpace(m.BackdropURL) != "" {
		return m.BackdropURL
	}
	if local := e.localMediaArtwork(ctx, m, "backdrop"); local != "" {
		return local
	}
	return m.GeneratedBackdropURL
}

func (e *EmbyService) localMediaArtwork(ctx context.Context, m *model.Media, imageType string) string {
	if e == nil || e.repo == nil || m == nil {
		return ""
	}
	mediaPath := strings.TrimSpace(m.Path)
	if mediaPath == "" || strings.Contains(mediaPath, "://") {
		return ""
	}
	// 每条媒体都要扫描目录找海报/NFO，慢速存储上大列表一次要好几秒；
	// 按 路径+类型 缓存两分钟。
	cacheKey := imageType + "|" + mediaPath
	if cached, ok := e.cachedFSProbe(cacheKey); ok {
		return cached
	}
	libraryRoot := ""
	if strings.TrimSpace(m.LibraryID) != "" {
		if lib, err := e.repo.Library.FindByID(ctx, m.LibraryID); err == nil && lib != nil {
			libraryRoot = strings.TrimSpace(lib.Path)
		}
	}
	value := ""
	meta, err := ReadLocalMetadata(mediaPath, libraryRoot, e.mediaShouldBeEpisode(ctx, m))
	if err == nil && meta != nil {
		switch strings.ToLower(strings.TrimSpace(imageType)) {
		case "backdrop", "art":
			value = strings.TrimSpace(meta.BackdropURL)
		default:
			value = strings.TrimSpace(meta.PosterURL)
		}
	}
	e.storeFSProbe(cacheKey, value)
	return value
}

// 海报/缩略图探测走慢速存储，且结果极少变化，缓存期放长一些。
const embyFSProbeCacheTTL = 30 * time.Minute

type embyFSProbeCacheEntry struct {
	value   string
	expires time.Time
}

func (e *EmbyService) cachedFSProbe(key string) (string, bool) {
	e.fsProbeMu.RLock()
	defer e.fsProbeMu.RUnlock()
	entry, ok := e.fsProbeCache[key]
	if !ok || time.Now().After(entry.expires) {
		return "", false
	}
	return entry.value, true
}

func (e *EmbyService) storeFSProbe(key, value string) {
	e.fsProbeMu.Lock()
	defer e.fsProbeMu.Unlock()
	if e.fsProbeCache == nil {
		e.fsProbeCache = make(map[string]embyFSProbeCacheEntry)
	}
	if len(e.fsProbeCache) > 20000 {
		e.fsProbeCache = make(map[string]embyFSProbeCacheEntry)
	}
	e.fsProbeCache[key] = embyFSProbeCacheEntry{value: value, expires: time.Now().Add(embyFSProbeCacheTTL)}
}
