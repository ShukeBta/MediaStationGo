// Package handler — system config + scheduler trigger + events ticket.
package handler

import (
	"crypto/rand"
	"encoding/hex"
	"net/http"
	"sync"
	"time"

	"github.com/gin-gonic/gin"

	"github.com/ShukeBta/MediaStationGo/internal/middleware"
	"github.com/ShukeBta/MediaStationGo/internal/model"
	"github.com/ShukeBta/MediaStationGo/internal/service"
)

// listSystemConfigHandler is the non-admin alias for /admin/settings.
// It returns the same key/value rows so the Vue UI's `system.getConfig`
// helper keeps working.
func listSystemConfigHandler(svc *service.Container) gin.HandlerFunc {
	return func(c *gin.Context) {
		rows, err := svc.Repo.Setting.All(c.Request.Context())
		if err != nil {
			c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
			return
		}
		// Hide secret-flavoured keys for non-admins.
		role, _ := c.Get(middleware.CtxUserRole)
		out := make([]model.Setting, 0, len(rows))
		for _, s := range rows {
			if role != "admin" && isSecretKey(s.Key) {
				s.Value = "********"
			}
			out = append(out, s)
		}
		c.JSON(http.StatusOK, gin.H{"items": out})
	}
}

func isSecretKey(k string) bool {
	for _, suffix := range []string{".token", ".secret", ".password", ".api_key", ".cookie"} {
		if endsWith(k, suffix) {
			return true
		}
	}
	return false
}

func endsWith(s, suffix string) bool {
	return len(s) >= len(suffix) && s[len(s)-len(suffix):] == suffix
}

// schemaHandler returns the curated settings schema (used by the
// `getSchema()` Vue helper). It mirrors the SettingsPage groupings but
// in JSON so the upstream UI can render its dynamic form.
func schemaHandler(_ *service.Container) gin.HandlerFunc {
	return func(c *gin.Context) {
		c.JSON(http.StatusOK, gin.H{
			"groups": []gin.H{
				{
					"key":   "general",
					"label": "常规",
					"items": []gin.H{
						{"key": "tmdb.language", "type": "select", "label": "TMDb 元数据语言"},
						{"key": "app.server_url", "type": "text", "label": "公开访问域名 / STRM 域名"},
						{"key": "transcode.enabled", "type": "toggle", "label": "启用转码"},
						{"key": "transcode.hw_accel", "type": "select", "label": "硬件编码器"},
						{"key": "transcode.hw_enabled", "type": "toggle", "label": "启用硬件加速"},
						{"key": "transcode.max_jobs", "type": "number", "label": "最大并发"},
						{"key": "transcode.realtime", "type": "toggle", "label": "按播放速度转码"},
						{"key": "transcode.threads", "type": "number", "label": "软件转码线程数"},
						{"key": "transcode.idle_timeout_seconds", "type": "number", "label": "转码空闲停止秒数"},
						{"key": "ffmpeg.path", "type": "text", "label": "FFmpeg 路径"},
						{"key": "ffprobe.path", "type": "text", "label": "FFprobe 路径"},
						{"key": "ffprobe.max_concurrent", "type": "number", "label": "FFprobe 最大并发"},
					},
				},
				{
					"key":   "cloud-upload",
					"label": "网盘转存",
					"items": []gin.H{
						{"key": "cloud.auto_sync_enabled", "type": "toggle", "label": "夜间自动同步网盘媒体库"},
						{"key": "cloud.sync_interval_seconds", "type": "number", "label": "夜间窗口检查间隔秒数"},
						{"key": "cloud.boot_scan_enabled", "type": "toggle", "label": "启动后立即扫描网盘"},
						{"key": "cloud.upload_auto_enabled", "type": "toggle", "label": "启用自动转存"},
						{"key": "cloud.upload_provider", "type": "select", "label": "转存目标", "options": []gin.H{
							{"value": "openlist", "label": "OpenList（推荐，可桥接 115/123/阿里等）"},
							{"value": "clouddrive2", "label": "CloudDrive2（推荐，可桥接 115/123/阿里等）"},
							{"value": "alist", "label": "Alist（可桥接多网盘）"},
							{"value": "webdav", "label": "WebDAV"},
							{"value": "cloud115", "label": "115 原生（待接分片上传）"},
						}},
						{"key": "cloud.upload_source_dir", "type": "text", "label": "本地源目录"},
						{"key": "cloud.upload_dest_path", "type": "text", "label": "网盘目标目录"},
						{"key": "cloud.upload_recursive", "type": "toggle", "label": "递归扫描源目录"},
						{"key": "cloud.upload_sidecars", "type": "toggle", "label": "同步 NFO / 海报 / 字幕"},
						{"key": "cloud.upload_overwrite", "type": "toggle", "label": "覆盖远端同名文件"},
						{"key": "cloud.upload_transfer_mode", "type": "select", "label": "自动转存方式", "options": []gin.H{
							{"value": "copy", "label": "复制"},
							{"value": "move", "label": "移动"},
						}},
						{"key": "cloud.upload_interval_seconds", "type": "number", "label": "自动转存间隔秒数"},
					},
				},
				{
					"key":   "adult",
					"label": "Adult / NSFW",
					"items": []gin.H{
						{"key": "adult.enabled", "type": "toggle"},
						{"key": "adult.require_pin", "type": "toggle"},
						{"key": "adult.pin", "type": "text"},
					},
				},
				{
					"key":   "qbittorrent",
					"label": "qBittorrent",
					"items": []gin.H{
						{"key": "qbittorrent.url", "type": "text"},
						{"key": "qbittorrent.username", "type": "text"},
						{"key": "qbittorrent.password", "type": "text"},
						{"key": "qbittorrent.savepath", "type": "text"},
					},
				},
				{
					"key":   "license",
					"label": "授权服务",
					"items": []gin.H{
						{"key": "license.server_url", "type": "text", "label": "License Server 地址"},
						{"key": "license.public_key", "type": "text", "label": "Ed25519 验签公钥"},
						{"key": "license.hmac_secret", "type": "text", "label": "HMAC 签名密钥（旧版兼容）"},
					},
				},
				{
					"key":   "system-update",
					"label": "系统更新",
					"items": []gin.H{
						{"key": "system.update.image", "type": "text", "label": "应用镜像"},
						{"key": "system.update.compose_dir", "type": "text", "label": "Docker Compose 安装目录"},
						{"key": "system.update.command", "type": "textarea", "label": "自定义更新命令"},
					},
				},
			},
		})
	}
}

