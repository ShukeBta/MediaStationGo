// Package handler — STRM (URL-as-file) admin endpoints.
//
// Setting a media row's strm_url makes the stream handler issue a 302
// redirect to that URL instead of opening a local file. This lets the
// operator expose WebDAV / Alist / S3 / HTTP direct links as ordinary
// MediaStationGo entries.
package handler

import (
	"context"
	"net/http"
	"strings"

	"github.com/gin-gonic/gin"

	"github.com/ShukeBta/MediaStationGo/internal/middleware"
	"github.com/ShukeBta/MediaStationGo/internal/model"
	"github.com/ShukeBta/MediaStationGo/internal/service"
)

type strmReq struct {
	URL string `json:"url" binding:"required"`
}

func setSTRMHandler(svc *service.Container) gin.HandlerFunc {
	return func(c *gin.Context) {
		var req strmReq
		if err := c.ShouldBindJSON(&req); err != nil {
			c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
			return
		}
		url := strings.TrimSpace(req.URL)
		if !strings.HasPrefix(url, "http://") && !strings.HasPrefix(url, "https://") {
			c.JSON(http.StatusBadRequest, gin.H{"error": "url must start with http:// or https://"})
			return
		}
		mediaID := c.Param("id")
		m, err := svc.Repo.Media.FindByID(c.Request.Context(), mediaID)
		if err != nil || m == nil {
			c.JSON(http.StatusNotFound, gin.H{"error": "media not found"})
			return
		}
		if err := svc.Repo.DB.WithContext(c.Request.Context()).
			Model(&model.Media{}).
			Where("id = ?", mediaID).
			Update("strm_url", url).Error; err != nil {
			c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
			return
		}
		c.JSON(http.StatusOK, gin.H{"strm_url": url})
	}
}

func clearSTRMHandler(svc *service.Container) gin.HandlerFunc {
	return func(c *gin.Context) {
		if err := svc.Repo.DB.WithContext(c.Request.Context()).
			Model(&model.Media{}).
			Where("id = ?", c.Param("id")).
			Update("strm_url", "").Error; err != nil {
			c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
			return
		}
		c.Status(http.StatusNoContent)
	}
}

// importSTRMHandler creates a media row directly from a (library_id, title, url)
// tuple — useful for adding a streaming-only entry without an on-disk file.
type importSTRMReq struct {
	LibraryID string `json:"library_id" binding:"required"`
	Title     string `json:"title" binding:"required"`
	URL       string `json:"url" binding:"required"`
}

func importSTRMHandler(svc *service.Container) gin.HandlerFunc {
	return func(c *gin.Context) {
		var req importSTRMReq
		if err := c.ShouldBindJSON(&req); err != nil {
			c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
			return
		}
		url := strings.TrimSpace(req.URL)
		if !strings.HasPrefix(url, "http://") && !strings.HasPrefix(url, "https://") {
			c.JSON(http.StatusBadRequest, gin.H{"error": "url must start with http:// or https://"})
			return
		}
		m := &model.Media{
			LibraryID: req.LibraryID,
			Title:     req.Title,
			Path:      url,
			STRMURL:   url,
			Container: "strm",
		}
		if err := svc.Repo.Media.Upsert(c.Request.Context(), m); err != nil {
			c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
			return
		}
		c.JSON(http.StatusCreated, m)
	}
}

type generateSTRMReq struct {
	LibraryID    string `json:"library_id"`
	OutputDir    string `json:"output_dir"`
	BaseURL      string `json:"base_url"`
	Enabled      bool   `json:"enabled"`
	Overwrite    bool   `json:"overwrite"`
	IncludeLocal *bool  `json:"include_local"`
	PreserveTree bool   `json:"preserve_tree"`
	Refresh      bool   `json:"refresh_library"`
	ScrapeAfter  bool   `json:"scrape_after"`
}

