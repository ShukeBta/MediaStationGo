// Package handler — Emby/Jellyfin compatibility shim.
//
// 路由挂在 /emby/* 和根路径下双前缀。Infuse / Yamby / Hills /
// Senplayer / Kodi 这类客户端会自动尝试 /System/Info 与 /emby/System/Info
// 两种 URL，我们都接住。
package handler

import (
	"errors"
	"io"
	"net/http"
	"strconv"
	"strings"
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
	if strings.HasPrefix(strings.ToLower(c.Request.URL.Path), "/emby") {
		out["ProductName"] = "Emby Server"
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

// ─── Users / Auth ────────────────────────────────────────────────────────────

type embyAuthByNameReq struct {
	Username string `json:"Username"`
	Pw       string `json:"Pw"`
	Password string `json:"Password"`
}

func parseEmbyAuthByNameReq(c *gin.Context) (embyAuthByNameReq, error) {
	req := embyAuthByNameReq{}
	if strings.Contains(strings.ToLower(c.GetHeader("Content-Type")), "json") {
		var body map[string]any
		if err := c.ShouldBindJSON(&body); err != nil && !errors.Is(err, io.EOF) {
			return req, err
		}
		req.Username = firstStringFromMap(body, "Username", "username", "Name", "name")
		req.Pw = firstStringFromMap(body, "Pw", "pw")
		req.Password = firstStringFromMap(body, "Password", "password")
	}

	if req.Username == "" || (req.Pw == "" && req.Password == "") {
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
	return req, nil
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
			embyError(c, http.StatusBadRequest, "missing username or password")
			return
		}
		resp, err := svc.Auth.Login(c.Request.Context(), req.Username, password)
		if err != nil {
			embyError(c, http.StatusUnauthorized, err.Error())
			return
		}
		userPayload, _ := svc.Emby.FindUser(c.Request.Context(), resp.User.ID)
		// Emby/Jellyfin 客户端没有 refresh token 机制：它们把这里返回的
		// AccessToken 长期保存并反复使用。若返回 60 分钟的普通 access
		// token，客户端每小时就会掉登录、无法播放、媒体库无法刷新。因此
		// 签发长期令牌（IssueEmbyToken）匹配 Emby 持久化令牌语义。
		accessToken := resp.Tokens.AccessToken
		if longLived, err := svc.Auth.IssueEmbyToken(resp.User); err == nil && longLived != "" {
			accessToken = longLived
		}
		c.JSON(http.StatusOK, gin.H{
			"AccessToken": accessToken,
			"ServerId":    "mediastation-go-001",
			"User":        userPayload,
			"SessionInfo": gin.H{
				"Id":         resp.User.ID,
				"UserId":     resp.User.ID,
				"UserName":   resp.User.Username,
				"Client":     c.GetHeader("X-Emby-Client"),
				"DeviceId":   c.GetHeader("X-Emby-Device-Id"),
				"DeviceName": c.GetHeader("X-Emby-Device-Name"),
			},
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
		"Name":                      "MediaStation",
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
		c.JSON(http.StatusOK, out)
	}
}

func embyVirtualFoldersHandler(svc *service.Container) gin.HandlerFunc {
	return func(c *gin.Context) {
		c.Header("Cache-Control", "no-store")
		libs, err := svc.Repo.Library.List(c.Request.Context())
		if err != nil {
			c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
			return
		}
		uid := embyUserID(c)
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
	}
}

// ─── Items ───────────────────────────────────────────────────────────────────

func parseEmbyItemsParams(c *gin.Context) service.ItemsParams {
	limit, _ := strconv.Atoi(c.DefaultQuery("Limit", "50"))
	offset, _ := strconv.Atoi(c.DefaultQuery("StartIndex", "0"))
	uid := c.Param("userId")
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
		ParentID:         c.Query("ParentId"),
		IDs:              splitOpt(c.Query("Ids")),
		SearchTerm:       c.Query("SearchTerm"),
		IncludeItemTypes: splitOpt(c.Query("IncludeItemTypes")),
		Recursive:        strings.EqualFold(c.Query("Recursive"), "true"),
		SortBy:           c.Query("SortBy"),
		SortOrder:        c.Query("SortOrder"),
		Limit:            limit,
		StartIndex:       offset,
	}
}

func embyItemsHandler(svc *service.Container) gin.HandlerFunc {
	return func(c *gin.Context) {
		out, err := svc.Emby.Items(c.Request.Context(), parseEmbyItemsParams(c))
		if err != nil {
			c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
			return
		}
		c.JSON(http.StatusOK, out)
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
			uid = embyUserID(c)
		}
		limit, _ := strconv.Atoi(c.DefaultQuery("Limit", "20"))
		out, err := svc.Emby.LatestItems(c.Request.Context(), uid, c.Query("ParentId"), limit)
		if err != nil {
			c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
			return
		}
		c.JSON(http.StatusOK, out)
	}
}

func embyResumeItemsHandler(svc *service.Container) gin.HandlerFunc {
	return func(c *gin.Context) {
		uid := c.Param("userId")
		if uid == "" {
			uid = embyUserID(c)
		}
		limit, _ := strconv.Atoi(c.DefaultQuery("Limit", "20"))
		out, err := svc.Emby.ResumeItems(c.Request.Context(), uid, limit)
		if err != nil {
			c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
			return
		}
		c.JSON(http.StatusOK, out)
	}
}

// ─── Images ──────────────────────────────────────────────────────────────────

// embyItemImageHandler 把 /Items/{id}/Images/Primary 等请求直接输出为图片。
// Emby 客户端缓存图片 URL 时经常不会继续携带 token；如果重定向到受保护的
// /api/img 会变成 401，所以这里复用 ImageProxy 但不再走 /api 路由。
func embyItemImageHandler(svc *service.Container) gin.HandlerFunc {
	return func(c *gin.Context) {
		id := c.Param("id")
		imgType := strings.ToLower(c.Param("type"))
		raw, err := svc.Emby.ImageURL(c.Request.Context(), id, imgType)
		if err != nil || raw == "" {
			c.Status(http.StatusNotFound)
			return
		}
		if svc.ImageProxy == nil {
			c.Status(http.StatusNotFound)
			return
		}
		if err := svc.ImageProxy.Serve(c.Request.Context(), c.Writer, c.Request, raw); err != nil {
			c.Status(http.StatusNotFound)
		}
	}
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
		c.JSON(http.StatusOK, out)
	}
}