// schedulerTriggerHandler is the alternate path for /admin/scheduler/:name/run.
func schedulerTriggerHandler(svc *service.Container) gin.HandlerFunc {
	return func(c *gin.Context) {
		if !triggerSchedulerJob(c, svc, c.Param("name")) {
			return
		}
		c.JSON(http.StatusAccepted, gin.H{"ok": true, "message": "任务已在后台触发"})
	}
}

// ─── SSE ticket store ───────────────────────────────────────────────────────
//
// The Vue UI's SSE event stream wants a one-time signed ticket so the
// EventSource (which can't set Authorization headers) can authenticate.
// We don't expose the SSE stream itself yet, but we persist short-lived
// tickets keyed to the user so the upstream consumer keeps working.

type ticket struct {
	userID  string
	expires time.Time
}

var (
	ticketStore   = map[string]ticket{}
	ticketStoreMu sync.Mutex
)

func newTicket(userID string) string {
	buf := make([]byte, 16)
	_, _ = rand.Read(buf)
	t := hex.EncodeToString(buf)
	ticketStoreMu.Lock()
	defer ticketStoreMu.Unlock()
	ticketStore[t] = ticket{userID: userID, expires: time.Now().Add(60 * time.Second)}
	// GC expired tickets opportunistically.
	for k, v := range ticketStore {
		if time.Now().After(v.expires) {
			delete(ticketStore, k)
		}
	}
	return t
}

func systemEventsTicketHandler(_ *service.Container) gin.HandlerFunc {
	return func(c *gin.Context) {
		uid, _ := c.Get(middleware.CtxUserID)
		c.JSON(http.StatusOK, gin.H{"ticket": newTicket(toString(uid))})
	}
}
