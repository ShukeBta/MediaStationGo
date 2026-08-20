package service

import (
	"context"
	"strings"
	"time"

	"github.com/ShukeBta/MediaStationGo/internal/model"
	"github.com/ShukeBta/MediaStationGo/internal/repository"
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
	if strings.HasPrefix(id, embyVirtualSeasonPrefix) {
		if raw, ok := e.cachedArtworkURL(id, imageType); ok {
			return raw, nil
		}
		return "", nil
	}
	if strings.HasPrefix(id, embyVirtualSeriesPrefix) {
		if raw, ok := e.cachedArtworkURL(id, imageType); ok {
			return raw, nil
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
		return pick(e.mediaPrimaryArtwork(ctx, m), e.mediaBackdropArtwork(ctx, m)), nil
	}
	if err != nil {
		return "", err
	}
	if series, ok, err := e.findSeriesGroup(ctx, id, ""); err != nil {
		return "", err
	} else if ok {
		return pick(series.PosterURL, series.BackdropURL), nil
	}
	// id 既不是媒体也不是剧集组时,还可能是一个媒体库(Emby 客户端通过
	// /Items/{libraryID}/Images/Primary 请求库封面)。复用网页的封面来源:
	// cover_url 优先,否则挑库内评分最高的一张成员海报。
	if raw := e.libraryCoverArtwork(ctx, id); raw != "" {
		return raw, nil
	}
	return "", nil
}

// cachedLibraryCover returns a previously resolved library cover URL within TTL.
func (e *EmbyService) cachedLibraryCover(id string) (string, bool) {
	if e == nil || strings.TrimSpace(id) == "" {
		return "", false
	}
	now := time.Now()
	e.libraryCoverMu.Lock()
	defer e.libraryCoverMu.Unlock()
	entry, ok := e.libraryCoverCache[id]
	if !ok || now.After(entry.expiresAt) {
		if ok {
			delete(e.libraryCoverCache, id)
		}
		return "", false
	}
	return entry.primary, true
}

func (e *EmbyService) rememberLibraryCover(id, cover string) {
	if e == nil || strings.TrimSpace(id) == "" || strings.TrimSpace(cover) == "" {
		return
	}
	e.libraryCoverMu.Lock()
	defer e.libraryCoverMu.Unlock()
	if e.libraryCoverCache == nil {
		e.libraryCoverCache = make(map[string]embyArtworkCacheEntry)
	}
	if len(e.libraryCoverCache) > 4000 {
		e.libraryCoverCache = make(map[string]embyArtworkCacheEntry)
	}
	e.libraryCoverCache[id] = embyArtworkCacheEntry{
		primary:   cover,
		expiresAt: time.Now().Add(embyVirtualCacheTTL),
	}
}

// libraryCoverArtwork 解析媒体库封面,与网页媒体库封面逻辑一致:
//  1. 用户手动设置的 cover_url 优先;
//  2. 否则取库内最近的一批成员,按 seriesArtworkScore(同网页 artworkScore)
//     挑出评分最高的那张海报作为封面。
//
// Emby 主图只能显示单张,因此返回的正是网页拼图里最靠前的那张。
func (e *EmbyService) libraryCoverArtwork(ctx context.Context, id string) string {
	if e == nil {
		return ""
	}
	if cached, ok := e.cachedLibraryCover(id); ok {
		return cached
	}
	lib, err := e.repo.Library.FindByID(ctx, id)
	if err != nil || lib == nil {
		return ""
	}
	if strings.TrimSpace(lib.CoverURL) != "" {
		cover := lib.CoverURL
		e.rememberLibraryCover(id, cover)
		return cover
	}
	var cover string
	if rows, _, err := e.repo.Media.ListByLibraryFiltered(ctx, id, 0, 12, repository.MediaQueryFilter{IncludeNSFW: true}); err == nil {
		var bestScore = -1
		for i := range rows {
			score := seriesArtworkScore(rows[i])
			if score <= bestScore {
				continue
			}
			bestScore = score
			cover = rows[i].PosterURL
			if cover == "" {
				cover = rows[i].BackdropURL
			}
		}
	}
	if cover != "" {
		e.rememberLibraryCover(id, cover)
	}
	return cover
}

// LibraryHasCover reports whether a library has an Emby-servable cover, so the
// Views / virtual-folders payloads can advertise ImageTags.Primary to clients.
func (e *EmbyService) LibraryHasCover(ctx context.Context, id string) bool {
	return e.libraryCoverArtwork(ctx, id) != ""
}

func (e *EmbyService) mediaPrimaryArtwork(ctx context.Context, m *model.Media) string {
	if m == nil {
		return ""
	}
	if e.mediaShouldBeEpisode(ctx, m) && strings.TrimSpace(m.BackdropURL) != "" {
		return m.BackdropURL
	}
	return m.PosterURL
}

func (e *EmbyService) mediaBackdropArtwork(ctx context.Context, m *model.Media) string {
	if m == nil {
		return ""
	}
	if e.mediaShouldBeEpisode(ctx, m) {
		return ""
	}
	return m.BackdropURL
}
