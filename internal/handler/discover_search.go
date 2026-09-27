package handler

import (
	"context"
	"errors"
	"net/http"
	"strings"
	"time"

	"github.com/gin-gonic/gin"
	"go.uber.org/zap"

	"github.com/ShukeBta/MediaStationGo/internal/service"
)

const discoverSearchTimeout = 5 * time.Second

var discoverSearchSourceLabels = map[string]string{
	"tmdb_movie":       "TMDb 电影",
	"tmdb_tv":          "TMDb 剧集",
	"douban":           "豆瓣",
	"bangumi":          "Bangumi 动漫",
	"javdb_performer":  "JavDB 女优",
	"javdb_adult":      "JavDB 作品",
	"fd2ppv_performer": "FC2 演员",
	"fd2ppv_adult":     "FC2 作品",
}

func discoverSearchHandler(svc *service.Container) gin.HandlerFunc {
	return func(c *gin.Context) {
		query := strings.TrimSpace(c.Query("q"))
		if length := len([]rune(query)); length < 1 || length > 100 {
			c.JSON(http.StatusBadRequest, gin.H{"error": "搜索词长度需为 1 到 100 个字符"})
			return
		}

		var tmdb *service.TMDbProvider
		if discoverProviderEnabled(c.Request.Context(), svc, "tmdb") {
			tmdb = svc.TMDb
		}
		var douban *service.DoubanProvider
		if discoverProviderEnabled(c.Request.Context(), svc, "douban") {
			douban = svc.Douban
		}
		var bangumi *service.BangumiProvider
		if discoverProviderEnabled(c.Request.Context(), svc, "bangumi") {
			bangumi = svc.Bangumi
		}
		var adult *service.AdultProvider
		if discoverAdultAllowed(c, svc) && discoverProviderEnabled(c.Request.Context(), svc, "adult") {
			adult = svc.Adult
		}

		searchCtx, cancel := context.WithTimeout(c.Request.Context(), discoverSearchTimeout)
		defer cancel()
		result := service.SearchDiscoverCatalog(searchCtx, query, tmdb, douban, bangumi, adult)
		if adult != nil {
			if err := markAdultPerformerFollows(c.Request.Context(), svc, currentUserID(c), result.Items); err != nil {
				c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
				return
			}
		}
		if source := strings.TrimSpace(c.Query("source")); source != "" && source != "all" {
			filtered := result.Items[:0]
			for _, item := range result.Items {
				if strings.EqualFold(item.Source, source) {
					filtered = append(filtered, item)
				}
			}
			result.Items = filtered
		}
		if mediaType := strings.TrimSpace(c.Query("media_type")); mediaType != "" && mediaType != "all" {
			filtered := result.Items[:0]
			for _, item := range result.Items {
				if strings.EqualFold(item.MediaType, mediaType) {
					filtered = append(filtered, item)
				}
			}
			result.Items = filtered
		}
		service.EnrichExternalMediaAvailability(c.Request.Context(), svc.Repo, result.Items)
		service.EnrichExternalMediaLibraryLinks(
			c.Request.Context(), svc.Repo, result.Items, mediaVisibilityForRequest(c, svc),
		)
		if svc.Discover != nil {
			svc.Discover.WarmExternalArtwork(result.Items)
		}

		errorsBySource := make(map[string]string, len(result.Errors))
		for source, err := range result.Errors {
			errorsBySource[source] = discoverSearchSourceErrorMessage(source, err)
			if svc.Log != nil {
				svc.Log.Warn("discover search source failed",
					zap.String("source", source), zap.String("query", query), zap.Error(err))
			}
		}
		c.JSON(http.StatusOK, gin.H{"items": result.Items, "errors": errorsBySource})
	}
}

func discoverSearchSourceErrorMessage(source string, err error) string {
	label := discoverSearchSourceLabels[source]
	if label == "" {
		label = source
	}
	if errorsIsTimeout(err) {
		return label + "搜索超时"
	}
	return label + "搜索暂不可用"
}

func errorsIsTimeout(err error) bool {
	if err == nil {
		return false
	}
	if errors.Is(err, context.DeadlineExceeded) {
		return true
	}
	var timeout interface{ Timeout() bool }
	return strings.Contains(strings.ToLower(err.Error()), "timeout") ||
		strings.Contains(strings.ToLower(err.Error()), "deadline exceeded") ||
		(errors.As(err, &timeout) && timeout.Timeout())
}
