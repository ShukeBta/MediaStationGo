// Package handler — HLS / image-proxy / scrape endpoints.
package handler

import (
	"context"
	"errors"
	"io"
	"net/http"

	"github.com/gin-gonic/gin"

	"github.com/ShukeBta/MediaStationGo/internal/service"
)

func hlsPlaylistHandler(svc *service.Container) gin.HandlerFunc {
	return func(c *gin.Context) {
		m, err := svc.Media.GetMedia(c.Request.Context(), c.Param("id"))
		if err != nil || m == nil || !mediaVisibleForRequest(c, svc, m) {
			c.JSON(http.StatusNotFound, gin.H{"error": "not found"})
			return
		}
		if !enforceScopedPlaybackToken(c, m.ID) {
			return
		}
		err = svc.Stream.ServeHLSPlaylist(c.Writer, c.Request, c.Param("id"))
		if errors.Is(err, service.ErrMediaNotFound) {
			c.JSON(http.StatusNotFound, gin.H{"error": "not found"})
			return
		}
		if errors.Is(err, service.ErrTranscodeDisabled) {
			c.JSON(http.StatusConflict, gin.H{"error": "transcode disabled"})
			return
		}
		if errors.Is(err, service.ErrTranscodeBusy) {
			c.JSON(http.StatusTooManyRequests, gin.H{"error": "transcode busy"})
			return
		}
		if err != nil {
			c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
			return
		}
	}
}

func hlsSegmentHandler(svc *service.Container) gin.HandlerFunc {
	return func(c *gin.Context) {
		m, err := svc.Media.GetMedia(c.Request.Context(), c.Param("id"))
		if err != nil || m == nil || !mediaVisibleForRequest(c, svc, m) {
			c.JSON(http.StatusNotFound, gin.H{"error": "not found"})
			return
		}
		if !enforceScopedPlaybackToken(c, m.ID) {
			return
		}
		err = svc.Stream.ServeHLSSegment(c.Writer, c.Request, c.Param("id"), c.Param("seg"))
		if err != nil {
			c.JSON(http.StatusNotFound, gin.H{"error": err.Error()})
			return
		}
	}
}

func stopTranscodeHandler(svc *service.Container) gin.HandlerFunc {
	return func(c *gin.Context) {
		svc.Transcoder.StopJob(c.Param("id"))
		c.Status(http.StatusNoContent)
	}
}

func imageProxyHandler(svc *service.Container) gin.HandlerFunc {
	return func(c *gin.Context) {
		raw := c.Query("url")
		if c.Query("refresh") != "" {
			_ = svc.ImageProxy.RemoveFailed(raw)
		} else if c.Query("retry") != "" {
			_ = svc.ImageProxy.RemoveFailed(raw)
		}
		// Serve handles upstream errors internally by returning a 1×1 PNG
		// placeholder, so the only error we can get back here is a malformed
		// URL. In that case we still return 400 to make the misuse visible.
		if err := svc.ImageProxy.Serve(c.Request.Context(), c.Writer, c.Request, raw); err != nil {
			c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
			return
		}
	}
}

func cloudArtworkProxyHandler(svc *service.Container) gin.HandlerFunc {
	return func(c *gin.Context) {
		typ := c.Param("type")
		ref := c.Query("ref")
		if !service.IsAdminCloudConfigurable(typ) {
			c.JSON(http.StatusBadRequest, gin.H{"error": "unsupported cloud provider"})
			return
		}
		if ref == "" || !isCloudImageRef(ref) {
			c.JSON(http.StatusBadRequest, gin.H{"error": "image ref required"})
			return
		}
		if svc == nil || svc.ImageProxy == nil {
			c.JSON(http.StatusServiceUnavailable, gin.H{"error": "image proxy unavailable"})
			return
		}
		stableKey := typ + ":" + ref
		if svc.ImageProxy.ServeCloudCached(c.Writer, c.Request, stableKey) {
			return
		}
		if svc.StorageCfg == nil {
			c.JSON(http.StatusServiceUnavailable, gin.H{"error": "cloud storage service unavailable"})
			return
		}
		link, err := svc.StorageCfg.CloudResolve(c.Request.Context(), typ, ref, c.Request.UserAgent())
		if err != nil {
			c.JSON(http.StatusBadGateway, gin.H{"error": err.Error()})
			return
		}
		if err := svc.ImageProxy.ServeCloudResolved(c.Request.Context(), c.Writer, c.Request, stableKey, link); err != nil {
			c.JSON(http.StatusBadGateway, gin.H{"error": err.Error()})
			return
		}
	}
}

type scrapeRequest struct {
	EpisodeArtwork *bool `json:"episode_artwork"`
	EpisodeImages  *bool `json:"episode_images"`
	RefreshMatched *bool `json:"refresh_matched"`
	IncludeMatched *bool `json:"include_matched"`
}

func (r scrapeRequest) episodeArtworkOption() *bool {
	if r.EpisodeImages != nil {
		return r.EpisodeImages
	}
	return r.EpisodeArtwork
}

func (r scrapeRequest) includeMatchedOption() bool {
	if r.IncludeMatched != nil {
		return *r.IncludeMatched
	}
	if r.RefreshMatched != nil {
		return *r.RefreshMatched
	}
	return false
}