func embyShowEpisodesHandler(svc *service.Container) gin.HandlerFunc {
	return func(c *gin.Context) {
		parentID := firstQueryValue(c, "SeasonId", "seasonId")
		if parentID == "" {
			parentID = c.Param("id")
		}
		params := service.ItemsParams{
			UserID:           firstQueryValue(c, "UserId", "userId"),
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
		c.JSON(http.StatusOK, out)
	}
}

// embyVideoStreamHandler 是 GET /Videos/{id}/stream 的入口，
// 直接代理到我们的 /api/stream/{id}（同一个 ServeFile）。
func embyVideoStreamHandler(svc *service.Container) gin.HandlerFunc {
	return func(c *gin.Context) {
		uid := embyUserID(c)
		item, err := svc.Emby.Item(c.Request.Context(), c.Param("id"), uid)
		if err != nil || item == nil {
			c.Status(http.StatusNotFound)
			return
		}
		// 直接调用 Stream service 写入 response
		err = svc.Stream.ServeFile(c.Writer, c.Request, c.Param("id"))
		if err != nil {
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
		_ = svc.Emby.RecordProgress(c.Request.Context(), uid, req.ItemId, req.PositionTicks, req.RunTimeTicks)
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
		out, _ := svc.Emby.Item(c.Request.Context(), mid, uid)
		if out != nil {
			c.JSON(http.StatusOK, out["UserData"])
			return
		}
		c.JSON(http.StatusOK, gin.H{"Played": played})
	}
}

// ─── Sessions / Branding 占位 ────────────────────────────────────────────────

func embySessionsHandler(_ *service.Container) gin.HandlerFunc {
	return func(c *gin.Context) {
		c.JSON(http.StatusOK, []any{})
	}
}

func embyEmptyItemsHandler(_ *service.Container) gin.HandlerFunc {
	return func(c *gin.Context) {
		c.JSON(http.StatusOK, gin.H{"Items": []any{}, "TotalRecordCount": 0})
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
		for _, path := range []string{"/System/Ping", "/system/ping"} {
			grp.GET(path, embyPingHandler(svc))
			grp.HEAD(path, embyPingHandler(svc))
			grp.POST(path, embyPingHandler(svc))
		}
		// 30/min per IP: many Emby clients sit behind a single NAT/reverse-proxy
		// IP, so a low limit would throttle legitimate logins into 429s.
		embyLoginLimiter := middleware.NewRateLimiter(30, 1*time.Minute)
		for _, path := range []string{"/Users/AuthenticateByName", "/users/authenticatebyname"} {
			grp.POST(path, middleware.RateLimit(embyLoginLimiter), embyAuthByNameHandler(svc))
		}
		for _, path := range []string{"/Users/Public", "/users/public"} {
			grp.GET(path, embyPublicUsersHandler(svc))
		}
		for _, path := range []string{"/Branding/Configuration", "/branding/configuration"} {
			grp.GET(path, embyBrandingConfigHandler(svc))
		}
		for _, path := range []string{"/Localization/Options", "/localization/options"} {
			grp.GET(path, embyLocalizationOptionsHandler(svc))
		}

		// 图片公开（Infuse 缓存 URL 时会丢 token）
		grp.GET("/Items/:id/Images/:type", embyItemImageHandler(svc))
		grp.GET("/Items/:id/Images/:type/:index", embyItemImageHandler(svc))
		grp.HEAD("/Items/:id/Images/:type", embyItemImageHandler(svc))

		// 鉴权后端点
		auth := grp.Group("", middleware.EmbyAuthRequired(jwtSecret))
		auth.GET("/Users/Me", embyMeHandler(svc))
		auth.GET("/Users", embyListUsersHandler(svc))
		auth.GET("/Users/:userId", embyGetUserByIDHandler(svc))
		auth.GET("/Users/:userId/Views", embyViewsHandler(svc))
		auth.GET("/Library/MediaFolders", embyViewsHandler(svc))
		auth.GET("/Library/VirtualFolders", embyVirtualFoldersHandler(svc))
		auth.GET("/Library/SelectableMediaFolders", embyVirtualFoldersHandler(svc))

		auth.GET("/Items", embyItemsHandler(svc))
		auth.GET("/Users/:userId/Items", embyItemsHandler(svc))
		auth.GET("/Items/:id", embyItemByIDHandler(svc))
		auth.GET("/Users/:userId/Items/:id", embyUserItemByIDHandler(svc))
		auth.GET("/Shows/:id/Seasons", embyShowSeasonsHandler(svc))
		auth.GET("/Shows/:id/Episodes", embyShowEpisodesHandler(svc))
		auth.GET("/Users/:userId/Shows/:id/Seasons", embyShowSeasonsHandler(svc))
		auth.GET("/Users/:userId/Shows/:id/Episodes", embyShowEpisodesHandler(svc))
		auth.GET("/Shows/NextUp", embyEmptyItemsHandler(svc))
		auth.GET("/Users/:userId/Shows/NextUp", embyEmptyItemsHandler(svc))
		auth.GET("/MediaSegments/:id", embyEmptyItemsHandler(svc))

		auth.GET("/Items/:id/PlaybackInfo", embyPlaybackInfoHandler(svc))
		auth.POST("/Items/:id/PlaybackInfo", embyPlaybackInfoHandler(svc))
		auth.GET("/Users/:userId/Items/:id/PlaybackInfo", embyPlaybackInfoHandler(svc))
		auth.POST("/Users/:userId/Items/:id/PlaybackInfo", embyPlaybackInfoHandler(svc))

		auth.GET("/Videos/:id/stream", embyVideoStreamHandler(svc))
		auth.HEAD("/Videos/:id/stream", embyVideoStreamHandler(svc))
		auth.GET("/Videos/:id/stream.:container", embyVideoStreamHandler(svc))
		auth.HEAD("/Videos/:id/stream.:container", embyVideoStreamHandler(svc))
		auth.GET("/Videos/:id/original", embyVideoStreamHandler(svc))
		auth.GET("/Videos/:id/original.:container", embyVideoStreamHandler(svc))

		auth.POST("/Sessions/Playing", embyPlayingProgressHandler(svc))
		auth.POST("/Sessions/Playing/Progress", embyPlayingProgressHandler(svc))
		auth.POST("/Sessions/Playing/Stopped", embyPlayingProgressHandler(svc))

		auth.POST("/Users/:userId/FavoriteItems/:itemId", embyFavoriteHandler(svc, true))
		auth.DELETE("/Users/:userId/FavoriteItems/:itemId", embyFavoriteHandler(svc, false))
		auth.POST("/Users/:userId/PlayedItems/:itemId", embyMarkPlayedHandler(svc, true))
		auth.DELETE("/Users/:userId/PlayedItems/:itemId", embyMarkPlayedHandler(svc, false))

		auth.GET("/Sessions", embySessionsHandler(svc))
		auth.GET("/DisplayPreferences/:id", func(c *gin.Context) {
			c.JSON(http.StatusOK, gin.H{"Id": c.Param("id"), "CustomPrefs": gin.H{}})
		})
		auth.POST("/DisplayPreferences/:id", func(c *gin.Context) {
			c.Status(http.StatusNoContent)
		})
	}
}
