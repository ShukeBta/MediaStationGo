package handler

import (
	"net/http"
	"strings"

	"github.com/gin-gonic/gin"
	"go.uber.org/zap"

	"github.com/ShukeBta/MediaStationGo/internal/middleware"
	"github.com/ShukeBta/MediaStationGo/internal/service"
)

// embyError 返回 Emby 风格的错误（顶层 Code/Message）。
func embyError(c *gin.Context, status int, msg string) {
	c.JSON(status, gin.H{"Code": status, "Message": msg})
}

// embyUserID 从中间件中获取 user id。Emby auth middleware 写入 CtxUserID。
func embyUserID(c *gin.Context) string {
	if uid, ok := c.Get(middleware.CtxUserID); ok {
		if s, ok := uid.(string); ok {
			return s
		}
	}
	return ""
}

const (
	embyIncomingAuthSourceContextKey = "emby_incoming_auth_source"
	embyIncomingTokenShapeContextKey = "emby_incoming_token_shape"
)

// Every request must present its own credential. IP, device IDs and user
// agents are client-controlled/shared metadata, never proof of a session.
func embyAuthRequiredWithDiagnostics(secret string, log *zap.Logger) gin.HandlerFunc {
	required := middleware.EmbyAuthRequired(secret)
	return func(c *gin.Context) {
		incomingAuthSource := embyRequestAuthSource(c)
		incomingToken := embyRequestToken(c)
		incomingTokenShape := embyCredentialShape(incomingToken)
		c.Set(embyIncomingAuthSourceContextKey, incomingAuthSource)
		c.Set(embyIncomingTokenShapeContextKey, incomingTokenShape)
		if isEmbyExternalSubtitleStreamPath(c.Request.URL.Path) && log != nil {
			defer func() {
				log.Info("emby subtitle request auth diagnostic",
					zap.String("event", "emby_subtitle_request_auth"),
					zap.String("path", c.Request.URL.Path),
					zap.Int("status", c.Writer.Status()),
					zap.String("incoming_auth_source", incomingAuthSource),
					zap.String("incoming_token_shape", incomingTokenShape),
					zap.String("query_api_key_shape", embyCredentialShape(c.Query("api_key"))),
				)
			}()
		}
		required(c)
	}
}

func embyIncomingAuthDiagnostics(c *gin.Context) (source, shape string) {
	if c == nil {
		return "none", "missing"
	}
	if value, ok := c.Get(embyIncomingAuthSourceContextKey); ok {
		source, _ = value.(string)
	}
	if value, ok := c.Get(embyIncomingTokenShapeContextKey); ok {
		shape, _ = value.(string)
	}
	if source == "" {
		source = embyRequestAuthSource(c)
	}
	if shape == "" {
		shape = embyCredentialShape(embyRequestToken(c))
	}
	return source, shape
}

func isEmbyExternalSubtitleStreamPath(path string) bool {
	path = strings.ToLower(strings.TrimSpace(path))
	return strings.Contains(path, "/videos/") && strings.Contains(path, "/subtitles/")
}

func embyAuthenticatedUserScopeRequired() gin.HandlerFunc {
	return func(c *gin.Context) {
		authenticatedUserID := embyUserID(c)
		if authenticatedUserID == "" {
			c.AbortWithStatusJSON(http.StatusUnauthorized, gin.H{"Code": 40101, "Message": "User not found"})
			return
		}
		if requestedUserID := strings.TrimSpace(c.Param("userId")); requestedUserID != "" && requestedUserID != authenticatedUserID {
			c.AbortWithStatusJSON(http.StatusForbidden, gin.H{"Code": 40304, "Message": "User scope denied"})
			return
		}
		for _, key := range []string{"UserId", "userId", "userid"} {
			if requestedUserID := strings.TrimSpace(c.Query(key)); requestedUserID != "" && requestedUserID != authenticatedUserID {
				c.AbortWithStatusJSON(http.StatusForbidden, gin.H{"Code": 40304, "Message": "User scope denied"})
				return
			}
		}
		c.Next()
	}
}

func embyRealtimeSessionActivity(svc *service.Container) gin.HandlerFunc {
	return func(c *gin.Context) {
		recordEmbySessionActivity(c, svc, embyUserID(c), embyContextUserName(c))
		c.Next()
	}
}

func recordEmbySessionActivity(c *gin.Context, svc *service.Container, userID, userName string) {
	if c == nil || svc == nil || svc.Sessions == nil || strings.TrimSpace(userID) == "" {
		return
	}
	clientInfo := embyClientInfoFromRequest(c)
	svc.Sessions.RecordActivity(c.Request.Context(), userID, userName,
		clientInfo.DeviceID,
		clientInfo.DeviceName,
		clientInfo.Client,
		c.ClientIP())
}

