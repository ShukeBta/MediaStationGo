package service

import (
	"context"
	"net/url"
	"strings"

	"github.com/google/uuid"

	"github.com/ShukeBta/MediaStationGo/internal/model"
)

// PlaybackInfo returns a PlaybackInfoResponse usable by Emby clients.
func (e *EmbyService) PlaybackInfo(ctx context.Context, mediaID, userID string) (map[string]any, error) {
	return e.PlaybackInfoForMediaSource(ctx, mediaID, userID, "", false)
}

// PlaybackInfoForMediaSource honors the source selected by an Emby client. A
// real playback request also records that selection for the user's next item
// detail and PlaybackInfo request.
func (e *EmbyService) PlaybackInfoForMediaSource(
	ctx context.Context,
	mediaID string,
	userID string,
	requestedSourceID string,
	remember bool,
) (map[string]any, error) {
	m, err := e.playableMedia(ctx, mediaID, userID)
	if err != nil || m == nil {
		return nil, err
	}
	sources, err := e.orderMediaSourcesForUser(
		ctx,
		m,
		userID,
		e.mediaSourcesForItem(ctx, m, false, e.directPlayOnly(ctx)),
		requestedSourceID,
		remember,
	)
	if err != nil {
		return nil, err
	}
	return map[string]any{
		"MediaSources":  sources,
		"PlaySessionId": uuid.NewString(),
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
	if !e.mediaVisibility(ctx, userID).Allows(m) {
		return nil, nil
	}
	return m, nil
}

// mediaSource 是 /Items 与 /PlaybackInfo 共享的 MediaSource 结构。
//
// asEmbedded=true：嵌在 /Items 列表里，不包含完整 stream URL（避免暴露
// 直链给搜索接口）。/PlaybackInfo 走 false 路径，URL 指向 Emby 兼容
// /Videos/{id}/stream（客户端会继续携带 X-Emby-Token 或 append api_key）。
func (e *EmbyService) mediaSource(ctx context.Context, m *model.Media, asEmbedded, directOnly bool, probeDoc *ProbeDocument) map[string]any {
	container := embyMediaContainer(m)
	metadataMedia := e.embySTRMMetadataMedia(ctx, m)
	if (container == "" || container == "strm") && metadataMedia != nil && metadataMedia != m {
		container = embyMediaContainer(metadataMedia)
	}
	if e == nil || e.cfg == nil || !e.cfg.Transcoder.Enabled {
		directOnly = true
	}
	strmTarget, _ := mediaSTRMTarget(m)
	_, localSTRM := localSTRMTargetPath(m.Path, strmTarget)
	isCloud := strmTarget != "" && !localSTRM
	playURL := e.embyMediaPlayURL(ctx, m, container, isCloud)
	if isCloud || container == "iso" {
		// Cloud/WebDAV media is already a direct/proxy stream. Advertising HLS
		// transcoding makes some Emby clients pick /master.m3u8, forcing this
		// lightweight server to pull remote bytes through ffmpeg and often
		// surfacing as "network/playback failed". ISO images require a
		// disc-capable client. Neither should advertise the generic HLS path.
		directOnly = true
	}
	// 云盘媒体的 Path 保留源路径标识(不暴露带 token 的播放地址);本地 STRM
	// 保留目标文件路径;普通本地文件的 Path 使用 Emby 流地址,供按 Path
	// 直连的第三方播放器(Protocol=Http)使用。SupportsDirectPlay/DirectStream
	// 对云盘媒体在 playURL 可用时保持 true:Infuse/Emby 官方客户端会优先挑选
	// DirectPlay 源,标 false 可能被判定为"没有可播放媒体源"。
	src := e.baseMediaSource(m, metadataMedia, container, isCloud, playURL, directOnly)
	if isLocalSTRMFile(m.Path) && !mediaHasStoredTrackMetadata(metadataMedia) {
		src["MediaStreams"] = []map[string]any{}
	}
	if !asEmbedded {
		if probeDoc != nil {
			src["MediaStreams"] = embyProbeStreams(probeDoc)
			src["DefaultAudioStreamIndex"], src["DefaultSubtitleStreamIndex"] = probeDefaultStreamIndexes(probeDoc)
		}
	}
	if !strings.HasPrefix(strings.ToLower(strings.TrimSpace(m.Path)), "cloud://") {
		sourcePath := playURL
		if sourcePath == "" {
			sourcePath = embyDirectStreamURL(m.ID, container)
		}
		src["Path"] = sourcePath
	}
	if playURL != "" {
		src["DirectStreamUrl"] = playURL
		// 直连解码模式下不下发 TranscodingUrl，迫使客户端本地解码直连，
		// 宿主机不参与转码。
		if !asEmbedded && !directOnly {
			src["TranscodingUrl"] = "/Videos/" + m.ID + "/master.m3u8"
		}
	}
	if isCloud && playURL != "" {
		// STRM / cloud:// media must stay behind a token-aware endpoint. When
		// STRM playback is enabled we expose /api/stream so third-party clients
		// follow the same STRM entry as generated .strm files; when disabled we
		// expose /Videos/{id}/stream so playback uses the Emby 302/proxy path.
		src["IsRemote"] = true
	}
	if !asEmbedded {
		e.attachExternalSubtitleStreams(ctx, m, src)
	}
	return src
}

func mediaHasStoredTrackMetadata(m *model.Media) bool {
	return m != nil && (m.VideoCodec != "" || m.Width > 0 || m.Height > 0 || m.VideoBitRate > 0 || m.FrameRate > 0 || m.VideoProfile != "" || m.VideoRange != "" || m.VideoBitDepth > 0 || m.AudioCodec != "" || m.AudioBitRate > 0 || m.AudioChannels > 0 || m.AudioChannelLayout != "" || m.AudioSampleRate > 0)
}

func (e *EmbyService) baseMediaSource(m, metadataMedia *model.Media, container string, isCloud bool, playURL string, directOnly bool) map[string]any {
	if metadataMedia == nil {
		metadataMedia = m
	}
	size := metadataMedia.SizeBytes
	bitrateSize := metadataMedia.SizeBytes
	if target, err := mediaSTRMTarget(m); err == nil {
		if local, ok := localSTRMTargetPath(m.Path, target); ok {
			if _, info, err := resolveAccessibleMappedPath(local); err == nil && info.Mode().IsRegular() {
				size = info.Size()
				bitrateSize = info.Size()
			}
		}
	}
	if metadataMedia != m && !strings.HasPrefix(strings.ToLower(strings.TrimSpace(m.Path)), "cloud://") {
		if _, info, err := resolveAccessibleMappedPath(metadataMedia.Path); err == nil && info.Mode().IsRegular() {
			size = info.Size()
			bitrateSize = info.Size()
		}
	}
	bitrate := effectiveMediaBitRate(metadataMedia.BitRate, bitrateSize, metadataMedia.DurationSec)
	if isLocalSTRMFile(m.Path) {
		// The scanner stores the .strm text-file size on the media row. It must
		// not be used to invent a bitrate for the referenced video.
		bitrate = metadataMedia.BitRate
	}
	sourcePath := embyMediaSourcePath(m)
	if !strings.HasPrefix(strings.ToLower(strings.TrimSpace(m.Path)), "cloud://") && playURL != "" {
		sourcePath = playURL
	}
	return map[string]any{
		"Id":                    m.ID,
		"Name":                  m.Title,
		"Path":                  sourcePath,
		"Container":             container,
		"Size":                  size,
		"Bitrate":               bitrate,
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
		"RequiredHttpHeaders":   map[string]string{},
		"RunTimeTicks":          int64(metadataMedia.DurationSec) * 10_000_000,
		"MediaStreams":          e.mediaStreams(metadataMedia),
	}
}

// embyMediaSourcePath keeps Emby's Path field as a source identity rather
// than a playback endpoint. OpenList and other path-based cloud providers
// carry the real provider path in the internal cloud-play ref query. For
// opaque provider refs, fall back to the display path stored in cloud://.
func embyMediaSourcePath(m *model.Media) string {
	if m == nil {
		return ""
	}
	if _, ref, ok := parseCloudMediaPlaybackURL(m.STRMURL); ok && strings.HasPrefix(ref, "/") {
		return ref
	}
	raw := strings.TrimSpace(m.Path)
	u, err := url.Parse(raw)
	if err != nil || !strings.EqualFold(u.Scheme, "cloud") {
		return embyDirectStreamURL(m.ID, embyMediaContainer(m))
	}
	sourcePath := strings.TrimSpace(u.Path)
	if decoded, decodeErr := url.PathUnescape(sourcePath); decodeErr == nil {
		sourcePath = decoded
	}
	if sourcePath == "" {
		return "/"
	}
	if !strings.HasPrefix(sourcePath, "/") {
		sourcePath = "/" + sourcePath
	}
	return sourcePath
}

func embyMediaContainer(m *model.Media) string {
	// embyPlaybackContainer 归一化 ffprobe 风格的容器列表(如 "mov,mp4,m4a")
	// 与别名(matroska→mkv),并优先采用路径扩展名。
	container := embyPlaybackContainer(m.Container, m.Path)
	if container == "strm" || container == "" {
		if targetContainer := STRMTargetContainer(m); targetContainer != "" {
			return targetContainer
		}
	}
	if container == "" && strings.TrimSpace(m.STRMURL) != "" {
		return "strm"
	}
	return container
}

func (e *EmbyService) embyMediaPlayURL(ctx context.Context, m *model.Media, container string, isCloud bool) string {
	if !isCloud || CloudPlaybackMode(ctx, e.repo) != "" {
		return embyDirectStreamURL(m.ID, container)
	}
	return ""
}

func (e *EmbyService) embySTRMMetadataMedia(ctx context.Context, m *model.Media) *model.Media {
	if e == nil || e.repo == nil || e.repo.Media == nil || m == nil {
		return m
	}
	target, err := mediaSTRMTarget(m)
	if err != nil {
		return m
	}
	id, ok := internalSTRMStreamMediaID(target, embyPlaybackRequestFromContext(ctx), PublicServerURL(ctx, e.repo, e.cfg))
	if !ok || id == "" || id == m.ID {
		return m
	}
	metadata, err := e.repo.Media.FindByID(ctx, id)
	if err != nil || metadata == nil {
		return m
	}
	return metadata
}
