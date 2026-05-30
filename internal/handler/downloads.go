// Package handler — download manager endpoints.
package handler

import (
	"context"
	"net/http"
	"net/url"
	"strings"

	"github.com/gin-gonic/gin"
	"go.uber.org/zap"

	"github.com/ShukeBta/MediaStationGo/internal/middleware"
	"github.com/ShukeBta/MediaStationGo/internal/model"
	"github.com/ShukeBta/MediaStationGo/internal/service"
)

type addDownloadReq struct {
	URL         string `json:"url" binding:"required"`
	SavePath    string `json:"save_path"`
	Title       string `json:"title"`
	PosterURL   string `json:"poster_url"`
	BackdropURL string `json:"backdrop_url"`
	Overview    string `json:"overview"`
}

// resolvePTDownloadURL 把站点搜索结果里的"详情/获取签名"URL 解析成 qb 能直接
// 拉到 .torrent 文件的真实下载 URL。
//
// 链路：
//
//  1. 拿 URL 的 host，到 sites 表里找 base_url 同源的站点。
//  2. 如果站点的 type 是已知 PT 框架（mteam/nexusphp/unit3d/...），
//     就用对应适配器的 GetDownloadURL，传入从 URL 里 parse 出来的 id。
//  3. 任一步失败都直接返回原 URL，让 qb 自己去拉（保持向后兼容）。
//
// 这一步存在的意义：M-Team 等站点的搜索结果里 download_url 是
// /api/torrent/genDlToken?id=xxx，需要带 x-api-key 才能调用，qb 自己
// 是没法识别这种 PT 专属端点的。
func resolvePTDownloadURL(ctx context.Context, svc *service.Container, raw string, log *zap.Logger) string {
	if raw == "" || svc == nil || svc.Site == nil {
		return raw
	}
	resolved := svc.Site.ResolveDownloadURL(ctx, raw)
	if resolved == raw {
		return raw
	}
	log.Info("resolved PT download URL",
		zap.String("from", redactDownloadURL(raw)),
		zap.String("to", redactDownloadURL(resolved)))
	return resolved
}

func addDownloadHandler(svc *service.Container) gin.HandlerFunc {
	return func(c *gin.Context) {
		var req addDownloadReq
		if err := c.ShouldBindJSON(&req); err != nil {
			c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
			return
		}
		uid, _ := c.Get(middleware.CtxUserID)
		// 把站点搜索 URL 转换成真实可下载 URL（M-Team 走 genDlToken 等）。
		realURL := resolvePTDownloadURL(c.Request.Context(), svc, req.URL, svc.Log)
		fallbackTitle := req.Title
		if strings.TrimSpace(fallbackTitle) == "" {
			fallbackTitle = realURL
		}
		meta := enrichDownloadTaskMeta(c.Request.Context(), svc, service.DownloadTaskMeta{
			Title:       req.Title,
			PosterURL:   req.PosterURL,
			BackdropURL: req.BackdropURL,
			Overview:    req.Overview,
		}, fallbackTitle, "")
		t, err := svc.Downloads.AddDownloadWithMeta(c.Request.Context(), uid.(string), realURL, req.SavePath, meta)
		if err != nil {
			c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
			return
		}
		svc.Audit.Record(c.Request.Context(), uid.(string), "download.add", redactDownloadURL(realURL), c.ClientIP(), "")
		c.JSON(http.StatusCreated, t)
	}
}

func listDownloadsHandler(svc *service.Container) gin.HandlerFunc {
	return func(c *gin.Context) {
		rows, live, err := svc.Downloads.List(c.Request.Context())
		if err != nil {
			c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
			return
		}
		rows = visibleDownloadRows(c, rows)
		enrichAndPersistDownloadRows(c.Request.Context(), svc, rows)
		if !isAdminRequest(c) {
			live = visibleLiveTorrents(rows, live)
		}
		taskViews, torrentViews := service.DownloadViews(rows, live)
		enrichDownloadTorrentViews(c.Request.Context(), svc, torrentViews)
		c.JSON(http.StatusOK, gin.H{
			"tasks":    taskViews,
			"torrents": torrentViews,
		})
	}
}

func visibleDownloadRows(c *gin.Context, rows []model.DownloadTask) []model.DownloadTask {
	if isAdminRequest(c) {
		return rows
	}
	uid, _ := c.Get(middleware.CtxUserID)
	userID, _ := uid.(string)
	filtered := make([]model.DownloadTask, 0, len(rows))
	for _, row := range rows {
		if row.UserID == userID {
			filtered = append(filtered, row)
		}
	}
	return filtered
}

func visibleLiveTorrents(rows []model.DownloadTask, live []service.QBitTorrent) []service.QBitTorrent {
	if len(rows) == 0 || len(live) == 0 {
		return nil
	}
	filtered := make([]service.QBitTorrent, 0, len(live))
	for _, torrent := range live {
		torrentTitle := normalizeTitle(torrent.Name)
		if torrentTitle == "" {
			continue
		}
		for _, row := range rows {
			rowTitle := normalizeTitle(row.Title)
			if rowTitle == "" {
				continue
			}
			if strings.Contains(torrentTitle, rowTitle) || strings.Contains(rowTitle, torrentTitle) {
				filtered = append(filtered, torrent)
				break
			}
		}
	}
	return filtered
}

func isAdminRequest(c *gin.Context) bool {
	role, _ := c.Get(middleware.CtxUserRole)
	return role == "admin"
}

func normalizeTitle(title string) string {
	title = strings.ToLower(title)
	var b strings.Builder
	for _, r := range title {
		if (r >= 'a' && r <= 'z') || (r >= '0' && r <= '9') || r > 127 {
			b.WriteRune(r)
		}
	}
	return b.String()
}

func redactDownloadURL(raw string) string {
	raw = strings.TrimSpace(raw)
	if raw == "" {
		return ""
	}
	if strings.HasPrefix(strings.ToLower(raw), "magnet:") {
		return "magnet:?xt=***"
	}
	u, err := url.Parse(raw)
	if err != nil || u.Host == "" {
		return "[redacted-download-url]"
	}
	u.RawQuery = ""
	u.Fragment = ""
	base := u.String()
	if base == "" {
		return u.Scheme + "://" + u.Host
	}
	return base
}

func deleteDownloadHandler(svc *service.Container) gin.HandlerFunc {
	return func(c *gin.Context) {
		hash := c.Param("hash")
		withFiles := c.Query("delete_files") == "true"
		if err := svc.Downloads.Delete(c.Request.Context(), hash, withFiles); err != nil {
			c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
			return
		}
		c.Status(http.StatusNoContent)
	}
}

func reloadDownloadConfigHandler(svc *service.Container) gin.HandlerFunc {
	return func(c *gin.Context) {
		if err := svc.Downloads.ReloadConfig(c.Request.Context()); err != nil {
			c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
			return
		}
		c.Status(http.StatusNoContent)
	}
}
