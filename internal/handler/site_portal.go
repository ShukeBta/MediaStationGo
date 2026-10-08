package handler

import (
	"context"
	"errors"
	"net/http"
	"strconv"
	"strings"

	"github.com/gin-gonic/gin"

	"github.com/ShukeBta/MediaStationGo/internal/middleware"
	"github.com/ShukeBta/MediaStationGo/internal/model"
	"github.com/ShukeBta/MediaStationGo/internal/service"
)

func siteCategoriesHandler(svc *service.Container) gin.HandlerFunc {
	return func(c *gin.Context) {
		items, err := svc.Site.Categories(c.Request.Context(), c.Query("site_id"))
		if err != nil {
			c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
			return
		}
		c.JSON(http.StatusOK, gin.H{"items": items})
	}
}

func siteBrowseHandler(svc *service.Container) gin.HandlerFunc {
	return func(c *gin.Context) {
		page, _ := strconv.Atoi(c.DefaultQuery("page", "1"))
		includeAdult, _ := strconv.ParseBool(c.Query("include_adult"))
		out, err := svc.Site.Browse(c.Request.Context(), service.SiteBrowseParams{
			SiteID:       c.Query("site_id"),
			Keyword:      firstNonEmptyString(c.Query("keyword"), c.Query("q")),
			Category:     c.Query("category"),
			Page:         page,
			IncludeAdult: includeAdult,
		})
		if err != nil {
			c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
			return
		}
		c.JSON(http.StatusOK, out)
	}
}

func siteDetailHandler(svc *service.Container) gin.HandlerFunc {
	return func(c *gin.Context) {
		detail, err := svc.Site.Detail(c.Request.Context(), c.Query("site_id"), c.Query("id"))
		if err != nil {
			c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
			return
		}
		c.JSON(http.StatusOK, detail)
	}
}

type siteDownloadReq struct {
	SiteID         string   `json:"site_id"`
	ID             string   `json:"id"`
	Title          string   `json:"title"`
	DownloadURL    string   `json:"download_url"`
	TorrentURL     string   `json:"torrent_url"`
	PosterURL      string   `json:"poster_url"`
	BackdropURL    string   `json:"backdrop_url"`
	Overview       string   `json:"overview"`
	SavePath       string   `json:"save_path"`
	MediaType      string   `json:"media_type"`
	MediaCategory  string   `json:"media_category"`
	SourceCategory string   `json:"source_category"`
	SelectedFiles  []string `json:"selected_files"`
}

type siteDownloadConfirmReq struct {
	SiteID              string `json:"site_id"`
	ID                  string `json:"id"`
	Title               string `json:"title"`
	DownloadURL         string `json:"download_url"`
	TorrentURL          string `json:"torrent_url"`
	PosterURL           string `json:"poster_url"`
	BackdropURL         string `json:"backdrop_url"`
	Overview            string `json:"overview"`
	SavePath            string `json:"save_path"`
	MediaType           string `json:"media_type"`
	MediaCategory       string `json:"media_category"`
	SourceCategory      string `json:"source_category"`
	Hash                string `json:"hash"`
	SelectedFileIndexes []int  `json:"selected_file_indexes"`
}

func (req siteDownloadConfirmReq) downloadReq() siteDownloadReq {
	return siteDownloadReq{
		SiteID:         req.SiteID,
		ID:             req.ID,
		Title:          req.Title,
		DownloadURL:    req.DownloadURL,
		TorrentURL:     req.TorrentURL,
		PosterURL:      req.PosterURL,
		BackdropURL:    req.BackdropURL,
		Overview:       req.Overview,
		SavePath:       req.SavePath,
		MediaType:      req.MediaType,
		MediaCategory:  req.MediaCategory,
		SourceCategory: req.SourceCategory,
	}
}

func enrichSiteTorrentDetailMeta(ctx context.Context, svc *service.Container, siteID, torrentID string, meta service.DownloadTaskMeta) service.DownloadTaskMeta {
	if svc == nil || svc.Site == nil || strings.TrimSpace(siteID) == "" || strings.TrimSpace(torrentID) == "" {
		return meta
	}
	detail, err := svc.Site.Detail(ctx, siteID, torrentID)
	if err != nil || detail == nil {
		return meta
	}
	if strings.TrimSpace(meta.Title) == "" {
		meta.Title = detail.Title
	}
	if strings.TrimSpace(meta.PosterURL) == "" {
		meta.PosterURL = detail.PosterURL
	}
	if strings.TrimSpace(meta.BackdropURL) == "" {
		meta.BackdropURL = detail.BackdropURL
	}
	if strings.TrimSpace(meta.Overview) == "" {
		meta.Overview = detail.Description
	}
	if strings.TrimSpace(meta.IMDBID) == "" {
		meta.IMDBID = service.NormalizeIMDBID(detail.ImdbID)
	}
	if meta.TMDbID <= 0 {
		meta.TMDbID = service.NormalizeTMDbID(detail.TMDbID)
	}
	if strings.TrimSpace(meta.DoubanID) == "" {
		meta.DoubanID = service.NormalizeDoubanID(detail.DoubanID)
	}
	return meta
}

