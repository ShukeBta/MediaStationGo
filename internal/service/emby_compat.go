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
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"net/url"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"sort"
	"strconv"
	"strings"
	"sync"
	"time"

	"go.uber.org/zap"
	"gorm.io/gorm"

	"github.com/ShukeBta/MediaStationGo/internal/config"
	"github.com/ShukeBta/MediaStationGo/internal/model"
	"github.com/ShukeBta/MediaStationGo/internal/repository"
	"github.com/ShukeBta/MediaStationGo/internal/service/cloud"
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

// ─── System ──────────────────────────────────────────────────────────────────

// SystemInfo returns the full Emby identity payload.
func (e *EmbyService) SystemInfo() map[string]any {
	return map[string]any{
		"Id":                     embyServerID,
		"ServerId":               embyServerID,
		"ServerName":             "MediaStationGo",
		"Version":                embyCompatVersion,
		"ServerVersion":          embyCompatVersion,
		"ProductName":            "Emby Server",
		"OperatingSystem":        "Windows",
		"Architecture":           "X64",
		"LocalAddress":           "",
		"WanAddress":             "",
		"HasPendingRestart":      false,
		"IsShuttingDown":         false,
		"SupportsLibraryMonitor": true,
		"SupportsHttps":          false,
		"SupportsAutoDiscovery":  true,
		"HttpServerPortNumber":   e.cfg.App.Port,
		"HttpsPortNumber":        0,
		"PublishedServerUrl":     "",
		"WebSocketPortNumber":    e.cfg.App.Port,
		"CompletedInstallations": []any{},
		"CanSelfRestart":         false,
		"CanLaunchWebBrowser":    false,
		"CanRestart":             false,
	}
}

// SystemInfoPublic 是不需要认证的精简版（Emby Web 客户端登陆前会拉）。
func (e *EmbyService) SystemInfoPublic() map[string]any {
	return map[string]any{
		"Id":                     embyServerID,
		"ServerId":               embyServerID,
		"ServerName":             "MediaStationGo",
		"Version":                embyCompatVersion,
		"ServerVersion":          embyCompatVersion,
		"ProductName":            "Emby Server",
		"OperatingSystem":        "Windows",
		"LocalAddress":           "",
		"WanAddress":             "",
		"HttpServerPortNumber":   e.cfg.App.Port,
		"HttpsPortNumber":        0,
		"SupportsHttps":          false,
		"SupportsAutoDiscovery":  true,
		"StartupWizardCompleted": true,
	}
}

// ─── Users ───────────────────────────────────────────────────────────────────

// ListUsers returns Emby-shaped users.
func (e *EmbyService) ListUsers(ctx context.Context) ([]map[string]any, error) {
	users, err := e.repo.User.List(ctx)
	if err != nil {
		return nil, err
	}
	out := make([]map[string]any, 0, len(users))
	for _, u := range users {
		out = append(out, e.userPayload(&u))
	}
	return out, nil
}

// FindUser 用 ID 查用户，用于 /Users/Me 与 /Users/{id}。
func (e *EmbyService) FindUser(ctx context.Context, id string) (map[string]any, error) {
	u, err := e.repo.User.FindByID(ctx, id)
	if err != nil || u == nil {
		return nil, err
	}
	return e.userPayload(u), nil
}

func (e *EmbyService) userPayload(u *model.User) map[string]any {
	canDownload := u.Role == "admin"
	return map[string]any{
		"Id":                        u.ID,
		"Name":                      u.Username,
		"ServerId":                  embyServerID,
		"ServerName":                "MediaStationGo",
		"HasPassword":               true,
		"HasConfiguredPassword":     true,
		"HasConfiguredEasyPassword": false,
		"EnableAutoLogin":           false,
		"LastLoginDate":             u.LastLoginAt,
		"LastActivityDate":          u.UpdatedAt,
		"Configuration": map[string]any{
			"PlayDefaultAudioTrack":      true,
			"DisplayCollectionsView":     true,
			"DisplayMissingEpisodes":     false,
			"SubtitleMode":               "Default",
			"EnableNextEpisodeAutoPlay":  true,
			"AudioLanguagePreference":    "",
			"SubtitleLanguagePreference": "",
		},
		"Policy": map[string]any{
			"IsAdministrator":                u.Role == "admin",
			"IsHidden":                       false,
			"IsDisabled":                     !u.IsActive,
			"EnableUserPreferenceAccess":     true,
			"EnableRemoteAccess":             true,
			"EnableMediaPlayback":            true,
			"EnableAudioPlaybackTranscoding": true,
			"EnableVideoPlaybackTranscoding": true,
			"EnablePlaybackRemuxing":         true,
			"EnableLiveTvAccess":             false,
			"EnableContentDownloading":       canDownload,
			"EnableSyncTranscoding":          canDownload,
			"EnableMediaConversion":          canDownload,
			"EnableAllChannels":              true,
			"EnableAllFolders":               true,
			"EnableAllDevices":               true,
			"AuthenticationProviderId":       embyLocalAuthenticationProviderID,
			"PasswordResetProviderId":        embyLocalPasswordResetProviderID,
		},
	}
}

// ─── Views / MediaFolders ────────────────────────────────────────────────────

// Views 返回 Emby 中"虚拟根目录"——每个 library 一个条目。
func (e *EmbyService) Views(ctx context.Context, userID string) (map[string]any, error) {
	libs, err := e.repo.Library.List(ctx)
	if err != nil {
		return nil, err
	}
	libs = FilterDisplayCloudLibraries(ctx, e.repo, libs)
	visibility := e.mediaVisibility(ctx, userID)
	items := make([]map[string]any, 0, len(libs))
	for _, l := range libs {
		if !e.libraryVisibleFromCachedVisibility(l, visibility) {
			continue
		}
		items = append(items, e.libraryAsViews(ctx, userID, &l)...)
	}
	return map[string]any{"Items": items, "TotalRecordCount": len(items), "StartIndex": 0}, nil
}

func (e *EmbyService) libraryAsView(ctx context.Context, l *model.Library) map[string]any {
	return e.libraryAsViewWith(ctx, "", l, l.ID, l.Name, e.libraryCollectionType(ctx, l))
}

func (e *EmbyService) libraryAsViews(ctx context.Context, userID string, l *model.Library) []map[string]any {
	if l == nil {
		return nil
	}
	realView := e.libraryAsViewWith(ctx, userID, l, l.ID, l.Name, e.libraryCollectionType(ctx, l))
	if strings.EqualFold(strings.TrimSpace(l.Type), "music") {
		return []map[string]any{realView}
	}
	shape, err := e.libraryMediaShape(ctx, l.ID)
	if err == nil && shape.HasMovies && shape.HasEpisodes {
		return []map[string]any{
			realView,
			e.libraryAsViewWith(ctx, userID, l, virtualLibraryID("movies", l.ID), l.Name+" · 电影", "movies"),
			e.libraryAsViewWith(ctx, userID, l, virtualLibraryID("shows", l.ID), l.Name+" · 剧集", "tvshows"),
		}
	}
	return []map[string]any{realView}
}

func (e *EmbyService) libraryAsViewWith(ctx context.Context, userID string, l *model.Library, id, name, collectionType string) map[string]any {
	counts := e.libraryViewCounts(ctx, userID, l, id, collectionType)
	userData := map[string]any{
		"PlaybackPositionTicks": 0,
		"PlayCount":             0,
		"IsFavorite":            false,
		"Played":                false,
		"UnplayedItemCount":     counts.Unplayed,
	}
	return map[string]any{
		"Id":                       id,
		"Name":                     name,
		"CollectionType":           collectionType,
		"ServerId":                 embyServerID,
		"Type":                     "CollectionFolder",
		"IsFolder":                 true,
		"Path":                     l.Path,
		"SortName":                 strings.ToLower(name),
		"DateCreated":              l.CreatedAt.UTC().Format(time.RFC3339),
		"CanDelete":                false,
		"CanDownload":              false,
		"DisplayPreferencesId":     id,
		"PrimaryImageItemId":       id,
		"PrimaryImageAspectRatio":  1.7777777777777777,
		"RecursiveItemCount":       counts.Recursive,
		"ChildCount":               counts.Child,
		"SpecialFeatureCount":      0,
		"EnableMediaSourceDisplay": true,
		"PlayAccess":               "Full",
		"ExternalUrls":             []any{},
		"ProviderIds":              map[string]string{},
		"Genres":                   []string{},
		"Tags":                     []string{},
		"ImageTags":                map[string]string{},
		"BackdropImageTags":        []string{},
		"UserData":                 userData,
	}
}

type embyLibraryViewCounts struct {
	Recursive int
	Child     int
	Unplayed  int
}

type embyMediaCounts struct {
	MovieCount   int
	SeriesCount  int
	EpisodeCount int
	ItemCount    int
}

func (e *EmbyService) libraryViewCounts(ctx context.Context, userID string, l *model.Library, viewID, collectionType string) embyLibraryViewCounts {
	if e == nil || l == nil {
		return embyLibraryViewCounts{}
	}
	counts, err := e.mediaCountsForParent(ctx, userID, viewID)
	if err != nil {
		return embyLibraryViewCounts{}
	}
	switch strings.ToLower(strings.TrimSpace(collectionType)) {
	case "tvshows":
		return embyLibraryViewCounts{Recursive: counts.EpisodeCount, Child: counts.SeriesCount, Unplayed: counts.EpisodeCount}
	case "movies":
		return embyLibraryViewCounts{Recursive: counts.MovieCount, Child: counts.MovieCount, Unplayed: counts.MovieCount}
	default:
		recursive := counts.MovieCount + counts.EpisodeCount
		child := counts.MovieCount + counts.SeriesCount
		if recursive == 0 {
			recursive = counts.ItemCount
		}
		if child == 0 {
			child = recursive
		}
		return embyLibraryViewCounts{Recursive: recursive, Child: child, Unplayed: recursive}
	}
}

func (e *EmbyService) ItemsCounts(ctx context.Context, userID, parentID string) (map[string]any, error) {
	counts, err := e.mediaCountsForParent(ctx, userID, parentID)
	if err != nil {
		return nil, err
	}
	return map[string]any{
		"MovieCount":   counts.MovieCount,
		"SeriesCount":  counts.SeriesCount,
		"EpisodeCount": counts.EpisodeCount,
		"ItemCount":    counts.ItemCount,
	}, nil
}

func (e *EmbyService) mediaCountsForParent(ctx context.Context, userID, parentID string) (embyMediaCounts, error) {
	if e == nil || e.repo == nil || e.repo.DB == nil {
		return embyMediaCounts{}, nil
	}
	var (
		libraryIDs    []string
		kind          string
		countParentID string
	)
	parentID = strings.TrimSpace(parentID)
	if libraryID, virtualKind, ok := parseVirtualLibraryID(parentID); ok {
		libraryIDs = e.mergedLibraryIDs(ctx, libraryID)
		kind = virtualKind
		countParentID = libraryID
	} else if parentID != "" {
		if lib, err := e.repo.Library.FindByID(ctx, parentID); err != nil {
			return embyMediaCounts{}, err
		} else if lib != nil {
			libraryIDs = e.mergedLibraryIDs(ctx, parentID)
			countParentID = parentID
		} else {
			return embyMediaCounts{}, nil
		}
	}
	counts, err := e.mediaCountsForLibraryIDs(ctx, userID, countParentID, libraryIDs)
	if err != nil {
		return embyMediaCounts{}, err
	}
	return counts.forVirtualKind(kind), nil
}

func (e *EmbyService) mediaCountsForLibraryIDs(ctx context.Context, userID, cacheParentID string, libraryIDs []string) (embyMediaCounts, error) {
	cacheKey := e.embyCountsCacheKey(userID, cacheParentID)
	var cached embyCountsCacheValue
	if e.cache != nil && e.cache.GetJSON(ctx, cacheKey, &cached) {
		return cached.Counts, nil
	}
	base := e.repo.DB.WithContext(ctx).Model(&model.Media{})
	if len(libraryIDs) > 0 {
		base = base.Where("library_id IN ?", libraryIDs)
	}
	base = e.applyUserMediaVisibility(ctx, base, userID)

	var agg struct {
		Total            int64
		SeriesIDEpisodes int64
		SeriesWithID     int64
	}
	if err := base.Session(&gorm.Session{}).Select(`
		COUNT(*) AS total,
		SUM(CASE WHEN series_id IS NOT NULL AND series_id <> '' THEN 1 ELSE 0 END) AS series_id_episodes,
		COUNT(DISTINCT NULLIF(series_id, '')) AS series_with_id
	`).Scan(&agg).Error; err != nil {
		return embyMediaCounts{}, err
	}

	noSeriesRows, err := e.noSeriesEpisodeCandidateRows(ctx, base)
	if err != nil {
		return embyMediaCounts{}, err
	}
	libraryTypes := e.libraryTypesForRows(ctx, noSeriesRows)
	noSeriesEpisodeRows := make([]model.Media, 0, len(noSeriesRows))
	for i := range noSeriesRows {
		if embyMediaShouldBeEpisodeWithLibraryTypes(&noSeriesRows[i], libraryTypes) {
			noSeriesEpisodeRows = append(noSeriesEpisodeRows, noSeriesRows[i])
		}
	}
	episodeCount := int(agg.SeriesIDEpisodes) + len(noSeriesEpisodeRows)
	movieCount := int(agg.Total) - episodeCount
	if movieCount < 0 {
		movieCount = 0
	}
	counts := embyMediaCounts{
		MovieCount:   movieCount,
		SeriesCount:  int(agg.SeriesWithID) + len(e.seriesGroupsFromMedia(noSeriesEpisodeRows)),
		EpisodeCount: episodeCount,
	}
	counts.ItemCount = counts.MovieCount + counts.EpisodeCount
	if e.cache != nil {
		e.cache.SetJSON(ctx, cacheKey, embyCountsCacheValue{Counts: counts}, time.Duration(e.mediaCacheTTLSeconds())*time.Second)
	}
	return counts, nil
}

func (e *EmbyService) noSeriesEpisodeCandidateRows(ctx context.Context, base *gorm.DB) ([]model.Media, error) {
	q := base.Session(&gorm.Session{}).
		Select("id", "library_id", "series_id", "title", "original_name", "path", "season_num", "episode_num", "created_at", "year", "tm_db_id", "bangumi_id").
		Where("(series_id IS NULL OR series_id = '') AND (season_num > 0 OR episode_num > 0)")
	var rows []model.Media
	if err := q.Find(&rows).Error; err != nil {
		return nil, err
	}
	return rows, nil
}

func (c embyMediaCounts) forVirtualKind(kind string) embyMediaCounts {
	switch strings.ToLower(strings.TrimSpace(kind)) {
	case "movies":
		return embyMediaCounts{MovieCount: c.MovieCount, ItemCount: c.MovieCount}
	case "shows", "tvshows", "series":
		return embyMediaCounts{SeriesCount: c.SeriesCount, EpisodeCount: c.EpisodeCount, ItemCount: c.EpisodeCount}
	default:
		return c
	}
}

func (e *EmbyService) libraryTypesForRows(ctx context.Context, rows []model.Media) map[string]string {
	if e == nil || e.repo == nil || e.repo.DB == nil || len(rows) == 0 {
		return nil
	}
	ids := make([]string, 0, len(rows))
	seen := map[string]struct{}{}
	for _, row := range rows {
		id := strings.TrimSpace(row.LibraryID)
		if id == "" {
			continue
		}
		if _, ok := seen[id]; ok {
			continue
		}
		seen[id] = struct{}{}
		ids = append(ids, id)
	}
	if len(ids) == 0 {
		return nil
	}
	var libs []model.Library
	if err := e.repo.DB.WithContext(ctx).Model(&model.Library{}).Select("id", "type").Where("id IN ?", ids).Find(&libs).Error; err != nil {
		return nil
	}
	out := make(map[string]string, len(libs))
	for _, lib := range libs {
		out[lib.ID] = lib.Type
	}
	return out
}

