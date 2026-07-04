// Package handler — Emby/Jellyfin compatibility shim.
//
// 路由挂在 /emby/* 和根路径下双前缀。Infuse / Yamby / Hills /
// Senplayer / Kodi 这类客户端会自动尝试 /System/Info 与 /emby/System/Info
// 两种 URL，我们都接住。
package handler

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/golang-jwt/jwt/v5"
	"github.com/gorilla/websocket"
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
		if embyRequestToken(c) == "" {
			if token := embyCompatSessionToken(c); token != "" {
				c.Request.Header.Set("X-Emby-Token", token)
			}
		}
		required(c)
	}
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
			auth["DeviceId"],
			auth["DeviceID"],
		),
		DeviceName: firstNonEmptyHeaderString(
			firstHeaderValue(c, "X-Emby-Device-Name", "X-Emby-DeviceName", "X-MediaBrowser-Device-Name", "X-MediaBrowser-DeviceName"),
			auth["Device"],
		),
		Client: firstNonEmptyHeaderString(
			firstHeaderValue(c, "X-Emby-Client", "X-MediaBrowser-Client"),
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

func embyLogClientRequest(svc *service.Container, c *gin.Context, event, mediaID, userID string, extra ...zap.Field) {
	embyLogClientRequestWithLevel(svc, c, event, mediaID, userID, false, extra...)
}

func embyLogClientRequestDebug(svc *service.Container, c *gin.Context, event, mediaID, userID string, extra ...zap.Field) {
	embyLogClientRequestWithLevel(svc, c, event, mediaID, userID, true, extra...)
}

func embyLogClientRequestWithLevel(svc *service.Container, c *gin.Context, event, mediaID, userID string, debug bool, extra ...zap.Field) {
	if svc == nil || svc.Log == nil || c == nil || c.Request == nil {
		return
	}
	info := embyClientInfoFromRequest(c)
	fields := []zap.Field{
		zap.String("media_id", strings.TrimSpace(mediaID)),
		zap.String("user_id", strings.TrimSpace(userID)),
		zap.String("client", strings.TrimSpace(info.Client)),
		zap.String("device_id", strings.TrimSpace(info.DeviceID)),
		zap.String("device_name", strings.TrimSpace(info.DeviceName)),
		zap.String("user_agent", strings.TrimSpace(c.GetHeader("User-Agent"))),
		zap.String("ip", c.ClientIP()),
		zap.String("method", c.Request.Method),
		zap.String("path", c.Request.URL.Path),
	}
	fields = append(fields, extra...)
	if debug {
		svc.Log.Debug(event, fields...)
		return
	}
	svc.Log.Info(event, fields...)
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

// ─── System ──────────────────────────────────────────────────────────────────

func embySystemInfoHandler(svc *service.Container) gin.HandlerFunc {
	return func(c *gin.Context) {
		c.JSON(http.StatusOK, embyWithRequestAddress(c, svc.Emby.SystemInfo()))
	}
}

func embySystemInfoPublicHandler(svc *service.Container) gin.HandlerFunc {
	return func(c *gin.Context) {
		c.JSON(http.StatusOK, embyWithRequestAddress(c, svc.Emby.SystemInfoPublic()))
	}
}

func embyRequestBaseURL(c *gin.Context) string {
	proto := strings.TrimSpace(c.GetHeader("X-Forwarded-Proto"))
	if proto == "" {
		if c.Request != nil && c.Request.TLS != nil {
			proto = "https"
		} else {
			proto = "http"
		}
	}
	if comma := strings.Index(proto, ","); comma >= 0 {
		proto = strings.TrimSpace(proto[:comma])
	}

	host := strings.TrimSpace(c.GetHeader("X-Forwarded-Host"))
	if host == "" && c.Request != nil {
		host = strings.TrimSpace(c.Request.Host)
	}
	if host == "" {
		return ""
	}
	return strings.TrimRight(proto+"://"+host, "/")
}

func embyWithRequestAddress(c *gin.Context, payload map[string]any) map[string]any {
	out := make(map[string]any, len(payload)+2)
	for key, value := range payload {
		out[key] = value
	}
	if address := embyRequestBaseURL(c); address != "" {
		out["LocalAddress"] = address
		out["WanAddress"] = address
		out["PublishedServerUrl"] = address
	}
	return out
}

func embySystemEndpointHandler(_ *service.Container) gin.HandlerFunc {
	return func(c *gin.Context) {
		c.JSON(http.StatusOK, gin.H{
			"IsLocal":     true,
			"IsInNetwork": true,
		})
	}
}

func embyPingHandler(_ *service.Container) gin.HandlerFunc {
	return func(c *gin.Context) {
		// Emby/Jellyfin 期望 plain text "Emby Server"
		c.String(http.StatusOK, "Emby Server")
	}
}

func embyRootHandler(svc *service.Container) gin.HandlerFunc {
	return func(c *gin.Context) {
		c.JSON(http.StatusOK, embyPublicSystemInfoPayload(c, svc))
	}
}

func embyPublicSystemInfoPayload(c *gin.Context, svc *service.Container) map[string]any {
	if svc != nil && svc.Emby != nil {
		return embyWithRequestAddress(c, svc.Emby.SystemInfoPublic())
	}
	return embyWithRequestAddress(c, map[string]any{
		"Id":                     "mediastation-go-001",
		"ServerId":               "mediastation-go-001",
		"ServerName":             "MediaStationGo",
		"Version":                "4.8.10.0",
		"ServerVersion":          "4.8.10.0",
		"ProductName":            "Emby Server",
		"OperatingSystem":        "Windows",
		"SupportsHttps":          false,
		"SupportsAutoDiscovery":  true,
		"StartupWizardCompleted": true,
	})
}

// ─── Users / Auth ────────────────────────────────────────────────────────────

type embyAuthByNameReq struct {
	Username     string `json:"Username"`
	Pw           string `json:"Pw"`
	Password     string `json:"Password"`
	PasswordMd5  string `json:"PasswordMd5"`
	PasswordSha1 string `json:"PasswordSha1"`
}

func parseEmbyAuthByNameReq(c *gin.Context) (embyAuthByNameReq, error) {
	req := embyAuthByNameReq{}
	if strings.Contains(strings.ToLower(c.GetHeader("Content-Type")), "json") {
		var body map[string]any
		if err := c.ShouldBindJSON(&body); err != nil && !errors.Is(err, io.EOF) {
			return req, err
		}
		fillEmbyAuthFromMap(&req, body)
	}

	if req.Username == "" || (req.Pw == "" && req.Password == "" && req.PasswordMd5 == "" && req.PasswordSha1 == "") {
		_ = c.Request.ParseForm()
		if req.Username == "" {
			req.Username = firstFormValue(c, "Username", "username", "Name", "name")
		}
		if req.Pw == "" {
			req.Pw = firstFormValue(c, "Pw", "pw")
		}
		if req.Password == "" {
			req.Password = firstFormValue(c, "Password", "password")
		}
		if req.PasswordMd5 == "" {
			req.PasswordMd5 = firstFormValue(c, "PasswordMd5", "passwordMd5", "password_md5")
		}
		if req.PasswordSha1 == "" {
			req.PasswordSha1 = firstFormValue(c, "PasswordSha1", "passwordSha1", "password_sha1")
		}
	}

	if req.Username == "" {
		req.Username = firstQueryValue(c, "Username", "username", "Name", "name")
	}
	if req.Pw == "" {
		req.Pw = firstQueryValue(c, "Pw", "pw")
	}
	if req.Password == "" {
		req.Password = firstQueryValue(c, "Password", "password")
	}
	if req.PasswordMd5 == "" {
		req.PasswordMd5 = firstQueryValue(c, "PasswordMd5", "passwordMd5", "password_md5")
	}
	if req.PasswordSha1 == "" {
		req.PasswordSha1 = firstQueryValue(c, "PasswordSha1", "passwordSha1", "password_sha1")
	}
	if req.Username == "" || (req.Pw == "" && req.Password == "" && req.PasswordMd5 == "" && req.PasswordSha1 == "") {
		fillEmbyAuthFromRawBody(c, &req)
	}
	return req, nil
}

func fillEmbyAuthFromMap(req *embyAuthByNameReq, body map[string]any) {
	if req.Username == "" {
		req.Username = firstStringFromMap(body, "Username", "username", "UserName", "userName", "Name", "name", "LoginName", "loginName")
	}
	if req.Pw == "" {
		req.Pw = firstStringFromMap(body, "Pw", "pw", "PW")
	}
	if req.Password == "" {
		req.Password = firstStringFromMap(body, "Password", "password", "Pass", "pass", "Pwd", "pwd")
	}
	if req.PasswordMd5 == "" {
		req.PasswordMd5 = firstStringFromMap(body, "PasswordMd5", "passwordMd5", "password_md5")
	}
	if req.PasswordSha1 == "" {
		req.PasswordSha1 = firstStringFromMap(body, "PasswordSha1", "passwordSha1", "password_sha1")
	}
}

func fillEmbyAuthFromRawBody(c *gin.Context, req *embyAuthByNameReq) {
	if c.Request == nil || c.Request.Body == nil {
		return
	}
	raw, err := io.ReadAll(io.LimitReader(c.Request.Body, 1<<20))
	if err != nil {
		return
	}
	c.Request.Body = io.NopCloser(bytes.NewReader(raw))
	raw = bytes.TrimSpace(raw)
	if len(raw) == 0 {
		return
	}
	if bytes.HasPrefix(raw, []byte("{")) {
		var body map[string]any
		if err := json.Unmarshal(raw, &body); err == nil {
			fillEmbyAuthFromMap(req, body)
		}
		return
	}
	if values, err := url.ParseQuery(string(raw)); err == nil {
		fillEmbyAuthFromValues(req, values)
	}
}

func fillEmbyAuthFromValues(req *embyAuthByNameReq, values url.Values) {
	if req.Username == "" {
		req.Username = firstValue(values, "Username", "username", "UserName", "userName", "Name", "name", "LoginName", "loginName")
	}
	if req.Pw == "" {
		req.Pw = firstValue(values, "Pw", "pw", "PW")
	}
	if req.Password == "" {
		req.Password = firstValue(values, "Password", "password", "Pass", "pass", "Pwd", "pwd")
	}
	if req.PasswordMd5 == "" {
		req.PasswordMd5 = firstValue(values, "PasswordMd5", "passwordMd5", "password_md5")
	}
	if req.PasswordSha1 == "" {
		req.PasswordSha1 = firstValue(values, "PasswordSha1", "passwordSha1", "password_sha1")
	}
}

func firstValue(values url.Values, keys ...string) string {
	for _, key := range keys {
		if value := strings.TrimSpace(values.Get(key)); value != "" {
			return value
		}
	}
	return ""
}

func firstStringFromMap(body map[string]any, keys ...string) string {
	if len(body) == 0 {
		return ""
	}
	for _, key := range keys {
		if value, ok := body[key]; ok {
			if s, ok := value.(string); ok {
				return strings.TrimSpace(s)
			}
		}
	}
	return ""
}

func firstFormValue(c *gin.Context, keys ...string) string {
	for _, key := range keys {
		if values, ok := c.Request.PostForm[key]; ok && len(values) > 0 {
			if value := strings.TrimSpace(values[0]); value != "" {
				return value
			}
		}
	}
	return ""
}

func firstQueryValue(c *gin.Context, keys ...string) string {
	for _, key := range keys {
		if value := strings.TrimSpace(c.Query(key)); value != "" {
			return value
		}
	}
	return ""
}

// embyAuthByNameHandler 处理 POST /Users/AuthenticateByName。
//
// 这是 Emby 客户端登录的唯一入口（Infuse / Yamby / Hills 等都走这里）。
// 用户名+密码 → 调用我们已有的 AuthService.Login → 返回 AccessToken + User。
func embyAuthByNameHandler(svc *service.Container) gin.HandlerFunc {
	return func(c *gin.Context) {
		req, err := parseEmbyAuthByNameReq(c)
		if err != nil {
			embyError(c, http.StatusBadRequest, "invalid body")
			return
		}
		password := req.Pw
		if password == "" {
			password = req.Password
		}
		if strings.TrimSpace(req.Username) == "" || password == "" {
			if req.PasswordMd5 != "" || req.PasswordSha1 != "" {
				embyError(c, http.StatusBadRequest, "plain password required")
				return
			}
			embyError(c, http.StatusBadRequest, "missing username or password")
			return
		}
		resp, err := svc.Auth.Login(c.Request.Context(), req.Username, password)
		if err != nil {
			embyError(c, http.StatusUnauthorized, err.Error())
			return
		}
		// 记录登录设备会话并执行防共享检测（登录客户端数 / 设备指纹）。
		clientInfo := embyClientInfoFromRequest(c)
		if svc.Device != nil {
			svc.Device.RecordLogin(c.Request.Context(), resp.User.ID,
				clientInfo.DeviceID,
				clientInfo.DeviceName,
				clientInfo.Client,
				c.ClientIP())
		}
		embyLogClientRequest(svc, c, "emby client login", "", resp.User.ID,
			zap.String("username", resp.User.Username),
		)
		userPayload, _ := svc.Emby.FindUser(c.Request.Context(), resp.User.ID)
		// Emby/Jellyfin 客户端没有 refresh token 机制：它们把这里返回的
		// AccessToken 长期保存并反复使用。若返回 60 分钟的普通 access
		// token，客户端每小时就会掉登录、无法播放、媒体库无法刷新。因此
		// 签发长期令牌（IssueEmbyToken）匹配 Emby 持久化令牌语义。
		accessToken := resp.Tokens.AccessToken
		if longLived, err := svc.Auth.IssueEmbyToken(resp.User); err == nil && longLived != "" {
			accessToken = longLived
		}
		embyRememberCompatSession(c, accessToken)
		sessionInfo := embyCurrentSessionInfo(c, resp.User.ID, resp.User.Username)
		c.JSON(http.StatusOK, gin.H{
			"AccessToken": accessToken,
			"ServerId":    "mediastation-go-001",
			"UserId":      resp.User.ID,
			"User":        userPayload,
			"SessionInfo": sessionInfo,
		})
	}
}

func embyPublicUsersHandler(svc *service.Container) gin.HandlerFunc {
	return func(c *gin.Context) {
		// 公开用户列表（Emby Web 客户端登录页拉这个，列出可见用户）。
		users, err := svc.Emby.ListUsers(c.Request.Context())
		if err != nil {
			c.JSON(http.StatusOK, []any{})
			return
		}
		// 公开版本只暴露 Id + Name，不包含 Policy。
		out := make([]map[string]any, 0, len(users))
		for _, u := range users {
			out = append(out, map[string]any{
				"Id":          u["Id"],
				"Name":        u["Name"],
				"ServerId":    u["ServerId"],
				"HasPassword": true,
			})
		}
		c.JSON(http.StatusOK, out)
	}
}

func embyListUsersHandler(svc *service.Container) gin.HandlerFunc {
	return func(c *gin.Context) {
		users, err := svc.Emby.ListUsers(c.Request.Context())
		if err != nil {
			c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
			return
		}
		c.JSON(http.StatusOK, users)
	}
}

func embyMeHandler(svc *service.Container) gin.HandlerFunc {
	return func(c *gin.Context) {
		uid := embyUserID(c)
		if uid == "" {
			embyError(c, http.StatusUnauthorized, "not authenticated")
			return
		}
		u, err := svc.Emby.FindUser(c.Request.Context(), uid)
		if err != nil || u == nil {
			embyError(c, http.StatusNotFound, "user not found")
			return
		}
		c.JSON(http.StatusOK, u)
	}
}

func embyGetUserByIDHandler(svc *service.Container) gin.HandlerFunc {
	return func(c *gin.Context) {
		u, err := svc.Emby.FindUser(c.Request.Context(), c.Param("userId"))
		if err == nil && u != nil {
			c.JSON(http.StatusOK, u)
			return
		}
		if authUID := embyUserID(c); authUID != "" && authUID != c.Param("userId") {
			u, err = svc.Emby.FindUser(c.Request.Context(), authUID)
			if err == nil && u != nil {
				c.JSON(http.StatusOK, u)
				return
			}
		}
		c.JSON(http.StatusOK, embyFallbackUser(c.Param("userId")))
	}
}

func embyFallbackUser(id string) gin.H {
	if strings.TrimSpace(id) == "" {
		id = "mediastation-user"
	}
	return gin.H{
		"Id":                        id,
		"Name":                      "MediaStationGo",
		"ServerId":                  "mediastation-go-001",
		"HasPassword":               true,
		"HasConfiguredPassword":     true,
		"HasConfiguredEasyPassword": false,
		"EnableAutoLogin":           false,
		"Policy": gin.H{
			"IsAdministrator":                 true,
			"EnableContentDeletion":           true,
			"EnableRemoteControlOfOtherUsers": true,
			"EnableSharedDeviceControl":       true,
			"EnableRemoteAccess":              true,
			"EnableAllDevices":                true,
			"EnableAllChannels":               true,
			"EnableAllFolders":                true,
		},
	}
}

// ─── Views / MediaFolders ────────────────────────────────────────────────────

func embyViewsHandler(svc *service.Container) gin.HandlerFunc {
	return func(c *gin.Context) {
		uid := c.Param("userId")
		if uid == "" {
			uid = embyUserID(c)
		}
		out, err := svc.Emby.Views(c.Request.Context(), uid)
		if err != nil {
			c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
			return
		}
		embyAttachRequestTokenToMediaSources(c, out)
		c.JSON(http.StatusOK, out)
	}
}

func embyVirtualFoldersHandler(svc *service.Container) gin.HandlerFunc {
	return func(c *gin.Context) {
		c.Header("Cache-Control", "no-store")
		uid := embyUserID(c)
		if svc.Emby == nil {
			libs, err := svc.Repo.Library.List(c.Request.Context())
			if err != nil {
				c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
				return
			}
			libs = service.FilterDisplayCloudLibraries(c.Request.Context(), svc.Repo, libs)
			visibility := service.UserDefaultMediaVisibility(c.Request.Context(), svc.Repo, uid)
			out := make([]gin.H, 0, len(libs))
			for _, lib := range libs {
				if !service.LibraryVisibleForUser(c.Request.Context(), svc.Repo, lib, visibility) {
					continue
				}
				collectionType := "movies"
				switch lib.Type {
				case "tv", "anime", "variety":
					collectionType = "tvshows"
				case "music":
					collectionType = "music"
				}
				out = append(out, gin.H{
					"Name":               lib.Name,
					"Locations":          []string{lib.Path},
					"CollectionType":     collectionType,
					"ItemId":             lib.ID,
					"Id":                 lib.ID,
					"PrimaryImageItemId": lib.ID,
					"RefreshStatus":      "Idle",
					"LibraryOptions":     gin.H{},
				})
			}
			c.JSON(http.StatusOK, out)
			return
		}
		views, err := svc.Emby.Views(c.Request.Context(), uid)
		if err != nil {
			c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
			return
		}
		rawItems, _ := views["Items"].([]map[string]any)
		out := make([]gin.H, 0, len(rawItems))
		for _, item := range rawItems {
			id, _ := item["Id"].(string)
			name, _ := item["Name"].(string)
			path, _ := item["Path"].(string)
			collectionType, _ := item["CollectionType"].(string)
			if id == "" || name == "" {
				continue
			}
			out = append(out, gin.H{
				"Name":               name,
				"Locations":          []string{path},
				"CollectionType":     collectionType,
				"ItemId":             id,
				"Id":                 id,
				"PrimaryImageItemId": id,
				"RefreshStatus":      "Idle",
				"LibraryOptions":     gin.H{},
			})
		}
		c.JSON(http.StatusOK, out)
	}
}

// ─── Items ───────────────────────────────────────────────────────────────────

const embyItemsListDefaultLimit = 50

func firstQueryValueWithPresence(c *gin.Context, keys ...string) (string, bool) {
	if c == nil || c.Request == nil {
		return "", false
	}
	values := c.Request.URL.Query()
	for _, key := range keys {
		if rawValues, ok := values[key]; ok {
			if len(rawValues) == 0 {
				return "", true
			}
			return strings.TrimSpace(rawValues[0]), true
		}
	}
	return "", false
}

func parseEmbyItemsParams(c *gin.Context) service.ItemsParams {
	limitRaw, limitSet := firstQueryValueWithPresence(c, "Limit", "limit")
	limit, _ := strconv.Atoi(embyFirstNonEmptyString(limitRaw, strconv.Itoa(embyItemsListDefaultLimit)))
	if limitSet && limit <= 0 {
		// Some clients send Limit=0 while probing. Keep it as the normal default
		// page size instead of expanding it into a multi-thousand row response.
		limit = embyItemsListDefaultLimit
	}
	offset, _ := strconv.Atoi(embyFirstNonEmptyString(firstQueryValue(c, "StartIndex", "startIndex", "startindex"), "0"))
	uid := c.Param("userId")
	if uid == "" {
		uid = firstQueryValue(c, "UserId", "userId", "userid")
	}
	if uid == "" {
		uid = embyUserID(c)
	}
	splitOpt := func(s string) []string {
		if s == "" {
			return nil
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
	return service.ItemsParams{
		UserID:           uid,
		ParentID:         firstQueryValue(c, "ParentId", "parentId", "parentid"),
		IDs:              splitOpt(firstQueryValue(c, "Ids", "ids")),
		SearchTerm:       firstQueryValue(c, "SearchTerm", "searchTerm", "searchterm"),
		IncludeItemTypes: splitOpt(firstQueryValue(c, "IncludeItemTypes", "includeItemTypes", "includeitemtypes")),
		Filters:          splitOpt(firstQueryValue(c, "Filters", "filters")),
		Recursive:        strings.EqualFold(firstQueryValue(c, "Recursive", "recursive"), "true"),
		SortBy:           firstQueryValue(c, "SortBy", "sortBy", "sortby"),
		SortOrder:        firstQueryValue(c, "SortOrder", "sortOrder", "sortorder"),
		Limit:            limit,
		StartIndex:       offset,
	}
}

func embyFirstNonEmptyString(values ...string) string {
	for _, value := range values {
		if strings.TrimSpace(value) != "" {
			return strings.TrimSpace(value)
		}
	}
	return ""
}

func embyItemsHandler(svc *service.Container) gin.HandlerFunc {
	return func(c *gin.Context) {
		params := parseEmbyItemsParams(c)
		out, err := svc.Emby.Items(c.Request.Context(), params)
		if err != nil {
			c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
			return
		}
		embyLogClientRequest(svc, c, "emby items",
			"",
			params.UserID,
			zap.String("parent_id", strings.TrimSpace(params.ParentID)),
			zap.Strings("include_item_types", params.IncludeItemTypes),
			zap.Strings("filters", params.Filters),
			zap.String("sort_by", strings.TrimSpace(params.SortBy)),
			zap.String("sort_order", strings.TrimSpace(params.SortOrder)),
			zap.Bool("recursive", params.Recursive),
			zap.Int("start_index", params.StartIndex),
			zap.Int("limit", params.Limit),
			zap.Int("returned", embyItemsEnvelopeReturnedCount(out)),
			zap.Any("total_record_count", embyItemsEnvelopeTotal(out)),
		)
		if params.Limit > 100 && !embyRequestFieldsContains(c, "MediaSources") {
			embyStripMediaSourcesFromItemsEnvelope(out)
		}
		embyAttachRequestTokenToMediaSources(c, out)
		c.JSON(http.StatusOK, out)
	}
}

func embyRequestFieldsContains(c *gin.Context, want string) bool {
	fields := firstQueryValue(c, "Fields", "fields")
	for _, field := range strings.Split(fields, ",") {
		if strings.EqualFold(strings.TrimSpace(field), want) {
			return true
		}
	}
	return false
}

func embyStripMediaSourcesFromItemsEnvelope(out any) {
	switch typed := out.(type) {
	case map[string]any:
		embyStripMediaSourcesFromItemsValue(typed["Items"])
	case gin.H:
		embyStripMediaSourcesFromItemsValue(typed["Items"])
	}
}

func embyStripMediaSourcesFromItemsValue(value any) {
	switch typed := value.(type) {
	case []map[string]any:
		for _, item := range typed {
			delete(item, "MediaSources")
		}
	case []any:
		for _, item := range typed {
			if itemMap, ok := item.(map[string]any); ok {
				delete(itemMap, "MediaSources")
			}
		}
	}
}

func embyItemsEnvelopeReturnedCount(out any) int {
	switch typed := out.(type) {
	case map[string]any:
		return embyItemsValueCount(typed["Items"])
	case gin.H:
		return embyItemsValueCount(typed["Items"])
	default:
		return 0
	}
}

func embyItemsValueCount(value any) int {
	switch typed := value.(type) {
	case []map[string]any:
		return len(typed)
	case []any:
		return len(typed)
	default:
		return 0
	}
}

func embyItemsEnvelopeTotal(out any) any {
	switch typed := out.(type) {
	case map[string]any:
		return typed["TotalRecordCount"]
	case gin.H:
		return typed["TotalRecordCount"]
	default:
		return nil
	}
}

func embyItemByIDHandler(svc *service.Container) gin.HandlerFunc {
	return func(c *gin.Context) {
		id := c.Param("id")
		uid := c.Param("userId")
		if uid == "" {
			uid = embyUserID(c)
		}
		out, err := svc.Emby.Item(c.Request.Context(), id, uid)
		if err != nil {
			c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
			return
		}
		if out == nil {
			embyError(c, http.StatusNotFound, "item not found")
			return
		}
		itemType, _ := out["Type"].(string)
		mediaType, _ := out["MediaType"].(string)
		embyLogClientRequest(svc, c, "emby item detail", id, uid,
			zap.String("item_type", strings.TrimSpace(itemType)),
			zap.String("media_type", strings.TrimSpace(mediaType)),
		)
		embyAttachRequestTokenToMediaSources(c, out)
		c.JSON(http.StatusOK, out)
	}
}

func embyUserItemByIDHandler(svc *service.Container) gin.HandlerFunc {
	return func(c *gin.Context) {
		switch strings.ToLower(c.Param("id")) {
		case "latest":
			embyLatestItemsHandler(svc)(c)
		case "resume":
			embyResumeItemsHandler(svc)(c)
		default:
			embyItemByIDHandler(svc)(c)
		}
	}
}

func embyLatestItemsHandler(svc *service.Container) gin.HandlerFunc {
	return func(c *gin.Context) {
		uid := c.Param("userId")
		if uid == "" {
			uid = firstQueryValue(c, "UserId", "userId", "userid")
		}
		if uid == "" {
			uid = embyUserID(c)
		}
		limit, _ := strconv.Atoi(embyFirstNonEmptyString(firstQueryValue(c, "Limit", "limit"), "20"))
		out, err := svc.Emby.LatestItems(c.Request.Context(), uid, firstQueryValue(c, "ParentId", "parentId", "parentid"), limit)
		if err != nil {
			c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
			return
		}
		embyAttachRequestTokenToMediaSources(c, out)
		c.JSON(http.StatusOK, out)
	}
}

func embyResumeItemsHandler(svc *service.Container) gin.HandlerFunc {
	return func(c *gin.Context) {
		uid := c.Param("userId")
		if uid == "" {
			uid = firstQueryValue(c, "UserId", "userId", "userid")
		}
		if uid == "" {
			uid = embyUserID(c)
		}
		limit, _ := strconv.Atoi(embyFirstNonEmptyString(firstQueryValue(c, "Limit", "limit"), "20"))
		out, err := svc.Emby.ResumeItems(c.Request.Context(), uid, limit)
		if err != nil {
			c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
			return
		}
		embyAttachRequestTokenToMediaSources(c, out)
		c.JSON(http.StatusOK, out)
	}
}

func embyItemsCountsHandler(svc *service.Container) gin.HandlerFunc {
	return func(c *gin.Context) {
		if svc != nil && svc.Emby != nil {
			uid := c.Param("userId")
			if uid == "" {
				uid = firstQueryValue(c, "UserId", "userId", "userid")
			}
			if uid == "" {
				uid = embyUserID(c)
			}
			out, err := svc.Emby.ItemsCounts(c.Request.Context(), uid, firstQueryValue(c, "ParentId", "parentId", "parentid"))
			if err != nil {
				c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
				return
			}
			c.JSON(http.StatusOK, out)
			return
		}
		c.JSON(http.StatusOK, embyEmptyItemsCountsPayload())
	}
}

func embyPublicItemsCountsHandler(svc *service.Container, jwtSecret string) gin.HandlerFunc {
	realCounts := embyItemsCountsHandler(svc)
	return func(c *gin.Context) {
		if embyApplyOptionalAuth(c, jwtSecret, svc) {
			realCounts(c)
			return
		}
		c.JSON(http.StatusOK, embyEmptyItemsCountsPayload())
	}
}

func embyEmptyItemsCountsPayload() gin.H {
	return gin.H{
		"MovieCount":   0,
		"SeriesCount":  0,
		"EpisodeCount": 0,
		"ItemCount":    0,
	}
}

func embyApplyOptionalAuth(c *gin.Context, secret string, svc *service.Container) bool {
	token := embyRequestToken(c)
	if token == "" {
		token = embyCompatSessionToken(c)
	}
	if token == "" || strings.TrimSpace(secret) == "" {
		return false
	}
	claims := &middleware.Claims{}
	parsed, err := jwt.ParseWithClaims(token, claims, func(t *jwt.Token) (interface{}, error) {
		if _, ok := t.Method.(*jwt.SigningMethodHMAC); !ok {
			return nil, errors.New("unexpected signing method")
		}
		return []byte(secret), nil
	})
	if err != nil || !parsed.Valid || claims.UserID == "" {
		return false
	}
	if svc != nil && svc.Repo != nil && svc.Repo.User != nil {
		u, err := svc.Repo.User.FindByID(c.Request.Context(), claims.UserID)
		if err != nil || u == nil || !u.IsActive || (u.ExpiredAt != nil && time.Now().After(*u.ExpiredAt)) {
			return false
		}
	}
	c.Set(middleware.EmbyCtxUserID, claims.UserID)
	c.Set(middleware.CtxUserID, claims.UserID)
	c.Set(middleware.CtxUserRole, claims.Role)
	c.Set(middleware.CtxUserTier, claims.Tier)
	return true
}

func embyDisplayPreferencesHandler(_ *service.Container) gin.HandlerFunc {
	return func(c *gin.Context) {
		c.JSON(http.StatusOK, gin.H{
			"Id":                 c.Param("id"),
			"ViewType":           "Poster",
			"SortBy":             "SortName",
			"SortOrder":          "Ascending",
			"IndexBy":            "SortName",
			"RememberIndexing":   false,
			"PrimaryImageHeight": 250,
			"PrimaryImageWidth":  250,
			"ScrollDirection":    "Horizontal",
			"ShowSidebar":        true,
			"CustomPrefs": gin.H{
				"homeexploresection": "1",
				"homesection0":       "smalllibrarytiles",
				"homesection1":       "resume",
				"homesection2":       "latestmedia",
				"homesection3":       "nextup",
				"homesection4":       "none",
				"homesection5":       "none",
				"homesection6":       "none",
				"latestItems":        "true",
				"landing-livetv":     "false",
			},
		})
	}
}

func embySaveDisplayPreferencesHandler(_ *service.Container) gin.HandlerFunc {
	return func(c *gin.Context) {
		c.Status(http.StatusNoContent)
	}
}

// ─── Images ──────────────────────────────────────────────────────────────────

var embyPlaceholderPNG = []byte{
	0x89, 0x50, 0x4e, 0x47, 0x0d, 0x0a, 0x1a, 0x0a,
	0x00, 0x00, 0x00, 0x0d, 0x49, 0x48, 0x44, 0x52,
	0x00, 0x00, 0x00, 0x01, 0x00, 0x00, 0x00, 0x01,
	0x08, 0x06, 0x00, 0x00, 0x00, 0x1f, 0x15, 0xc4,
	0x89, 0x00, 0x00, 0x00, 0x0d, 0x49, 0x44, 0x41,
	0x54, 0x78, 0x9c, 0x63, 0x50, 0xd1, 0x30, 0xf8,
	0x0f, 0x00, 0x02, 0x6c, 0x01, 0x7c, 0x30, 0xed,
	0x6e, 0x0a, 0x00, 0x00, 0x00, 0x00, 0x49, 0x45,
	0x4e, 0x44, 0xae, 0x42, 0x60, 0x82,
}

// embyItemImageHandler 把 /Items/{id}/Images/Primary 等请求直接输出为图片。
// Emby 客户端缓存图片 URL 时经常不会继续携带 token；如果重定向到受保护的
// /api/img 会变成 401，所以这里复用 ImageProxy 但不再走 /api 路由。
func embyItemImageHandler(svc *service.Container) gin.HandlerFunc {
	return func(c *gin.Context) {
		ctx, cancel := context.WithTimeout(c.Request.Context(), 8*time.Second)
		defer cancel()
		req := c.Request.WithContext(ctx)
		id := c.Param("id")
		imgType := strings.ToLower(c.Param("type"))
		raw, err := svc.Emby.ImageURL(ctx, id, imgType)
		if err != nil {
			if errors.Is(err, service.ErrMediaNotFound) {
				c.Status(http.StatusNotFound)
				return
			}
			embyServePlaceholderImage(c)
			return
		}
		if raw == "" {
			embyServePlaceholderImage(c)
			return
		}
		if typ, ref, ok := parseCloudPlayImageURL(raw); ok {
			c.Request = req
			serveCloudResolvedLink(svc, c, typ, ref)
			return
		}
		if svc.ImageProxy == nil {
			embyServePlaceholderImage(c)
			return
		}
		if err := svc.ImageProxy.Serve(ctx, c.Writer, req, raw); err != nil {
			embyServePlaceholderImage(c)
		}
	}
}

func embyServePlaceholderImage(c *gin.Context) {
	c.Header("Content-Type", "image/png")
	c.Header("Cache-Control", "public, max-age=3600")
	c.Header("Content-Length", strconv.Itoa(len(embyPlaceholderPNG)))
	if c.Request.Method == http.MethodHead {
		c.Status(http.StatusOK)
		return
	}
	c.Data(http.StatusOK, "image/png", embyPlaceholderPNG)
}

func parseCloudPlayImageURL(raw string) (string, string, bool) {
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
	if !strings.HasPrefix(path, prefix) {
		return "", "", false
	}
	typ := strings.TrimSpace(strings.TrimPrefix(path, prefix))
	ref := strings.TrimSpace(u.Query().Get("ref"))
	if typ == "" || ref == "" {
		return "", "", false
	}
	return typ, ref, true
}

func embyShowSeasonsHandler(svc *service.Container) gin.HandlerFunc {
	return func(c *gin.Context) {
		params := service.ItemsParams{
			UserID:   firstQueryValue(c, "UserId", "userId"),
			ParentID: c.Param("id"),
			Limit:    500,
		}
		out, err := svc.Emby.Items(c.Request.Context(), params)
		if err != nil {
			c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
			return
		}
		embyAttachRequestTokenToMediaSources(c, out)
		c.JSON(http.StatusOK, out)
	}
}

func embyShowEpisodesHandler(svc *service.Container) gin.HandlerFunc {
	return func(c *gin.Context) {
		parentID := firstQueryValue(c, "SeasonId", "seasonId")
		if parentID == "" {
			parentID = c.Param("id")
		}
		uid := firstQueryValue(c, "UserId", "userId")
		if uid == "" {
			uid = embyUserID(c)
		}
		params := service.ItemsParams{
			UserID:           uid,
			ParentID:         parentID,
			IncludeItemTypes: []string{"Episode"},
			Recursive:        true,
			Limit:            500,
		}
		out, err := svc.Emby.Items(c.Request.Context(), params)
		if err != nil {
			c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
			return
		}
		embyAttachRequestTokenToMediaSources(c, out)
		c.JSON(http.StatusOK, out)
	}
}

// ─── Playback ────────────────────────────────────────────────────────────────

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
		embyLogClientRequest(svc, c, "emby playback info",
			c.Param("id"),
			uid,
			zap.Bool("absolute_playback_url", embyClientNeedsAbsolutePlaybackURL(c)),
		)
		embyAttachRequestTokenToMediaSources(c, out)
		c.JSON(http.StatusOK, out)
	}
}

func embyAttachRequestTokenToMediaSources(c *gin.Context, out any) {
	token := embyRequestToken(c)
	if token == "" || out == nil {
		return
	}
	embyAttachTokenToMediaSourcesValue(c, out, token)
}

func embyAttachTokenToMediaSourcesValue(c *gin.Context, value any, token string) {
	switch typed := value.(type) {
	case map[string]any:
		embyAttachTokenToMediaSourcesMap(c, typed, token)
	case gin.H:
		embyAttachTokenToMediaSourcesMap(c, map[string]any(typed), token)
	case []map[string]any:
		for _, item := range typed {
			embyAttachTokenToMediaSourcesMap(c, item, token)
		}
	case []any:
		for _, item := range typed {
			embyAttachTokenToMediaSourcesValue(c, item, token)
		}
	}
}

func embyAttachTokenToMediaSourcesMap(c *gin.Context, out map[string]any, token string) {
	if out == nil {
		return
	}
	if raw, ok := out["Path"].(string); ok && embyMediaSourcePathNeedsAPIKey(raw) {
		out["Path"] = embyAbsolutePlaybackURL(c, embyAppendAPIKey(raw, token))
	}
	if sources, ok := out["MediaSources"].([]map[string]any); ok {
		embyAttachTokenToMediaSources(c, sources, token)
	} else if sources, ok := out["MediaSources"].([]any); ok {
		for _, source := range sources {
			if sourceMap, ok := source.(map[string]any); ok {
				embyAttachTokenToMediaSources(c, []map[string]any{sourceMap}, token)
			}
		}
	}
	if items, ok := out["Items"]; ok {
		embyAttachTokenToMediaSourcesValue(c, items, token)
	}
}

func embyAttachTokenToMediaSources(c *gin.Context, sources []map[string]any, token string) {
	for _, source := range sources {
		for _, key := range []string{"DirectStreamUrl", "TranscodingUrl"} {
			raw, ok := source[key].(string)
			if !ok {
				continue
			}
			source[key] = embyAbsolutePlaybackURL(c, embyAppendAPIKey(raw, token))
		}
		if raw, ok := source["Path"].(string); ok && embyMediaSourcePathNeedsAPIKey(raw) {
			source["Path"] = embyAbsolutePlaybackURL(c, embyAppendAPIKey(raw, token))
		}
	}
}

func embyAbsolutePlaybackURL(c *gin.Context, raw string) string {
	raw = strings.TrimSpace(raw)
	if raw == "" || strings.HasPrefix(raw, "//") {
		return raw
	}
	u, err := url.Parse(raw)
	if err != nil || u.IsAbs() || c == nil || c.Request == nil || !embyClientNeedsAbsolutePlaybackURL(c) {
		return raw
	}
	if strings.HasPrefix(raw, "/") {
		return absoluteRequestURL(c, raw)
	}
	return raw
}

func embyClientNeedsAbsolutePlaybackURL(c *gin.Context) bool {
	info := embyClientInfoFromRequest(c)
	client := strings.ToLower(strings.TrimSpace(info.Client))
	return strings.Contains(client, "yamby") ||
		strings.Contains(client, "senplayer") ||
		strings.Contains(client, "lenna")
}

func embyMediaSourcePathNeedsAPIKey(raw string) bool {
	path := strings.TrimSpace(raw)
	if path == "" || strings.HasPrefix(path, "//") {
		return false
	}
	u, err := url.Parse(path)
	if err != nil || u.IsAbs() {
		return false
	}
	return strings.HasPrefix(strings.TrimPrefix(u.Path, "/"), "Videos/")
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
		rangeHeader := strings.TrimSpace(c.GetHeader("Range"))
		logStreamRequest := embyLogClientRequest
		if rangeHeader != "" && !strings.HasPrefix(rangeHeader, "bytes=0-") {
			logStreamRequest = embyLogClientRequestDebug
		}
		logStreamRequest(svc, c, "emby video stream",
			c.Param("id"),
			uid,
			zap.String("cloud_mode", cloudMode),
			zap.String("range", rangeHeader),
		)
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

// ─── 播放进度 / 收藏 / 已看 ────────────────────────────────────────────────

type embyPlayingReq struct {
	ItemId        string `json:"ItemId"`
	PositionTicks int64  `json:"PositionTicks"`
	RunTimeTicks  int64  `json:"RunTimeTicks"`
}

func embyPlayingProgressHandler(svc *service.Container) gin.HandlerFunc {
	return func(c *gin.Context) {
		uid := embyUserID(c)
		if uid == "" {
			c.Status(http.StatusUnauthorized)
			return
		}
		var req embyPlayingReq
		_ = c.ShouldBindJSON(&req)
		// 兼容 query 形式（一些客户端在 /Sessions/Playing/* 用 query）
		if req.ItemId == "" {
			req.ItemId = c.Query("ItemId")
		}
		if req.PositionTicks == 0 {
			req.PositionTicks, _ = strconv.ParseInt(c.Query("PositionTicks"), 10, 64)
		}
		if req.RunTimeTicks == 0 {
			req.RunTimeTicks, _ = strconv.ParseInt(c.Query("RunTimeTicks"), 10, 64)
		}
		if req.ItemId == "" {
			c.Status(http.StatusOK) // Emby 期望 2xx；不是关键操作
			return
		}
		// 被「一键踢下线」的设备拒绝继续播放，直到重新登录。
		clientInfo := embyClientInfoFromRequest(c)
		if svc.Device != nil && svc.Device.IsDeviceKicked(c.Request.Context(), uid, clientInfo.DeviceID) {
			c.Status(http.StatusUnauthorized)
			return
		}
		_ = svc.Emby.RecordProgress(c.Request.Context(), uid, req.ItemId, req.PositionTicks, req.RunTimeTicks)
		// 标记该设备正在播放并执行并发播放防共享检测。
		if svc.Device != nil {
			svc.Device.RecordPlayback(c.Request.Context(), uid,
				clientInfo.DeviceID,
				clientInfo.DeviceName,
				clientInfo.Client)
		}
		c.Status(http.StatusNoContent)
	}
}

func embyFavoriteHandler(svc *service.Container, fav bool) gin.HandlerFunc {
	return func(c *gin.Context) {
		uid := c.Param("userId")
		if uid == "" {
			uid = embyUserID(c)
		}
		mid := c.Param("itemId")
		if uid == "" || mid == "" {
			c.Status(http.StatusBadRequest)
			return
		}
		if err := svc.Emby.SetFavorite(c.Request.Context(), uid, mid, fav); err != nil {
			c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
			return
		}
		// Emby 期望返回 UserItemDataDto；最小可工作版本：echo Item 即可。
		out, _ := svc.Emby.Item(c.Request.Context(), mid, uid)
		if out != nil {
			c.JSON(http.StatusOK, out["UserData"])
			return
		}
		c.JSON(http.StatusOK, gin.H{"IsFavorite": fav})
	}
}

func embyMarkPlayedHandler(svc *service.Container, played bool) gin.HandlerFunc {
	return func(c *gin.Context) {
		uid := c.Param("userId")
		if uid == "" {
			uid = embyUserID(c)
		}
		mid := c.Param("itemId")
		if uid == "" || mid == "" {
			c.Status(http.StatusBadRequest)
			return
		}
		if err := svc.Emby.MarkPlayed(c.Request.Context(), uid, mid, played); err != nil {
			c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
			return
		}
		if played && svc.Device != nil {
			clientInfo := embyClientInfoFromRequest(c)
			svc.Device.RecordPlayback(c.Request.Context(), uid, clientInfo.DeviceID, clientInfo.DeviceName, clientInfo.Client)
		}
		out, _ := svc.Emby.Item(c.Request.Context(), mid, uid)
		if out != nil {
			c.JSON(http.StatusOK, out["UserData"])
			return
		}
		c.JSON(http.StatusOK, gin.H{"Played": played})
	}
}

// ─── Sessions / Branding 占位 ────────────────────────────────────────────────

func embyCurrentSessionInfo(c *gin.Context, userID, userName string) gin.H {
	info := embyClientInfoFromRequest(c)
	sessionID := strings.TrimSpace(info.DeviceID)
	if sessionID == "" {
		sessionID = strings.TrimSpace(userID)
	}
	if sessionID == "" {
		sessionID = "mediastation-session"
	}
	if userName == "" {
		userName = "MediaStationGo"
	}
	now := time.Now().UTC().Format(time.RFC3339Nano)
	return gin.H{
		"Id":                    sessionID,
		"ServerId":              "mediastation-go-001",
		"UserId":                userID,
		"UserName":              userName,
		"Client":                strings.TrimSpace(info.Client),
		"ApplicationVersion":    "4.8.10.0",
		"DeviceId":              strings.TrimSpace(info.DeviceID),
		"DeviceName":            strings.TrimSpace(info.DeviceName),
		"RemoteEndPoint":        c.ClientIP(),
		"LastActivityDate":      now,
		"NowPlayingItem":        nil,
		"NowViewingItem":        nil,
		"NowPlayingQueue":       []any{},
		"SupportsRemoteControl": false,
		"PlayState": gin.H{
			"CanSeek":                false,
			"IsPaused":               false,
			"IsMuted":                false,
			"RepeatMode":             "RepeatNone",
			"PositionTicks":          0,
			"PlaybackStartTimeTicks": 0,
		},
	}
}

func embySessionsHandler(svc *service.Container) gin.HandlerFunc {
	return func(c *gin.Context) {
		uid := embyUserID(c)
		userName := ""
		if svc != nil && svc.Emby != nil && uid != "" {
			if u, err := svc.Emby.FindUser(c.Request.Context(), uid); err == nil && u != nil {
				userName, _ = u["Name"].(string)
			}
		}
		c.JSON(http.StatusOK, []gin.H{embyCurrentSessionInfo(c, uid, userName)})
	}
}

func embySessionInfoHandler(svc *service.Container) gin.HandlerFunc {
	return func(c *gin.Context) {
		uid := embyUserID(c)
		userName := ""
		if svc != nil && svc.Emby != nil && uid != "" {
			if u, err := svc.Emby.FindUser(c.Request.Context(), uid); err == nil && u != nil {
				userName, _ = u["Name"].(string)
			}
		}
		c.JSON(http.StatusOK, embyCurrentSessionInfo(c, uid, userName))
	}
}

func embyNoContentHandler(_ *service.Container) gin.HandlerFunc {
	return func(c *gin.Context) {
		c.Status(http.StatusNoContent)
	}
}

func embyWebSocketHandler(_ *service.Container) gin.HandlerFunc {
	return func(c *gin.Context) {
		if !websocket.IsWebSocketUpgrade(c.Request) {
			c.Status(http.StatusNoContent)
			return
		}
		conn, err := wsUpgrader.Upgrade(c.Writer, c.Request, nil)
		if err != nil {
			return
		}
		defer conn.Close()

		done := make(chan struct{})
		go func() {
			defer close(done)
			for {
				if _, _, err := conn.NextReader(); err != nil {
					return
				}
			}
		}()

		ticker := time.NewTicker(30 * time.Second)
		defer ticker.Stop()
		for {
			select {
			case <-done:
				return
			case <-ticker.C:
				_ = conn.SetWriteDeadline(time.Now().Add(10 * time.Second))
				if err := conn.WriteMessage(websocket.PingMessage, nil); err != nil {
					return
				}
			}
		}
	}
}

func embyServerConfigurationHandler(_ *service.Container) gin.HandlerFunc {
	return func(c *gin.Context) {
		c.JSON(http.StatusOK, gin.H{
			"EnableFolderView":                   true,
			"EnableGroupingIntoCollections":      true,
			"EnableExternalContentInSuggestions": false,
			"ImageSavingConvention":              "Compatible",
		})
	}
}

func embyPublicServerConfigurationHandler(_ *service.Container) gin.HandlerFunc {
	return func(c *gin.Context) {
		c.JSON(http.StatusOK, gin.H{
			"IsStartupWizardCompleted": true,
			"EnableRemoteAccess":       true,
			"EnableUPnP":               false,
			"EnableHttps":              false,
			"RequireHttps":             false,
			"LocalNetworkSubnets":      []string{},
			"LocalNetworkAddresses":    []string{},
			"RemoteClientBitrateLimit": 0,
		})
	}
}

func embyStartupConfigurationHandler(_ *service.Container) gin.HandlerFunc {
	return func(c *gin.Context) {
		c.JSON(http.StatusOK, gin.H{
			"IsStartupWizardCompleted":  true,
			"StartupWizardCompleted":    true,
			"EnableRemoteAccess":        true,
			"UICulture":                 "zh-CN",
			"MetadataCountryCode":       "CN",
			"PreferredMetadataLanguage": "zh-CN",
		})
	}
}

func embyQuickConnectEnabledHandler(_ *service.Container) gin.HandlerFunc {
	return func(c *gin.Context) {
		c.JSON(http.StatusOK, false)
	}
}

func embyEmptyItemsHandler(_ *service.Container) gin.HandlerFunc {
	return func(c *gin.Context) {
		c.JSON(http.StatusOK, gin.H{"Items": []any{}, "TotalRecordCount": 0})
	}
}

func embyEmptyArrayHandler(_ *service.Container) gin.HandlerFunc {
	return func(c *gin.Context) {
		c.JSON(http.StatusOK, []any{})
	}
}

func embyCustomCSSJSScriptsHandler(_ *service.Container) gin.HandlerFunc {
	return func(c *gin.Context) {
		c.Data(http.StatusOK, "application/javascript; charset=utf-8", nil)
	}
}

func embyLocalizationCulturesHandler(_ *service.Container) gin.HandlerFunc {
	return func(c *gin.Context) {
		c.JSON(http.StatusOK, []gin.H{
			{
				"DisplayName":                 "简体中文",
				"Name":                        "zh-CN",
				"ThreeLetterISOLanguageName":  "zho",
				"TwoLetterISOLanguageName":    "zh",
				"ThreeLetterISOLanguageNames": []string{"zho", "chi"},
				"IsRightToLeft":               false,
			},
			{
				"DisplayName":                 "English",
				"Name":                        "en-US",
				"ThreeLetterISOLanguageName":  "eng",
				"TwoLetterISOLanguageName":    "en",
				"ThreeLetterISOLanguageNames": []string{"eng"},
				"IsRightToLeft":               false,
			},
		})
	}
}

func embyThemeMediaHandler(_ *service.Container) gin.HandlerFunc {
	return func(c *gin.Context) {
		empty := gin.H{"Items": []any{}, "TotalRecordCount": 0}
		c.JSON(http.StatusOK, gin.H{
			"ThemeVideosResult":     empty,
			"ThemeSongsResult":      empty,
			"SoundtrackSongsResult": empty,
		})
	}
}

func embyServerDomainsHandler(_ *service.Container) gin.HandlerFunc {
	return func(c *gin.Context) {
		c.JSON(http.StatusOK, []any{})
	}
}

func embyDanmuRawHandler(_ *service.Container) gin.HandlerFunc {
	return func(c *gin.Context) {
		c.Data(http.StatusOK, "text/plain; charset=utf-8", nil)
	}
}

func embyBrandingConfigHandler(_ *service.Container) gin.HandlerFunc {
	return func(c *gin.Context) {
		c.JSON(http.StatusOK, gin.H{
			"LoginDisclaimer":     "",
			"CustomCss":           "",
			"SplashscreenEnabled": false,
		})
	}
}

func embyBrandingCSSHandler(_ *service.Container) gin.HandlerFunc {
	return func(c *gin.Context) {
		c.Data(http.StatusOK, "text/css; charset=utf-8", []byte(""))
	}
}

func embyLocalizationOptionsHandler(_ *service.Container) gin.HandlerFunc {
	return func(c *gin.Context) {
		c.JSON(http.StatusOK, []map[string]any{
			{"Name": "简体中文", "Value": "zh-CN"},
			{"Name": "English", "Value": "en-US"},
		})
	}
}

// registerEmbyRoutes 在 r 上挂双前缀（"" + "/emby"）的 Emby 兼容路由。
func registerEmbyRoutes(r *gin.Engine, jwtSecret string, svc *service.Container) {
	for _, prefix := range []string{"/emby", ""} {
		grp := r.Group(prefix)
		grp.Use(func(c *gin.Context) {
			c.Header("Cache-Control", "no-store")
			c.Header("Pragma", "no-cache")
			c.Header("Expires", "0")
			c.Next()
		})

		if prefix == "/emby" {
			grp.GET("", embyRootHandler(svc))
			grp.HEAD("", embyRootHandler(svc))
			grp.GET("/", embyRootHandler(svc))
			grp.HEAD("/", embyRootHandler(svc))
		}

		// 公开端点
		for _, path := range []string{"/System/Info/Public", "/system/info/public"} {
			grp.GET(path, embySystemInfoPublicHandler(svc))
			grp.HEAD(path, embySystemInfoPublicHandler(svc))
		}
		for _, path := range []string{"/System/Info", "/system/info"} {
			grp.GET(path, embySystemInfoHandler(svc))
			grp.HEAD(path, embySystemInfoHandler(svc))
		}
		for _, path := range []string{"/System/Endpoint", "/system/endpoint"} {
			grp.GET(path, embySystemEndpointHandler(svc))
		}
		for _, path := range []string{"/System/Ext/ServerDomains", "/system/ext/serverdomains"} {
			grp.GET(path, embyServerDomainsHandler(svc))
			grp.HEAD(path, embyServerDomainsHandler(svc))
		}
		for _, path := range []string{"/System/Configuration/Public", "/system/configuration/public"} {
			grp.GET(path, embyPublicServerConfigurationHandler(svc))
			grp.HEAD(path, embyPublicServerConfigurationHandler(svc))
		}
		for _, path := range []string{"/Startup/Configuration", "/startup/configuration"} {
			grp.GET(path, embyStartupConfigurationHandler(svc))
			grp.HEAD(path, embyStartupConfigurationHandler(svc))
		}
		for _, path := range []string{"/Startup/Complete", "/startup/complete"} {
			grp.POST(path, embyNoContentHandler(svc))
		}
		for _, path := range []string{"/QuickConnect/Enabled", "/quickconnect/enabled"} {
			grp.GET(path, embyQuickConnectEnabledHandler(svc))
			grp.HEAD(path, embyQuickConnectEnabledHandler(svc))
		}
		for _, path := range []string{"/System/Ping", "/system/ping"} {
			grp.GET(path, embyPingHandler(svc))
			grp.HEAD(path, embyPingHandler(svc))
			grp.POST(path, embyPingHandler(svc))
		}
		for _, path := range []string{"/Sessions/Capabilities", "/Sessions/Capabilities/Full", "/sessions/capabilities", "/sessions/capabilities/full"} {
			grp.POST(path, embyNoContentHandler(svc))
		}
		// 30/min per IP: many Emby clients sit behind a single NAT/reverse-proxy
		// IP, so a low limit would throttle legitimate logins into 429s.
		embyLoginLimiter := middleware.NewRateLimiter(30, 1*time.Minute)
		for _, path := range []string{"/Users/AuthenticateByName", "/Users/authenticatebyname", "/users/AuthenticateByName", "/users/authenticatebyname"} {
			grp.POST(path, middleware.RateLimit(embyLoginLimiter), embyAuthByNameHandler(svc))
		}
		for _, path := range []string{"/Users/Public", "/users/public"} {
			grp.GET(path, embyPublicUsersHandler(svc))
		}
		for _, path := range []string{"/Branding/Configuration", "/branding/configuration"} {
			grp.GET(path, embyBrandingConfigHandler(svc))
		}
		for _, path := range []string{"/Branding/Css", "/branding/css"} {
			grp.GET(path, embyBrandingCSSHandler(svc))
			grp.HEAD(path, embyBrandingCSSHandler(svc))
		}
		for _, path := range []string{"/Localization/Options", "/localization/options"} {
			grp.GET(path, embyLocalizationOptionsHandler(svc))
		}
		for _, path := range []string{"/Localization/Cultures", "/Localization/cultures", "/localization/cultures"} {
			grp.GET(path, embyLocalizationCulturesHandler(svc))
		}
		for _, path := range []string{"/CustomCssJS/Scripts", "/customcssjs/scripts"} {
			grp.GET(path, embyCustomCSSJSScriptsHandler(svc))
			grp.HEAD(path, embyCustomCSSJSScriptsHandler(svc))
		}
		for _, path := range []string{"/embywebsocket", "/EmbyWebSocket"} {
			grp.GET(path, embyWebSocketHandler(svc))
			grp.HEAD(path, embyNoContentHandler(svc))
		}
		for _, path := range []string{"/Sessions/Logout", "/sessions/logout"} {
			grp.POST(path, embyNoContentHandler(svc))
		}
		grp.GET("/DisplayPreferences/:id", embyDisplayPreferencesHandler(svc))
		grp.POST("/DisplayPreferences/:id", embySaveDisplayPreferencesHandler(svc))
		grp.GET("/displaypreferences/:id", embyDisplayPreferencesHandler(svc))
		grp.POST("/displaypreferences/:id", embySaveDisplayPreferencesHandler(svc))

		// 部分 iOS 客户端会在登录前探测 Items/Counts，返回 401 会导致反复登录。
		// 未鉴权时只返回空计数；携带有效 token 时再返回真实数量，避免公开泄露库规模。
		publicCounts := embyPublicItemsCountsHandler(svc, jwtSecret)
		grp.GET("/Items/Counts", publicCounts)
		grp.GET("/Users/:userId/Items/Counts", publicCounts)
		grp.GET("/items/counts", publicCounts)
		grp.GET("/users/:userId/items/counts", publicCounts)

		// 图片公开（Infuse 缓存 URL 时会丢 token）
		grp.GET("/Items/:id/Images/:type", embyItemImageHandler(svc))
		grp.GET("/Items/:id/Images/:type/:index", embyItemImageHandler(svc))
		grp.HEAD("/Items/:id/Images/:type", embyItemImageHandler(svc))
		grp.GET("/items/:id/images/:type", embyItemImageHandler(svc))
		grp.GET("/items/:id/images/:type/:index", embyItemImageHandler(svc))
		grp.HEAD("/items/:id/images/:type", embyItemImageHandler(svc))

		// 鉴权后端点
		auth := grp.Group("", embyAuthRequiredWithSessionFallback(jwtSecret), activeEmbyUserRequired(svc))
		auth.GET("/Users/Me", embyMeHandler(svc))
		auth.GET("/Users", embyListUsersHandler(svc))
		auth.GET("/Users/:userId", embyGetUserByIDHandler(svc))
		auth.GET("/Users/:userId/Views", embyViewsHandler(svc))
		auth.GET("/Library/MediaFolders", embyViewsHandler(svc))
		auth.GET("/Library/VirtualFolders", embyVirtualFoldersHandler(svc))
		auth.GET("/Library/SelectableMediaFolders", embyVirtualFoldersHandler(svc))

		auth.GET("/Items", embyItemsHandler(svc))
		auth.GET("/Users/:userId/Items", embyItemsHandler(svc))
		auth.GET("/Items/Latest", embyLatestItemsHandler(svc))
		auth.GET("/Items/Resume", embyResumeItemsHandler(svc))
		auth.GET("/Items/:id", embyItemByIDHandler(svc))
		auth.GET("/Users/:userId/Items/:id", embyUserItemByIDHandler(svc))
		auth.GET("/Shows/:id/Seasons", embyShowSeasonsHandler(svc))
		auth.GET("/Shows/:id/Episodes", embyShowEpisodesHandler(svc))
		auth.GET("/Users/:userId/Shows/:id/Seasons", embyShowSeasonsHandler(svc))
		auth.GET("/Users/:userId/Shows/:id/Episodes", embyShowEpisodesHandler(svc))
		auth.GET("/Shows/NextUp", embyEmptyItemsHandler(svc))
		auth.GET("/Users/:userId/Shows/NextUp", embyEmptyItemsHandler(svc))
		auth.GET("/MediaSegments/:id", embyEmptyItemsHandler(svc))
		auth.GET("/Artists", embyEmptyItemsHandler(svc))
		auth.GET("/Persons", embyEmptyItemsHandler(svc))
		auth.GET("/Genres", embyEmptyItemsHandler(svc))
		auth.GET("/Studios", embyEmptyItemsHandler(svc))
		auth.GET("/Shows/Upcoming", embyEmptyItemsHandler(svc))
		auth.GET("/Users/:userId/Shows/Upcoming", embyEmptyItemsHandler(svc))
		auth.GET("/Items/:id/Similar", embyEmptyItemsHandler(svc))
		auth.GET("/Items/:id/ThumbnailSet", embyEmptyItemsHandler(svc))
		auth.GET("/Items/:id/ThemeMedia", embyThemeMediaHandler(svc))
		auth.GET("/Users/:userId/Items/:id/SpecialFeatures", embyEmptyItemsHandler(svc))
		auth.GET("/Users/:userId/Items/:id/Intros", embyEmptyItemsHandler(svc))
		auth.GET("/Items/:id/SpecialFeatures", embyEmptyItemsHandler(svc))
		auth.GET("/Items/:id/Intros", embyEmptyItemsHandler(svc))
		auth.GET("/api/danmu/:id/raw", embyDanmuRawHandler(svc))

		auth.GET("/Items/:id/PlaybackInfo", embyPlaybackInfoHandler(svc))
		auth.POST("/Items/:id/PlaybackInfo", embyPlaybackInfoHandler(svc))
		auth.GET("/Users/:userId/Items/:id/PlaybackInfo", embyPlaybackInfoHandler(svc))
		auth.POST("/Users/:userId/Items/:id/PlaybackInfo", embyPlaybackInfoHandler(svc))

		auth.GET("/Videos/:id/stream", embyVideoStreamHandler(svc, service.CloudPlaybackModeRedirectProxy))
		auth.HEAD("/Videos/:id/stream", embyVideoStreamHandler(svc, service.CloudPlaybackModeRedirectProxy))
		auth.GET("/Videos/:id/stream.:container", embyVideoStreamHandler(svc, service.CloudPlaybackModeRedirectProxy))
		auth.HEAD("/Videos/:id/stream.:container", embyVideoStreamHandler(svc, service.CloudPlaybackModeRedirectProxy))
		auth.GET("/Videos/:id/original", embyVideoStreamHandler(svc, service.CloudPlaybackModeRedirectProxy))
		auth.HEAD("/Videos/:id/original", embyVideoStreamHandler(svc, service.CloudPlaybackModeRedirectProxy))
		auth.GET("/Videos/:id/original.:container", embyVideoStreamHandler(svc, service.CloudPlaybackModeRedirectProxy))
		auth.HEAD("/Videos/:id/original.:container", embyVideoStreamHandler(svc, service.CloudPlaybackModeRedirectProxy))
		if prefix == "/emby" {
			auth.GET("/api/stream/:id", embyVideoStreamHandler(svc, service.CloudPlaybackModeSTRM))
			auth.HEAD("/api/stream/:id", embyVideoStreamHandler(svc, service.CloudPlaybackModeSTRM))
		}
		auth.GET("/Videos/:id/master.m3u8", embyVideoHLSPlaylistHandler(svc))
		auth.HEAD("/Videos/:id/master.m3u8", embyVideoHLSPlaylistHandler(svc))
		auth.GET("/Videos/:id/main.m3u8", embyVideoHLSPlaylistHandler(svc))
		auth.HEAD("/Videos/:id/main.m3u8", embyVideoHLSPlaylistHandler(svc))
		auth.GET("/Videos/:id/:seg", embyVideoHLSSegmentHandler(svc))

		auth.POST("/Sessions/Playing", embyPlayingProgressHandler(svc))
		auth.POST("/Sessions/Playing/Progress", embyPlayingProgressHandler(svc))
		auth.POST("/Sessions/Playing/Stopped", embyPlayingProgressHandler(svc))

		auth.POST("/Users/:userId/FavoriteItems/:itemId", embyFavoriteHandler(svc, true))
		auth.DELETE("/Users/:userId/FavoriteItems/:itemId", embyFavoriteHandler(svc, false))
		auth.POST("/Users/:userId/PlayedItems/:itemId", embyMarkPlayedHandler(svc, true))
		auth.DELETE("/Users/:userId/PlayedItems/:itemId", embyMarkPlayedHandler(svc, false))

		auth.GET("/Sessions", embySessionsHandler(svc))
		auth.GET("/Sessions/Info", embySessionInfoHandler(svc))
		auth.GET("/System/Configuration", embyServerConfigurationHandler(svc))
		auth.GET("/System/WakeOnLanInfo", embyEmptyArrayHandler(svc))
		auth.GET("/ScheduledTasks", embyEmptyArrayHandler(svc))
		auth.GET("/LiveTv/Recordings", embyEmptyItemsHandler(svc))
		auth.GET("/System/ActivityLog/Entries", embyEmptyItemsHandler(svc))
		auth.GET("/Web/ConfigurationPages", embyEmptyArrayHandler(svc))
		auth.POST("/Users/:userId/Configuration", embyNoContentHandler(svc))

		registerLowercaseEmbyAuthRoutes(auth, svc)
	}
}

func registerLowercaseEmbyAuthRoutes(auth *gin.RouterGroup, svc *service.Container) {
	auth.GET("/users/me", embyMeHandler(svc))
	auth.GET("/users", embyListUsersHandler(svc))
	auth.GET("/users/:userId", embyGetUserByIDHandler(svc))
	auth.GET("/users/:userId/views", embyViewsHandler(svc))
	auth.GET("/library/mediafolders", embyViewsHandler(svc))
	auth.GET("/library/virtualfolders", embyVirtualFoldersHandler(svc))
	auth.GET("/library/selectablemediafolders", embyVirtualFoldersHandler(svc))

	auth.GET("/items", embyItemsHandler(svc))
	auth.GET("/users/:userId/items", embyItemsHandler(svc))
	auth.GET("/items/latest", embyLatestItemsHandler(svc))
	auth.GET("/items/resume", embyResumeItemsHandler(svc))
	auth.GET("/items/:id", embyItemByIDHandler(svc))
	auth.GET("/users/:userId/items/:id", embyUserItemByIDHandler(svc))
	auth.GET("/shows/:id/seasons", embyShowSeasonsHandler(svc))
	auth.GET("/shows/:id/episodes", embyShowEpisodesHandler(svc))
	auth.GET("/users/:userId/shows/:id/seasons", embyShowSeasonsHandler(svc))
	auth.GET("/users/:userId/shows/:id/episodes", embyShowEpisodesHandler(svc))
	auth.GET("/shows/nextup", embyEmptyItemsHandler(svc))
	auth.GET("/users/:userId/shows/nextup", embyEmptyItemsHandler(svc))
	auth.GET("/mediasegments/:id", embyEmptyItemsHandler(svc))
	auth.GET("/artists", embyEmptyItemsHandler(svc))
	auth.GET("/persons", embyEmptyItemsHandler(svc))
	auth.GET("/genres", embyEmptyItemsHandler(svc))
	auth.GET("/studios", embyEmptyItemsHandler(svc))
	auth.GET("/shows/upcoming", embyEmptyItemsHandler(svc))
	auth.GET("/users/:userId/shows/upcoming", embyEmptyItemsHandler(svc))
	auth.GET("/items/:id/similar", embyEmptyItemsHandler(svc))
	auth.GET("/items/:id/thumbnailset", embyEmptyItemsHandler(svc))
	auth.GET("/items/:id/thememedia", embyThemeMediaHandler(svc))
	auth.GET("/users/:userId/items/:id/specialfeatures", embyEmptyItemsHandler(svc))
	auth.GET("/users/:userId/items/:id/intros", embyEmptyItemsHandler(svc))
	auth.GET("/items/:id/specialfeatures", embyEmptyItemsHandler(svc))
	auth.GET("/items/:id/intros", embyEmptyItemsHandler(svc))

	auth.GET("/items/:id/playbackinfo", embyPlaybackInfoHandler(svc))
	auth.POST("/items/:id/playbackinfo", embyPlaybackInfoHandler(svc))
	auth.GET("/users/:userId/items/:id/playbackinfo", embyPlaybackInfoHandler(svc))
	auth.POST("/users/:userId/items/:id/playbackinfo", embyPlaybackInfoHandler(svc))

	auth.GET("/videos/:id/stream", embyVideoStreamHandler(svc, service.CloudPlaybackModeRedirectProxy))
	auth.HEAD("/videos/:id/stream", embyVideoStreamHandler(svc, service.CloudPlaybackModeRedirectProxy))
	auth.GET("/videos/:id/stream.:container", embyVideoStreamHandler(svc, service.CloudPlaybackModeRedirectProxy))
	auth.HEAD("/videos/:id/stream.:container", embyVideoStreamHandler(svc, service.CloudPlaybackModeRedirectProxy))
	auth.GET("/videos/:id/original", embyVideoStreamHandler(svc, service.CloudPlaybackModeRedirectProxy))
	auth.HEAD("/videos/:id/original", embyVideoStreamHandler(svc, service.CloudPlaybackModeRedirectProxy))
	auth.GET("/videos/:id/original.:container", embyVideoStreamHandler(svc, service.CloudPlaybackModeRedirectProxy))
	auth.HEAD("/videos/:id/original.:container", embyVideoStreamHandler(svc, service.CloudPlaybackModeRedirectProxy))
	auth.GET("/videos/:id/master.m3u8", embyVideoHLSPlaylistHandler(svc))
	auth.HEAD("/videos/:id/master.m3u8", embyVideoHLSPlaylistHandler(svc))
	auth.GET("/videos/:id/main.m3u8", embyVideoHLSPlaylistHandler(svc))
	auth.HEAD("/videos/:id/main.m3u8", embyVideoHLSPlaylistHandler(svc))
	auth.GET("/videos/:id/:seg", embyVideoHLSSegmentHandler(svc))

	auth.POST("/sessions/playing", embyPlayingProgressHandler(svc))
	auth.POST("/sessions/playing/progress", embyPlayingProgressHandler(svc))
	auth.POST("/sessions/playing/stopped", embyPlayingProgressHandler(svc))

	auth.POST("/users/:userId/favoriteitems/:itemId", embyFavoriteHandler(svc, true))
	auth.DELETE("/users/:userId/favoriteitems/:itemId", embyFavoriteHandler(svc, false))
	auth.POST("/users/:userId/playeditems/:itemId", embyMarkPlayedHandler(svc, true))
	auth.DELETE("/users/:userId/playeditems/:itemId", embyMarkPlayedHandler(svc, false))

	auth.GET("/sessions", embySessionsHandler(svc))
	auth.GET("/sessions/info", embySessionInfoHandler(svc))
	auth.GET("/system/configuration", embyServerConfigurationHandler(svc))
	auth.GET("/system/wakeonlaninfo", embyEmptyArrayHandler(svc))
	auth.GET("/scheduledtasks", embyEmptyArrayHandler(svc))
	auth.GET("/livetv/recordings", embyEmptyItemsHandler(svc))
	auth.GET("/system/activitylog/entries", embyEmptyItemsHandler(svc))
	auth.GET("/web/configurationpages", embyEmptyArrayHandler(svc))
	auth.POST("/users/:userId/configuration", embyNoContentHandler(svc))
}
