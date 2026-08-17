package handler

import (
	"strings"
	"sync"
	"time"

	"github.com/gin-gonic/gin"

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

const embyCompatSessionTTL = 30 * time.Minute

type embyCompatSession struct {
	token     string
	expiresAt time.Time
}

var embyCompatSessions = struct {
	sync.RWMutex
	items map[string]embyCompatSession
}{items: map[string]embyCompatSession{}}

func embyAuthRequiredWithSessionFallback(secret string) gin.HandlerFunc {
	required := middleware.EmbyAuthRequired(secret)
	return func(c *gin.Context) {
		// 兼容小幻影视等客户端的「UserId 直连」凭据格式：
		// token 形如 X-Emby-Token=UserId="<uuid>"，不是 JWT。解析出 uuid
		// 注入 CtxUserID，交由后续 activeEmbyUserRequired 查库验证用户
		// 存在且有效（未禁用/未过期），避免这类客户端每次 401 Invalid token。
		if uid := userIdDirectToken(c); uid != "" {
			c.Set(middleware.CtxUserID, uid)
			c.Set(middleware.EmbyCtxUserID, uid)
			c.Next()
			return
		}
		if embyRequestToken(c) == "" {
			if token := embyCompatSessionToken(c); token != "" {
				c.Request.Header.Set("X-Emby-Token", token)
			}
		}
		required(c)
	}
}

// userIdDirectToken 从 Emby token 来源中识别形如 UserId="<uuid>" 的直连凭据，
// 返回其中的 uuid；不是该格式则返回空串。用于兼容小幻影视（RodelPlayer）等
// 客户端把用户 ID 当作 token 提交的行为。
// 识别三种来源：
//  1. URL query：X-Emby-Token=UserId="<uuid>"
//  2. Authorization / X-Emby-Authorization / X-MediaBrowser-Authorization 头：
//     Emby UserId="<uuid>", Client="...", ...（无 Token=）
//  3. X-Emby-Token / X-MediaBrowser-Token 头直传 UserId="<uuid>"
func userIdDirectToken(c *gin.Context) string {
	if c == nil || c.Request == nil {
		return ""
	}
	const prefix = `UserId="`
	// 从一段文本中提取 UserId="<uuid>" 中的 uuid；不存在则返回空。
	extract := func(raw string) string {
		raw = strings.TrimSpace(raw)
		if raw == "" {
			return ""
		}
		idx := strings.Index(raw, prefix)
		if idx < 0 {
			return ""
		}
		rest := raw[idx+len(prefix):]
		end := strings.Index(rest, `"`)
		if end <= 0 {
			return ""
		}
		uid := strings.TrimSpace(rest[:end])
		if len(uid) > 0 && len(uid) <= 64 {
			return uid
		}
		return ""
	}
	// 1) URL query 参数直传 UserId="..."。
	for _, key := range []string{"X-Emby-Token", "X-MediaBrowser-Token", "token", "api_key", "apiKey", "ApiKey"} {
		if uid := extract(c.Query(key)); uid != "" {
			return uid
		}
	}
	// 2) 认证头中的 Emby/MediaBrowser UserId="..."（无 Token= 的直连凭据）。
	for _, header := range []string{"Authorization", "X-Emby-Authorization", "X-MediaBrowser-Authorization"} {
		if uid := extract(c.GetHeader(header)); uid != "" {
			// 仅当该头不是标准 Token= 形式时才当作直连凭据，避免误拦截。
			if !strings.Contains(c.GetHeader(header), "Token=") {
				return uid
			}
		}
	}
	// 3) X-Emby-Token / X-MediaBrowser-Token 头直传 UserId="..."。
	for _, header := range []string{"X-Emby-Token", "X-MediaBrowser-Token"} {
		if uid := extract(c.GetHeader(header)); uid != "" {
			return uid
		}
	}
	return ""
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

func embyRememberCompatSession(c *gin.Context, token string) {
	token = strings.TrimSpace(token)
	if token == "" {
		return
	}
	keys := embyCompatSessionKeys(c)
	if len(keys) == 0 {
		return
	}
	expiresAt := time.Now().Add(embyCompatSessionTTL)
	embyCompatSessions.Lock()
	defer embyCompatSessions.Unlock()
	if len(embyCompatSessions.items) > 1000 {
		now := time.Now()
		for key, session := range embyCompatSessions.items {
			if now.After(session.expiresAt) {
				delete(embyCompatSessions.items, key)
			}
		}
		if len(embyCompatSessions.items) > 1000 {
			embyCompatSessions.items = map[string]embyCompatSession{}
		}
	}
	for _, key := range keys {
		embyCompatSessions.items[key] = embyCompatSession{token: token, expiresAt: expiresAt}
	}
}

func embyCompatSessionToken(c *gin.Context) string {
	keys := embyCompatSessionKeys(c)
	if len(keys) == 0 {
		return ""
	}
	now := time.Now()
	embyCompatSessions.RLock()
	defer embyCompatSessions.RUnlock()
	for _, key := range keys {
		session, ok := embyCompatSessions.items[key]
		if ok && now.Before(session.expiresAt) {
			return session.token
		}
	}
	return ""
}

func embyCompatSessionKeys(c *gin.Context) []string {
	if c == nil {
		return nil
	}
	ip := strings.TrimSpace(c.ClientIP())
	if ip == "" {
		return nil
	}
	keys := []string{}
	add := func(kind, value string) {
		value = strings.TrimSpace(value)
		if value != "" {
			keys = append(keys, ip+"\x00"+kind+"\x00"+value)
		}
	}
	add("device", firstHeaderValue(c, "X-Emby-Device-Id", "X-Emby-DeviceId", "X-MediaBrowser-Device-Id", "X-MediaBrowser-DeviceId"))
	add("ua", c.GetHeader("User-Agent"))
	return keys
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
