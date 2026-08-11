package service

import (
	"context"
	"fmt"
	"net/url"
	"path/filepath"
	"strings"
	"time"

	"github.com/ShukeBta/MediaStationGo/internal/model"
)

// PlaybackInfo returns a PlaybackInfoResponse usable by Emby clients.
func (e *EmbyService) PlaybackInfo(ctx context.Context, mediaID, userID string) (map[string]any, error) {
	m, err := e.playableMedia(ctx, mediaID, userID)
	if err != nil || m == nil {
		return nil, err
	}
	return map[string]any{
		"MediaSources":  e.mediaSourcesForItem(ctx, m, false, e.directPlayOnly(ctx)),
		"PlaySessionId": fmt.Sprintf("%s-%d", m.ID, time.Now().Unix()),
	}, nil
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
// asEmbedded=true：嵌在 /Items 列表里，不包含完整 stream URL（避免暴露
// 直链给搜索接口）。/PlaybackInfo 走 false 路径，URL 指向 Emby 兼容
// /Videos/{id}/stream（客户端会继续携带 X-Emby-Token 或 append api_key）。
func (e *EmbyService) mediaSource(ctx context.Context, m *model.Media, asEmbedded, directOnly bool) map[string]any {
	container := embyMediaContainer(m)
	isCloud := strings.TrimSpace(m.STRMURL) != ""
	playURL := e.embyMediaPlayURL(ctx, m, container, isCloud)
	if isCloud {
		// Cloud/WebDAV media is already a direct/proxy stream. Advertising HLS
		// transcoding makes some Emby clients pick /master.m3u8, forcing this
		// lightweight server to pull remote bytes through ffmpeg and often
		// surfacing as "network/playback failed". Keep cloud media direct-only.
		directOnly = true
	}
	src := e.baseMediaSource(m, container, isCloud, playURL, directOnly)
	if !asEmbedded && playURL != "" {
		src["DirectStreamUrl"] = playURL
		// 直连解码模式下不下发 TranscodingUrl，迫使客户端本地解码直连，
		// 宿主机不参与转码。
		if !directOnly {
			src["TranscodingUrl"] = "/Videos/" + m.ID + "/master.m3u8"
		}
	}
	if strings.TrimSpace(m.STRMURL) != "" && playURL != "" {
		// STRM / cloud:// media must stay behind a token-aware endpoint. When
		// STRM playback is enabled we expose /api/stream so third-party clients
		// follow the same STRM entry as generated .strm files; when disabled we
		// expose /Videos/{id}/stream so playback uses the Emby 302/proxy path.
		src["IsRemote"] = true
		src["Path"] = playURL
	}
	if !asEmbedded {
		e.attachExternalSubtitleStreams(ctx, m, src)
	}
	return src
}

func (e *EmbyService) baseMediaSource(m *model.Media, container string, isCloud bool, playURL string, directOnly bool) map[string]any {
	return map[string]any{
		"Id":                    m.ID,
		"Name":                  m.Title,
		"Path":                  m.Path,
		"Container":             container,
		"Size":                  m.SizeBytes,
		"Bitrate":               effectiveMediaBitRate(m.BitRate, m.SizeBytes, m.DurationSec),
		"Protocol":              "Http",
		"Type":                  "Default",
		"IsRemote":              isCloud,
		"RequiresOpening":       false,
		"RequiresClosing":       false,
		"ReadAtNativeFramerate": false,
		"SupportsTranscoding":   !directOnly,
		"SupportsDirectStream":  !isCloud || playURL != "",
		"SupportsDirectPlay":    !isCloud || playURL != "",
		"SupportsProbing":       true,
		"RunTimeTicks":          int64(m.DurationSec) * 10_000_000,
		"MediaStreams":          e.mediaStreams(m),
	}
}

func embyMediaContainer(m *model.Media) string {
	container := strings.Trim(strings.ToLower(m.Container), ". ")
	if container == "" {
		container = strings.TrimPrefix(strings.ToLower(filepath.Ext(m.Path)), ".")
	}
	if container == "" && strings.TrimSpace(m.STRMURL) != "" {
		return "strm"
	}
	return container
}

func (e *EmbyService) embyMediaPlayURL(ctx context.Context, m *model.Media, container string, isCloud bool) string {
	if !isCloud {
		return embyDirectStreamURL(m.ID, container)
	}
	switch CloudPlaybackMode(ctx, e.repo) {
	case CloudPlaybackModeSTRM:
		return embySTRMStreamURL(m.ID)
	case CloudPlaybackModeRedirectProxy:
		return embyDirectStreamURL(m.ID, container)
	default:
		return ""
	}
}
