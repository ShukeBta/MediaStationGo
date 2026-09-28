package middleware

import (
	"encoding/json"
	"regexp"
	"strings"
	"time"

	"github.com/ShukeBta/MediaStationGo/internal/model"
	"github.com/gin-gonic/gin"
	"go.uber.org/zap"
)

var playerLogValue = regexp.MustCompile(`^[A-Za-z0-9_. ,:-]{0,128}$`)

func PlayerRequestLogger(log *zap.Logger, record func(model.PlayerRequestLog)) gin.HandlerFunc {
	return func(c *gin.Context) {
		start := time.Now()
		c.Next()
		query := map[string]string{}
		allowed := map[string]bool{"mediasourceid": true, "itemid": true, "startindex": true, "limit": true, "includeitemtypes": true, "recursive": true, "positionticks": true, "runtimeticks": true, "ispaused": true}
		for key, values := range c.Request.URL.Query() {
			if !allowed[strings.ToLower(key)] || len(values) == 0 {
				continue
			}
			value := values[0]
			if !playerLogValue.MatchString(value) {
				value = "[redacted]"
			}
			query[key] = value
		}
		body, _ := json.Marshal(query)
		route := c.FullPath()
		if route == "" {
			route = "[unmatched]"
		}
		userID, _ := c.Get(CtxUserID)
		uid, _ := userID.(string)
		row := model.PlayerRequestLog{RequestedAt: start.UTC(), Method: c.Request.Method, Route: route, Status: c.Writer.Status(), DurationMS: time.Since(start).Milliseconds(), IP: c.ClientIP(), UserID: uid, Query: string(body)}
		if record != nil {
			record(row)
		}
		if log != nil {
			log.Debug("player request", zap.String("route", route), zap.Int("status", row.Status), zap.Int64("duration_ms", row.DurationMS), zap.String("query", row.Query))
		}
	}
}
