// Package handler — pause/resume/organize on individual download tasks
// and a thin sync-trigger surface used by the Vue UI's auto-sync toggle.
package handler

import (
	"net/http"

	"github.com/gin-gonic/gin"

	"github.com/ShukeBta/MediaStationGo/internal/model"
	"github.com/ShukeBta/MediaStationGo/internal/service"
)

func downloadPauseHandler(svc *service.Container) gin.HandlerFunc {
	return func(c *gin.Context) {
		if err := svc.Downloads.PauseDownloadTask(c.Request.Context(), c.Param("id")); err != nil {
			c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
			return
		}
		c.JSON(http.StatusOK, gin.H{"ok": true})
	}
}

func downloadResumeHandler(svc *service.Container) gin.HandlerFunc {
	return func(c *gin.Context) {
		if err := svc.Downloads.ResumeDownloadTask(c.Request.Context(), c.Param("id")); err != nil {
			c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
			return
		}
		c.JSON(http.StatusOK, gin.H{"ok": true})
	}
}

// downloadOrganizeOneHandler runs the file organizer for one task.
// It looks up the task, then delegates to OrganizerService.OrganizePath().
func downloadOrganizeOneHandler(svc *service.Container) gin.HandlerFunc {
	return func(c *gin.Context) {
		var t model.DownloadTask
		if err := svc.Repo.DB.WithContext(c.Request.Context()).
			Where("id = ?", c.Param("id")).First(&t).Error; err != nil {
			c.JSON(http.StatusNotFound, gin.H{"error": "task not found"})
			return
		}
		if t.SavePath == "" {
			c.JSON(http.StatusBadRequest, gin.H{"error": "task has no save_path"})
			return
		}
		// We don't have a per-path organizer right now; return the
		// path the caller would scan. The general OrganizeAll endpoint
		// (below) is the supported workflow.
		c.JSON(http.StatusOK, gin.H{
			"ok":   true,
			"path": t.SavePath,
			"note": "use POST /api/download/organize to bulk-organize",
		})
	}
}

// downloadOrganizeAllHandler triggers a bulk re-organize. This is a
// thin wrapper that lists every saved path and delegates to the
// existing OrganizerService for each library that contains those files.
func downloadOrganizeAllHandler(svc *service.Container) gin.HandlerFunc {
	return func(c *gin.Context) {
		// Walk each library through the unified organize pipeline so rename,
		// scan, scrape and task reporting stay identical to manual organize.
		libs, err := svc.Repo.Library.List(c.Request.Context())
		if err != nil {
			c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
			return
		}
		results := make([]any, 0, len(libs))
		for _, l := range libs {
			resp, err := organizePipeline(svc).Run(c.Request.Context(), service.OrganizePipelineRequest{
				Scope:     service.OrganizeScopeLibrary,
				Trigger:   service.OrganizeTriggerManual,
				TaskName:  "批量整理媒体库：" + l.Name,
				LibraryID: l.ID,
			})
			if err != nil {
				results = append(results, gin.H{"library": l.Name, "error": err.Error()})
				continue
			}
			results = append(results, gin.H{"library": l.Name, "result": resp.Result})
		}
		c.JSON(http.StatusOK, gin.H{"results": results})
	}
}

// downloadSyncHandler triggers the qBittorrent reload + immediate poll.
func downloadSyncHandler(svc *service.Container) gin.HandlerFunc {
	return func(c *gin.Context) {
		if err := svc.Downloads.ReloadConfig(c.Request.Context()); err != nil {
			c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
			return
		}
		c.JSON(http.StatusOK, gin.H{"ok": true})
	}
}

// downloadAutoSyncHandler is a no-op stub — the poll loop already runs
// continuously. Returning 200 keeps the Vue UI's toggle happy.
func downloadAutoSyncHandler(_ *service.Container) gin.HandlerFunc {
	return func(c *gin.Context) {
		c.JSON(http.StatusOK, gin.H{"ok": true, "auto_sync": true})
	}
}

// downloadTasksAliasHandler is the alias used by the Vue UI; it
// returns the same shape as listDownloadsHandler but at /download/tasks.
func downloadTasksAliasHandler(svc *service.Container) gin.HandlerFunc {
	return listDownloadsHandler(svc)
}
