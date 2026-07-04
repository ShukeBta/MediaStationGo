package handler

import (
	"net/http"
	"strings"

	"github.com/gin-gonic/gin"
	"github.com/golang-jwt/jwt/v5"

	"github.com/ShukeBta/MediaStationGo/internal/middleware"
	"github.com/ShukeBta/MediaStationGo/internal/service"
)

func embySessionsHandler(svc *service.Container) gin.HandlerFunc {
	return func(c *gin.Context) {
		if svc.Sessions == nil {
			c.JSON(http.StatusOK, []any{})
			return
		}
		out := make([]gin.H, 0)
		for _, sess := range svc.Sessions.List(c.Request.Context()) {
			last := sess.LastActivityAt
			itemID := sess.ItemID
			playState := gin.H{
				"PositionTicks": sess.PositionTicks,
				"IsPaused":      sess.IsPaused,
				"PlayMethod":    "DirectStream",
				"CanSeek":       true,
			}
			row := gin.H{
				"Id":                    sess.ID,
				"ServerId":              "mediastation-go-001",
				"Client":                sess.Client,
				"DeviceId":              sess.DeviceID,
				"DeviceName":            sess.DeviceName,
				"UserId":                sess.UserID,
				"UserName":              sess.UserName,
				"LastActivityDate":      last,
				"RemoteEndPoint":        sess.RemoteEndPoint,
				"PlayState":             playState,
				"SupportsRemoteControl": true,
			}
			if itemID != "" && sess.IsPlaying {
				row["NowPlayingItem"] = gin.H{"Id": itemID}
			}
			out = append(out, row)
		}
		c.Header("Cache-Control", "no-store")
		c.JSON(http.StatusOK, out)
	}
}

func embySessionLogoutHandler(svc *service.Container, jwtSecret string) gin.HandlerFunc {
	return func(c *gin.Context) {
		if svc.Sessions != nil {
			uid, _ := embyPublicSessionIdentity(c, svc, jwtSecret)
			clientInfo := embyClientInfoFromRequest(c)
			svc.Sessions.Logout(c.Request.Context(), uid, clientInfo.DeviceID, c.ClientIP())
		}
		c.Status(http.StatusNoContent)
	}
}

func embySessionCapabilitiesHandler(svc *service.Container, jwtSecret string) gin.HandlerFunc {
	return func(c *gin.Context) {
		recordEmbyPublicSessionActivity(c, svc, jwtSecret)
		c.Status(http.StatusNoContent)
	}
}

func recordEmbyPublicSessionActivity(c *gin.Context, svc *service.Container, jwtSecret string) {
	uid, username := embyPublicSessionIdentity(c, svc, jwtSecret)
	recordEmbySessionActivity(c, svc, uid, username)
}

func embyPublicSessionIdentity(c *gin.Context, svc *service.Container, jwtSecret string) (string, string) {
	if uid := embyUserID(c); uid != "" {
		return uid, embyContextUserName(c)
	}
	token := embyRequestToken(c)
	if strings.TrimSpace(token) == "" || strings.TrimSpace(jwtSecret) == "" {
		return "", ""
	}
	claims := &middleware.Claims{}
	parsed, err := jwt.ParseWithClaims(token, claims, func(t *jwt.Token) (interface{}, error) {
		if _, ok := t.Method.(*jwt.SigningMethodHMAC); !ok {
			return nil, jwt.ErrTokenSignatureInvalid
		}
		return []byte(jwtSecret), nil
	})
	if err != nil || !parsed.Valid {
		return "", ""
	}
	if strings.TrimSpace(claims.Purpose) != "" {
		return "", ""
	}
	uid := strings.TrimSpace(claims.UserID)
	if uid == "" {
		return "", ""
	}
	if svc != nil && svc.Repo != nil && svc.Repo.User != nil {
		if user, err := svc.Repo.User.FindByID(c.Request.Context(), uid); err == nil && user != nil {
			return uid, user.Username
		}
	}
	return uid, ""
}