func embyContextUserName(c *gin.Context) string {
	if c == nil {
		return ""
	}
	if value, ok := c.Get(embyCtxUserName); ok {
		if username, ok := value.(string); ok {
			return strings.TrimSpace(username)
		}
	}
	return ""
}

func firstHeaderValue(c *gin.Context, names ...string) string {
	for _, name := range names {
		if value := strings.TrimSpace(c.GetHeader(name)); value != "" {
			return value
		}
	}
	return ""
}

type embyClientInfo struct {
	DeviceID   string
	DeviceName string
	Client     string
}

func embyClientInfoFromRequest(c *gin.Context) embyClientInfo {
	auth := parseMediaBrowserAuthorization(firstHeaderValue(c,
		"X-Emby-Authorization",
		"X-MediaBrowser-Authorization",
		"Authorization",
	))
	info := embyClientInfo{
		DeviceID: firstNonEmptyHeaderString(
			firstHeaderValue(c, "X-Emby-Device-Id", "X-Emby-DeviceId", "X-MediaBrowser-Device-Id", "X-MediaBrowser-DeviceId"),
			c.Query("DeviceId"),
			c.Query("DeviceID"),
			c.Query("deviceId"),
			c.Query("deviceID"),
			auth["DeviceId"],
			auth["DeviceID"],
		),
		DeviceName: firstNonEmptyHeaderString(
			firstHeaderValue(c, "X-Emby-Device-Name", "X-Emby-DeviceName", "X-MediaBrowser-Device-Name", "X-MediaBrowser-DeviceName"),
			c.Query("Device"),
			c.Query("DeviceName"),
			c.Query("device"),
			c.Query("deviceName"),
			auth["Device"],
		),
		Client: firstNonEmptyHeaderString(
			firstHeaderValue(c, "X-Emby-Client", "X-MediaBrowser-Client"),
			c.Query("Client"),
			c.Query("client"),
			auth["Client"],
		),
	}
	ua := strings.TrimSpace(c.GetHeader("User-Agent"))
	if info.Client == "" {
		info.Client = embyClientFromUserAgent(ua)
	}
	if info.DeviceName == "" {
		info.DeviceName = embyDeviceFromUserAgent(ua)
	}
	return info
}

func parseMediaBrowserAuthorization(raw string) map[string]string {
	out := map[string]string{}
	raw = strings.TrimSpace(raw)
	if raw == "" {
		return out
	}
	for _, prefix := range []string{"MediaBrowser ", "Emby "} {
		if strings.HasPrefix(raw, prefix) {
			raw = strings.TrimSpace(strings.TrimPrefix(raw, prefix))
			break
		}
	}
	for _, part := range strings.Split(raw, ",") {
		key, value, ok := strings.Cut(strings.TrimSpace(part), "=")
		if !ok {
			continue
		}
		key = strings.TrimSpace(key)
		value = strings.Trim(strings.TrimSpace(value), `"`)
		if key != "" && value != "" {
			out[key] = value
		}
	}
	return out
}

func firstNonEmptyHeaderString(values ...string) string {
	for _, value := range values {
		if strings.TrimSpace(value) != "" {
			return strings.TrimSpace(value)
		}
	}
	return ""
}

func embyClientFromUserAgent(ua string) string {
	ua = strings.TrimSpace(ua)
	lower := strings.ToLower(ua)
	switch {
	case strings.Contains(lower, "infuse"):
		return "Infuse"
	case strings.Contains(lower, "emby"):
		return "Emby"
	case strings.Contains(lower, "jellyfin"):
		return "Jellyfin"
	case strings.Contains(lower, "yamby"):
		return "Yamby"
	case strings.Contains(lower, "vidhub"):
		return "VidHub"
	case strings.Contains(lower, "hills"):
		return "Hills"
	default:
		return ua
	}
}

func embyDeviceFromUserAgent(ua string) string {
	lower := strings.ToLower(strings.TrimSpace(ua))
	switch {
	case strings.Contains(lower, "android"):
		return "Android"
	case strings.Contains(lower, "iphone"):
		return "iPhone"
	case strings.Contains(lower, "ipad"):
		return "iPad"
	case strings.Contains(lower, "ios"):
		return "iOS"
	case strings.Contains(lower, "windows"):
		return "Windows PC"
	case strings.Contains(lower, "macintosh") || strings.Contains(lower, "mac os"):
		return "Mac"
	case strings.Contains(lower, "linux"):
		return "Linux PC"
	case strings.Contains(lower, "appletv") || strings.Contains(lower, "apple tv"):
		return "Apple TV"
	default:
		return ""
	}
}
