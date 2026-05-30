// Package handler — media file organizer endpoints.
package handler

import (
	"net/http"
	"strings"

	"github.com/gin-gonic/gin"

	"github.com/ShukeBta/MediaStationGo/internal/service"
)

// organizeReq carries optional per-request overrides. 留空则沿用系统设置。
type organizeReq struct {
	TargetPath   string `json:"target_path"`
	TransferMode string `json:"transfer_mode"`
}

// bindOrganizeOptions parses the optional JSON body into OrganizeOptions.
// A missing/empty body is fine — it means "use the configured defaults".
func bindOrganizeOptions(c *gin.Context) service.OrganizeOptions {
	var req organizeReq
	_ = c.ShouldBindJSON(&req)
	opts := service.OrganizeOptions{TargetPath: strings.TrimSpace(req.TargetPath)}
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