type generateSTRMTreeReq struct {
	Provider          string   `json:"provider"`
	TreeText          string   `json:"tree_text"`
	Paths             []string `json:"paths"`
	SourceRoot        string   `json:"source_root"`
	OutputPrefix      string   `json:"output_prefix"`
	OutputDir         string   `json:"output_dir"`
	BaseURL           string   `json:"base_url"`
	Overwrite         bool     `json:"overwrite"`
	Cleanup           bool     `json:"cleanup"`
	DryRun            bool     `json:"dry_run"`
	BatchLimit        int      `json:"batch_limit"`
	RecognizeRename   bool     `json:"recognize_rename"`
	TransferSubtitles bool     `json:"transfer_subtitles"`
	MissingOnly       bool     `json:"missing_only"`
	RefreshLibrary    bool     `json:"refresh_library"`
	ScrapeAfter       bool     `json:"scrape_after"`
}

type repairSTRMReq struct {
	OutputDir      string `json:"output_dir" binding:"required"`
	BaseURL        string `json:"base_url"`
	DryRun         bool   `json:"dry_run"`
	RefreshLibrary bool   `json:"refresh_library"`
	ScrapeAfter    bool   `json:"scrape_after"`
}

func listSTRMOutputPresetsHandler(svc *service.Container) gin.HandlerFunc {
	return func(c *gin.Context) {
		items, err := service.STRMOutputPresets(c.Request.Context(), svc.Repo)
		if err != nil {
			c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
			return
		}
		c.JSON(http.StatusOK, gin.H{"items": items})
	}
}

func generateSTRMHandler(svc *service.Container) gin.HandlerFunc {
	return func(c *gin.Context) {
		var req generateSTRMReq
		if err := c.ShouldBindJSON(&req); err != nil {
			c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
			return
		}
		strmSvc := svc.STRM
		if strmSvc == nil {
			strmSvc = service.NewSTRMService(svc.Log, svc.Repo, svc.Cfg)
		}
		baseURL := strings.TrimRight(strings.TrimSpace(req.BaseURL), "/")
		if baseURL == "" {
			baseURL = strings.TrimRight(absoluteRequestURL(c, "/"), "/")
		}
		includeLocal := true
		if req.IncludeLocal != nil {
			includeLocal = *req.IncludeLocal
		}
		options := service.GenerateSTRMOptions{
			LibraryID:     req.LibraryID,
			OutputDir:     req.OutputDir,
			BaseURL:       baseURL,
			Enabled:       req.Enabled,
			Overwrite:     req.Overwrite,
			IncludeLocal:  includeLocal,
			PreserveTree:  req.PreserveTree,
			PlaybackToken: strmPlaybackTokenForRequest(c, svc),
		}
		var res *service.GenerateSTRMResult
		var err error
		if strings.TrimSpace(req.LibraryID) == "*" {
			res, err = strmSvc.GenerateForAllLibraries(c.Request.Context(), options)
		} else {
			res, err = strmSvc.GenerateForLibrary(c.Request.Context(), options)
		}
		if err != nil {
			c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
			return
		}
		if req.Refresh {
			res.Refresh = queueSTRMRefreshAfterChanges(c.Request.Context(), svc, res.OutputDir, strmRefreshQueueOptions{
				TaskName:    "STRM 生成后刷新媒体库",
				Changed:     strmGenerationChanged(res),
				ScrapeAfter: req.ScrapeAfter,
			})
		}
		c.JSON(http.StatusOK, res)
	}
}

func generateSTRMFromTreeHandler(svc *service.Container) gin.HandlerFunc {
	return func(c *gin.Context) {
		var req generateSTRMTreeReq
		if err := c.ShouldBindJSON(&req); err != nil {
			c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
			return
		}
		strmSvc := svc.STRM
		if strmSvc == nil {
			strmSvc = service.NewSTRMService(svc.Log, svc.Repo, svc.Cfg)
		}
		baseURL := strings.TrimRight(strings.TrimSpace(req.BaseURL), "/")
		if baseURL == "" {
			baseURL = strings.TrimRight(absoluteRequestURL(c, "/"), "/")
		}
		res, err := strmSvc.GenerateFromTree(c.Request.Context(), service.GenerateSTRMTreeOptions{
			Provider:          req.Provider,
			TreeText:          req.TreeText,
			Paths:             req.Paths,
			SourceRoot:        req.SourceRoot,
			OutputPrefix:      req.OutputPrefix,
			OutputDir:         req.OutputDir,
			BaseURL:           baseURL,
			Overwrite:         req.Overwrite,
			Cleanup:           req.Cleanup,
			DryRun:            req.DryRun,
			BatchLimit:        req.BatchLimit,
			RecognizeRename:   req.RecognizeRename,
			TransferSubtitles: req.TransferSubtitles,
			MissingOnly:       req.MissingOnly,
		})
		if err != nil {
			c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
			return
		}
		if req.RefreshLibrary && !req.DryRun {
			res.Refresh = queueSTRMRefreshAfterChanges(c.Request.Context(), svc, res.OutputDir, strmRefreshQueueOptions{
				TaskName:    "STRM 目录树生成后刷新媒体库",
				Changed:     strmGenerationChanged(res),
				ScrapeAfter: req.ScrapeAfter,
			})
		}
		c.JSON(http.StatusOK, res)
	}
}