func siteDownloadMeta(ctx context.Context, svc *service.Container, req siteDownloadReq, realURL string) service.DownloadTaskMeta {
	meta := service.DownloadTaskMeta{
		IdentityTitle:  req.Title,
		PosterURL:      req.PosterURL,
		BackdropURL:    req.BackdropURL,
		Overview:       req.Overview,
		MediaType:      req.MediaType,
		MediaCategory:  req.MediaCategory,
		SourceCategory: req.SourceCategory,
		SelectedFiles:  req.SelectedFiles,
	}
	meta = enrichSiteTorrentDetailMeta(ctx, svc, req.SiteID, req.ID, meta)
	if strings.TrimSpace(meta.Title) == "" {
		meta.Title = req.Title
	}
	return enrichDownloadTaskMeta(ctx, svc, meta, firstNonEmptyString(req.Title, meta.Title, realURL), req.MediaType)
}

func siteDownloadConflictResponse(code, message, reason string, req siteDownloadReq, meta service.DownloadTaskMeta) gin.H {
	out := gin.H{
		"error":  message,
		"code":   code,
		"reason": reason,
	}
	if title := strings.TrimSpace(firstNonEmptyString(meta.Title, req.Title, meta.IdentityTitle)); title != "" {
		out["title"] = title
	}
	return out
}

func siteRealDownloadURL(ctx context.Context, svc *service.Container, req siteDownloadReq) (string, error) {
	raw := firstNonEmptyString(req.DownloadURL, req.TorrentURL)
	realURL, err := svc.Site.DownloadURL(ctx, req.SiteID, req.ID, raw)
	if err != nil || strings.TrimSpace(realURL) == "" {
		if err == nil {
			err = errors.New("download url unavailable")
		}
		return "", err
	}
	return realURL, nil
}

func siteDownloadHandler(svc *service.Container) gin.HandlerFunc {
	return func(c *gin.Context) {
		var req siteDownloadReq
		if err := c.ShouldBindJSON(&req); err != nil {
			c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
			return
		}
		realURL, err := siteRealDownloadURL(c.Request.Context(), svc, req)
		if err != nil {
			c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
			return
		}
		uid, _ := c.Get(middleware.CtxUserID)
		meta := siteDownloadMeta(c.Request.Context(), svc, req, realURL)
		task, err := svc.Downloads.AddDownloadWithMeta(c.Request.Context(), uid.(string), realURL, req.SavePath, meta)
		if err != nil {
			if errors.Is(err, service.ErrMediaAlreadyInLibrary) {
				c.JSON(http.StatusConflict, siteDownloadConflictResponse(
					"media_already_in_library",
					"media already exists in library",
					"媒体库疑似已有该资源，已阻止重复下载。",
					req,
					meta,
				))
				return
			}
			if errors.Is(err, service.ErrDownloadAlreadyExists) {
				c.JSON(http.StatusOK, task)
				return
			}
			c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
			return
		}
		svc.Audit.Record(c.Request.Context(), uid.(string), "site.download", redactDownloadURL(realURL), c.ClientIP(), "")
		c.JSON(http.StatusCreated, task)
	}
}

func siteDownloadPrepareHandler(svc *service.Container) gin.HandlerFunc {
	return func(c *gin.Context) {
		var req siteDownloadReq
		if err := c.ShouldBindJSON(&req); err != nil {
			c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
			return
		}
		realURL, err := siteRealDownloadURL(c.Request.Context(), svc, req)
		if err != nil {
			c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
			return
		}
		uid, _ := c.Get(middleware.CtxUserID)
		meta := siteDownloadMeta(c.Request.Context(), svc, req, realURL)
		prepared, err := svc.Downloads.PrepareDownloadWithMeta(c.Request.Context(), uid.(string), realURL, req.SavePath, meta)
		if err != nil {
			if errors.Is(err, service.ErrMediaAlreadyInLibrary) {
				c.JSON(http.StatusConflict, siteDownloadConflictResponse(
					"media_already_in_library",
					"media already exists in library",
					"媒体库疑似已有该资源，已阻止重复下载。",
					req,
					meta,
				))
				return
			}
			if errors.Is(err, service.ErrDownloadAlreadyExists) {
				c.JSON(http.StatusConflict, siteDownloadConflictResponse(
					"download_already_exists",
					"download already exists",
					"下载任务或 qB 种子已存在，已阻止重复提交；可到下载中心或 qB 检查同名任务。",
					req,
					meta,
				))
				return
			}
			c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
			return
		}
		svc.Audit.Record(c.Request.Context(), uid.(string), "site.download.prepare", redactDownloadURL(realURL), c.ClientIP(), "")
		c.JSON(http.StatusCreated, prepared)
	}
}