func embyMediaShouldBeEpisodeWithLibraryTypes(m *model.Media, libraryTypes map[string]string) bool {
	if m == nil {
		return false
	}
	if strings.TrimSpace(m.SeriesID) != "" {
		return true
	}
	if m.SeasonNum <= 0 && m.EpisodeNum <= 0 {
		return false
	}
	if embyMediaHasStrongEpisodeSignal(m) {
		return true
	}
	return embyLibraryTypeIsEpisodic(libraryTypes[strings.TrimSpace(m.LibraryID)])
}

type embyLibraryMediaShape struct {
	HasMovies   bool
	HasEpisodes bool
}

func (e *EmbyService) libraryCollectionType(ctx context.Context, l *model.Library) string {
	if l == nil {
		return "movies"
	}
	if strings.EqualFold(strings.TrimSpace(l.Type), "music") {
		return "music"
	}
	shape, err := e.libraryMediaShape(ctx, l.ID)
	if err == nil {
		switch {
		case shape.HasMovies && shape.HasEpisodes:
			return "mixed"
		case shape.HasEpisodes:
			return "tvshows"
		case shape.HasMovies:
			return "movies"
		}
	}
	switch strings.ToLower(strings.TrimSpace(l.Type)) {
	case "tv", "anime", "variety":
		return "tvshows"
	case "music":
		return "music"
	default:
		return "mixed"
	}
}

func (e *EmbyService) libraryMediaShape(ctx context.Context, libraryID string) (embyLibraryMediaShape, error) {
	var shape embyLibraryMediaShape
	if e == nil || e.repo == nil || strings.TrimSpace(libraryID) == "" {
		return shape, nil
	}
	var rows []model.Media
	err := e.repo.DB.WithContext(ctx).
		Model(&model.Media{}).
		Select("id", "library_id", "series_id", "title", "original_name", "path", "season_num", "episode_num").
		Where("library_id IN ?", e.mergedLibraryIDs(ctx, libraryID)).
		Order("media.created_at desc").
		Limit(embySeriesGroupingLimit).
		Find(&rows).Error
	if err != nil {
		return shape, err
	}
	for i := range rows {
		if e.mediaShouldBeEpisode(ctx, &rows[i]) {
			shape.HasEpisodes = true
		} else {
			shape.HasMovies = true
		}
		if shape.HasMovies && shape.HasEpisodes {
			break
		}
	}
	return shape, nil
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
	embyVirtualMoviesPrefix = "msgo-lib-movies-"
	embyVirtualShowsPrefix  = "msgo-lib-shows-"
	embyVirtualCacheTTL     = 10 * time.Minute
	embyVisibilityCacheTTL  = 30 * time.Second
	embyMaxItemsPageSize    = 5000
	embySeriesGroupingLimit = 5000
)

var (
	embySeasonDirRE    = regexp.MustCompile(`(?i)^(season[\s._-]*\d+|s\d+|specials?|sp|ova|oad|extra|extras|第\s*[0-9一二三四五六七八九十百零两]+\s*季|特别篇|特別篇|番外|特典)$`)
	embyYearSuffixRE   = regexp.MustCompile(`\s*[\(（\[]\d{4}[\)）\]]\s*$`)
	embyEpisodeTitleRE = regexp.MustCompile(`(?i)\s*[-_ ]*s\d{1,2}e\d{1,3}.*$`)
	embyStrongSEnERE   = regexp.MustCompile(`(?i)(?:^|[^a-z0-9])s\d{1,2}e\d{1,3}(?:[^a-z0-9]|$)`)
	embyStrongNxERE    = regexp.MustCompile(`(?i)(?:^|[^0-9])\d{1,2}x\d{1,3}(?:[^0-9]|$)`)
	embyStrongEPRE     = regexp.MustCompile(`(?i)(?:^|[^a-z])(?:e|ep)\.?\s*\d{1,3}(?:[^0-9]|$)`)
	embyStrongCNRE     = regexp.MustCompile(`第\s*[0-9一二三四五六七八九十百零两]+\s*[集话話期]`)
	embyDashEpisodeRE  = regexp.MustCompile(`[\s._-][-–—]\s*\d{1,3}(?:\s*(?:v\d+)?)?(?:\s*[\[\(._-]|$)`)
	embyEpisodeHintRE  = regexp.MustCompile(`(?i)(season|episode|episodes|anime|bangumi|番|年番|连载|連載|剧集|劇集|集|话|話|期)`)
)

var embyLocalThumbnailSem = make(chan struct{}, 2)

type embySeriesGroup struct {
	ID          string
	LibraryID   string
	Name        string
	PosterURL   string
	BackdropURL string
	Overview    string
	Rating      float32
	Year        int
	TMDbID      int
	BangumiID   int
	CreatedAt   time.Time
	Episodes    []model.Media
}

type embySeasonGroup struct {
	ID        string
	SeriesID  string
	LibraryID string
	Name      string
	SeasonNum int
	Series    embySeriesGroup
	Episodes  []model.Media
}

type embySeriesCacheEntry struct {
	group     embySeriesGroup
	expiresAt time.Time
}

type embySeasonCacheEntry struct {
	season    embySeasonGroup
	expiresAt time.Time
}

type embyArtworkCacheEntry struct {
	primary   string
	backdrop  string
	expiresAt time.Time
}

type embyVisibilityCacheEntry struct {
	visibility MediaVisibility
	expiresAt  time.Time
}

