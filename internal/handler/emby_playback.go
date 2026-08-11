package handler

import (
	"errors"
	"net/http"
	"net/url"
	"strconv"
	"strings"

	"github.com/gin-gonic/gin"

	"github.com/ShukeBta/MediaStationGo/internal/service"
)

func embyPlaybackInfoHandler(svc *service.Container) gin.HandlerFunc {
	return func(c *gin.Context) {
		uid := embyUserID(c)
		out, err := svc.Emby.PlaybackInfo(c.Request.Context(), c.Param("id"), uid)
		if err != nil {
			c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
			return
		}
		if out == nil {
			embyError(c, http.StatusNotFound, "not found")
			return
		}
		embyAttachRequestTokenToMediaSources(c, out)
		c.JSON(http.StatusOK, out)
	}
}

func embyAttachRequestTokenToMediaSources(c *gin.Context, out any) {
	token := embyRequestToken(c)
	if token == "" || out == nil {
		return
	}
	embyAttachTokenToMediaSourcesValue(out, token)
}

func embyAttachTokenToMediaSourcesValue(value any, token string) {
	switch typed := value.(type) {
	case map[string]any:
		embyAttachTokenToMediaSourcesMap(typed, token)
	case gin.H:
		embyAttachTokenToMediaSourcesMap(map[string]any(typed), token)
	case []map[string]any:
		for _, item := range typed {
			embyAttachTokenToMediaSourcesMap(item, token)
		}
	case []any:
		for _, item := range typed {
			embyAttachTokenToMediaSourcesValue(item, token)
		}
	}
}

func embyAttachTokenToMediaSourcesMap(out map[string]any, token string) {
	if out == nil {
		return
	}
	if sources, ok := out["MediaSources"].([]map[string]any); ok {
		embyAttachTokenToMediaSources(sources, token)
	} else if sources, ok := out["MediaSources"].([]any); ok {
		for _, source := range sources {
			if sourceMap, ok := source.(map[string]any); ok {
				embyAttachTokenToMediaSources([]map[string]any{sourceMap}, token)
			}
		}
	}
	if items, ok := out["Items"]; ok {
		embyAttachTokenToMediaSourcesValue(items, token)
	}
}

func embyAttachTokenToMediaSources(sources []map[string]any, token string) {
	for _, source := range sources {
		for _, key := range []string{"DirectStreamUrl", "TranscodingUrl"} {
			raw, ok := source[key].(string)
			if !ok {
				continue
			}
			source[key] = embyAppendAPIKey(raw, token)
		}
		embyAttachTokenToMediaStreams(source, token)
	}
}

func embyAttachTokenToMediaStreams(source map[string]any, token string) {
	streams, ok := source["MediaStreams"].([]map[string]any)
	if !ok {
		return
	}
	for _, stream := range streams {
		raw, ok := stream["DeliveryUrl"].(string)
		if !ok || raw == "" {
			continue
		}
		stream["DeliveryUrl"] = embyAppendAPIKey(raw, token)
	}
}

func embyRequestToken(c *gin.Context) string {
	if c == nil {
		return ""
	}
	for _, key := range []string{"api_key", "apiKey", "ApiKey", "token", "X-Emby-Token", "X-MediaBrowser-Token"} {
		if value := strings.TrimSpace(c.Query(key)); value != "" {
			return value
		}
	}
	for _, header := range []string{"X-Emby-Token", "X-MediaBrowser-Token"} {
		if value := strings.TrimSpace(c.GetHeader(header)); value != "" {
			return value
		}
	}
	for _, header := range []string{"Authorization", "X-Emby-Authorization", "X-MediaBrowser-Authorization"} {
		if token := embyTokenFromAuthHeader(c.GetHeader(header)); token != "" {
			return token
		}
	}
	return ""
}

