package handler

import (
	"net/http"
	"strings"

	"github.com/gin-gonic/gin"

	"github.com/ShukeBta/MediaStationGo/internal/service"
)

// ─── Users / Auth ────────────────────────────────────────────────────────────

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
		if svc.Sessions != nil {
			svc.Sessions.RecordLogin(c.Request.Context(), resp.User.ID, resp.User.Username,
				clientInfo.DeviceID,
				clientInfo.DeviceName,
				clientInfo.Client,
				c.ClientIP())
		}
		if svc.Device != nil {
			svc.Device.RecordLogin(c.Request.Context(), resp.User.ID,
				clientInfo.DeviceID,
				clientInfo.DeviceName,
				clientInfo.Client,
				c.ClientIP())
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
		embyRememberCompatSession(c, accessToken)
		c.JSON(http.StatusOK, gin.H{
			"AccessToken": accessToken,
			"ServerId":    "mediastation-go-001",
			"User":        userPayload,
			"SessionInfo": gin.H{
				"Id":         resp.User.ID,
				"UserId":     resp.User.ID,
				"UserName":   resp.User.Username,
				"Client":     clientInfo.Client,
				"DeviceId":   clientInfo.DeviceID,
				"DeviceName": clientInfo.DeviceName,
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
