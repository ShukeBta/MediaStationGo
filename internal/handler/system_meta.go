// Package handler — system metadata endpoints used by the React shell
// (footer "powered by", admin status panel, scheduled-task page).
//
// These mirror the Vue surface (/api/system/info, /api/system/status,
// /api/system/scheduler) so the React port can reuse the same calls.
package handler

import (
	"net/http"
	"runtime"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/shirou/gopsutil/v3/cpu"
	"github.com/shirou/gopsutil/v3/disk"
	"github.com/shirou/gopsutil/v3/mem"

	"github.com/ShukeBta/MediaStationGo/internal/service"
)

// startedAt is captured at first call so /system/status can report uptime
// without threading state through the container.
var startedAt = time.Now()

func systemInfoHandler(svc *service.Container) gin.HandlerFunc {
	return func(c *gin.Context) {
		c.JSON(http.StatusOK, gin.H{
			"name":      "MediaStationGo",
			"version":   "0.1.0",
			"go":        runtime.Version(),
			"os":        runtime.GOOS,
			"arch":      runtime.GOARCH,
			"data_dir":  svc.Cfg.App.DataDir,
			"cache_dir": svc.Cfg.Cache.CacheDir,
		})
	}
}

func systemStatusHandler(svc *service.Container) gin.HandlerFunc {
	return func(c *gin.Context) {
		out := gin.H{
			"uptime_seconds": int64(time.Since(startedAt).Seconds()),
			"goroutines":     runtime.NumGoroutine(),
		}
		if usage, err := cpu.Percent(0, false); err == nil && len(usage) > 0 {
			out["cpu_percent"] = usage[0]
		}
		if v, err := mem.VirtualMemory(); err == nil {
			out["memory_used"] = v.Used
			out["memory_total"] = v.Total
		}
		if d, err := disk.Usage(svc.Cfg.App.DataDir); err == nil {
			out["disk_used"] = d.Used
			out["disk_total"] = d.Total
		}
		c.JSON(http.StatusOK, out)
	}
}

// systemSchedulerHandler is the read-only (non-admin) variant of
// /admin/scheduler — handy on user-facing dashboards.
func systemSchedulerHandler(svc *service.Container) gin.HandlerFunc {
	return func(c *gin.Context) {
		c.JSON(http.StatusOK, gin.H{"jobs": svc.Scheduler.Status()})
	}
}
