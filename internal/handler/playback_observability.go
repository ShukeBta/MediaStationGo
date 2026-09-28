package handler

import (
	"errors"
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/ShukeBta/MediaStationGo/internal/repository"
	"github.com/ShukeBta/MediaStationGo/internal/service"
	"github.com/gin-gonic/gin"
	"go.uber.org/zap"
)

func operationalPage(c *gin.Context) (int, int, error) {
	page, err := strconv.Atoi(c.DefaultQuery("page", "1"))
	if err != nil || page < 1 || page > 1_000_000 {
		return 0, 0, errors.New("无效的页码")
	}
	size, err := strconv.Atoi(c.DefaultQuery("page_size", "25"))
	if err != nil || size < 1 || size > 100 {
		return 0, 0, errors.New("每页条数须为 1–100")
	}
	return page, size, nil
}

func operationalRange(c *gin.Context, days int) (time.Time, time.Time, error) {
	now := time.Now().UTC()
	from, err := time.Parse("2006-01-02", c.DefaultQuery("from", now.AddDate(0, 0, -days+1).Format("2006-01-02")))
	if err != nil {
		return time.Time{}, time.Time{}, errors.New("无效的开始日期")
	}
	to, err := time.Parse("2006-01-02", c.DefaultQuery("to", now.Format("2006-01-02")))
	if err != nil || from.After(to) || to.Sub(from) > 366*24*time.Hour {
		return time.Time{}, time.Time{}, errors.New("日期范围须按顺序且不超过一年")
	}
	return from, to.AddDate(0, 0, 1), nil
}

func playbackStatsHandler(svc *service.Container) gin.HandlerFunc {
	return func(c *gin.Context) {
		page, size, err := operationalPage(c)
		if err != nil {
			c.JSON(400, gin.H{"error": err.Error()})
			return
		}
		from, to, err := operationalRange(c, 30)
		if err != nil {
			c.JSON(400, gin.H{"error": err.Error()})
			return
		}
		grain := c.DefaultQuery("grain", "day")
		if grain != "day" && grain != "week" && grain != "month" {
			c.JSON(400, gin.H{"error": "粒度须为 day/week/month"})
			return
		}
		kind := c.Query("media_type")
		if kind != "" && kind != "movie" && kind != "tv" {
			c.JSON(400, gin.H{"error": "媒体类型无效"})
			return
		}
		rankStart, err := time.Parse("2006-01-02", c.DefaultQuery("rank_date", to.AddDate(0, 0, -1).Format("2006-01-02")))
		if err != nil || rankStart.Before(from) || !rankStart.Before(to) {
			c.JSON(400, gin.H{"error": "热榜日期须在所选范围内"})
			return
		}
		rankEnd := rankStart.AddDate(0, 0, 1)
		switch c.DefaultQuery("rank_grain", "day") {
		case "day":
		case "week":
			rankStart = rankStart.AddDate(0, 0, -(int(rankStart.Weekday())+6)%7)
			rankEnd = rankStart.AddDate(0, 0, 7)
		default:
			c.JSON(400, gin.H{"error": "热榜粒度须为 day/week"})
			return
		}
		if rankStart.Before(from) {
			rankStart = from
		}
		if rankEnd.After(to) {
			rankEnd = to
		}
		result, err := svc.Repo.PlaybackStats(c.Request.Context(), repository.PlaybackStatsFilter{From: from, To: to, RankFrom: rankStart, RankTo: rankEnd, Grain: grain, MediaType: kind, UserID: c.Query("user_id"), LibraryID: c.Query("library_id"), Page: page, PageSize: size})
		if err != nil {
			c.JSON(500, gin.H{"error": err.Error()})
			return
		}
		c.JSON(http.StatusOK, result)
	}
}

func playerRequestLogsHandler(svc *service.Container) gin.HandlerFunc {
	return func(c *gin.Context) {
		page, size, err := operationalPage(c)
		if err != nil {
			c.JSON(400, gin.H{"error": err.Error()})
			return
		}
		from, to, err := operationalRange(c, 1)
		if err != nil {
			c.JSON(400, gin.H{"error": err.Error()})
			return
		}
		status := 0
		if raw := c.Query("status"); raw != "" {
			status, err = strconv.Atoi(raw)
			if err != nil || status < 100 || status > 599 {
				c.JSON(400, gin.H{"error": "无效HTTP状态码"})
				return
			}
		}
		method := strings.ToUpper(strings.TrimSpace(c.Query("method")))
		if len(method) > 16 || len(c.Query("route")) > 255 {
			c.JSON(400, gin.H{"error": "筛选参数过长"})
			return
		}
		items, total, err := svc.Repo.ListPlayerRequestLogs(c.Request.Context(), repository.PlayerRequestLogFilter{From: from, To: to, Route: c.Query("route"), Method: method, Status: status, UserID: c.Query("user_id"), Page: page, PageSize: size})
		if err != nil {
			c.JSON(500, gin.H{"error": err.Error()})
			return
		}
		c.JSON(200, gin.H{"items": items, "total": total, "page": page, "page_size": size, "dropped": svc.PlayerRequestLogs.Dropped()})
	}
}

func recordPlaybackStats(c *gin.Context, svc *service.Container, mediaID, sessionID, deviceID, client string, positionMS, durationMS int64, stopped bool) {
	uid := currentUserID(c)
	if uid == "" {
		uid = embyUserID(c)
	}
	if svc.Sessions != nil {
		sessionID = svc.Sessions.PlaybackEventSession(uid, deviceID, mediaID, sessionID, stopped)
	}
	if err := svc.RecordPlaybackEvent(c.Request.Context(), uid, mediaID, sessionID, client, positionMS, durationMS); err != nil && svc.Log != nil {
		svc.Log.Warn("record playback statistics failed", zap.Error(err))
	}
}