// Items paginates media in Emby's hierarchy. Episodic libraries are exposed as
// Series -> Season -> Episode so Infuse/Vidhub/SenPlayer stop treating every
// episode as a separate movie card.
func (e *EmbyService) Items(ctx context.Context, p ItemsParams) (map[string]any, error) {
	if p.Limit <= 0 {
		p.Limit = 50
	}
	if p.Limit > embyMaxItemsPageSize {
		p.Limit = embyMaxItemsPageSize
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

	if libraryID, kind, ok := parseVirtualLibraryID(p.ParentID); ok {
		return e.itemsForVirtualLibrary(ctx, libraryID, kind, p)
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
		if shape, err := e.libraryMediaShape(ctx, p.ParentID); err != nil {
			return nil, err
		} else if shape.HasEpisodes && !p.Recursive && !containsItemType(p.IncludeItemTypes, "Episode") {
			if shape.HasMovies && (len(p.IncludeItemTypes) == 0 || containsItemType(p.IncludeItemTypes, "Movie")) {
				return e.libraryTopLevelItems(ctx, p.ParentID, p)
			}
			if !containsItemType(p.IncludeItemTypes, "Movie") {
				return e.seriesItemsForLibrary(ctx, p.ParentID, p)
			}
		}
	}

	if containsItemType(p.IncludeItemTypes, "Series") && !containsItemType(p.IncludeItemTypes, "Episode") {
		return e.seriesItemsForLibrary(ctx, p.ParentID, p)
	}
	if p.ParentID != "" {
		if lib, err := e.repo.Library.FindByID(ctx, p.ParentID); err != nil {
			return nil, err
		} else if lib != nil {
			if containsItemType(p.IncludeItemTypes, "Movie") && !containsItemType(p.IncludeItemTypes, "Episode") && !containsItemType(p.IncludeItemTypes, "Series") {
				return e.movieItemsForLibrary(ctx, p.ParentID, p)
			}
			if containsItemType(p.IncludeItemTypes, "Episode") && !containsItemType(p.IncludeItemTypes, "Movie") {
				return e.episodeItemsForLibrary(ctx, p.ParentID, p)
			}
		}
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

func (e *EmbyService) itemsForVirtualLibrary(ctx context.Context, libraryID, kind string, p ItemsParams) (map[string]any, error) {
	p.ParentID = libraryID
	switch kind {
	case "movies":
		if len(p.IncludeItemTypes) > 0 && !containsItemType(p.IncludeItemTypes, "Movie") && !containsItemType(p.IncludeItemTypes, "Video") {
			return emptyItemsEnvelope(p.StartIndex), nil
		}
		return e.movieItemsForLibrary(ctx, libraryID, p)
	case "shows":
		if len(p.IncludeItemTypes) > 0 &&
			!containsItemType(p.IncludeItemTypes, "Series") &&
			!containsItemType(p.IncludeItemTypes, "Season") &&
			!containsItemType(p.IncludeItemTypes, "Episode") &&
			!containsItemType(p.IncludeItemTypes, "Folder") {
			return emptyItemsEnvelope(p.StartIndex), nil
		}
		if containsItemType(p.IncludeItemTypes, "Episode") && !containsItemType(p.IncludeItemTypes, "Series") {
			return e.episodeItemsForLibrary(ctx, libraryID, p)
		}
		return e.seriesItemsForLibrary(ctx, libraryID, p)
	default:
		return emptyItemsEnvelope(p.StartIndex), nil
	}
}

// movieLibraryHasEpisodicContent 报告电影类型库里是否混入了「剧集结构」内容
// (有季集号且路径形如剧集,例如 .../国产剧/某剧/Season 01/某剧 - S01E01.mkv)。
// 用于决定是否需要走 movieLibraryItems 把这些内容聚成 Series 卡片。普通电影库
// 没有这类行时返回 false,继续走常规 mediaItems。
func (e *EmbyService) movieLibraryHasEpisodicContent(ctx context.Context, libraryID string) (bool, error) {
	clause, args := embyLikelyEpisodicPathSQL()
	if clause == "" {
		return false, nil
	}
	q := e.repo.DB.WithContext(ctx).Model(&model.Media{}).
		Where("library_id IN ?", e.mergedLibraryIDs(ctx, libraryID)).
		Where("(season_num > 0 OR episode_num > 0) AND ("+clause+")", args...)
	var count int64
	if err := q.Limit(1).Count(&count).Error; err != nil {
		return false, err
	}
	return count > 0, nil
}

// movieLibraryItems 处理电影类型库的常规浏览,返回「真正的电影(Movie)」与
// 「库内剧集结构内容聚成的 Series 卡片」的合并列表(按 DateCreated 倒序分页)。
// 与 mediaItems 的区别: 后者会把剧集结构行当散装 Episode 漏出;这里改为聚合成
// Series,从根本上消除「电影库里整部剧被拆成单集」的现象。
func (e *EmbyService) movieLibraryItems(ctx context.Context, p ItemsParams) (map[string]any, error) {
	libIDs := e.mergedLibraryIDs(ctx, p.ParentID)
	apply := func(q *gorm.DB) *gorm.DB {
		q = e.applyUserMediaVisibility(ctx, q, p.UserID)
		q = q.Where("library_id IN ?", libIDs)
		if p.SearchTerm != "" {
			q = q.Where("title LIKE ? OR original_name LIKE ?", "%"+p.SearchTerm+"%", "%"+p.SearchTerm+"%")
		}
		if containsEmbyFilter(p.Filters, "IsFavorite") {
			if strings.TrimSpace(p.UserID) == "" {
				return nil
			}
			q = q.Joins("JOIN favorites ON favorites.media_id = media.id AND favorites.user_id = ? AND favorites.deleted_at IS NULL", p.UserID)
		}
		return q
	}

	// 剧集结构内容 → Series 卡片。
	clause, args := embyLikelyEpisodicPathSQL()
	var episodicRows []model.Media
	if clause != "" {
		epQ := apply(e.repo.DB.WithContext(ctx).Model(&model.Media{}))
		if epQ == nil {
			return map[string]any{"Items": []map[string]any{}, "TotalRecordCount": 0, "StartIndex": p.StartIndex}, nil
		}
		epQ = epQ.Where("(season_num > 0 OR episode_num > 0) AND ("+clause+")", args...).
			Order("media.created_at desc").Limit(embySeriesGroupingLimit)
		if err := epQ.Find(&episodicRows).Error; err != nil {
			return nil, err
		}
	}
	seriesGroups := e.seriesGroupsFromMedia(episodicRows)

	// 真正的电影 → Movie 项(剔除剧集结构行)。
	movieQ := apply(e.repo.DB.WithContext(ctx).Model(&model.Media{}))
	if movieQ == nil {
		return map[string]any{"Items": []map[string]any{}, "TotalRecordCount": 0, "StartIndex": p.StartIndex}, nil
	}
	movieQ = filterLikelyEpisodicPathsFromMovieQuery(movieQ).
		Order("media.created_at desc").Limit(embySeriesGroupingLimit)
	var movieRows []model.Media
	if err := movieQ.Find(&movieRows).Error; err != nil {
		return nil, err
	}
	movieItems, err := e.payloadsForMedia(ctx, movieRows, p.UserID)
	if err != nil {
		return nil, err
	}

	// 合并: Series 卡片 + Movie 项, 统一按 DateCreated 倒序。
	type entry struct {
		createdAt time.Time
		payload   map[string]any
	}
	entries := make([]entry, 0, len(seriesGroups)+len(movieItems))
	for _, g := range seriesGroups {
		entries = append(entries, entry{createdAt: g.CreatedAt, payload: e.seriesPayload(g)})
	}
	for _, item := range movieItems {
		entries = append(entries, entry{createdAt: embyPayloadCreatedAt(item), payload: item})
	}
	sort.SliceStable(entries, func(i, j int) bool {
		return entries[i].createdAt.After(entries[j].createdAt)
	})
	total := len(entries)
	paged := pageSlice(entries, p.StartIndex, p.Limit)
	items := make([]map[string]any, 0, len(paged))
	for _, en := range paged {
		items = append(items, en.payload)
	}
	return map[string]any{"Items": items, "TotalRecordCount": total, "StartIndex": p.StartIndex}, nil
}

// embyPayloadCreatedAt 从 item payload 里取 DateCreated(time.Time),用于合并排序。
func embyPayloadCreatedAt(item map[string]any) time.Time {
	if item == nil {
		return time.Time{}
	}
	if v, ok := item["DateCreated"].(time.Time); ok {
		return v
	}
	return time.Time{}
}

func (e *EmbyService) mediaItems(ctx context.Context, p ItemsParams) (map[string]any, error) {
	cacheKey := e.embyItemsCacheKey("items", p)
	var cached embyItemsCacheValue
	if e.cache != nil && e.cache.GetJSON(ctx, cacheKey, &cached) {
		return map[string]any{"Items": cached.Items, "TotalRecordCount": cached.TotalRecordCount, "StartIndex": cached.StartIndex}, nil
	}
	q := e.repo.DB.WithContext(ctx).Model(&model.Media{})
	q = e.applyUserMediaVisibility(ctx, q, p.UserID)
	if p.ParentID != "" {
		q = q.Where("library_id IN ? OR series_id = ?", e.mergedLibraryIDs(ctx, p.ParentID), p.ParentID)
	}
	if p.SearchTerm != "" {
		q = q.Where("title LIKE ? OR original_name LIKE ?", "%"+p.SearchTerm+"%", "%"+p.SearchTerm+"%")
	}
	if containsEmbyFilter(p.Filters, "IsFavorite") {
		if strings.TrimSpace(p.UserID) == "" {
			return map[string]any{"Items": []map[string]any{}, "TotalRecordCount": int64(0), "StartIndex": p.StartIndex}, nil
		}
		q = q.Joins("JOIN favorites ON favorites.media_id = media.id AND favorites.user_id = ? AND favorites.deleted_at IS NULL", p.UserID)
	}
	resumeFilter := containsEmbyFilter(p.Filters, "IsResumable")
	if resumeFilter {
		if strings.TrimSpace(p.UserID) == "" {
			return map[string]any{"Items": []map[string]any{}, "TotalRecordCount": int64(0), "StartIndex": p.StartIndex}, nil
		}
		q = q.Joins(`JOIN (
			SELECT media_id, MAX(watched_at) AS watched_at
			FROM playback_histories
			WHERE user_id = ? AND completed = ? AND position_ms > 0
			GROUP BY media_id
		) AS resume ON resume.media_id = media.id`, p.UserID, false)
	}
	filterBySeasonNumbers := true
	parentKnownNonEpisodic := false
	if p.ParentID != "" {
		if episodic, err := e.libraryIsEpisodic(ctx, p.ParentID); err == nil && !episodic {
			filterBySeasonNumbers = false
			parentKnownNonEpisodic = true
		}
	}
	if parentKnownNonEpisodic && containsItemType(p.IncludeItemTypes, "Episode") && !containsItemType(p.IncludeItemTypes, "Movie") {
		return emptyItemsEnvelope(p.StartIndex), nil
	}
	if filterBySeasonNumbers && containsItemType(p.IncludeItemTypes, "Movie") && !containsItemType(p.IncludeItemTypes, "Episode") {
		q = e.filterMovieItems(ctx, q)
	}
	if parentKnownNonEpisodic && containsItemType(p.IncludeItemTypes, "Movie") && !containsItemType(p.IncludeItemTypes, "Episode") {
		q = filterLikelyEpisodicPathsFromMovieQuery(q)
	}
	if filterBySeasonNumbers && containsItemType(p.IncludeItemTypes, "Episode") && !containsItemType(p.IncludeItemTypes, "Movie") {
		q = e.filterEpisodeItems(ctx, q)
	}

	var total int64
	if err := q.Count(&total).Error; err != nil {
		return nil, err
	}
	order := "media.created_at desc"
	switch primarySupportedEmbySort(p.SortBy, resumeFilter) {
	case "sortname", "name":
		order = "media.title"
	case "premieredate", "productionyear":
		order = "media.year"
	case "datecreated":
		order = "media.created_at"
	case "dateplayed":
		order = "resume.watched_at"
	case "communityrating":
		order = "media.rating"
	}
	if strings.EqualFold(firstCSVValue(p.SortOrder), "Descending") {
		if !strings.HasSuffix(order, " desc") {
			order = order + " desc"
		}
	}

	fetchLimit := p.Limit
	fetchOffset := p.StartIndex
	if fetchLimit > 0 && e.shouldCollapseMediaVersions(ctx, p) {
		// Duplicates across merged local/cloud libraries collapse into one Emby
		// item with multiple MediaSources. Fetch a wider window so duplicates do
		// not consume the whole requested page.
		fetchOffset = 0
		fetchLimit = p.StartIndex + maxInt(p.Limit*4, p.Limit)
	}
	var rows []model.Media
	if err := q.Order(order).Offset(fetchOffset).Limit(fetchLimit).Find(&rows).Error; err != nil {
		return nil, err
	}
	if e.shouldCollapseMediaVersions(ctx, p) {
		rows = e.collapseMediaVersionRows(ctx, rows)
		rows = pageSlice(rows, p.StartIndex, p.Limit)
	}
	items, err := e.payloadsForMedia(ctx, rows, p.UserID)
	if err != nil {
		return nil, err
	}
	out := map[string]any{"Items": items, "TotalRecordCount": total, "StartIndex": p.StartIndex}
	if e.cache != nil {
		e.cache.SetJSON(ctx, cacheKey, embyItemsCacheValue{Items: items, TotalRecordCount: total, StartIndex: p.StartIndex}, time.Duration(e.mediaCacheTTLSeconds())*time.Second)
	}
	return out, nil
}

type embyItemsCacheValue struct {
	Items            []map[string]any `json:"items"`
	TotalRecordCount int64            `json:"total_record_count"`
	StartIndex       int              `json:"start_index"`
}

type embyLatestCacheValue struct {
	Items []map[string]any `json:"items"`
}

type embyCountsCacheValue struct {
	Counts embyMediaCounts `json:"counts"`
}

func (e *EmbyService) embyItemsCacheKey(kind string, p ItemsParams) string {
	includeTypes := append([]string(nil), p.IncludeItemTypes...)
	filters := append([]string(nil), p.Filters...)
	ids := append([]string(nil), p.IDs...)
	sort.Strings(includeTypes)
	sort.Strings(filters)
	sort.Strings(ids)
	sum := sha256.Sum256([]byte(strings.Join([]string{
		kind,
		p.UserID,
		p.ParentID,
		strings.Join(ids, ","),
		p.SearchTerm,
		strings.Join(includeTypes, ","),
		strings.Join(filters, ","),
		strconv.FormatBool(p.Recursive),
		p.SortBy,
		p.SortOrder,
		strconv.Itoa(p.StartIndex),
		strconv.Itoa(p.Limit),
	}, "|")))
	return "media:emby:" + hex.EncodeToString(sum[:])
}

func (e *EmbyService) embyLatestCacheKey(userID, parentID string, limit int) string {
	sum := sha256.Sum256([]byte(strings.Join([]string{"latest", userID, parentID, strconv.Itoa(limit)}, "|")))
	return "media:emby:" + hex.EncodeToString(sum[:])
}

func (e *EmbyService) embyCountsCacheKey(userID, parentID string) string {
	sum := sha256.Sum256([]byte(strings.Join([]string{"counts", userID, parentID}, "|")))
	return "media:emby:" + hex.EncodeToString(sum[:])
}

func (e *EmbyService) mediaCacheTTLSeconds() int {
	if e == nil || e.cfg == nil || e.cfg.Cache.MediaTTLSeconds < 1 {
		return 15
	}
	return e.cfg.Cache.MediaTTLSeconds
}

func (e *EmbyService) episodeItems(ctx context.Context, rows []model.Media, p ItemsParams) (map[string]any, error) {
	rows = e.filterEpisodeRows(ctx, rows)
	rows = e.filterMediaRowsForUser(ctx, rows, p.UserID)
	if p.SearchTerm != "" {
		filtered := rows[:0]
		needle := strings.ToLower(p.SearchTerm)
		for _, row := range rows {
			if strings.Contains(strings.ToLower(row.Title), needle) || strings.Contains(strings.ToLower(row.OriginalName), needle) {
				filtered = append(filtered, row)
			}
		}
		rows = filtered
	}
	sort.SliceStable(rows, func(i, j int) bool {
		if rows[i].SeasonNum != rows[j].SeasonNum {
			return rows[i].SeasonNum < rows[j].SeasonNum
		}
		if rows[i].EpisodeNum != rows[j].EpisodeNum {
			return rows[i].EpisodeNum < rows[j].EpisodeNum
		}
		return rows[i].CreatedAt.Before(rows[j].CreatedAt)
	})
	total := len(rows)
	items, err := e.payloadsForMedia(ctx, pageSlice(rows, p.StartIndex, p.Limit), p.UserID)
	if err != nil {
		return nil, err
	}
	return map[string]any{"Items": items, "TotalRecordCount": total, "StartIndex": p.StartIndex}, nil
}

type embyTopLevelEntry struct {
	Payload   map[string]any
	Name      string
	CreatedAt time.Time
	Year      int
	Rating    float32
}

func (e *EmbyService) libraryTopLevelItems(ctx context.Context, libraryID string, p ItemsParams) (map[string]any, error) {
	includeMovies := len(p.IncludeItemTypes) == 0 || containsItemType(p.IncludeItemTypes, "Movie")
	includeSeries := len(p.IncludeItemTypes) == 0 || containsItemType(p.IncludeItemTypes, "Series")
	if !includeMovies && !includeSeries {
		return emptyItemsEnvelope(p.StartIndex), nil
	}

	rowLimit := p.StartIndex + maxInt(p.Limit*40, 1000)
	if rowLimit < p.Limit {
		rowLimit = p.Limit
	}
	if rowLimit > embySeriesGroupingLimit {
		rowLimit = embySeriesGroupingLimit
	}

	q := e.repo.DB.WithContext(ctx).Model(&model.Media{}).Where("library_id IN ?", e.mergedLibraryIDs(ctx, libraryID))
	q = e.applyUserMediaVisibility(ctx, q, p.UserID)
	if p.SearchTerm != "" {
		q = q.Where("title LIKE ? OR original_name LIKE ?", "%"+p.SearchTerm+"%", "%"+p.SearchTerm+"%")
	}
	if containsEmbyFilter(p.Filters, "IsFavorite") {
		if strings.TrimSpace(p.UserID) == "" {
			return emptyItemsEnvelope(p.StartIndex), nil
		}
		q = q.Joins("JOIN favorites ON favorites.media_id = media.id AND favorites.user_id = ? AND favorites.deleted_at IS NULL", p.UserID)
	}

	var rows []model.Media
	if err := q.Order("media.created_at desc").Limit(rowLimit).Find(&rows).Error; err != nil {
		return nil, err
	}

	movieRows := make([]model.Media, 0, len(rows))
	episodeRows := make([]model.Media, 0, len(rows))
	for i := range rows {
		if e.mediaShouldBeEpisode(ctx, &rows[i]) {
			episodeRows = append(episodeRows, rows[i])
		} else {
			movieRows = append(movieRows, rows[i])
		}
	}

	entries := make([]embyTopLevelEntry, 0, len(movieRows)+len(episodeRows))
	if includeMovies && len(movieRows) > 0 {
		payloads, err := e.payloadsForMedia(ctx, movieRows, p.UserID)
		if err != nil {
			return nil, err
		}
		for i, payload := range payloads {
			row := movieRows[i]
			entries = append(entries, embyTopLevelEntry{
				Payload:   payload,
				Name:      row.Title,
				CreatedAt: row.CreatedAt,
				Year:      row.Year,
				Rating:    row.Rating,
			})
		}
	}
	if includeSeries && len(episodeRows) > 0 {
		groups := e.seriesGroupsFromMedia(episodeRows)
		for _, group := range groups {
			entries = append(entries, embyTopLevelEntry{
				Payload:   e.seriesPayload(group),
				Name:      group.Name,
				CreatedAt: group.CreatedAt,
				Year:      group.Year,
				Rating:    group.Rating,
			})
		}
	}

	sortTopLevelEntries(entries, p)
	total := len(entries)
	paged := pageSlice(entries, p.StartIndex, p.Limit)
	items := make([]map[string]any, 0, len(paged))
	for _, entry := range paged {
		items = append(items, entry.Payload)
	}
	return map[string]any{"Items": items, "TotalRecordCount": total, "StartIndex": p.StartIndex}, nil
}

func (e *EmbyService) movieItemsForLibrary(ctx context.Context, libraryID string, p ItemsParams) (map[string]any, error) {
	rows, err := e.mediaRowsForLibraryAutoClassification(ctx, libraryID, p)
	if err != nil {
		return nil, err
	}
	movieRows := make([]model.Media, 0, len(rows))
	for i := range rows {
		if !e.mediaShouldBeEpisode(ctx, &rows[i]) {
			movieRows = append(movieRows, rows[i])
		}
	}
	total := len(movieRows)
	items, err := e.payloadsForMedia(ctx, pageSlice(movieRows, p.StartIndex, p.Limit), p.UserID)
	if err != nil {
		return nil, err
	}
	return map[string]any{"Items": items, "TotalRecordCount": total, "StartIndex": p.StartIndex}, nil
}

func (e *EmbyService) episodeItemsForLibrary(ctx context.Context, libraryID string, p ItemsParams) (map[string]any, error) {
	rows, err := e.mediaRowsForLibraryAutoClassification(ctx, libraryID, p)
	if err != nil {
		return nil, err
	}
	return e.episodeItems(ctx, rows, p)
}

func (e *EmbyService) mediaRowsForLibraryAutoClassification(ctx context.Context, libraryID string, p ItemsParams) ([]model.Media, error) {
	q := e.repo.DB.WithContext(ctx).Model(&model.Media{}).Where("library_id IN ?", e.mergedLibraryIDs(ctx, libraryID))
	q = e.applyUserMediaVisibility(ctx, q, p.UserID)
	if p.SearchTerm != "" {
		q = q.Where("title LIKE ? OR original_name LIKE ?", "%"+p.SearchTerm+"%", "%"+p.SearchTerm+"%")
	}
	if containsEmbyFilter(p.Filters, "IsFavorite") {
		if strings.TrimSpace(p.UserID) == "" {
			return []model.Media{}, nil
		}
		q = q.Joins("JOIN favorites ON favorites.media_id = media.id AND favorites.user_id = ? AND favorites.deleted_at IS NULL", p.UserID)
	}
	resumeFilter := containsEmbyFilter(p.Filters, "IsResumable")
	if resumeFilter {
		if strings.TrimSpace(p.UserID) == "" {
			return []model.Media{}, nil
		}
		q = q.Joins(`JOIN (
			SELECT media_id, MAX(watched_at) AS watched_at
			FROM playback_histories
			WHERE user_id = ? AND completed = ? AND position_ms > 0
			GROUP BY media_id
		) AS resume ON resume.media_id = media.id`, p.UserID, false)
	}
	var rows []model.Media
	if err := q.Order(embyMediaOrderClause(p.SortBy, p.SortOrder, resumeFilter)).Limit(embySeriesGroupingLimit).Find(&rows).Error; err != nil {
		return nil, err
	}
	return rows, nil
}

func embyMediaOrderClause(sortBy, sortOrder string, resumeFilter bool) string {
	order := "media.created_at desc"
	switch primarySupportedEmbySort(sortBy, resumeFilter) {
	case "sortname", "name":
		order = "media.title"
	case "premieredate", "productionyear":
		order = "media.year"
	case "datecreated":
		order = "media.created_at"
	case "dateplayed":
		order = "resume.watched_at"
	case "communityrating":
		order = "media.rating"
	}
	if strings.EqualFold(firstCSVValue(sortOrder), "Descending") {
		if !strings.HasSuffix(order, " desc") {
			order += " desc"
		}
	}
	return order
}

func sortTopLevelEntries(entries []embyTopLevelEntry, p ItemsParams) {
	sortBy := primarySupportedEmbySort(p.SortBy, false)
	desc := strings.EqualFold(firstCSVValue(p.SortOrder), "Descending")
	if sortBy == "" {
		sortBy = "datecreated"
		desc = true
	}
	sort.SliceStable(entries, func(i, j int) bool {
		switch sortBy {
		case "sortname", "name":
			less := strings.ToLower(entries[i].Name) < strings.ToLower(entries[j].Name)
			if desc {
				return !less && strings.ToLower(entries[i].Name) != strings.ToLower(entries[j].Name)
			}
			return less
		case "premieredate", "productionyear":
			if entries[i].Year != entries[j].Year {
				if desc {
					return entries[i].Year > entries[j].Year
				}
				return entries[i].Year < entries[j].Year
			}
		case "communityrating":
			if entries[i].Rating != entries[j].Rating {
				if desc {
					return entries[i].Rating > entries[j].Rating
				}
				return entries[i].Rating < entries[j].Rating
			}
		}
		if desc {
			return entries[i].CreatedAt.After(entries[j].CreatedAt)
		}
		return entries[i].CreatedAt.Before(entries[j].CreatedAt)
	})
}

func (e *EmbyService) filterEpisodeRows(ctx context.Context, rows []model.Media) []model.Media {
	if len(rows) == 0 {
		return rows
	}
	out := rows[:0]
	for i := range rows {
		if e.mediaShouldBeEpisode(ctx, &rows[i]) {
			out = append(out, rows[i])
		}
	}
	return out
}

func (e *EmbyService) payloadsForMedia(ctx context.Context, rows []model.Media, userID string) ([]map[string]any, error) {
	rows = e.collapseMediaVersionRows(ctx, rows)
	userFavs := map[string]bool{}
	userPos := map[string]int64{}
	if userID != "" && len(rows) > 0 {
		mediaIDs := make([]string, 0, len(rows))
		for _, row := range rows {
			if strings.TrimSpace(row.ID) != "" {
				mediaIDs = append(mediaIDs, row.ID)
			}
		}
		if len(mediaIDs) == 0 {
			mediaIDs = []string{"__none__"}
		}
		var favs []model.Favorite
		favQuery := e.repo.DB.WithContext(ctx).Where("user_id = ?", userID).Where("media_id IN ?", mediaIDs)
		_ = favQuery.Find(&favs).Error
		for _, f := range favs {
			userFavs[f.MediaID] = true
		}
		var hist []model.PlaybackHistory
		histQuery := e.repo.DB.WithContext(ctx).Where("user_id = ?", userID).Where("media_id IN ?", mediaIDs)
		_ = histQuery.Find(&hist).Error
		for _, h := range hist {
			userPos[h.MediaID] = h.PositionMs
		}
	}

	items := make([]map[string]any, 0, len(rows))
	for _, m := range rows {
		items = append(items, e.itemPayload(ctx, &m, userFavs[m.ID], userPos[m.ID]))
	}
	return items, nil
}

func (e *EmbyService) shouldCollapseMediaVersions(ctx context.Context, p ItemsParams) bool {
	if containsItemType(p.IncludeItemTypes, "Series") || containsItemType(p.IncludeItemTypes, "Season") {
		return false
	}
	if containsItemType(p.IncludeItemTypes, "Episode") && !containsItemType(p.IncludeItemTypes, "Movie") {
		return true
	}
	if p.ParentID == "" {
		return true
	}
	episodic, err := e.libraryIsEpisodic(ctx, p.ParentID)
	return err == nil && !episodic
}

func (e *EmbyService) collapseMediaVersionRows(ctx context.Context, rows []model.Media) []model.Media {
	if len(rows) < 2 {
		return rows
	}
	out := make([]model.Media, 0, len(rows))
	indexByKey := make(map[string]int, len(rows))
	for _, row := range rows {
		key := e.mediaVersionKey(ctx, &row)
		if key == "" {
			out = append(out, row)
			continue
		}
		if idx, ok := indexByKey[key]; ok {
			if preferMediaVersion(row, out[idx]) {
				out[idx] = row
			}
			continue
		}
		indexByKey[key] = len(out)
		out = append(out, row)
	}
	return out
}

// Item 单条目详情。
func (e *EmbyService) Item(ctx context.Context, mediaID, userID string) (map[string]any, error) {
	if libraryID, kind, ok := parseVirtualLibraryID(mediaID); ok {
		lib, err := e.repo.Library.FindByID(ctx, libraryID)
		if err != nil || lib == nil {
			return nil, err
		}
		libs := FilterDisplayCloudLibraries(ctx, e.repo, []model.Library{*lib})
		if len(libs) == 0 {
			return nil, nil
		}
		visibility := e.mediaVisibility(ctx, userID)
		if !e.libraryVisibleFromCachedVisibility(libs[0], visibility) {
			return nil, nil
		}
		if kind == "shows" {
			return e.libraryAsViewWith(ctx, userID, &libs[0], mediaID, libs[0].Name+" · 剧集", "tvshows"), nil
		}
		return e.libraryAsViewWith(ctx, userID, &libs[0], mediaID, libs[0].Name+" · 电影", "movies"), nil
	}
	if lib, err := e.repo.Library.FindByID(ctx, mediaID); err != nil {
		return nil, err
	} else if lib != nil {
		libs := FilterDisplayCloudLibraries(ctx, e.repo, []model.Library{*lib})
		if len(libs) == 0 {
			return nil, nil
		}
		visibility := e.mediaVisibility(ctx, userID)
		if !e.libraryVisibleFromCachedVisibility(libs[0], visibility) {
			return nil, nil
		}
		return e.libraryAsViewWith(ctx, userID, &libs[0], libs[0].ID, libs[0].Name, e.libraryCollectionType(ctx, &libs[0])), nil
	}
	if strings.HasPrefix(mediaID, embyVirtualSeasonPrefix) {
		if season, ok, err := e.findSeasonGroup(ctx, mediaID, userID); err != nil {
			return nil, err
		} else if ok {
			return e.seasonPayload(season), nil
		}
	}
	if strings.HasPrefix(mediaID, embyVirtualSeriesPrefix) {
		if series, ok, err := e.findSeriesGroup(ctx, mediaID, userID); err != nil {
			return nil, err
		} else if ok {
			return e.seriesPayload(series), nil
		}
	}
	m, err := e.repo.Media.FindByID(ctx, mediaID)
	if err != nil {
		return nil, err
	}
	if m == nil {
		if series, ok, err := e.findSeriesGroup(ctx, mediaID, userID); err != nil {
			return nil, err
		} else if ok {
			return e.seriesPayload(series), nil
		}
		return nil, nil
	}
	if !UserDefaultMediaVisibility(ctx, e.repo, userID).Allows(m) {
		return nil, nil
	}
	fav := false
	pos := int64(0)
	if userID != "" {
		var f model.Favorite
		ferr := e.repo.DB.WithContext(ctx).Where("user_id = ? AND media_id = ?", userID, mediaID).First(&f).Error
		if ferr == nil {
			fav = true
		}
		var h model.PlaybackHistory
		herr := e.repo.DB.WithContext(ctx).Where("user_id = ? AND media_id = ?", userID, mediaID).
			Order("watched_at desc").First(&h).Error
		if herr == nil {
			pos = h.PositionMs
		}
	}
	return e.itemPayload(ctx, m, fav, pos), nil
}

// LatestItems 最近添加，全库或指定库。
func (e *EmbyService) LatestItems(ctx context.Context, userID, parentID string, limit int) ([]map[string]any, error) {
	if limit <= 0 || limit > 100 {
		limit = 20
	}
	cacheKey := e.embyLatestCacheKey(userID, parentID, limit)
	var cached embyLatestCacheValue
	if e.cache != nil && e.cache.GetJSON(ctx, cacheKey, &cached) {
		return cached.Items, nil
	}
	if libraryID, kind, ok := parseVirtualLibraryID(parentID); ok {
		var (
			out []map[string]any
			err error
		)
		switch kind {
		case "movies":
			out, err = e.latestMovieItemsForLibrary(ctx, userID, libraryID, limit)
		case "shows":
			out, err = e.latestSeriesItemsForLibrary(ctx, userID, libraryID, limit)
		default:
			out, err = nil, nil
		}
		if err == nil && e.cache != nil {
			e.cache.SetJSON(ctx, cacheKey, embyLatestCacheValue{Items: out}, time.Duration(e.mediaCacheTTLSeconds())*time.Second)
		}
		return out, err
	}
	q := e.repo.DB.WithContext(ctx).Model(&model.Media{}).Where("deleted_at IS NULL")
	q = e.applyUserMediaVisibility(ctx, q, userID)
	if parentID != "" {
		if episodic, err := e.libraryIsEpisodic(ctx, parentID); err == nil && episodic {
			out, err := e.latestSeriesItemsForLibrary(ctx, userID, parentID, limit)
			if err == nil && e.cache != nil {
				e.cache.SetJSON(ctx, cacheKey, embyLatestCacheValue{Items: out}, time.Duration(e.mediaCacheTTLSeconds())*time.Second)
			}
			return out, err
		}
		q = q.Where("library_id IN ?", e.mergedLibraryIDs(ctx, parentID))
	}
	var rows []model.Media
	if err := q.Order("media.created_at desc").Limit(limit).Find(&rows).Error; err != nil {
		return nil, err
	}
	favs := map[string]bool{}
	if userID != "" && len(rows) > 0 {
		mediaIDs := make([]string, 0, len(rows))
		for _, row := range rows {
			if strings.TrimSpace(row.ID) != "" {
				mediaIDs = append(mediaIDs, row.ID)
			}
		}
		if len(mediaIDs) == 0 {
			mediaIDs = []string{"__none__"}
		}
		var fr []model.Favorite
		_ = e.repo.DB.WithContext(ctx).Where("user_id = ? AND media_id IN ?", userID, mediaIDs).Find(&fr).Error
		for _, f := range fr {
			favs[f.MediaID] = true
		}
	}
	out := make([]map[string]any, 0, len(rows))
	for _, m := range rows {
		out = append(out, e.itemPayload(ctx, &m, favs[m.ID], 0))
	}
	if e.cache != nil {
		e.cache.SetJSON(ctx, cacheKey, embyLatestCacheValue{Items: out}, time.Duration(e.mediaCacheTTLSeconds())*time.Second)
	}
	return out, nil
}

func (e *EmbyService) latestMovieItemsForLibrary(ctx context.Context, userID, libraryID string, limit int) ([]map[string]any, error) {
	if limit <= 0 || limit > 100 {
		limit = 20
	}
	rowLimit := limit * 8
	if rowLimit < 100 {
		rowLimit = 100
	}
	if rowLimit > embySeriesGroupingLimit {
		rowLimit = embySeriesGroupingLimit
	}
	q := e.repo.DB.WithContext(ctx).Model(&model.Media{}).
		Where("library_id IN ?", e.mergedLibraryIDs(ctx, libraryID))
	q = e.applyUserMediaVisibility(ctx, q, userID)
	var rows []model.Media
	if err := q.Order("media.created_at desc").Limit(rowLimit).Find(&rows).Error; err != nil {
		return nil, err
	}
	movieRows := make([]model.Media, 0, minInt(len(rows), limit))
	for i := range rows {
		if e.mediaShouldBeEpisode(ctx, &rows[i]) {
			continue
		}
		movieRows = append(movieRows, rows[i])
		if len(movieRows) >= limit {
			break
		}
	}
	return e.payloadsForMedia(ctx, movieRows, userID)
}

func (e *EmbyService) latestSeriesItemsForLibrary(ctx context.Context, userID, libraryID string, limit int) ([]map[string]any, error) {
	if limit <= 0 || limit > 100 {
		limit = 20
	}
	rowLimit := limit * 40
	if rowLimit < 200 {
		rowLimit = 200
	}
	if rowLimit > embySeriesGroupingLimit {
		rowLimit = embySeriesGroupingLimit
	}
	q := e.repo.DB.WithContext(ctx).Model(&model.Media{}).
		Where("library_id IN ? AND (season_num > 0 OR episode_num > 0)", e.mergedLibraryIDs(ctx, libraryID))
	q = e.applyUserMediaVisibility(ctx, q, userID)
	var rows []model.Media
	if err := q.Order("media.created_at desc").Limit(rowLimit).Find(&rows).Error; err != nil {
		return nil, err
	}
	rows = e.filterEpisodeRows(ctx, rows)
	groups := e.seriesGroupsFromMedia(rows)
	sortSeriesGroups(groups, ItemsParams{SortBy: "datecreated", SortOrder: "Descending"})
	if len(groups) > limit {
		groups = groups[:limit]
	}
	items := make([]map[string]any, 0, len(groups))
	for _, group := range groups {
		items = append(items, e.seriesPayload(group))
	}
	return items, nil
}

// ResumeItems 列出有未完成播放进度的媒体。
func (e *EmbyService) ResumeItems(ctx context.Context, userID string, limit int) (map[string]any, error) {
	if limit <= 0 || limit > 100 {
		limit = 20
	}
	type row struct {
		MediaID    string
		PositionMs int64
		DurationMs int64
	}
	var hist []model.PlaybackHistory
	if err := e.repo.DB.WithContext(ctx).
		Where("user_id = ? AND completed = ? AND position_ms > 0", userID, false).
		Order("watched_at desc").Limit(limit).Find(&hist).Error; err != nil {
		return nil, err
	}
	if len(hist) == 0 {
		return map[string]any{"Items": []any{}, "TotalRecordCount": 0}, nil
	}
	ids := make([]string, 0, len(hist))
	posByID := map[string]int64{}
	for _, h := range hist {
		ids = append(ids, h.MediaID)
		posByID[h.MediaID] = h.PositionMs
	}
	var medias []model.Media
	q := e.repo.DB.WithContext(ctx).Where("id IN ?", ids)
	q = e.applyUserMediaVisibility(ctx, q, userID)
	if err := q.Find(&medias).Error; err != nil {
		return nil, err
	}
	// 维持时间倒序
	byID := map[string]*model.Media{}
	for i := range medias {
		byID[medias[i].ID] = &medias[i]
	}
	items := make([]map[string]any, 0, len(hist))
	for _, h := range hist {
		if m, ok := byID[h.MediaID]; ok {
			items = append(items, e.itemPayload(ctx, m, false, posByID[h.MediaID]))
		}
	}
	return map[string]any{"Items": items, "TotalRecordCount": len(items)}, nil
}

func (e *EmbyService) itemPayload(ctx context.Context, m *model.Media, fav bool, posMs int64) map[string]any {
	itemType := "Movie"
	name := m.Title
	parentID := m.LibraryID
	seriesID := m.SeriesID
	seriesName := ""
	seasonID := ""
	if e.mediaShouldBeEpisode(ctx, m) {
		itemType = "Episode"
		seriesID = e.seriesIDForMedia(m)
		seriesName = e.seriesNameForMedia(m)
		seasonID = e.seasonIDForMedia(m)
		parentID = seasonID
		originalName := strings.TrimSpace(m.OriginalName)
		if originalName != "" && !strings.EqualFold(originalName, seriesName) && !strings.EqualFold(originalName, m.Title) {
			name = m.OriginalName
		} else if m.EpisodeNum > 0 {
			name = fmt.Sprintf("第 %d 集", m.EpisodeNum)
		}
	}
	imageTags := map[string]string{}
	backdropTags := []string{}
	primaryArtwork := e.mediaPrimaryArtwork(ctx, m)
	backdropArtwork := e.mediaBackdropArtwork(ctx, m)
	if primaryArtwork != "" || e.mediaCanAdvertiseLocalThumbnail(m) {
		imageTags["Primary"] = m.ID
	}
	if backdropArtwork != "" {
		backdropTags = append(backdropTags, m.ID+"-bd")
	}
	container := embyPlaybackContainer(m.Container, m.Path)
	itemPath := m.Path
	if playURL := e.mediaPlayURL(ctx, m, container); playURL != "" {
		itemPath = playURL
	}

	runTimeTicks := int64(m.DurationSec) * 10_000_000
	durationMs := int64(m.DurationSec) * 1000
	sources := e.mediaSourcesForItem(ctx, m, true, false)
	played := posMs > 0 && durationMs > 0 && posMs >= durationMs*9/10
	pct := 0.0
	if durationMs > 0 {
		pct = float64(posMs) / float64(durationMs) * 100
	}

	item := map[string]any{
		"Id":                       m.ID,
		"Name":                     name,
		"OriginalTitle":            m.OriginalName,
		"SortName":                 strings.ToLower(name),
		"ServerId":                 embyServerID,
		"Type":                     itemType,
		"MediaType":                "Video",
		"VideoType":                "VideoFile",
		"IsFolder":                 false,
		"LocationType":             "FileSystem",
		"PlayAccess":               "Full",
		"CanPlay":                  true,
		"CanDownload":              true,
		"SupportsSync":             true,
		"EnableMediaSourceDisplay": true,
		"DisplayPreferencesId":     m.ID,
		"PresentationUniqueKey":    m.ID,
		"ProductionYear":           m.Year,
		"ParentIndexNumber":        m.SeasonNum,
		"IndexNumber":              m.EpisodeNum,
		"Overview":                 m.Overview,
		"RunTimeTicks":             runTimeTicks,
		"CommunityRating":          m.Rating,
		"OfficialRating":           "",
		"Container":                container,
		"Size":                     m.SizeBytes,
		"Width":                    m.Width,
		"Height":                   m.Height,
		"DateCreated":              m.CreatedAt,
		"Path":                     itemPath,
		"FileName":                 filepath.Base(m.Path),
		"ParentId":                 parentID,
		"SeasonId":                 seasonID,
		"SeasonName":               seasonName(m.SeasonNum),
		"SeriesId":                 seriesID,
		"SeriesName":               seriesName,
		"ImageTags":                imageTags,
		"BackdropImageTags":        backdropTags,
		"Genres":                   splitCSV(m.Genres),
		"People":                   []any{},
		"Studios":                  []any{},
		"Taglines":                 []string{},
		"Tags":                     []string{},
		"ExternalUrls":             []any{},
		"RemoteTrailers":           []any{},
		"LocalTrailerCount":        0,
		"SpecialFeatureCount":      0,
		"Chapters":                 []any{},
		"ProviderIds": map[string]string{
			"Tmdb":    intToStr(m.TMDbID),
			"Bangumi": intToStr(m.BangumiID),
		},
		"MediaSourceCount": len(sources),
		"UserData": map[string]any{
			"PlaybackPositionTicks": posMs * 10_000,
			"PlayCount":             0,
			"IsFavorite":            fav,
			"Played":                played,
			"PlayedPercentage":      pct,
		},
		"MediaSources": sources,
	}
	if itemType == "Movie" {
		delete(item, "ParentIndexNumber")
		delete(item, "IndexNumber")
		delete(item, "SeasonId")
		delete(item, "SeasonName")
		delete(item, "SeriesId")
		delete(item, "SeriesName")
	}
	return item
}

func (e *EmbyService) seriesItemsForLibrary(ctx context.Context, libraryID string, p ItemsParams) (map[string]any, error) {
	q := e.repo.DB.WithContext(ctx).Model(&model.Media{}).Where("season_num > 0 OR episode_num > 0")
	q = e.applyUserMediaVisibility(ctx, q, p.UserID)
	if libraryID != "" {
		q = q.Where("library_id IN ?", e.mergedLibraryIDs(ctx, libraryID))
	}
	if p.SearchTerm != "" {
		q = q.Where("title LIKE ? OR original_name LIKE ?", "%"+p.SearchTerm+"%", "%"+p.SearchTerm+"%")
	}
	if containsEmbyFilter(p.Filters, "IsFavorite") {
		if strings.TrimSpace(p.UserID) == "" {
			return map[string]any{"Items": []map[string]any{}, "TotalRecordCount": 0, "StartIndex": p.StartIndex}, nil
		}
		q = q.Joins("JOIN favorites ON favorites.media_id = media.id AND favorites.user_id = ? AND favorites.deleted_at IS NULL", p.UserID)
	}
	rowLimit := p.StartIndex + maxInt(p.Limit*40, 1000)
	if rowLimit < p.Limit {
		rowLimit = p.Limit
	}
	if rowLimit > embySeriesGroupingLimit {
		rowLimit = embySeriesGroupingLimit
	}
	var rows []model.Media
	if err := q.Order("media.created_at desc").Limit(rowLimit).Find(&rows).Error; err != nil {
		return nil, err
	}
	rows = e.filterEpisodeRows(ctx, rows)
	groups := e.seriesGroupsFromMedia(rows)
	sortSeriesGroups(groups, p)
	total := len(groups)
	items := make([]map[string]any, 0, minInt(p.Limit, len(groups)))
	for _, group := range pageSlice(groups, p.StartIndex, p.Limit) {
		items = append(items, e.seriesPayload(group))
	}
	return map[string]any{"Items": items, "TotalRecordCount": total, "StartIndex": p.StartIndex}, nil
}

func (e *EmbyService) libraryIsEpisodic(ctx context.Context, libraryID string) (bool, error) {
	if strings.TrimSpace(libraryID) == "" {
		return false, nil
	}
	shape, err := e.libraryMediaShape(ctx, libraryID)
	return shape.HasEpisodes, err
}

func (e *EmbyService) mediaShouldBeEpisode(ctx context.Context, m *model.Media) bool {
	if m == nil {
		return false
	}
	if strings.TrimSpace(m.SeriesID) != "" {
		return true
	}
	if m.SeasonNum <= 0 && m.EpisodeNum <= 0 {
		return false
	}
	if embyMediaHasStrongEpisodeSignal(m) {
		return true
	}
	if e == nil || e.repo == nil || strings.TrimSpace(m.LibraryID) == "" {
		return false
	}
	lib, err := e.repo.Library.FindByID(ctx, m.LibraryID)
	return err == nil && lib != nil && embyLibraryTypeIsEpisodic(lib.Type)
}

func embyMediaHasStrongEpisodeSignal(m *model.Media) bool {
	if m == nil {
		return false
	}
	values := []string{m.Path, m.Title, m.OriginalName}
	for _, value := range values {
		if embyTextHasExplicitEpisodeSignal(value) {
			return true
		}
	}
	joined := strings.Join(values, " ")
	if embyDashEpisodeRE.MatchString(joined) && embyEpisodeHintRE.MatchString(joined) {
		return true
	}
	return false
}

func embyTextHasExplicitEpisodeSignal(value string) bool {
	value = strings.TrimSpace(value)
	if value == "" {
		return false
	}
	if embyStrongSEnERE.MatchString(value) || embyStrongNxERE.MatchString(value) || embyStrongEPRE.MatchString(value) || embyStrongCNRE.MatchString(value) {
		return true
	}
	if _, ok := seasonFromParents(value); ok {
		return true
	}
	return false
}

func embyLibraryTypeIsEpisodic(typ string) bool {
	switch strings.ToLower(strings.TrimSpace(typ)) {
	case "tv", "anime", "variety":
		return true
	default:
		return false
	}
}

func (e *EmbyService) filterMovieItems(ctx context.Context, q *gorm.DB) *gorm.DB {
	episodicIDs := e.episodicLibraryIDs(ctx)
	if len(episodicIDs) == 0 {
		return filterLikelyEpisodicPathsFromMovieQuery(q)
	}
	q = q.Where("(media.season_num = 0 AND media.episode_num = 0) OR media.library_id NOT IN ?", episodicIDs)
	return filterLikelyEpisodicPathsFromMovieQuery(q)
}

func (e *EmbyService) filterEpisodeItems(ctx context.Context, q *gorm.DB) *gorm.DB {
	episodicIDs := e.episodicLibraryIDs(ctx)
	if len(episodicIDs) == 0 {
		return q.Where("1 = 0")
	}
	return q.Where("media.library_id IN ? AND (media.season_num > 0 OR media.episode_num > 0)", episodicIDs)
}

func (e *EmbyService) episodicLibraryIDs(ctx context.Context) []string {
	if e == nil || e.repo == nil || e.repo.DB == nil {
		return nil
	}
	var ids []string
	if err := e.repo.DB.WithContext(ctx).Model(&model.Library{}).
		Where("LOWER(type) IN ?", []string{"tv", "anime", "variety"}).
		Pluck("id", &ids).Error; err != nil {
		return nil
	}
	return ids
}

func filterLikelyEpisodicPathsFromMovieQuery(q *gorm.DB) *gorm.DB {
	clause, args := embyLikelyEpisodicPathSQL()
	if clause == "" {
		return q
	}
	return q.Where("NOT ((media.season_num > 0 OR media.episode_num > 0) AND ("+clause+"))", args...)
}

func embyLikelyEpisodicPathSQL() (string, []any) {
	patterns := []string{
		"%/season %/%", "%/season.%/%", "%/season-%/%", "%/season_%/%",
		"%/s0%/%", "%/s1%/%", "%/s2%/%", "%/s3%/%", "%/s4%/%", "%/s5%/%", "%/s6%/%", "%/s7%/%", "%/s8%/%", "%/s9%/%",
		"%/special/%", "%/specials/%", "%/sp/%", "%/ova/%", "%/oad/%", "%/extra/%", "%/extras/%",
		"%/电视剧/%", "%/剧集/%", "%/国产剧/%", "%/欧美剧/%", "%/日韩剧/%", "%/日剧/%", "%/韩剧/%",
		"%/日番/%", "%/国漫/%", "%/番剧/%", "%/动漫/%", "%/特别篇/%", "%/特別篇/%", "%/番外/%", "%/特典/%",
	}
	clauses := make([]string, 0, len(patterns)*2)
	args := make([]any, 0, len(patterns)*2)
	for _, pattern := range patterns {
		clauses = append(clauses, "LOWER(media.path) LIKE ?")
		args = append(args, pattern)
		if strings.Contains(pattern, "/") {
			clauses = append(clauses, "LOWER(media.path) LIKE ?")
			args = append(args, strings.ReplaceAll(pattern, "/", `\`))
		}
	}
	return strings.Join(clauses, " OR "), args
}

func embyMediaPathLooksEpisodic(path string) bool {
	normalized := strings.ToLower(strings.ReplaceAll(strings.TrimSpace(path), "\\", "/"))
	if normalized == "" {
		return false
	}
	for _, marker := range []string{
		"/season ", "/season.", "/season-", "/season_", "/special/", "/specials/", "/sp/", "/ova/", "/oad/", "/extra/", "/extras/",
		"/电视剧/", "/剧集/", "/国产剧/", "/欧美剧/", "/日韩剧/", "/日剧/", "/韩剧/",
		"/日番/", "/国漫/", "/番剧/", "/动漫/", "/特别篇/", "/特別篇/", "/番外/", "/特典/",
	} {
		if strings.Contains(normalized, marker) {
			return true
		}
	}
	for _, marker := range []string{"/s0", "/s1", "/s2", "/s3", "/s4", "/s5", "/s6", "/s7", "/s8", "/s9"} {
		if idx := strings.Index(normalized, marker); idx >= 0 {
			after := idx + len(marker)
			if after < len(normalized) && normalized[after] >= '0' && normalized[after] <= '9' {
				slash := after + 1
				if slash < len(normalized) && normalized[slash] == '/' {
					return true
				}
			}
		}
	}
	return false
}

func (e *EmbyService) rememberSeriesGroup(group embySeriesGroup) {
	if e == nil || strings.TrimSpace(group.ID) == "" {
		return
	}
	expiresAt := time.Now().Add(embyVirtualCacheTTL)
	e.virtualMu.Lock()
	defer e.virtualMu.Unlock()
	if e.virtualSeries == nil {
		e.virtualSeries = make(map[string]embySeriesCacheEntry)
	}
	if e.virtualSeasons == nil {
		e.virtualSeasons = make(map[string]embySeasonCacheEntry)
	}
	if e.virtualArtwork == nil {
		e.virtualArtwork = make(map[string]embyArtworkCacheEntry)
	}
	if len(e.virtualSeries) > 2000 || len(e.virtualSeasons) > 5000 || len(e.virtualArtwork) > 7000 {
		e.virtualSeries = make(map[string]embySeriesCacheEntry)
		e.virtualSeasons = make(map[string]embySeasonCacheEntry)
		e.virtualArtwork = make(map[string]embyArtworkCacheEntry)
	}
	e.virtualSeries[group.ID] = embySeriesCacheEntry{group: group, expiresAt: expiresAt}
	e.virtualArtwork[group.ID] = embyArtworkCacheEntry{primary: group.PosterURL, backdrop: group.BackdropURL, expiresAt: expiresAt}
	e.virtualArtwork[group.ID+"-bd"] = embyArtworkCacheEntry{primary: group.PosterURL, backdrop: group.BackdropURL, expiresAt: expiresAt}
	for _, season := range e.seasonsForSeries(group) {
		e.virtualSeasons[season.ID] = embySeasonCacheEntry{season: season, expiresAt: expiresAt}
		e.virtualArtwork[season.ID] = embyArtworkCacheEntry{primary: season.Series.PosterURL, backdrop: season.Series.BackdropURL, expiresAt: expiresAt}
		e.virtualArtwork[season.ID+"-bd"] = embyArtworkCacheEntry{primary: season.Series.PosterURL, backdrop: season.Series.BackdropURL, expiresAt: expiresAt}
	}
}

func (e *EmbyService) rememberSeasonGroup(season embySeasonGroup) {
	if e == nil || strings.TrimSpace(season.ID) == "" {
		return
	}
	expiresAt := time.Now().Add(embyVirtualCacheTTL)
	e.virtualMu.Lock()
	defer e.virtualMu.Unlock()
	if e.virtualSeasons == nil {
		e.virtualSeasons = make(map[string]embySeasonCacheEntry)
	}
	if e.virtualArtwork == nil {
		e.virtualArtwork = make(map[string]embyArtworkCacheEntry)
	}
	e.virtualSeasons[season.ID] = embySeasonCacheEntry{season: season, expiresAt: expiresAt}
	e.virtualArtwork[season.ID] = embyArtworkCacheEntry{primary: season.Series.PosterURL, backdrop: season.Series.BackdropURL, expiresAt: expiresAt}
	e.virtualArtwork[season.ID+"-bd"] = embyArtworkCacheEntry{primary: season.Series.PosterURL, backdrop: season.Series.BackdropURL, expiresAt: expiresAt}
}

func (e *EmbyService) cachedSeriesGroup(id string) (embySeriesGroup, bool) {
	if e == nil || strings.TrimSpace(id) == "" {
		return embySeriesGroup{}, false
	}
	now := time.Now()
	e.virtualMu.RLock()
	entry, ok := e.virtualSeries[id]
	e.virtualMu.RUnlock()
	if !ok || now.After(entry.expiresAt) {
		if ok {
			e.virtualMu.Lock()
			delete(e.virtualSeries, id)
			e.virtualMu.Unlock()
		}
		return embySeriesGroup{}, false
	}
	return entry.group, true
}

func (e *EmbyService) cachedSeasonGroup(id string) (embySeasonGroup, bool) {
	if e == nil || strings.TrimSpace(id) == "" {
		return embySeasonGroup{}, false
	}
	now := time.Now()
	e.virtualMu.RLock()
	entry, ok := e.virtualSeasons[id]
	e.virtualMu.RUnlock()
	if !ok || now.After(entry.expiresAt) {
		if ok {
			e.virtualMu.Lock()
			delete(e.virtualSeasons, id)
			e.virtualMu.Unlock()
		}
		return embySeasonGroup{}, false
	}
	return entry.season, true
}

func (e *EmbyService) cachedArtworkURL(id, imageType string) (string, bool) {
	if e == nil || strings.TrimSpace(id) == "" {
		return "", false
	}
	now := time.Now()
	e.virtualMu.RLock()
	entry, ok := e.virtualArtwork[id]
	e.virtualMu.RUnlock()
	if !ok || now.After(entry.expiresAt) {
		if ok {
			e.virtualMu.Lock()
			delete(e.virtualArtwork, id)
			e.virtualMu.Unlock()
		}
		return "", false
	}
	switch strings.ToLower(imageType) {
	case "backdrop", "art":
		if entry.backdrop != "" {
			return entry.backdrop, true
		}
	}
	if entry.primary != "" {
		return entry.primary, true
	}
	return entry.backdrop, entry.backdrop != ""
}

func embyWantsPrimaryImage(imageType string) bool {
	imageType = strings.ToLower(strings.TrimSpace(imageType))
	return imageType == "" || imageType == "primary"
}

func (e *EmbyService) findSeriesGroup(ctx context.Context, id, userID string) (embySeriesGroup, bool, error) {
	if strings.TrimSpace(id) == "" {
		return embySeriesGroup{}, false, nil
	}
	if strings.HasPrefix(id, embyVirtualSeriesPrefix) {
		if group, ok := e.cachedSeriesGroup(id); ok {
			return group, true, nil
		}
	}
	var rows []model.Media
	q := e.repo.DB.WithContext(ctx).Model(&model.Media{}).Where("season_num > 0 OR episode_num > 0")
	q = e.applyUserMediaVisibility(ctx, q, userID)
	if !strings.HasPrefix(id, embyVirtualSeriesPrefix) {
		q = q.Where("series_id = ?", id)
	}
	if err := q.Order("media.season_num asc, media.episode_num asc, media.created_at asc").Limit(embySeriesGroupingLimit).Find(&rows).Error; err != nil {
		return embySeriesGroup{}, false, err
	}
	rows = e.filterEpisodeRows(ctx, rows)
	for _, group := range e.seriesGroupsFromMedia(rows) {
		if group.ID == id {
			e.rememberSeriesGroup(group)
			return group, true, nil
		}
	}
	if !strings.HasPrefix(id, embyVirtualSeriesPrefix) {
		if series, err := e.repo.Series.FindByID(ctx, id); err != nil {
			return embySeriesGroup{}, false, err
		} else if series != nil {
			return embySeriesGroup{
				ID:          series.ID,
				LibraryID:   series.LibraryID,
				Name:        series.Title,
				PosterURL:   series.PosterURL,
				BackdropURL: series.BackdropURL,
				Overview:    series.Overview,
				Rating:      series.Rating,
				Year:        series.Year,
				TMDbID:      series.TMDbID,
				BangumiID:   series.BangumiID,
				CreatedAt:   series.CreatedAt,
			}, true, nil
		}
	}
	return embySeriesGroup{}, false, nil
}

func (e *EmbyService) findSeasonGroup(ctx context.Context, id, userID string) (embySeasonGroup, bool, error) {
	if strings.TrimSpace(id) == "" || !strings.HasPrefix(id, embyVirtualSeasonPrefix) {
		return embySeasonGroup{}, false, nil
	}
	if season, ok := e.cachedSeasonGroup(id); ok {
		return season, true, nil
	}
	var rows []model.Media
	q := e.repo.DB.WithContext(ctx).Model(&model.Media{}).
		Where("season_num > 0 OR episode_num > 0")
	q = e.applyUserMediaVisibility(ctx, q, userID)
	if err := q.
		Order("media.season_num asc, media.episode_num asc, media.created_at asc").
		Limit(embySeriesGroupingLimit).
		Find(&rows).Error; err != nil {
		return embySeasonGroup{}, false, err
	}
	rows = e.filterEpisodeRows(ctx, rows)
	for _, series := range e.seriesGroupsFromMedia(rows) {
		for _, season := range e.seasonsForSeries(series) {
			if season.ID == id {
				e.rememberSeriesGroup(series)
				return season, true, nil
			}
		}
	}
	return embySeasonGroup{}, false, nil
}

func (e *EmbyService) seriesGroupsFromMedia(rows []model.Media) []embySeriesGroup {
	byID := map[string]*embySeriesGroup{}
	order := []string{}
	for _, row := range rows {
		row := row
		seriesID := e.seriesIDForMedia(&row)
		group, ok := byID[seriesID]
		if !ok {
			group = &embySeriesGroup{
				ID:        seriesID,
				LibraryID: row.LibraryID,
				Name:      e.seriesNameForMedia(&row),
				Year:      row.Year,
				TMDbID:    row.TMDbID,
				BangumiID: row.BangumiID,
				CreatedAt: row.CreatedAt,
			}
			byID[seriesID] = group
			order = append(order, seriesID)
		}
		if row.CreatedAt.After(group.CreatedAt) {
			group.CreatedAt = row.CreatedAt
		}
		if group.PosterURL == "" && row.PosterURL != "" {
			group.PosterURL = row.PosterURL
		}
		if group.BackdropURL == "" && row.BackdropURL != "" {
			group.BackdropURL = row.BackdropURL
		}
		if group.Overview == "" && row.Overview != "" {
			group.Overview = row.Overview
		}
		if group.Rating == 0 && row.Rating > 0 {
			group.Rating = row.Rating
		}
		if group.Year == 0 && row.Year > 0 {
			group.Year = row.Year
		}
		group.Episodes = append(group.Episodes, row)
	}
	groups := make([]embySeriesGroup, 0, len(order))
	for _, id := range order {
		group := *byID[id]
		sort.SliceStable(group.Episodes, func(i, j int) bool {
			if group.Episodes[i].SeasonNum != group.Episodes[j].SeasonNum {
				return group.Episodes[i].SeasonNum < group.Episodes[j].SeasonNum
			}
			if group.Episodes[i].EpisodeNum != group.Episodes[j].EpisodeNum {
				return group.Episodes[i].EpisodeNum < group.Episodes[j].EpisodeNum
			}
			return group.Episodes[i].CreatedAt.Before(group.Episodes[j].CreatedAt)
		})
		groups = append(groups, group)
	}
	return groups
}

func (e *EmbyService) seasonsForSeries(series embySeriesGroup) []embySeasonGroup {
	bySeason := map[int]*embySeasonGroup{}
	order := []int{}
	for _, episode := range series.Episodes {
		seasonNum := episode.SeasonNum
		if seasonNum < 0 {
			seasonNum = 1
		}
		season, ok := bySeason[seasonNum]
		if !ok {
			season = &embySeasonGroup{
				ID:        seasonID(series.ID, seasonNum),
				SeriesID:  series.ID,
				LibraryID: series.LibraryID,
				Name:      seasonName(seasonNum),
				SeasonNum: seasonNum,
				Series:    series,
			}
			bySeason[seasonNum] = season
			order = append(order, seasonNum)
		}
		season.Episodes = append(season.Episodes, episode)
	}
	sort.Ints(order)
	out := make([]embySeasonGroup, 0, len(order))
	for _, seasonNum := range order {
		out = append(out, *bySeason[seasonNum])
	}
	return out
}

func (e *EmbyService) firstLocalThumbnailMedia(rows []model.Media) *model.Media {
	for i := range rows {
		if e.mediaCanAdvertiseLocalThumbnail(&rows[i]) {
			return &rows[i]
		}
	}
	return nil
}

func (e *EmbyService) mediaRowsCanGenerateLocalThumbnail(rows []model.Media) bool {
	return e.firstLocalThumbnailMedia(rows) != nil
}

func (e *EmbyService) localThumbnailFromMediaRows(ctx context.Context, rows []model.Media) (string, error) {
	m := e.firstLocalThumbnailMedia(rows)
	if m == nil {
		return "", nil
	}
	return e.localVideoThumbnail(ctx, m)
}

func (e *EmbyService) seriesPayload(group embySeriesGroup) map[string]any {
	e.rememberSeriesGroup(group)
	imageTags := map[string]string{}
	backdropTags := []string{}
	if group.PosterURL != "" || e.mediaRowsCanGenerateLocalThumbnail(group.Episodes) {
		imageTags["Primary"] = group.ID
	}
	if group.BackdropURL != "" {
		backdropTags = append(backdropTags, group.ID+"-bd")
	}
	return map[string]any{
		"Id":                 group.ID,
		"Name":               group.Name,
		"ServerId":           embyServerID,
		"Type":               "Series",
		"MediaType":          "Video",
		"IsFolder":           true,
		"LocationType":       "Virtual",
		"PlayAccess":         "Full",
		"CanPlay":            false,
		"CanDownload":        false,
		"SupportsSync":       false,
		"ParentId":           group.LibraryID,
		"ProductionYear":     group.Year,
		"Overview":           group.Overview,
		"CommunityRating":    group.Rating,
		"RecursiveItemCount": len(group.Episodes),
		"ChildCount":         len(e.seasonsForSeries(group)),
		"DateCreated":        group.CreatedAt,
		"ImageTags":          imageTags,
		"BackdropImageTags":  backdropTags,
		"ProviderIds": map[string]string{
			"Tmdb":    intToStr(group.TMDbID),
			"Bangumi": intToStr(group.BangumiID),
		},
		"UserData": emptyUserData(),
	}
}

func (e *EmbyService) seasonPayload(season embySeasonGroup) map[string]any {
	e.rememberSeasonGroup(season)
	imageTags := map[string]string{}
	backdropTags := []string{}
	if season.Series.PosterURL != "" || e.mediaRowsCanGenerateLocalThumbnail(season.Episodes) {
		imageTags["Primary"] = season.ID
	}
	if season.Series.BackdropURL != "" {
		backdropTags = append(backdropTags, season.ID+"-bd")
	}
	return map[string]any{
		"Id":                season.ID,
		"Name":              season.Name,
		"ServerId":          embyServerID,
		"Type":              "Season",
		"MediaType":         "Video",
		"IsFolder":          true,
		"LocationType":      "Virtual",
		"PlayAccess":        "Full",
		"CanPlay":           false,
		"CanDownload":       false,
		"SupportsSync":      false,
		"ParentId":          season.SeriesID,
		"SeriesId":          season.SeriesID,
		"SeriesName":        season.Series.Name,
		"IndexNumber":       season.SeasonNum,
		"ChildCount":        len(season.Episodes),
		"ImageTags":         imageTags,
		"BackdropImageTags": backdropTags,
		"UserData":          emptyUserData(),
	}
}

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
		if embyWantsPrimaryImage(imageType) {
			if season, ok := e.cachedSeasonGroup(id); ok {
				return e.localThumbnailFromMediaRows(ctx, season.Episodes)
			}
			if season, ok, err := e.findSeasonGroup(ctx, id, ""); err != nil {
				return "", err
			} else if ok {
				return e.localThumbnailFromMediaRows(ctx, season.Episodes)
			}
		}
		return "", nil
	}
	if strings.HasPrefix(id, embyVirtualSeriesPrefix) {
		if raw, ok := e.cachedArtworkURL(id, imageType); ok {
			return raw, nil
		}
		if embyWantsPrimaryImage(imageType) {
			if series, ok := e.cachedSeriesGroup(id); ok {
				return e.localThumbnailFromMediaRows(ctx, series.Episodes)
			}
			if series, ok, err := e.findSeriesGroup(ctx, id, ""); err != nil {
				return "", err
			} else if ok {
				return e.localThumbnailFromMediaRows(ctx, series.Episodes)
			}
		}
		return "", nil
	}
	m, err := e.repo.Media.FindByID(ctx, id)
	if err != nil {
		return "", err
	}
	if m != nil {
		if e.mediaShouldBeEpisode(ctx, m) {
			switch strings.ToLower(imageType) {
			case "backdrop", "art":
				return "", nil
			}
		}
		if raw := pick(e.mediaPrimaryArtwork(ctx, m), e.mediaBackdropArtwork(ctx, m)); raw != "" {
			return raw, nil
		}
		if strings.ToLower(imageType) == "primary" || imageType == "" {
			return e.localVideoThumbnail(ctx, m)
		}
		return "", nil
	}
	if series, ok, err := e.findSeriesGroup(ctx, id, ""); err != nil {
		return "", err
	} else if ok {
		return pick(series.PosterURL, series.BackdropURL), nil
	}
	return "", ErrMediaNotFound
}

func (e *EmbyService) mediaCanGenerateLocalThumbnail(m *model.Media) bool {
	if e == nil || m == nil {
		return false
	}
	if strings.TrimSpace(m.PosterURL) != "" || strings.TrimSpace(m.STRMURL) != "" {
		return false
	}
	path := strings.TrimSpace(m.Path)
	if path == "" || isHTTPish(path) || strings.HasPrefix(strings.ToLower(path), "cloud://") {
		return false
	}
	switch strings.ToLower(filepath.Ext(path)) {
	case ".mp4", ".mkv", ".avi", ".mov", ".m4v", ".webm", ".ts", ".m2ts", ".wmv", ".flv", ".rmvb":
		return true
	default:
		return false
	}
}

func (e *EmbyService) mediaCanAdvertiseLocalThumbnail(m *model.Media) bool {
	if !e.mediaCanGenerateLocalThumbnail(m) {
		return false
	}
	source, err := filepath.Abs(filepath.Clean(m.Path))
	if err != nil {
		return false
	}
	if stat, err := os.Stat(source); err != nil || stat.IsDir() {
		return false
	}
	cachePath, failPath := e.localVideoThumbnailPaths(source)
	if stat, err := os.Stat(cachePath); err == nil && stat.Size() > 0 {
		return true
	}
	if freshNegativeImageCache(failPath) {
		data, _ := os.ReadFile(failPath) // #nosec G304 -- failPath is derived from a SHA-256 cache key under the configured cache directory.
		if !localVideoThumbnailFailureRetryable(string(data)) {
			return false
		}
		_ = os.Remove(failPath)
	}
	return true
}

func (e *EmbyService) localVideoThumbnail(ctx context.Context, m *model.Media) (string, error) {
	if !e.mediaCanGenerateLocalThumbnail(m) {
		return "", nil
	}
	source, err := filepath.Abs(filepath.Clean(m.Path))
	if err != nil {
		return "", nil
	}
	if stat, err := os.Stat(source); err != nil || stat.IsDir() {
		return "", nil
	}
	cachePath, failPath := e.localVideoThumbnailPaths(source)
	if stat, err := os.Stat(cachePath); err == nil && stat.Size() > 0 {
		return cachePath, nil
	}
	if freshNegativeImageCache(failPath) {
		data, _ := os.ReadFile(failPath) // #nosec G304 -- failPath is derived from a SHA-256 cache key under the configured cache directory.
		if !localVideoThumbnailFailureRetryable(string(data)) {
			return "", nil
		}
		_ = os.Remove(failPath)
	}
	if err := os.MkdirAll(filepath.Dir(cachePath), 0o750); err != nil {
		return "", err
	}
	bin, err := resolveLocalExecutable(e.cfg.App.FFmpegPath, "ffmpeg")
	if err != nil {
		_ = os.WriteFile(failPath, []byte(err.Error()), 0o600)
		return "", nil
	}
	select {
	case embyLocalThumbnailSem <- struct{}{}:
		defer func() { <-embyLocalThumbnailSem }()
	case <-ctx.Done():
		return "", nil
	}
	thumbCtx, cancel := context.WithTimeout(ctx, 20*time.Second)
	defer cancel()
	tmp := cachePath + ".tmp.jpg"
	_ = os.Remove(cachePath + ".tmp")
	_ = os.Remove(tmp)
	args := []string{
		"-hide_banner",
		"-loglevel", "error",
		"-nostdin",
		"-ss", localThumbnailSeekPosition(m.DurationSec),
		"-noaccurate_seek",
		"-i", source,
		"-map", "0:v:0",
		"-frames:v", "1",
		"-vf", "scale='min(480,iw)':-2",
		"-q:v", "6",
		"-y", tmp,
	}
	output, err := exec.CommandContext(thumbCtx, bin, args...).CombinedOutput() // #nosec G204 -- ffmpeg path is resolved locally and args are not shell-expanded.
	if err != nil {
		_ = os.Remove(tmp)
		message := strings.TrimSpace(string(output))
		if message == "" {
			message = err.Error()
		} else {
			message = err.Error() + ": " + message
		}
		if localVideoThumbnailFailureRetryable(message) || thumbCtx.Err() != nil {
			_ = os.Remove(failPath)
			return "", nil
		}
		_ = os.WriteFile(failPath, []byte(message), 0o600)
		return "", nil
	}
	if stat, err := os.Stat(tmp); err != nil || stat.Size() == 0 {
		_ = os.Remove(tmp)
		_ = os.WriteFile(failPath, []byte("empty thumbnail"), 0o600)
		return "", nil
	}
	if err := os.Rename(tmp, cachePath); err != nil {
		_ = os.Remove(tmp)
		return "", err
	}
	_ = os.Remove(failPath)
	return cachePath, nil
}

func (e *EmbyService) localVideoThumbnailPaths(source string) (string, string) {
	cacheDir := filepath.Join(e.cfg.Cache.CacheDir, "images", "video-thumbs")
	sum := sha256.Sum256([]byte(source))
	cachePath := filepath.Join(cacheDir, hex.EncodeToString(sum[:])+".jpg")
	return cachePath, cachePath + ".fail"
}

func localThumbnailSeekPosition(durationSec int) string {
	switch {
	case durationSec <= 2:
		return "00:00:00"
	case durationSec < 10:
		return "00:00:01"
	default:
		return "00:00:02"
	}
}

func localVideoThumbnailFailureRetryable(message string) bool {
	message = strings.ToLower(strings.TrimSpace(message))
	if message == "" {
		return false
	}
	for _, needle := range []string{
		"exit status 234",
		"signal: killed",
		"context canceled",
		"context deadline exceeded",
		"deadline exceeded",
		"operation was canceled",
	} {
		if strings.Contains(message, needle) {
			return true
		}
	}
	return false
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

func (e *EmbyService) seriesIDForMedia(m *model.Media) string {
	if strings.TrimSpace(m.SeriesID) != "" {
		return m.SeriesID
	}
	return stableEmbyID(embyVirtualSeriesPrefix, m.LibraryID, e.seriesNameForMedia(m))
}

func (e *EmbyService) seasonIDForMedia(m *model.Media) string {
	return seasonID(e.seriesIDForMedia(m), m.SeasonNum)
}

func (e *EmbyService) seriesNameForMedia(m *model.Media) string {
	if strings.TrimSpace(m.SeriesID) != "" {
		if series, err := e.repo.Series.FindByID(context.Background(), m.SeriesID); err == nil && series != nil && strings.TrimSpace(series.Title) != "" {
			return series.Title
		}
	}
	if name := inferSeriesNameFromPath(m.Path); name != "" {
		return name
	}
	name := strings.TrimSpace(m.Title)
	name = embyEpisodeTitleRE.ReplaceAllString(name, "")
	name = embyYearSuffixRE.ReplaceAllString(name, "")
	if name == "" {
		name = strings.TrimSpace(m.OriginalName)
	}
	return name
}

func inferSeriesNameFromPath(path string) string {
	path = strings.TrimSpace(path)
	if path == "" {
		return ""
	}
	dir := filepath.Dir(path)
	base := filepath.Base(dir)
	if embySeasonDirRE.MatchString(base) {
		dir = filepath.Dir(dir)
		base = filepath.Base(dir)
	}
	base = strings.TrimSpace(embyYearSuffixRE.ReplaceAllString(base, ""))
	if base == "." || base == string(filepath.Separator) {
		return ""
	}
	return base
}

func stableEmbyID(prefix string, parts ...string) string {
	h := sha256.New()
	for _, part := range parts {
		_, _ = h.Write([]byte(strings.ToLower(strings.TrimSpace(part))))
		_, _ = h.Write([]byte{0})
	}
	return prefix + hex.EncodeToString(h.Sum(nil))[:32]
}

func virtualLibraryID(kind, libraryID string) string {
	switch strings.ToLower(strings.TrimSpace(kind)) {
	case "movies":
		return embyVirtualMoviesPrefix + strings.TrimSpace(libraryID)
	case "shows", "tvshows", "series":
		return embyVirtualShowsPrefix + strings.TrimSpace(libraryID)
	default:
		return strings.TrimSpace(libraryID)
	}
}

func parseVirtualLibraryID(id string) (string, string, bool) {
	id = strings.TrimSpace(id)
	switch {
	case strings.HasPrefix(id, embyVirtualMoviesPrefix):
		libraryID := strings.TrimSpace(strings.TrimPrefix(id, embyVirtualMoviesPrefix))
		return libraryID, "movies", libraryID != ""
	case strings.HasPrefix(id, embyVirtualShowsPrefix):
		libraryID := strings.TrimSpace(strings.TrimPrefix(id, embyVirtualShowsPrefix))
		return libraryID, "shows", libraryID != ""
	default:
		return "", "", false
	}
}

func seasonID(seriesID string, seasonNum int) string {
	if seasonNum < 0 {
		seasonNum = 1
	}
	return stableEmbyID(embyVirtualSeasonPrefix, seriesID, strconv.Itoa(seasonNum))
}

func seasonName(seasonNum int) string {
	if seasonNum == 0 {
		return "特别篇"
	}
	if seasonNum < 0 {
		seasonNum = 1
	}
	return fmt.Sprintf("第 %d 季", seasonNum)
}

func sortSeriesGroups(groups []embySeriesGroup, p ItemsParams) {
	switch strings.ToLower(p.SortBy) {
	case "sortname", "name":
		sort.SliceStable(groups, func(i, j int) bool {
			if strings.EqualFold(p.SortOrder, "Descending") {
				return groups[i].Name > groups[j].Name
			}
			return groups[i].Name < groups[j].Name
		})
	default:
		sort.SliceStable(groups, func(i, j int) bool {
			if strings.EqualFold(p.SortOrder, "Ascending") {
				return groups[i].CreatedAt.Before(groups[j].CreatedAt)
			}
			return groups[i].CreatedAt.After(groups[j].CreatedAt)
		})
	}
}

func containsItemType(types []string, want string) bool {
	for _, t := range types {
		if strings.EqualFold(strings.TrimSpace(t), want) {
			return true
		}
	}
	return false
}

func containsSupportedEmbyItemType(types []string) bool {
	for _, itemType := range types {
		switch strings.ToLower(strings.TrimSpace(itemType)) {
		case "movie", "series", "season", "episode", "video", "folder", "collectionfolder":
			return true
		}
	}
	return false
}

func containsOnlyFolderItemTypes(types []string) bool {
	if len(types) == 0 {
		return false
	}
	for _, itemType := range types {
		switch strings.ToLower(strings.TrimSpace(itemType)) {
		case "folder", "collectionfolder":
		default:
			return false
		}
	}
	return true
}

func emptyItemsEnvelope(startIndex int) map[string]any {
	return map[string]any{
		"Items":            []map[string]any{},
		"TotalRecordCount": int64(0),
		"StartIndex":       startIndex,
	}
}

func containsEmbyFilter(filters []string, want string) bool {
	for _, filter := range filters {
		if strings.EqualFold(strings.TrimSpace(filter), want) {
			return true
		}
	}
	return false
}

func firstCSVValue(value string) string {
	if i := strings.Index(value, ","); i >= 0 {
		value = value[:i]
	}
	return strings.TrimSpace(value)
}

func primarySupportedEmbySort(sortBy string, resumeFilter bool) string {
	for _, part := range strings.Split(sortBy, ",") {
		key := strings.ToLower(strings.TrimSpace(part))
		switch key {
		case "sortname", "name", "premieredate", "productionyear", "datecreated", "communityrating":
			return key
		case "dateplayed":
			if resumeFilter {
				return key
			}
		}
	}
	return strings.ToLower(strings.TrimSpace(firstCSVValue(sortBy)))
}

func pageSlice[T any](items []T, start, limit int) []T {
	if start < 0 {
		start = 0
	}
	if limit <= 0 {
		limit = len(items)
	}
	if start >= len(items) {
		return []T{}
	}
	end := start + limit
	if end > len(items) {
		end = len(items)
	}
	return items[start:end]
}

func emptyUserData() map[string]any {
	return map[string]any{
		"PlaybackPositionTicks": 0,
		"PlayCount":             0,
		"IsFavorite":            false,
		"Played":                false,
		"PlayedPercentage":      0,
	}
}

func (e *EmbyService) applyUserMediaVisibility(ctx context.Context, q *gorm.DB, userID string) *gorm.DB {
	visibility := e.mediaVisibility(ctx, userID)
	if !visibility.IncludeNSFW {
		q = q.Where("nsfw = ?", false)
		if hidden := visibility.HiddenLibraryIDs; len(hidden) > 0 {
			q = q.Where("library_id NOT IN ?", hidden)
		}
	}
	if len(visibility.AllowedLibraryIDs) > 0 {
		q = q.Where("library_id IN ?", visibility.AllowedLibraryIDs)
	}
	return q
}

func (e *EmbyService) filterMediaRowsForUser(ctx context.Context, rows []model.Media, userID string) []model.Media {
	visibility := e.mediaVisibility(ctx, userID)
	if visibility.IncludeNSFW && len(visibility.AllowedLibraryIDs) == 0 {
		return rows
	}
	allowed := map[string]bool{}
	for _, id := range visibility.AllowedLibraryIDs {
		allowed[id] = true
	}
	hiddenLibraries := map[string]bool{}
	for _, id := range visibility.HiddenLibraryIDs {
		hiddenLibraries[id] = true
	}
	out := rows[:0]
	for _, row := range rows {
		if row.NSFW && !visibility.IncludeNSFW {
			continue
		}
		if hiddenLibraries[row.LibraryID] {
			continue
		}
		if len(allowed) > 0 && !allowed[row.LibraryID] {
			continue
		}
		out = append(out, row)
	}
	return out
}

func (e *EmbyService) mediaVisibility(ctx context.Context, userID string) MediaVisibility {
	if e == nil {
		return MediaVisibility{IncludeNSFW: true}
	}
	key := strings.TrimSpace(userID)
	now := time.Now()
	e.visibilityMu.RLock()
	entry, ok := e.visibilityCache[key]
	e.visibilityMu.RUnlock()
	if ok && now.Before(entry.expiresAt) {
		return cloneMediaVisibility(entry.visibility)
	}

	visibility := UserDefaultMediaVisibility(ctx, e.repo, userID)
	if !visibility.IncludeNSFW {
		visibility.HiddenLibraryIDs = e.hiddenLibraryIDs(ctx, visibility)
	}
	visibility = ExpandMediaVisibilityForMergedCloudLibraries(ctx, e.repo, visibility)
	visibility = cloneMediaVisibility(visibility)

	e.visibilityMu.Lock()
	if e.visibilityCache == nil {
		e.visibilityCache = make(map[string]embyVisibilityCacheEntry)
	}
	if len(e.visibilityCache) > 1000 {
		e.visibilityCache = make(map[string]embyVisibilityCacheEntry)
	}
	e.visibilityCache[key] = embyVisibilityCacheEntry{
		visibility: cloneMediaVisibility(visibility),
		expiresAt:  now.Add(embyVisibilityCacheTTL),
	}
	e.visibilityMu.Unlock()

	return visibility
}

func (e *EmbyService) mergedLibraryIDs(ctx context.Context, libraryID string) []string {
	ids, err := MergedLibraryIDsForLibrary(ctx, e.repo, libraryID)
	if err != nil || len(ids) == 0 {
		return []string{libraryID}
	}
	return ids
}

func cloneMediaVisibility(visibility MediaVisibility) MediaVisibility {
	if visibility.AllowedLibraryIDs != nil {
		visibility.AllowedLibraryIDs = append([]string(nil), visibility.AllowedLibraryIDs...)
	}
	if visibility.HiddenLibraryIDs != nil {
		visibility.HiddenLibraryIDs = append([]string(nil), visibility.HiddenLibraryIDs...)
	}
	return visibility
}

func (e *EmbyService) libraryVisibleFromCachedVisibility(lib model.Library, visibility MediaVisibility) bool {
	if len(visibility.AllowedLibraryIDs) > 0 {
		allowed := false
		for _, id := range visibility.AllowedLibraryIDs {
			if id == lib.ID {
				allowed = true
				break
			}
		}
		if !allowed {
			return false
		}
	}
	if visibility.IncludeNSFW {
		return true
	}
	for _, id := range visibility.HiddenLibraryIDs {
		if id == lib.ID {
			return false
		}
	}
	return true
}

func (e *EmbyService) hiddenLibraryIDs(ctx context.Context, visibility MediaVisibility) []string {
	if visibility.IncludeNSFW {
		return nil
	}
	libs, err := e.repo.Library.List(ctx)
	if err != nil {
		return nil
	}
	shadowed := ShadowedCloudLibraryIDSet(libs)
	ids := make([]string, 0)
	for _, lib := range libs {
		if shadowed[lib.ID] || !LibraryVisibleForUser(ctx, e.repo, lib, visibility) {
			ids = append(ids, lib.ID)
		}
	}
	return ids
}

func minInt(a, b int) int {
	if a < b {
		return a
	}
	return b
}

func maxInt(a, b int) int {
	if a > b {
		return a
	}
	return b
}

// ─── Playback ────────────────────────────────────────────────────────────────

// PlaybackInfo returns a PlaybackInfoResponse usable by Emby clients.
func (e *EmbyService) PlaybackInfo(ctx context.Context, mediaID, userID string) (map[string]any, error) {
	m, err := e.playableMedia(ctx, mediaID, userID)
	if err != nil || m == nil {
		return nil, err
	}
	e.ensureCloudTrackMetadata(ctx, m)
	return map[string]any{
		"MediaSources":  e.mediaSourcesForItem(ctx, m, false, e.directPlayOnly(ctx)),
		"PlaySessionId": fmt.Sprintf("%s-%d", m.ID, time.Now().Unix()),
	}, nil
}

// ensureCloudTrackMetadata 在后台补齐云盘媒体的轨道元数据。
//
// 注意必须是异步的：此前这里在 PlaybackInfo 请求路径上同步执行
// CloudResolve + ffprobe(HTTP)（最长 8 秒），既把第三方播放器的起播时间
// 拖长到秒级，又让每一次点开详情/起播都可能触发一次云盘数据下载，是
// Docker 部署下 CPU/带宽长期居高的来源之一。探测结果落库后，下一次
// 请求自然能读到完整元数据。
func (e *EmbyService) ensureCloudTrackMetadata(ctx context.Context, m *model.Media) {
	if e == nil || m == nil || e.storage == nil || e.probe == nil || !mediaTrackMetadataMissing(m) {
		return
	}
	typ, ref, ok := parseCloudMediaPlaybackURL(m.STRMURL)
	if !ok {
		return
	}
	mediaID := m.ID
	e.cloudProbeMu.Lock()
	if e.cloudProbeInFlight == nil {
		e.cloudProbeInFlight = make(map[string]struct{})
	}
	if _, busy := e.cloudProbeInFlight[mediaID]; busy {
		e.cloudProbeMu.Unlock()
		return
	}
	e.cloudProbeInFlight[mediaID] = struct{}{}
	e.cloudProbeMu.Unlock()

	go func() {
		defer func() {
			e.cloudProbeMu.Lock()
			delete(e.cloudProbeInFlight, mediaID)
			e.cloudProbeMu.Unlock()
		}()
		probeCtx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
		defer cancel()
		link, err := e.storage.CloudResolve(probeCtx, typ, ref, "")
		if err != nil {
			if e.log != nil {
				e.log.Debug("resolve cloud media for playback probe failed", zap.String("media_id", mediaID), zap.Error(err))
			}
			return
		}
		probe, err := e.probe.ProbeHTTP(probeCtx, link.URL, link.Headers)
		if err != nil {
			if e.log != nil {
				e.log.Debug("playback cloud ffprobe failed", zap.String("media_id", mediaID), zap.Error(err))
			}
			return
		}
		updates := probeResultUpdates(probe)
		if len(updates) == 0 {
			return
		}
		if err := e.repo.DB.WithContext(probeCtx).Model(&model.Media{}).Where("id = ?", mediaID).Updates(updates).Error; err != nil && e.log != nil {
			e.log.Debug("persist playback cloud probe failed", zap.String("media_id", mediaID), zap.Error(err))
		}
	}()
}

func mediaTrackMetadataMissing(m *model.Media) bool {
	return m.DurationSec <= 0 ||
		m.Width <= 0 ||
		m.Height <= 0 ||
		strings.TrimSpace(m.VideoCodec) == "" ||
		strings.TrimSpace(m.AudioCodec) == ""
}

func parseCloudMediaPlaybackURL(raw string) (string, string, bool) {
	raw = strings.TrimSpace(raw)
	if raw == "" {
		return "", "", false
	}
	u, err := url.Parse(raw)
	if err != nil {
		return "", "", false
	}
	path := strings.Trim(u.Path, "/")
	const prefix = "api/cloud/play/"
	idx := strings.Index(strings.ToLower(path), prefix)
	if idx < 0 {
		return "", "", false
	}
	typ := strings.TrimSpace(path[idx+len(prefix):])
	ref := strings.TrimSpace(u.Query().Get("ref"))
	return typ, ref, typ != "" && ref != ""
}

func applyProbeResultToMediaValue(m *model.Media, probe *ProbeResult) {
	if m == nil || probe == nil {
		return
	}
	if probe.DurationSec > 0 {
		m.DurationSec = probe.DurationSec
	}
	if probe.Width > 0 {
		m.Width = probe.Width
	}
	if probe.Height > 0 {
		m.Height = probe.Height
	}
	if strings.TrimSpace(probe.VideoCodec) != "" {
		m.VideoCodec = probe.VideoCodec
	}
	if strings.TrimSpace(probe.AudioCodec) != "" {
		m.AudioCodec = probe.AudioCodec
	}
	if strings.TrimSpace(probe.Container) != "" {
		m.Container = probe.Container
	}
}

// directPlayOnly reports whether the admin enabled「客户端直连解码」mode.
// In that mode the host never transcodes; clients must direct-play.
func (e *EmbyService) directPlayOnly(ctx context.Context) bool {
	if e.repo == nil || e.repo.Setting == nil {
		return false
	}
	v, err := e.repo.Setting.Get(ctx, PlaybackDirectOnlySettingKey)
	if err != nil {
		return false
	}
	return parseBoolSetting(v, false)
}

func (e *EmbyService) playableMedia(ctx context.Context, id, userID string) (*model.Media, error) {
	if season, ok, err := e.findSeasonGroup(ctx, id, userID); err != nil {
		return nil, err
	} else if ok && len(season.Episodes) > 0 {
		return &season.Episodes[0], nil
	}
	if series, ok, err := e.findSeriesGroup(ctx, id, userID); err != nil {
		return nil, err
	} else if ok && len(series.Episodes) > 0 {
		return &series.Episodes[0], nil
	}
	m, err := e.repo.Media.FindByID(ctx, id)
	if err != nil || m == nil {
		return m, err
	}
	if !UserDefaultMediaVisibility(ctx, e.repo, userID).Allows(m) {
		return nil, nil
	}
	return m, nil
}

// mediaSource 是 /Items 与 /PlaybackInfo 共享的 MediaSource 结构。
//
// /Items 与 /PlaybackInfo 都下发 Emby 兼容 /Videos/{id}/stream。
// 避免 Yamby/SenPlayer/iOS 这类客户端把容器内文件路径当成 HTTP URL 请求。
func (e *EmbyService) mediaSource(ctx context.Context, m *model.Media, asEmbedded, directOnly bool) map[string]any {
	container := embyPlaybackContainer(m.Container, m.Path)
	if container == "" && strings.TrimSpace(m.STRMURL) != "" {
		container = "strm"
	}
	isCloud := strings.TrimSpace(m.STRMURL) != ""
	playURL := e.mediaPlayURL(ctx, m, container)
	if isCloud {
		// Cloud/WebDAV media is already a direct/proxy stream. Advertising HLS
		// transcoding makes some Emby clients pick /master.m3u8, forcing this
		// lightweight server to pull remote bytes through ffmpeg and often
		// surfacing as "network/playback failed". Keep cloud media direct-only.
		directOnly = true
	}
	sourcePath := m.Path
	if playURL != "" {
		sourcePath = playURL
	}
	src := map[string]any{
		"Id":                    m.ID,
		"Name":                  m.Title,
		"Path":                  sourcePath,
		"Container":             container,
		"Size":                  m.SizeBytes,
		"Protocol":              "Http",
		"Type":                  "Default",
		"IsRemote":              isCloud,
		"RequiresOpening":       false,
		"RequiresClosing":       false,
		"ReadAtNativeFramerate": false,
		"SupportsTranscoding":   !directOnly,
		// 云盘媒体的 Path 在 PlaybackInfo 阶段会被补上 api_key，且最终
		// 302 到云盘直链。Infuse/Emby 官方客户端会优先挑选 DirectPlay
		// 源；如果这里标 false，即使 DirectStreamUrl 可用，也可能被判定
		// 为“没有可播放媒体源”。
		"SupportsDirectStream": !isCloud || playURL != "",
		"SupportsDirectPlay":   !isCloud || playURL != "",
		"SupportsProbing":      true,
		"RunTimeTicks":         int64(m.DurationSec) * 10_000_000,
		"MediaStreams":         e.mediaStreams(m),
	}
	if playURL != "" {
		src["DirectStreamUrl"] = playURL
		// 直连解码模式下不下发 TranscodingUrl，迫使客户端本地解码直连，
		// 宿主机不参与转码。
		if !asEmbedded && !directOnly {
			src["TranscodingUrl"] = "/Videos/" + m.ID + "/master.m3u8"
		}
	}
	if strings.TrimSpace(m.STRMURL) != "" && playURL != "" {
		// STRM / cloud:// media must stay behind a token-aware endpoint. When
		// STRM playback is enabled we expose /api/stream so third-party clients
		// follow the same STRM entry as generated .strm files; when disabled we
		// expose /Videos/{id}/stream so playback uses the Emby 302/proxy path.
		src["IsRemote"] = true
	}
	return src
}

func (e *EmbyService) mediaSourcesForItem(ctx context.Context, m *model.Media, asEmbedded, directOnly bool) []map[string]any {
	siblings := e.mediaVersionSiblings(ctx, m)
	if len(siblings) == 0 {
		return []map[string]any{e.mediaSource(ctx, m, asEmbedded, directOnly)}
	}
	sources := make([]map[string]any, 0, len(siblings))
	for i := range siblings {
		media := siblings[i]
		sources = append(sources, e.mediaSource(ctx, &media, asEmbedded, directOnly))
	}
	return sources
}

func (e *EmbyService) mediaPlayURL(ctx context.Context, m *model.Media, container string) string {
	if m == nil {
		return ""
	}
	playURL := embyDirectStreamURL(m.ID, container)
	if strings.TrimSpace(m.STRMURL) == "" {
		return playURL
	}
	switch CloudPlaybackMode(ctx, e.repo) {
	case CloudPlaybackModeSTRM:
		return embySTRMStreamURL(m.ID)
	case CloudPlaybackModeRedirectProxy:
		return playURL
	default:
		return ""
	}
}

func (e *EmbyService) mediaVersionSiblings(ctx context.Context, m *model.Media) []model.Media {
	if e == nil || e.repo == nil || e.repo.DB == nil || m == nil || strings.TrimSpace(m.ID) == "" {
		return nil
	}
	libraryIDs := e.mergedLibraryIDs(ctx, m.LibraryID)
	if len(libraryIDs) == 0 {
		libraryIDs = []string{m.LibraryID}
	}
	q := e.repo.DB.WithContext(ctx).Model(&model.Media{}).
		Where("library_id IN ?", libraryIDs).
		Where("season_num = ? AND episode_num = ?", m.SeasonNum, m.EpisodeNum)
	if m.TMDbID > 0 {
		q = q.Where("tm_db_id = ?", m.TMDbID)
	} else if m.BangumiID > 0 {
		q = q.Where("bangumi_id = ?", m.BangumiID)
	} else {
		title := strings.TrimSpace(m.Title)
		if title == "" {
			title = strings.TrimSpace(m.OriginalName)
		}
		if title == "" {
			return []model.Media{*m}
		}
		q = q.Where("LOWER(title) = ?", strings.ToLower(title))
		if m.Year > 0 {
			q = q.Where("year = ?", m.Year)
		}
	}
	var rows []model.Media
	if err := q.Find(&rows).Error; err != nil || len(rows) == 0 {
		return []model.Media{*m}
	}
	rows = e.collapseExactPathRows(rows)
	sort.SliceStable(rows, func(i, j int) bool {
		if rows[i].ID == m.ID {
			return true
		}
		if rows[j].ID == m.ID {
			return false
		}
		return preferMediaVersion(rows[i], rows[j])
	})
	return rows
}

func (e *EmbyService) collapseExactPathRows(rows []model.Media) []model.Media {
	if len(rows) < 2 {
		return rows
	}
	out := rows[:0]
	seen := map[string]struct{}{}
	for _, row := range rows {
		path := strings.TrimSpace(row.Path)
		if path != "" {
			if _, ok := seen[path]; ok {
				continue
			}
			seen[path] = struct{}{}
		}
		out = append(out, row)
	}
	return out
}

func (e *EmbyService) mediaVersionKey(ctx context.Context, m *model.Media) string {
	if e == nil || m == nil {
		return ""
	}
	ids := e.mergedLibraryIDs(ctx, m.LibraryID)
	sort.Strings(ids)
	libraryGroup := strings.Join(ids, ",")
	if libraryGroup == "" {
		libraryGroup = strings.TrimSpace(m.LibraryID)
	}
	if m.TMDbID > 0 {
		return fmt.Sprintf("%s|tmdb:%d|s:%d|e:%d", libraryGroup, m.TMDbID, m.SeasonNum, m.EpisodeNum)
	}
	if m.BangumiID > 0 {
		return fmt.Sprintf("%s|bangumi:%d|s:%d|e:%d", libraryGroup, m.BangumiID, m.SeasonNum, m.EpisodeNum)
	}
	title := strings.ToLower(strings.TrimSpace(m.Title))
	if title == "" {
		title = strings.ToLower(strings.TrimSpace(m.OriginalName))
	}
	if title == "" {
		return ""
	}
	return fmt.Sprintf("%s|title:%s|y:%d|s:%d|e:%d", libraryGroup, title, m.Year, m.SeasonNum, m.EpisodeNum)
}

func preferMediaVersion(candidate, current model.Media) bool {
	candidateCloud := strings.TrimSpace(candidate.STRMURL) != "" || strings.HasPrefix(strings.ToLower(strings.TrimSpace(candidate.Path)), "cloud://")
	currentCloud := strings.TrimSpace(current.STRMURL) != "" || strings.HasPrefix(strings.ToLower(strings.TrimSpace(current.Path)), "cloud://")
	if candidateCloud != currentCloud {
		return !candidateCloud
	}
	if candidate.Width != current.Width {
		return candidate.Width > current.Width
	}
	if candidate.SizeBytes != current.SizeBytes {
		return candidate.SizeBytes > current.SizeBytes
	}
	return candidate.CreatedAt.After(current.CreatedAt)
}

func embySTRMStreamURL(mediaID string) string {
	return "/api/stream/" + url.PathEscape(strings.TrimSpace(mediaID))
}

func embyPlaybackContainer(raw, mediaPath string) string {
	pathExt := embyNormalizeStreamContainer(strings.TrimPrefix(strings.ToLower(filepath.Ext(mediaPath)), "."))
	if pathExt != "" && pathExt != "strm" {
		return pathExt
	}
	raw = strings.Trim(strings.ToLower(raw), ". ")
	if raw == "" {
		return pathExt
	}
	tokens := strings.FieldsFunc(raw, func(r rune) bool {
		return r == ',' || r == ';' || r == ' ' || r == '\t' || r == '/'
	})
	if len(tokens) == 0 {
		return embyNormalizeStreamContainer(raw)
	}
	normalized := make([]string, 0, len(tokens))
	for _, token := range tokens {
		if value := embyNormalizeStreamContainer(token); value != "" {
			normalized = append(normalized, value)
		}
	}
	for _, preferred := range []string{"mkv", "mp4", "mov", "webm", "avi", "ts", "m2ts", "wmv", "flv", "rmvb", "mpg"} {
		for _, value := range normalized {
			if value == preferred {
				return value
			}
		}
	}
	if len(normalized) > 0 {
		return normalized[0]
	}
	return ""
}

func embyNormalizeStreamContainer(container string) string {
	container = strings.Trim(strings.ToLower(container), ". ")
	switch container {
	case "", "unknown":
		return ""
	case "matroska":
		return "mkv"
	case "quicktime":
		return "mov"
	case "mpegts":
		return "ts"
	case "mpeg":
		return "mpg"
	case "asf":
		return "wmv"
	default:
		return container
	}
}

func embyDirectStreamURL(mediaID, container string) string {
	mediaID = strings.TrimSpace(mediaID)
	container = embyNormalizeStreamContainer(container)
	if container == "" || container == "strm" {
		return "/Videos/" + mediaID + "/stream"
	}
	return "/Videos/" + mediaID + "/stream." + container
}

func (e *EmbyService) mediaStreams(m *model.Media) []map[string]any {
	streams := []map[string]any{}
	if m.VideoCodec != "" || m.Width > 0 {
		streams = append(streams, map[string]any{
			"Codec":        m.VideoCodec,
			"Type":         "Video",
			"Index":        0,
			"Width":        m.Width,
			"Height":       m.Height,
			"AspectRatio":  "",
			"IsDefault":    true,
			"IsForced":     false,
			"IsExternal":   false,
			"DisplayTitle": fmt.Sprintf("%dx%d %s", m.Width, m.Height, m.VideoCodec),
		})
	}
	if m.AudioCodec != "" {
		streams = append(streams, map[string]any{
			"Codec":      m.AudioCodec,
			"Type":       "Audio",
			"Index":      1,
			"IsDefault":  true,
			"IsForced":   false,
			"IsExternal": false,
		})
	}
	if len(streams) == 0 {
		streams = append(streams, map[string]any{
			"Codec":        "unknown",
			"Type":         "Video",
			"Index":        0,
			"IsDefault":    true,
			"IsForced":     false,
			"IsExternal":   false,
			"DisplayTitle": "Video",
		})
	}
	return streams
}

// ─── 收藏 / 已看（Emby 客户端写路径） ──────────────────────────────────────

// SetFavorite 把 mediaID 标为 userID 的收藏。
func (e *EmbyService) SetFavorite(ctx context.Context, userID, mediaID string, favorite bool) error {
	if favorite {
		var f model.Favorite
		err := e.repo.DB.WithContext(ctx).
			Where("user_id = ? AND media_id = ?", userID, mediaID).First(&f).Error
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return e.repo.DB.WithContext(ctx).Create(&model.Favorite{
				UserID: userID, MediaID: mediaID,
			}).Error
		}
		return err
	}
	return e.repo.DB.WithContext(ctx).
		Where("user_id = ? AND media_id = ?", userID, mediaID).
		Delete(&model.Favorite{}).Error
}

// MarkPlayed 把 mediaID 标为已看（写一个 100% 进度的 history 行）。
func (e *EmbyService) MarkPlayed(ctx context.Context, userID, mediaID string, played bool) error {
	if !played {
		return e.repo.DB.WithContext(ctx).
			Where("user_id = ? AND media_id = ?", userID, mediaID).
			Delete(&model.PlaybackHistory{}).Error
	}
	m, err := e.repo.Media.FindByID(ctx, mediaID)
	if err != nil || m == nil {
		return errors.New("media not found")
	}
	dur := int64(m.DurationSec) * 1000
	if dur <= 0 {
		dur = 1
	}
	return e.repo.History.Upsert(ctx, &model.PlaybackHistory{
		UserID:     userID,
		MediaID:    mediaID,
		PositionMs: dur,
		DurationMs: dur,
		WatchedAt:  time.Now(),
		Completed:  true,
	})
}

// RecordProgress 记录播放进度（来自 Emby 客户端的 /Sessions/Playing/Progress）。
func (e *EmbyService) RecordProgress(ctx context.Context, userID, mediaID string, positionTicks, runtimeTicks int64) error {
	pos := positionTicks / 10_000
	dur := runtimeTicks / 10_000
	if dur <= 0 {
		// runtimeTicks 缺失时回退到 media.DurationSec
		if m, _ := e.repo.Media.FindByID(ctx, mediaID); m != nil {
			dur = int64(m.DurationSec) * 1000
		}
	}
	completed := dur > 0 && pos >= dur*9/10
	return e.repo.History.Upsert(ctx, &model.PlaybackHistory{
		UserID:     userID,
		MediaID:    mediaID,
		PositionMs: pos,
		DurationMs: dur,
		WatchedAt:  time.Now(),
		Completed:  completed,
	})
}

// ─── Helpers ─────────────────────────────────────────────────────────────────

func splitCSV(s string) []string {
	if strings.TrimSpace(s) == "" {
		return []string{}
	}
	parts := strings.Split(s, ",")
	out := make([]string, 0, len(parts))
	for _, p := range parts {
		p = strings.TrimSpace(p)
		if p != "" {
			out = append(out, p)
		}
	}
	return out
}

func intToStr(v int) string {
	if v == 0 {
		return ""
	}
	return strconv.Itoa(v)
}
