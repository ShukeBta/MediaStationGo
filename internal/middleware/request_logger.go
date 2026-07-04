package middleware

import (
	"strings"
	"time"

	"github.com/gin-gonic/gin"
	"go.uber.org/zap"
)

// RequestLogger logs one structured line per request.
//
// 健康检查与静态资源的成功请求被跳过：healthcheck 每 30s 一次、SPA 静态
// 文件每页几十个请求，全部记 INFO 会让日志在几小时内膨胀到几十 MB，
// 在 Docker json-file 日志驱动下白白消耗磁盘 IO。
func RequestLogger(log *zap.Logger) gin.HandlerFunc {
	return func(c *gin.Context) {
		start := time.Now()
		c.Next()
		path := c.Request.URL.Path
		status := c.Writer.Status()
		if status < 400 {
			if path == "/api/health" || strings.HasPrefix(path, "/assets/") ||
				path == "/favicon.ico" || path == "/favicon.svg" {
				return
			}
		}
		log.Info("http",
			zap.String("method", c.Request.Method),
			zap.String("path", path),
			zap.Int("status", status),
			zap.Duration("dur", time.Since(start)),
			zap.String("ip", c.ClientIP()),
		)
	}
}
