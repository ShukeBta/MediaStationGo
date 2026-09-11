package handler

import (
	"errors"
	"net/http"
	"strconv"
	"strings"

	"github.com/gin-gonic/gin"

	"github.com/ShukeBta/MediaStationGo/internal/service"
)

// 弹幕接口。约定：
//   - 配置/上游不可用 -> 503 且带 code=danmaku_unavailable（绝不伪装成"这一集没有弹幕"）
//   - 自动匹配失败     -> 404 且带 code=danmaku_unmatched（客户端据此展示手动匹配入口）
//   - 正常返回         -> 归一化后的弹弹play 结构（comments[].cid/p/m + 结构化字段）

type danmakuUpdateRequest struct {
	Provider      string  `json:"provider"`
	EpisodeID     string  `json:"episode_id"`
	AnimeTitle    string  `json:"anime_title"`
	EpisodeTitle  string  `json:"episode_title"`
	OffsetSeconds float64 `json:"offset_seconds"`
}

func mediaDanmakuHandler(svc *service.Container) gin.HandlerFunc {
	return func(c *gin.Context) {
		if svc == nil || svc.Danmaku == nil {
			c.JSON(http.StatusServiceUnavailable, gin.H{"error": "danmaku service unavailable", "code": "danmaku_unavailable"})
			return
		}
		chConvert, ok := parseDanmakuChConvert(c)
		if !ok {
			return
		}
		offset, ok := parseDanmakuOffset(c)
		if !ok {
			return
		}
		payload, err := svc.Danmaku.Payload(c.Request.Context(), c.Param("id"), service.DanmakuOptions{
			ChConvert:     chConvert,
			OffsetSeconds: offset,
			WithRelated:   c.DefaultQuery("with_related", "true") != "false",
			ForceRefresh:  c.Query("refresh") == "true",
		})
		if err != nil {
			writeDanmakuError(c, err)
			return
		}
		c.JSON(http.StatusOK, payload)
	}
}

func mediaDanmakuStateHandler(svc *service.Container) gin.HandlerFunc {
	return func(c *gin.Context) {
		if svc == nil || svc.Danmaku == nil {
			c.JSON(http.StatusServiceUnavailable, gin.H{"error": "danmaku service unavailable", "code": "danmaku_unavailable"})
			return
		}
		state, err := svc.Danmaku.State(c.Request.Context(), c.Param("id"))
		if err != nil {
			writeDanmakuError(c, err)
			return
		}
		c.JSON(http.StatusOK, state)
	}
}

func matchMediaDanmakuHandler(svc *service.Container) gin.HandlerFunc {
	return func(c *gin.Context) {
		if svc == nil || svc.Danmaku == nil {
			c.JSON(http.StatusServiceUnavailable, gin.H{"error": "danmaku service unavailable", "code": "danmaku_unavailable"})
			return
		}
		result, err := svc.Danmaku.Match(c.Request.Context(), c.Param("id"))
		if err != nil {
			writeDanmakuError(c, err)
			return
		}
		c.JSON(http.StatusOK, result)
	}
}

func updateMediaDanmakuHandler(svc *service.Container) gin.HandlerFunc {
	return func(c *gin.Context) {
		if svc == nil || svc.Danmaku == nil {
			c.JSON(http.StatusServiceUnavailable, gin.H{"error": "danmaku service unavailable", "code": "danmaku_unavailable"})
			return
		}
		var in danmakuUpdateRequest
		if c.Request.ContentLength != 0 {
			if err := c.ShouldBindJSON(&in); err != nil {
				c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
				return
			}
		}
		ctx := c.Request.Context()
		mediaID := c.Param("id")
		// 只调 offset 时不必重新指定 episode：先落偏移，再返回最新状态。
		if strings.TrimSpace(in.EpisodeID) == "" {
			row, err := svc.Danmaku.SetOffset(ctx, mediaID, in.OffsetSeconds)
			if err != nil {
				writeDanmakuError(c, err)
				return
			}
			if row != nil {
				c.JSON(http.StatusOK, row)
				return
			}
		}
		row, err := svc.Danmaku.SetManual(ctx, mediaID, in.Provider, in.EpisodeID, in.AnimeTitle, in.EpisodeTitle, in.OffsetSeconds)
		if err != nil {
			writeDanmakuError(c, err)
			return
		}
		c.JSON(http.StatusOK, row)
	}
}