func siteDownloadConfirmHandler(svc *service.Container) gin.HandlerFunc {
	return func(c *gin.Context) {
		var req siteDownloadConfirmReq
		if err := c.ShouldBindJSON(&req); err != nil {
			c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
			return
		}
		downloadReq := req.downloadReq()
		uid, _ := c.Get(middleware.CtxUserID)
		meta := service.DownloadTaskMeta{
			IdentityTitle:  downloadReq.Title,
			Title:          downloadReq.Title,
			PosterURL:      downloadReq.PosterURL,
			BackdropURL:    downloadReq.BackdropURL,
			Overview:       downloadReq.Overview,
			MediaType:      downloadReq.MediaType,
			MediaCategory:  downloadReq.MediaCategory,
			SourceCategory: downloadReq.SourceCategory,
		}
		rawURL := firstNonEmptyString(downloadReq.DownloadURL, downloadReq.TorrentURL)
		task, err := svc.Downloads.ConfirmPreparedDownload(c.Request.Context(), uid.(string), req.Hash, rawURL, req.SavePath, meta, req.SelectedFileIndexes)
		if err != nil {
			c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
			return
		}
		svc.Audit.Record(c.Request.Context(), uid.(string), "site.download.confirm", req.Hash, c.ClientIP(), "")
		c.JSON(http.StatusCreated, task)
	}
}

func siteDownloadCancelHandler(svc *service.Container) gin.HandlerFunc {
	return func(c *gin.Context) {
		var req struct {
			Hash string `json:"hash"`
		}
		if err := c.ShouldBindJSON(&req); err != nil {
			c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
			return
		}
		if err := svc.Downloads.CancelPreparedDownload(c.Request.Context(), req.Hash); err != nil {
			c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
			return
		}
		uid, _ := c.Get(middleware.CtxUserID)
		svc.Audit.Record(c.Request.Context(), uid.(string), "site.download.cancel", req.Hash, c.ClientIP(), "")
		c.JSON(http.StatusOK, gin.H{"ok": true})
	}
}

type siteSubscribeReq struct {
	Resolution          string  `json:"resolution"`
	Quality             string  `json:"quality"`
	Effects             string  `json:"effects"`
	ReleaseGroups       string  `json:"release_groups"`
	ExcludeWords        string  `json:"exclude_words"`
	MinSeeders          int     `json:"min_seeders"`
	MaxSeeders          int     `json:"max_seeders"`
	MinSizeGB           float64 `json:"min_size_gb"`
	MaxSizeGB           float64 `json:"max_size_gb"`
	FreeOnly            bool    `json:"free_only"`
	WashEnabled         bool    `json:"wash_enabled"`
	WashPriority        string  `json:"wash_priority"`
	SeasonNumber        int     `json:"season_number"`
	TotalEpisodes       int     `json:"total_episodes"`
	PollIntervalMinutes int     `json:"poll_interval_minutes"`
	SiteID              string  `json:"site_id"`
	ID                  string  `json:"id"`
	Category            string  `json:"category"`
	IncludeAdult        bool    `json:"include_adult"`
	Name                string  `json:"name"`
	Keyword             string  `json:"keyword"`
	Filter              string  `json:"filter"`
	OriginalTitle       string  `json:"original_title"`
	Year                int     `json:"year"`
	MediaType           string  `json:"media_type"`
	MediaCategory       string  `json:"media_category"`
	PosterURL           string  `json:"poster_url"`
	BackdropURL         string  `json:"backdrop_url"`
	Overview            string  `json:"overview"`
	SavePath            string  `json:"save_path"`
	Enabled             *bool   `json:"enabled"`
}

func siteSubscribeExplanation(req siteSubscribeReq, searchKeyword string, queued int) []string {
	explanation := make([]string, 0, 4)
	if keyword := strings.TrimSpace(searchKeyword); keyword != "" {
		explanation = append(explanation, "关键词："+keyword)
	}
	if category := strings.TrimSpace(req.Category); category != "" {
		explanation = append(explanation, "分类："+category)
	}
	if req.IncludeAdult {
		explanation = append(explanation, "包含成人分类")
	}
	if queued > 0 {
		explanation = append(explanation, "本次立即执行已加入下载："+strconv.Itoa(queued))
	} else {
		explanation = append(explanation, "本次立即执行未加入下载，常见原因是站点无结果、媒体/下载已存在或过滤规则未命中。")
	}
	return explanation
}

