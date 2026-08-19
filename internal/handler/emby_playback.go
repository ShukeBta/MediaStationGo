package handler

import (
	"errors"
	"net/http"
	"net/url"
	"strings"

	"github.com/gin-gonic/gin"

	"github.com/ShukeBta/MediaStationGo/internal/service"
)

func embyPlaybackInfoHandler(svc *service.Container) gin.HandlerFunc {
	return func(c *gin.Context) {
		uid := c.Param("userId")
		if uid == "" {
			uid = embyUserID(c)
		}
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

// embySubtitleStreamHandler serves an external subtitle track advertised in a
// MediaSource's MediaStreams via its Emby index
// (/Videos/:id/Subtitles/:index/Stream). The index maps to a discovered
// sideloaded subtitle file next to the video (SRT/ASS/SSA/VTT, local or
// cloud://), following the same layout appended by mediaStreams.
func embySubtitleStreamHandler(svc *service.Container) gin.HandlerFunc {
	return func(c *gin.Context) {
		uid := c.Param("userId")
		if uid == "" {
			uid = embyUserID(c)
		}
		ctx := c.Request.Context()
		// The official-format route carries a :format suffix (Stream.ass /
		// Stream.vtt); prefer it for the Content-Type when present, otherwise
		// fall back to the discovered source codec. :mediaSourceId is ignored —
		// the item is located by :id and subtitles by :index (1:1 in this shim).
		format := strings.TrimSpace(c.Param("format"))
		codec := svc.Emby.SubtitleStreamCodec(ctx, c.Param("id"), c.Param("index"), uid)
		if codec != "" {
			if format != "" {
				codec = service.SubtitleCodecFromFormat(format)
			}
			c.Header("Content-Type", service.SubtitleContentType(codec))
		}
		c.Header("Cache-Control", "public, max-age=3600")
		if err := svc.Emby.ServeSubtitleStream(ctx, c.Writer, c.Param("id"), c.Param("index"), uid); err != nil {
			embyError(c, http.StatusNotFound, "subtitle not found")
			return
		}
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
		// Subtitle streams advertise a DeliveryUrl; the official Emby client
		// fetches it directly, so it must carry the auth token too.
		if streams, ok := source["MediaStreams"].([]map[string]any); ok {
			for _, stream := range streams {
				if stream["Type"] != "Subtitle" {
					continue
				}
				raw, ok := stream["DeliveryUrl"].(string)
				if !ok {
					continue
				}
				stream["DeliveryUrl"] = embyAppendAPIKey(raw, token)
			}
		}
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
	// 优先提取 Token="..." 引号内的纯 token。RodelPlayer 等客户端会把
	// UserId 和 Token 一起放进同一个 Emby/MediaBrowser 头里，例如
	// `Emby UserId="..", Client="..", Token="<jwt>"`。此时必须取 Token 引号内的
	// 纯 JWT，不能取整个头，否则 JWT 解析会因多余杂质失败。
	if strings.Contains(value, "Token=") {
		return embyTokenFromAuthHeaderTokenPart(value)
	}
	for _, prefix := range []string{"Bearer ", "Emby "} {
		if strings.HasPrefix(value, prefix) {
			return strings.TrimSpace(strings.TrimPrefix(value, prefix))
		}
	}
	// 其它格式（例如只带 Client/Device 信息的 MediaBrowser 头）不是令牌，
	// 不能整串返回，否则会被当作 JWT 解析导致 "Invalid token"。
	if strings.HasPrefix(value, "MediaBrowser ") || strings.HasPrefix(value, "Emby ") {
		return ""
	}
	return value
}

// embyTokenFromAuthHeaderTokenPart 从 "Token=..." 形如的字段中取出引号内的纯 token。
func embyTokenFromAuthHeaderTokenPart(value string) string {
	for _, part := range strings.Split(value, ",") {
		part = strings.TrimSpace(strings.TrimPrefix(strings.TrimSpace(part), "MediaBrowser "))
		if !strings.HasPrefix(part, "Token=") {
			continue
		}
		token := strings.TrimSpace(strings.TrimPrefix(part, "Token="))
		return strings.Trim(token, `"`)
	}
	return ""
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

// embyVideoStreamHandler 是 GET /Videos/{id}/stream 的入口，
// 直接代理到我们的 /api/stream/{id}（同一个 ServeFile）。
func embyVideoStreamHandler(svc *service.Container, cloudMode string) gin.HandlerFunc {
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
		if embyShouldRedirectVideoStreamToSTRM(c, svc, c.Param("id"), cloudMode) {
			target := "/api/stream/" + url.PathEscape(strings.TrimSpace(c.Param("id")))
			if token := embyPlaybackRedirectToken(c, svc); token != "" {
				target = embyAppendAPIKey(target, token)
			}
			setRedirectNoStoreHeaders(c)
			c.Redirect(http.StatusFound, absoluteRequestURL(c, target))
			return
		}
		// 直接调用 Stream service 写入 response。
		// 此前这里把所有错误一律吞成 404：云盘 Cookie 过期、直链解析失败、
		// STRM 播放被关闭……在第三方播放器上全部表现为「404 不存在」，
		// 无法排查。现在区分：行不存在→404；云盘播放不可用/上游故障→502+原因。
		err = svc.Stream.ServeFileWithCloudMode(c.Writer, c.Request, c.Param("id"), cloudMode)
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

func embyPlaybackRedirectToken(c *gin.Context, svc *service.Container) string {
	if token := embyRequestToken(c); token != "" {
		return token
	}
	if c == nil || svc == nil || svc.Auth == nil || svc.Repo == nil || svc.Repo.User == nil {
		return ""
	}
	uid := embyUserID(c)
	if uid == "" {
		return ""
	}
	u, err := svc.Repo.User.FindByID(c.Request.Context(), uid)
	if err != nil || u == nil {
		return ""
	}
	token, err := svc.Auth.IssueEmbyToken(u)
	if err != nil {
		return ""
	}
	return token
}

func embyShouldRedirectVideoStreamToSTRM(c *gin.Context, svc *service.Container, mediaID, cloudMode string) bool {
	if c == nil || svc == nil || svc.Repo == nil || svc.Repo.Media == nil || cloudMode != service.CloudPlaybackModeRedirectProxy {
		return false
	}
	settings := service.CloudPlaybackSettings(c.Request.Context(), svc.Repo)
	if settings.PreferredMode != service.CloudPlaybackModeSTRM || !settings.STRMEnabled {
		return false
	}
	m, err := svc.Repo.Media.FindByID(c.Request.Context(), mediaID)
	if err != nil || m == nil {
		return false
	}
	return strings.TrimSpace(m.STRMURL) != ""
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
