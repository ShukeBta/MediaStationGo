// Package handler — TMDb discovery endpoints.
package handler

import (
	"net/http"

	"github.com/gin-gonic/gin"
	"go.uber.org/zap"

	"github.com/ShukeBta/MediaStationGo/internal/service"
)

// trendingHandler 返回 TMDb 当日热门列表。
//
// 当本机无法连接 TMDb（GFW / 代理未配 / API key 无效）时，TMDb 调用会
// 在 15 秒后超时；这种情况下不应该让首页显示 500 错误，而是把空列表
// 直接返回——前端按 items.length === 0 渲染"暂无推荐"即可。
func trendingHandler(svc *service.Container) gin.HandlerFunc {
	return func(c *gin.Context) {
		items, err := svc.Discover.Trending(c.Request.Context())
		if err != nil {
			svc.Log.Warn("discover trending failed (returning empty list)", zap.Error(err))
			c.JSON(http.StatusOK, gin.H{"items": []service.Match{}, "error": err.Error()})
			return
		}
		if items == nil {
			items = []service.Match{}
		}
		svc.Discover.WarmMatchArtwork(items)
		c.JSON(http.StatusOK, gin.H{"items": items})
	}
}

func popularHandler(svc *service.Container) gin.HandlerFunc {
	return func(c *gin.Context) {
		items, err := svc.Discover.Popular(c.Request.Context())
		if err != nil {
			svc.Log.Warn("discover popular failed (returning empty list)", zap.Error(err))
			c.JSON(http.StatusOK, gin.H{"items": []service.Match{}, "error": err.Error()})
			return
		}
		if items == nil {
			items = []service.Match{}
		}
		svc.Discover.WarmMatchArtwork(items)
		c.JSON(http.StatusOK, gin.H{"items": items})
	}
}