func embyTokenFromAuthHeader(value string) string {
	value = strings.TrimSpace(value)
	if value == "" {
		return ""
	}
	for _, prefix := range []string{"Bearer ", "Emby "} {
		if strings.HasPrefix(value, prefix) {
			return strings.TrimSpace(strings.TrimPrefix(value, prefix))
		}
	}
	for _, part := range strings.Split(value, ",") {
		part = strings.TrimSpace(strings.TrimPrefix(strings.TrimSpace(part), "MediaBrowser "))
		if !strings.HasPrefix(part, "Token=") {
			continue
		}
		token := strings.TrimSpace(strings.TrimPrefix(part, "Token="))
		return strings.Trim(token, `"`)
	}
	if strings.Contains(value, "Token=") {
		return ""
	}
	return value
}

func embyAppendAPIKey(raw, token string) string {
	raw = strings.TrimSpace(raw)
	token = strings.TrimSpace(token)
	if raw == "" || token == "" {
		return raw
	}
	if strings.HasPrefix(raw, "//") {
		return raw
	}
	u, err := url.Parse(raw)
	if err != nil || u.IsAbs() {
		return raw
	}
	q := u.Query()
	if q.Get("api_key") == "" && q.Get("apiKey") == "" && q.Get("token") == "" {
		q.Set("api_key", token)
		u.RawQuery = q.Encode()
	}
	return u.String()
}

// embyVideoStreamHandler serves every Emby-compatible direct-stream route
// through the same configuration-driven StreamService path.
func embyVideoStreamHandler(svc *service.Container) gin.HandlerFunc {
	return func(c *gin.Context) {
		uid := embyUserID(c)
		item, err := svc.Emby.Item(c.Request.Context(), c.Param("id"), uid)
		if err != nil {
			c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
			return
		}
		if item == nil {
			c.Status(http.StatusNotFound)
			return
		}
		// 直接调用 Stream service 写入 response。
		// 此前这里把所有错误一律吞成 404：云盘 Cookie 过期、直链解析失败、
		// STRM 播放被关闭……在第三方播放器上全部表现为「404 不存在」，
		// 无法排查。现在区分：行不存在→404；云盘播放不可用/上游故障→502+原因。
		err = svc.Stream.ServeFileForUser(c.Writer, c.Request, c.Param("id"), uid)
		switch {
		case err == nil:
		case errors.Is(err, service.ErrMediaNotFound):
			c.Status(http.StatusNotFound)
		case errors.Is(err, service.ErrCloudPlaybackDisabled):
			if !c.Writer.Written() {
				c.JSON(http.StatusConflict, gin.H{"error": err.Error()})
			}
		default:
			if !c.Writer.Written() {
				c.JSON(http.StatusBadGateway, gin.H{"error": err.Error()})
			}
		}
	}
}

func embySubtitleStreamHandler(svc *service.Container) gin.HandlerFunc {
	return func(c *gin.Context) {
		trackIndex, err := strconv.Atoi(strings.TrimSpace(c.DefaultQuery("mp_track", "0")))
		if err != nil {
			c.Status(http.StatusNotFound)
			return
		}
		serveEmbySubtitleTrack(svc, c, c.Param("id"), trackIndex, c.Param("format"))
	}
}

func embyLegacySubtitleStreamHandler(svc *service.Container) gin.HandlerFunc {
	return func(c *gin.Context) {
		if rawTrackIndex := strings.TrimSpace(c.Query("mp_track")); rawTrackIndex != "" {
			trackIndex, err := strconv.Atoi(rawTrackIndex)
			if err != nil {
				c.Status(http.StatusNotFound)
				return
			}
			serveEmbySubtitleTrack(svc, c, c.Param("seg"), trackIndex, c.Param("format"))
			return
		}
		streamIndex, err := strconv.Atoi(strings.TrimSpace(c.Param("stream")))
		if err != nil {
			c.Status(http.StatusNotFound)
			return
		}
		serveEmbyStandardSubtitleStream(svc, c, c.Param("id"), c.Param("seg"), streamIndex, c.Param("format"))
	}
}

