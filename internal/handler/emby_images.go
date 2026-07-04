package handler

import (
	"context"
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/gin-gonic/gin"

	"github.com/ShukeBta/MediaStationGo/internal/service"
)

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
		clearEmbyImageNoStoreHeaders(c)
		ctx, cancel := context.WithTimeout(c.Request.Context(), 8*time.Second)
		defer cancel()
		req := c.Request.WithContext(ctx)
		id := c.Param("id")
		imgType := strings.ToLower(c.Param("type"))
		raw, err := svc.Emby.ImageURL(ctx, id, imgType)
		if err != nil || raw == "" {
			embyServePlaceholderImage(c)
			return
		}
		if typ, ref, ok := service.ParseCloudArtworkURL(raw); ok {
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

func clearEmbyImageNoStoreHeaders(c *gin.Context) {
	c.Writer.Header().Del("Pragma")
	c.Writer.Header().Del("Expires")
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
