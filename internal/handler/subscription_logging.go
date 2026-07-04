package handler

import (
	"net/url"
	"strings"

	"github.com/gin-gonic/gin"
	"go.uber.org/zap"

	"github.com/ShukeBta/MediaStationGo/internal/middleware"
	"github.com/ShukeBta/MediaStationGo/internal/service"
)

func subscriptionRequestUserID(c *gin.Context) string {
	if c == nil {
		return ""
	}
	if uid, ok := c.Get(middleware.CtxUserID); ok {
		if userID, ok := uid.(string); ok {
			return userID
		}
	}
	return ""
}

func subscriptionFeedKind(feedURL string) string {
	raw := strings.TrimSpace(feedURL)
	if raw == "" {
		return "empty"
	}
	lower := strings.ToLower(raw)
	if strings.HasPrefix(lower, "site-search://") {
		return "site-search"
	}
	parsed, err := url.Parse(raw)
	if err == nil && parsed.Scheme != "" {
		return parsed.Scheme
	}
	return "unknown"
}

func logSubscriptionInfo(svc *service.Container, msg string, fields ...zap.Field) {
	if svc == nil || svc.Log == nil {
		return
	}
	svc.Log.Info(msg, fields...)
}

func logSubscriptionWarn(svc *service.Container, msg string, fields ...zap.Field) {
	if svc == nil || svc.Log == nil {
		return
	}
	svc.Log.Warn(msg, fields...)
}