func serveEmbyStandardSubtitleStream(svc *service.Container, c *gin.Context, itemID, sourceID string, streamIndex int, format string) {
	uid := embyUserID(c)
	item, err := svc.Emby.Item(c.Request.Context(), itemID, uid)
	if err != nil || item == nil || svc.Subtitle == nil {
		c.Status(http.StatusNotFound)
		return
	}
	trackIndex, ok := embyExternalSubtitleTrackIndex(item, sourceID, streamIndex)
	if !ok {
		c.Status(http.StatusNotFound)
		return
	}
	serveEmbySubtitleTrack(svc, c, sourceID, trackIndex, format)
}

func embyExternalSubtitleTrackIndex(item map[string]any, sourceID string, streamIndex int) (int, bool) {
	sources, ok := item["MediaSources"].([]map[string]any)
	if !ok {
		return 0, false
	}
	sourceID = strings.TrimSpace(sourceID)
	for _, source := range sources {
		id, _ := source["Id"].(string)
		if strings.TrimSpace(id) != sourceID {
			continue
		}
		streams, ok := source["MediaStreams"].([]map[string]any)
		if !ok {
			return 0, false
		}
		trackIndex := 0
		for _, stream := range streams {
			if stream["Type"] != "Subtitle" || stream["IsExternal"] != true {
				continue
			}
			index, ok := stream["Index"].(int)
			if ok && index == streamIndex {
				return trackIndex, true
			}
			trackIndex++
		}
		return 0, false
	}
	return 0, false
}

func serveEmbySubtitleTrack(svc *service.Container, c *gin.Context, mediaID string, trackIndex int, format string) {
	uid := embyUserID(c)
	item, err := svc.Emby.Item(c.Request.Context(), mediaID, uid)
	if err != nil || item == nil || svc.Subtitle == nil {
		c.Status(http.StatusNotFound)
		return
	}
	tracks, err := svc.Subtitle.Discover(c.Request.Context(), mediaID)
	if err != nil || len(tracks) == 0 {
		c.Status(http.StatusNotFound)
		return
	}
	if trackIndex < 0 || trackIndex >= len(tracks) {
		c.Status(http.StatusNotFound)
		return
	}
	contentType, ok := service.SubtitleContentType(format)
	if !ok {
		c.Status(http.StatusNotFound)
		return
	}
	c.Header("Content-Type", contentType)
	c.Header("Cache-Control", "public, max-age=3600")
	if c.Request.Method == http.MethodHead {
		c.Status(http.StatusOK)
		return
	}
	if err := svc.Subtitle.ServeAs(c.Request.Context(), mediaID, tracks[trackIndex].Path, format, c.Writer); err != nil {
		c.Status(http.StatusNotFound)
	}
}

func embyVideoHLSPlaylistHandler(svc *service.Container) gin.HandlerFunc {
	return func(c *gin.Context) {
		uid := embyUserID(c)
		item, err := svc.Emby.Item(c.Request.Context(), c.Param("id"), uid)
		if err != nil || item == nil || svc.Stream == nil {
			c.Status(http.StatusNotFound)
			return
		}
		err = svc.Stream.ServeHLSPlaylist(c.Writer, c.Request, c.Param("id"))
		if errors.Is(err, service.ErrTranscodeDisabled) {
			c.JSON(http.StatusConflict, gin.H{"error": "transcode disabled"})
			return
		}
		if errors.Is(err, service.ErrTranscodeBusy) {
			c.JSON(http.StatusTooManyRequests, gin.H{"error": "transcode busy"})
			return
		}
		if err != nil {
			c.Status(http.StatusNotFound)
		}
	}
}

func embyVideoHLSSegmentHandler(svc *service.Container) gin.HandlerFunc {
	return func(c *gin.Context) {
		uid := embyUserID(c)
		item, err := svc.Emby.Item(c.Request.Context(), c.Param("id"), uid)
		if err != nil || item == nil || svc.Stream == nil {
			c.Status(http.StatusNotFound)
			return
		}
		if err := svc.Stream.ServeHLSSegment(c.Writer, c.Request, c.Param("id"), c.Param("seg")); err != nil {
			c.Status(http.StatusNotFound)
		}
	}
}
