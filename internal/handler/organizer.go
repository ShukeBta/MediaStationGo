// Package handler — media file organizer endpoints.
package handler

import (
	"net/http"
	"strings"

	"github.com/gin-gonic/gin"

	"github.com/ShukeBta/MediaStationGo/internal/service"
)

// organizeReq carries optional per-request overrides. 留空则沿用系统设置。
//
// source_path = 源目录（待整理），dest_path = 目的地目录（整理输出）。
// target_path 为 dest_path 的向后兼容别名。
type organizeReq struct {
	SourcePath   string `json:"source_path"`
	DestPath     string `json:"dest_path"`
	TargetPath   string `json:"target_path"` // deprecated alias for dest_path
	TransferMode string `json:"transfer_mode"`
}

// bindOrganizeOptions parses the optional JSON body into OrganizeOptions.
// A missing/empty body is fine — it means "use the configured defaults".
func bindOrganizeOptions(c *gin.Context) service.OrganizeOptions {
	var req organizeReq
	_ = c.ShouldBindJSON(&req)
	dest := strings.TrimSpace(req.DestPath)
	if dest == "" {
		dest = strings.TrimSpace(req.TargetPath)
	}
	opts := service.OrganizeOptions{
		SourcePath: strings.TrimSpace(req.SourcePath),
		DestPath:   dest,
	}
	if m := strings.TrimSpace(req.TransferMode); m != "" {
		opts.TransferMode = service.TransferMode(m)
	}
	return opts
}

func organizeMediaHandler(svc *service.Container) gin.HandlerFunc {
	return func(c *gin.Context) {
		opts := bindOrganizeOptions(c)
		dst, err := svc.Organizer.OrganizeMediaWithOptions(c.Request.Context(), c.Param("id"), opts)
		if err != nil {
			c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
			return
		}
		c.JSON(http.StatusOK, gin.H{"path": dst})
	}
}

func organizeLibraryHandler(svc *service.Container) gin.HandlerFunc {
	return func(c *gin.Context) {
		opts := bindOrganizeOptions(c)
		res, err := svc.Organizer.OrganizeLibraryWithOptions(c.Request.Context(), c.Param("id"), opts)
		if err != nil {
			c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
			return
		}
		c.JSON(http.StatusOK, res)
	}
}

// organizeSourcesHandler lists selectable organize source directories (download
// dir + media dir) so the UI can offer them alongside registered libraries.
func organizeSourcesHandler(svc *service.Container) gin.HandlerFunc {
	return func(c *gin.Context) {
		c.JSON(http.StatusOK, gin.H{"sources": svc.Organizer.OrganizeSourceCandidates()})
	}
}

// organizeDirectoryHandler organizes an arbitrary source directory (e.g. the
// download directory) into the destination with dedup + 洗版.
func organizeDirectoryHandler(svc *service.Container) gin.HandlerFunc {
	return func(c *gin.Context) {
		opts := bindOrganizeOptions(c)
		res, err := svc.Organizer.OrganizeDirectory(c.Request.Context(), opts)
		if err != nil {
			c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
			return
		}
		c.JSON(http.StatusOK, res)
	}
}