func deleteMediaDanmakuHandler(svc *service.Container) gin.HandlerFunc {
	return func(c *gin.Context) {
		if svc == nil || svc.Danmaku == nil {
			c.JSON(http.StatusServiceUnavailable, gin.H{"error": "danmaku service unavailable", "code": "danmaku_unavailable"})
			return
		}
		if err := svc.Danmaku.Clear(c.Request.Context(), c.Param("id")); err != nil {
			writeDanmakuError(c, err)
			return
		}
		c.JSON(http.StatusOK, gin.H{"deleted": true})
	}
}

func searchDanmakuHandler(svc *service.Container) gin.HandlerFunc {
	return func(c *gin.Context) {
		if svc == nil || svc.Danmaku == nil {
			c.JSON(http.StatusServiceUnavailable, gin.H{"error": "danmaku service unavailable", "code": "danmaku_unavailable"})
			return
		}
		keyword := strings.TrimSpace(c.Query("keyword"))
		if keyword == "" {
			c.JSON(http.StatusBadRequest, gin.H{"error": "keyword is required"})
			return
		}
		episode := 0
		if raw := strings.TrimSpace(c.Query("episode")); raw != "" {
			value, err := strconv.Atoi(raw)
			if err != nil || value < 1 || value > 9999 {
				c.JSON(http.StatusBadRequest, gin.H{"error": "episode must be between 1 and 9999"})
				return
			}
			episode = value
		}
		result, err := svc.Danmaku.Search(c.Request.Context(), keyword, episode)
		if err != nil {
			writeDanmakuError(c, err)
			return
		}
		c.JSON(http.StatusOK, result)
	}
}

// embyDanmuRawHandler 是 Emby 客户端探测路径的兼容端点，返回 B 站 XML。
//
// 这个端点刻意保持 200：它的存在就是为了避免客户端因为 404 而中断播放。
// 取不到弹幕时返回空的 <i> 文档，并用 X-Danmaku-Status 说明原因，不静默假装有数据。
func embyDanmuRawHandler(svc *service.Container) gin.HandlerFunc {
	return func(c *gin.Context) {
		if svc == nil || svc.Danmaku == nil {
			c.Header("X-Danmaku-Status", "unavailable")
			c.Data(http.StatusOK, "application/xml; charset=utf-8", service.DanmakuXML(service.DanmakuPayload{}))
			return
		}
		payload, err := svc.Danmaku.Payload(c.Request.Context(), c.Param("id"), service.DanmakuOptions{
			WithRelated: true,
		})
		if err != nil {
			status := "unavailable"
			if errors.Is(err, service.ErrDanmakuUnmatched) {
				status = "unmatched"
			}
			c.Header("X-Danmaku-Status", status)
			c.Data(http.StatusOK, "application/xml; charset=utf-8", service.DanmakuXML(service.DanmakuPayload{}))
			return
		}
		c.Header("X-Danmaku-Status", "matched")
		c.Data(http.StatusOK, "application/xml; charset=utf-8", service.DanmakuXML(payload))
	}
}

func parseDanmakuChConvert(c *gin.Context) (int, bool) {
	raw := strings.TrimSpace(c.DefaultQuery("ch_convert", "0"))
	value, err := strconv.Atoi(raw)
	if err != nil || value < 0 || value > 2 {
		c.JSON(http.StatusBadRequest, gin.H{"error": "ch_convert must be 0, 1 or 2"})
		return 0, false
	}
	return value, true
}

func parseDanmakuOffset(c *gin.Context) (float64, bool) {
	raw := strings.TrimSpace(c.Query("offset_seconds"))
	if raw == "" {
		return 0, true
	}
	value, err := strconv.ParseFloat(raw, 64)
	if err != nil || value < -600 || value > 600 {
		c.JSON(http.StatusBadRequest, gin.H{"error": "offset_seconds must be a number within -600..600"})
		return 0, false
	}
	return value, true
}

func writeDanmakuError(c *gin.Context, err error) {
	switch {
	case errors.Is(err, service.ErrDanmakuInvalidInput):
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
	case errors.Is(err, service.ErrDanmakuUnavailable):
		c.JSON(http.StatusServiceUnavailable, gin.H{"error": err.Error(), "code": "danmaku_unavailable"})
	case errors.Is(err, service.ErrDanmakuUnmatched):
		c.JSON(http.StatusNotFound, gin.H{"error": err.Error(), "code": "danmaku_unmatched"})
	default:
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
	}
}
