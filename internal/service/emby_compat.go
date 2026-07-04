// Package service — Emby/Jellyfin compatibility shim.
//
// EmbyService produces JSON envelopes shaped like the most-consumed
// Emby-API endpoints so existing players (Infuse / Yamby / Hills /
// Senplayer / Kodi NextPVR extension / iOS native clients) can talk to
// MediaStationGo without a custom plugin.
//
// The shim is read-mostly: items, images, playback are fully covered;
// 播放进度上报 / 收藏切换 是写路径但走我们自己的 PlaybackHistory /
// Favorite 表，所以 Emby 客户端的"标记已看 / 收藏"也会反向同步到
// 我们自己的 React UI。
package service

import (
	"context"
	"regexp"
	"sync"
	"time"

	"github.com/ShukeBta/MediaStationGo/internal/config"
	"github.com/ShukeBta/MediaStationGo/internal/repository"
	"github.com/ShukeBta/MediaStationGo/internal/service/cloud"
	"go.uber.org/zap"
)

// 用一个固定的 ServerId 字符串。Emby 客户端会缓存这个 id，第一次见到
// 该 id 后会把所有派生数据（cookie/收藏/历史）和它绑定。
const embyServerID = "mediastation-go-001"

// embyCompatVersion deliberately reports an Emby 4.x server. Official Emby
// clients reject Jellyfin-style 10.x identities as unsupported/too old during
// the login handshake, even when the API shape is compatible enough for us.
const embyCompatVersion = "4.8.10.0"

const (
	embyLocalAuthenticationProviderID = "Emby.Server.Implementations.LocalAuthenticationProvider" // #nosec G101 -- Emby provider identifier, not a credential.
	embyLocalPasswordResetProviderID  = "Emby.Server.Implementations.LocalPasswordResetProvider"  // #nosec G101 -- Emby provider identifier, not a credential.
)

// PlaybackDirectOnlySettingKey 控制「客户端直连解码」模式：开启后宿主机
// 不再提供转码，所有播放交给第三方客户端本地解码（direct play / 302 直链），
// 以释放宿主机 CPU 资源。
const PlaybackDirectOnlySettingKey = "playback.direct_only"

// EmbyService produces Emby-shaped JSON.
type EmbyService struct {
	cfg     *config.Config
	log     *zap.Logger
	repo    *repository.Container
	storage cloudPlaybackResolver
	probe   cloudPlaybackProber
	cache   *RuntimeCacheService

	virtualMu      sync.RWMutex
	virtualSeries  map[string]embySeriesCacheEntry
	virtualSeasons map[string]embySeasonCacheEntry
	virtualArtwork map[string]embyArtworkCacheEntry

	visibilityMu    sync.RWMutex
	visibilityCache map[string]embyVisibilityCacheEntry

	cloudProbeMu       sync.Mutex
	cloudProbeInFlight map[string]struct{}
}

type cloudPlaybackResolver interface {
	CloudResolve(ctx context.Context, typ, fileRef, clientUA string) (*cloud.DirectLink, error)
}

type cloudPlaybackProber interface {
	ProbeHTTP(ctx context.Context, rawURL string, headers map[string]string) (*ProbeResult, error)
}

// NewEmbyService is the constructor.
func NewEmbyService(cfg *config.Config, log *zap.Logger, repo *repository.Container) *EmbyService {
	return &EmbyService{cfg: cfg, log: log, repo: repo}
}

func (e *EmbyService) SetRuntimeCache(cache *RuntimeCacheService) *EmbyService {
	if e != nil {
		e.cache = cache
	}
	return e
}

func (e *EmbyService) SetCloudProbe(storage cloudPlaybackResolver, probe cloudPlaybackProber) {
	if e == nil {
		return
	}
	e.storage = storage
	e.probe = probe
}

// ─── Items ───────────────────────────────────────────────────────────────────

// ItemsParams 是 /Items 与 /Users/{uid}/Items 共用的查询参数。
type ItemsParams struct {
	UserID           string
	ParentID         string
	IDs              []string
	SearchTerm       string
	IncludeItemTypes []string
	Filters          []string
	Recursive        bool
	SortBy           string
	SortOrder        string
	Limit            int
	StartIndex       int
}

const (
	embyVirtualSeriesPrefix = "msgo-series-"
	embyVirtualSeasonPrefix = "msgo-season-"
	embyVirtualCacheTTL     = 10 * time.Minute
	embyVisibilityCacheTTL  = 30 * time.Second
	embySeriesGroupingLimit = maxMediaSearchLimit
)