func repairSTRMHandler(svc *service.Container) gin.HandlerFunc {
	return func(c *gin.Context) {
		var req repairSTRMReq
		if err := c.ShouldBindJSON(&req); err != nil {
			c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
			return
		}
		strmSvc := svc.STRM
		if strmSvc == nil {
			strmSvc = service.NewSTRMService(svc.Log, svc.Repo, svc.Cfg)
		}
		baseURL := strings.TrimRight(strings.TrimSpace(req.BaseURL), "/")
		if baseURL == "" {
			baseURL = strings.TrimRight(absoluteRequestURL(c, "/"), "/")
		}
		res, err := strmSvc.RepairFiles(c.Request.Context(), service.RepairSTRMOptions{
			OutputDir: req.OutputDir,
			BaseURL:   baseURL,
			DryRun:    req.DryRun,
		})
		if err != nil {
			c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
			return
		}
		if req.RefreshLibrary && !req.DryRun {
			res.Refresh = queueSTRMRefreshAfterChanges(c.Request.Context(), svc, res.OutputDir, strmRefreshQueueOptions{
				TaskName:    "STRM 修复后刷新媒体库",
				Changed:     res.Repaired > 0,
				ScrapeAfter: req.ScrapeAfter,
			})
		}
		c.JSON(http.StatusOK, res)
	}
}

func strmGenerationChanged(res *service.GenerateSTRMResult) bool {
	return res != nil && (res.Generated > 0 || res.Updated > 0 || res.Cleaned > 0)
}

type strmRefreshQueueOptions struct {
	TaskName    string
	Changed     bool
	ScrapeAfter bool
}

type strmRefreshRunOptions struct {
	ScrapeAfter bool
}

func queueSTRMRefreshAfterChanges(ctx context.Context, svc *service.Container, outputDir string, options strmRefreshQueueOptions) *service.STRMRefreshResult {
	refresh := &service.STRMRefreshResult{Requested: true, ScrapeRequested: options.ScrapeAfter}
	if !options.Changed {
		refresh.Reason = "no strm changes"
		refresh.ScrapeReason = strmRefreshScrapeSkipReason(refresh)
		return refresh
	}
	if svc == nil || svc.Scan == nil {
		refresh.Reason = "scanner unavailable"
		refresh.ScrapeReason = strmRefreshScrapeSkipReason(refresh)
		return refresh
	}
	targets, err := service.FindSTRMRefreshTargets(ctx, svc.Repo, outputDir)
	if err != nil {
		refresh.Reason = err.Error()
		refresh.ScrapeReason = strmRefreshScrapeSkipReason(refresh)
		return refresh
	}
	if len(targets) == 0 {
		refresh.Reason = "no matching local library"
		refresh.ScrapeReason = strmRefreshScrapeSkipReason(refresh)
		return refresh
	}
	refresh.Targets = targets
	if options.ScrapeAfter && svc.Scraper == nil {
		refresh.ScrapeReason = "scraper unavailable"
	}
	for _, target := range targets {
		key := target.LibraryID
		if target.RootID != "" {
			key += ":" + target.RootID
		}
		finishScan, ok := svc.Scan.TryBeginLocalScan(key)
		if !ok {
			continue
		}
		refresh.Queued = true
		runOptions := strmRefreshRunOptions{ScrapeAfter: options.ScrapeAfter && svc.Scraper != nil}
		if runOptions.ScrapeAfter {
			refresh.ScrapeQueued = true
		}
		task := startScanHTTPTask(svc, options.TaskName, target.Name, target.Path)
		go runSTRMRefreshScan(svc, target, task, finishScan, runOptions)
	}
	if !refresh.Queued {
		refresh.Reason = "matching library already scanning"
		refresh.ScrapeReason = strmRefreshScrapeSkipReason(refresh)
	}
	return refresh
}