func scrapeOptionsFromRequest(c *gin.Context, retryNoMatch bool) (service.ScrapeOptions, error) {
	options := service.ScrapeOptions{RetryNoMatch: retryNoMatch}
	if c.Request.Body == nil || c.Request.ContentLength == 0 {
		return options, nil
	}
	var req scrapeRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		if errors.Is(err, io.EOF) {
			return options, nil
		}
		return options, err
	}
	options.EpisodeArtwork = req.episodeArtworkOption()
	options.IncludeMatched = req.includeMatchedOption()
	return options, nil
}

// scrapeOneHandler enriches a single media via the configured scraper chain.
func scrapeOneHandler(svc *service.Container) gin.HandlerFunc {
	return func(c *gin.Context) {
		options, err := scrapeOptionsFromRequest(c, true)
		if err != nil {
			c.JSON(http.StatusBadRequest, gin.H{"error": "invalid scrape options"})
			return
		}
		options.IncludeMatched = true
		m, err := svc.Repo.Media.FindByID(c.Request.Context(), c.Param("id"))
		if err != nil || m == nil {
			c.JSON(http.StatusNotFound, gin.H{"error": "not found"})
			return
		}
		task := startScrapeHTTPTask(svc, "手动刮削媒体", m.Title, m.Path)
		if err := svc.Scraper.EnrichOneWithOptions(c.Request.Context(), m, options); err != nil {
			finishHTTPTask(task, err, "scrape", "手动刮削媒体失败", nil, nil)
			c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
			return
		}
		reclassified := reclassifyMediaAfterScrape(c.Request.Context(), svc, m.ID)
		refreshed, _ := svc.Repo.Media.FindByID(c.Request.Context(), m.ID)
		metrics := map[string]int64{"processed": 1}
		if refreshed != nil && refreshed.ScrapeStatus == "matched" {
			metrics["matched"] = 1
		}
		if reclassified > 0 {
			metrics["reclassified"] = int64(reclassified)
		}
		finishHTTPTask(task, nil, "completed", "手动刮削媒体结束", metrics, nil)
		c.JSON(http.StatusOK, refreshed)
	}
}

// scrapeLibraryHandler manually refreshes every scrapeable row in a library.
func scrapeLibraryHandler(svc *service.Container) gin.HandlerFunc {
	return func(c *gin.Context) {
		libID := c.Param("id")
		options, err := scrapeOptionsFromRequest(c, true)
		if err != nil {
			c.JSON(http.StatusBadRequest, gin.H{"error": "invalid scrape options"})
			return
		}
		options.IncludeMatched = true
		var task *service.TaskHandle
		if lib, err := svc.Repo.Library.FindByID(c.Request.Context(), libID); err == nil && lib != nil {
			task = startScrapeHTTPTask(svc, "手动刮削媒体库", lib.Name, lib.Path)
		} else {
			task = startScrapeHTTPTask(svc, "手动刮削媒体库", libID, "")
		}
		// Run in the background so HTTP returns instantly; the WS hub
		// pushes per-item progress on the "scrape" topic.
		go func(libID string, task *service.TaskHandle, options service.ScrapeOptions) {
			result, err := svc.Scraper.EnrichLibraryDetailedWithOptions(context.Background(), libID, options)
			reclassified := 0
			if result.Processed > 0 {
				reclassified = reclassifyLibraryAfterScrape(context.Background(), svc, libID)
			}
			metrics := map[string]int64{
				"matched":    int64(result.Matched),
				"processed":  int64(result.Processed),
				"candidates": int64(result.Candidates),
			}
			if reclassified > 0 {
				metrics["reclassified"] = int64(reclassified)
			}
			if result.Failed > 0 {
				metrics["errors"] = int64(result.Failed)
			}
			stage := "completed"
			message := "手动刮削媒体库结束"
			if err != nil {
				stage = "scrape"
				message = "手动刮削媒体库失败"
			}
			finishHTTPTask(task, err, stage, message, metrics, nil)
		}(libID, task, options)
		c.JSON(http.StatusAccepted, gin.H{"status": "scraping"})
	}
}

func startScrapeHTTPTask(svc *service.Container, name, title, path string) *service.TaskHandle {
	if svc == nil || svc.Tasks == nil {
		return nil
	}
	if title != "" {
		name += "：" + title
	}
	return svc.Tasks.Start(service.TaskKindScrape, name, service.TaskUpdate{
		Stage:      "scrape",
		SourcePath: path,
		Message:    "正在刮削元数据",
	})
}

// reprobeHandler re-runs ffprobe against a single media. Admin-only.
func reprobeHandler(svc *service.Container) gin.HandlerFunc {
	return func(c *gin.Context) {
		if err := svc.Stream.Probe(c.Request.Context(), c.Param("id"), svc.FFprobe); err != nil {
			if errors.Is(err, service.ErrMediaNotFound) {
				c.JSON(http.StatusNotFound, gin.H{"error": "not found"})
				return
			}
			// ffprobe unavailable or file inaccessible — still 200 with error info
			c.JSON(http.StatusOK, gin.H{"code": 1, "error": err.Error()})
			return
		}
		c.JSON(http.StatusOK, gin.H{"code": 0, "message": "ok"})
	}
}