func siteSubscribeHandler(svc *service.Container) gin.HandlerFunc {
	return func(c *gin.Context) {
		var req siteSubscribeReq
		createCtx, err := bindSubscriptionCreation(c, &req)
		if err != nil {
			c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
			return
		}
		keyword := strings.TrimSpace(firstNonEmptyString(req.Keyword, req.Filter, req.Name))
		if keyword == "" {
			c.JSON(http.StatusBadRequest, gin.H{"error": "keyword required"})
			return
		}
		enabled := true
		if req.Enabled != nil {
			enabled = *req.Enabled
		}
		name := strings.TrimSpace(req.Name)
		if name == "" {
			name = keyword
		}
		uid, _ := c.Get(middleware.CtxUserID)
		sub := &model.Subscription{
			Resolution: req.Resolution, Quality: req.Quality, Effects: req.Effects,
			ReleaseGroups: req.ReleaseGroups, ExcludeWords: req.ExcludeWords,
			MinSeeders: req.MinSeeders, MaxSeeders: req.MaxSeeders, MinSizeGB: req.MinSizeGB, MaxSizeGB: req.MaxSizeGB,
			FreeOnly: req.FreeOnly, WashEnabled: req.WashEnabled, WashPriority: req.WashPriority,
			UserID:              uid.(string),
			Name:                name,
			Filter:              firstNonEmptyString(req.Filter, keyword),
			MediaType:           req.MediaType,
			MediaCategory:       req.MediaCategory,
			PosterURL:           req.PosterURL,
			BackdropURL:         req.BackdropURL,
			Overview:            req.Overview,
			OriginalName:        strings.TrimSpace(req.OriginalTitle),
			Year:                req.Year,
			SavePath:            req.SavePath,
			SearchMode:          "keyword",
			Source:              "site_search",
			DeliveryMode:        "download",
			SeasonNumber:        req.SeasonNumber,
			TotalEpisodes:       req.TotalEpisodes,
			PollIntervalMinutes: req.PollIntervalMinutes,
			Enabled:             enabled,
		}
		if detailMeta := enrichSiteTorrentDetailMeta(c.Request.Context(), svc, req.SiteID, req.ID, service.DownloadTaskMeta{
			Title:       sub.Name,
			PosterURL:   sub.PosterURL,
			BackdropURL: sub.BackdropURL,
			Overview:    sub.Overview,
		}); detailMeta.Title != "" || detailMeta.PosterURL != "" || detailMeta.BackdropURL != "" || detailMeta.Overview != "" || detailMeta.IMDBID != "" || detailMeta.TMDbID > 0 || detailMeta.DoubanID != "" {
			sub.Name = firstNonEmptyString(detailMeta.Title, sub.Name)
			sub.PosterURL = firstNonEmptyString(detailMeta.PosterURL, sub.PosterURL)
			sub.BackdropURL = firstNonEmptyString(detailMeta.BackdropURL, sub.BackdropURL)
			sub.Overview = firstNonEmptyString(sub.Overview, detailMeta.Overview)
			sub.IMDBID = firstNonEmptyString(sub.IMDBID, detailMeta.IMDBID)
			if sub.TMDbID <= 0 {
				sub.TMDbID = detailMeta.TMDbID
			}
			sub.DoubanID = firstNonEmptyString(sub.DoubanID, detailMeta.DoubanID)
		}
		searchKeyword := service.BuildSiteSubscriptionKeyword(sub.Name, req.Keyword, req.Filter, keyword)
		if searchKeyword == "" {
			searchKeyword = keyword
		}
		sub.FeedURL = service.SiteSearchURL(searchKeyword, req.SiteID, req.Category, req.IncludeAdult)
		sub.Filter = firstNonEmptyString(searchKeyword, sub.Filter)
		if err := svc.Subscription.Create(createCtx, sub); err != nil {
			if writeSubscriptionConflict(c, err) {
				return
			}
			c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
			return
		}
		enriched := []model.Subscription{*sub}
		svc.Subscription.EnrichProgress(c.Request.Context(), enriched)
		*sub = enriched[0]
		queued := 0
		runError := ""
		if enabled {
			if n, err := svc.Subscription.RunNow(c.Request.Context(), sub.ID); err != nil {
				runError = err.Error()
			} else {
				queued = n
			}
		}
		explanation := siteSubscribeExplanation(req, searchKeyword, queued)
		if runError != "" {
			explanation[len(explanation)-1] = "订阅已保存，但本次搜索下载失败：" + runError
		} else if !enabled {
			explanation[len(explanation)-1] = "订阅已保存为暂停状态，启用后将定时搜索下载。"
		}
		c.JSON(http.StatusCreated, gin.H{
			"subscription":   sub,
			"queued":         queued,
			"search_keyword": searchKeyword,
			"category":       req.Category,
			"include_adult":  req.IncludeAdult,
			"explanation":    explanation,
			"run_error":      runError,
		})
	}
}
