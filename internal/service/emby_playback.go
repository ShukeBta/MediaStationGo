package service

import (
	"context"
	"fmt"
	"net/url"
	"path/filepath"
	"strings"
	"time"

	"go.uber.org/zap"

	"github.com/ShukeBta/MediaStationGo/internal/model"
)

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

	go e.probeCloudTrackMetadata(mediaID, typ, ref)
}

func (e *EmbyService) probeCloudTrackMetadata(mediaID, typ, ref string) {
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
	return src
}

func (e *EmbyService) baseMediaSource(m *model.Media, container string, isCloud bool, playURL string, directOnly bool) map[string]any {
	return map[string]any{
		"Id":                    m.ID,
		"Name":                  m.Title,
		"Path":                  m.Path,
		"Container":             container,
		"Size":                  m.SizeBytes,
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