func runSTRMRefreshScan(svc *service.Container, target service.STRMRefreshTarget, task *service.TaskHandle, finish func(), options strmRefreshRunOptions) {
	defer finish()
	var (
		res *service.ScanResult
		err error
	)
	if target.RootID != "" {
		res, err = svc.Scan.ScanLibraryRoot(context.Background(), target.LibraryID, target.RootID)
	} else {
		res, err = svc.Scan.ScanLibrary(context.Background(), target.LibraryID)
	}
	if err != nil {
		finishHTTPTask(task, err, "scan", "STRM 刷新媒体库失败", scanTaskMetrics(res), scanTaskDetails(res, 20))
		return
	}
	if !options.ScrapeAfter {
		finishHTTPTask(task, nil, "completed", "STRM 刷新媒体库结束", scanTaskMetrics(res), scanTaskDetails(res, 20))
		return
	}
	scrape, reclassified, scrapeErr := runSTRMRefreshScrape(svc, target, task)
	metrics := strmRefreshTaskMetrics(res, scrape, reclassified)
	if scrapeErr != nil {
		finishHTTPTask(task, scrapeErr, "scrape", "STRM 刷新媒体库完成，刮削失败", metrics, scanTaskDetails(res, 20))
		return
	}
	finishHTTPTask(task, nil, "completed", "STRM 刷新媒体库和刮削结束", metrics, scanTaskDetails(res, 20))
}

func runSTRMRefreshScrape(svc *service.Container, target service.STRMRefreshTarget, task *service.TaskHandle) (service.EnrichLibraryResult, int, error) {
	if task != nil {
		task.Update(service.TaskUpdate{
			Stage:      "scrape",
			SourcePath: target.Path,
			Message:    "正在刮削 STRM 媒体库",
		})
	}
	result, err := svc.Scraper.EnrichLibraryDetailedWithOptions(context.Background(), target.LibraryID, service.ScrapeOptions{RetryNoMatch: true})
	reclassified := 0
	if result.Processed > 0 {
		reclassified = reclassifyLibraryAfterScrape(context.Background(), svc, target.LibraryID)
	}
	return result, reclassified, err
}

func strmRefreshTaskMetrics(scan *service.ScanResult, scrape service.EnrichLibraryResult, reclassified int) map[string]int64 {
	metrics := scanTaskMetrics(scan)
	if metrics == nil {
		metrics = map[string]int64{}
	}
	metrics["scrape_matched"] = int64(scrape.Matched)
	metrics["scrape_processed"] = int64(scrape.Processed)
	metrics["scrape_candidates"] = int64(scrape.Candidates)
	if scrape.Failed > 0 {
		metrics["scrape_failed"] = int64(scrape.Failed)
	}
	if reclassified > 0 {
		metrics["scrape_reclassified"] = int64(reclassified)
	}
	return metrics
}

func strmRefreshScrapeSkipReason(refresh *service.STRMRefreshResult) string {
	if refresh == nil || !refresh.ScrapeRequested {
		return ""
	}
	if refresh.Reason != "" {
		return "refresh not queued: " + refresh.Reason
	}
	return "refresh not queued"
}

func strmPlaybackTokenForRequest(c *gin.Context, svc *service.Container) string {
	if svc == nil || svc.Auth == nil || svc.Repo == nil || svc.Repo.User == nil {
		return ""
	}
	uid := middleware.GetUserID(c)
	if uid == "" {
		return ""
	}
	u, err := svc.Repo.User.FindByID(c.Request.Context(), uid)
	if err != nil || u == nil {
		return ""
	}
	token, err := svc.Auth.IssueEmbyToken(u)
	if err != nil {
		return ""
	}
	return token
}