var (
	embySeasonDirRE    = regexp.MustCompile(`(?i)^(season[\s._-]*\d+|s\d+|specials?|sp|ova|oad|extra|extras|第\s*[0-9一二三四五六七八九十百零两]+\s*季|特别篇|特別篇|番外|特典)$`)
	embyYearSuffixRE   = regexp.MustCompile(`\s*[\(（\[]\d{4}[\)）\]]\s*$`)
	embyEpisodeTitleRE = regexp.MustCompile(`(?i)\s*[-_ ]*s\d{1,2}e\d{1,3}.*$`)
)

type embyVisibilityCacheEntry struct {
	visibility MediaVisibility
	expiresAt  time.Time
}

// Items paginates media in Emby's hierarchy. Episodic libraries are exposed as
// Series -> Season -> Episode so Infuse/Vidhub/SenPlayer stop treating every
// episode as a separate movie card.
func (e *EmbyService) Items(ctx context.Context, p ItemsParams) (map[string]any, error) {
	if p.Limit <= 0 || p.Limit > 500 {
		p.Limit = 50
	}
	if p.StartIndex < 0 {
		p.StartIndex = 0
	}
	if len(p.IncludeItemTypes) > 0 && !containsSupportedEmbyItemType(p.IncludeItemTypes) {
		return emptyItemsEnvelope(p.StartIndex), nil
	}

	if len(p.IDs) > 0 {
		items := make([]map[string]any, 0, len(p.IDs))
		for _, id := range p.IDs {
			item, err := e.Item(ctx, id, p.UserID)
			if err != nil {
				return nil, err
			}
			if item != nil {
				items = append(items, item)
			}
		}
		return map[string]any{"Items": items, "TotalRecordCount": len(items), "StartIndex": 0}, nil
	}

	if containsOnlyFolderItemTypes(p.IncludeItemTypes) {
		if p.ParentID == "" {
			return e.Views(ctx, p.UserID)
		}
		if episodic, err := e.libraryIsEpisodic(ctx, p.ParentID); err != nil {
			return nil, err
		} else if episodic {
			return e.seriesItemsForLibrary(ctx, p.ParentID, p)
		}
		return map[string]any{"Items": []map[string]any{}, "TotalRecordCount": 0, "StartIndex": p.StartIndex}, nil
	}

	if p.ParentID == "" && p.SearchTerm == "" && !p.Recursive && len(p.IncludeItemTypes) == 0 && len(p.Filters) == 0 {
		return e.Views(ctx, p.UserID)
	}

	if season, ok, err := e.findSeasonGroup(ctx, p.ParentID, p.UserID); err != nil {
		return nil, err
	} else if ok {
		return e.episodeItems(ctx, season.Episodes, p)
	}

	if series, ok, err := e.findSeriesGroup(ctx, p.ParentID, p.UserID); err != nil {
		return nil, err
	} else if ok {
		if p.Recursive || containsItemType(p.IncludeItemTypes, "Episode") {
			return e.episodeItems(ctx, series.Episodes, p)
		}
		seasons := e.seasonsForSeries(series)
		items := make([]map[string]any, 0, len(seasons))
		for _, season := range pageSlice(seasons, p.StartIndex, p.Limit) {
			items = append(items, e.seasonPayload(season))
		}
		return map[string]any{"Items": items, "TotalRecordCount": len(seasons), "StartIndex": p.StartIndex}, nil
	}

	if p.ParentID != "" {
		if episodic, err := e.libraryIsEpisodic(ctx, p.ParentID); err != nil {
			return nil, err
		} else if episodic && !p.Recursive && !containsItemType(p.IncludeItemTypes, "Episode") {
			return e.seriesItemsForLibrary(ctx, p.ParentID, p)
		}
	}

	if containsItemType(p.IncludeItemTypes, "Series") && !containsItemType(p.IncludeItemTypes, "Episode") {
		return e.seriesItemsForLibrary(ctx, p.ParentID, p)
	}

	// 电影库的「常规浏览」(未指定 IncludeItemTypes): 电影库里偶尔混入按
	// Season/SxxE 结构整理的内容(如整合成剧集的剧场版 / 合集动画)。这些行若按
	// 散装单集(Episode)漏出,在 Infuse/yamby 等客户端表现为「整部剧被拆成一堆
	// 单集卡片」。方案 B: 把它们按整剧聚成 Series 卡片,与真正的电影(Movie)并列
	// 展示在同一电影库视图。仅在该电影库确实含此类内容时才走此分支,普通电影库
	// 仍走 mediaItems(保留其缓存 / 版本合并逻辑)。
	if p.ParentID != "" && !p.Recursive && len(p.IncludeItemTypes) == 0 {
		if episodic, err := e.libraryIsEpisodic(ctx, p.ParentID); err == nil && !episodic {
			if has, err := e.movieLibraryHasEpisodicContent(ctx, p.ParentID); err == nil && has {
				return e.movieLibraryItems(ctx, p)
			}
		}
	}
	return e.mediaItems(ctx, p)
}
