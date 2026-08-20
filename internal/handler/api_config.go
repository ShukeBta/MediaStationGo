// Package handler — third-party API config (TMDb / Bangumi / TheTVDB / …).
//
// All routes live under /api/admin/api-configs/* so only administrators
// can list / update / delete provider keys.
package handler

import (
	"net/http"

	"github.com/gin-gonic/gin"

	"github.com/ShukeBta/MediaStationGo/internal/service"
)

func listAPIConfigsHandler(svc *service.Container) gin.HandlerFunc {
	return func(c *gin.Context) {
		items, err := svc.APIConfig.List(c.Request.Context())
		if err != nil {
			c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
			return
		}
		c.JSON(http.StatusOK, gin.H{"items": items})
	}
}

func getAPIConfigHandler(svc *service.Container) gin.HandlerFunc {
	return func(c *gin.Context) {
		view, err := svc.APIConfig.Get(c.Request.Context(), c.Param("provider"))
		if err != nil {
			c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
			return
		}
		if view == nil {
			c.JSON(http.StatusNotFound, gin.H{"error": "not found"})
			return
		}
		c.JSON(http.StatusOK, view)
	}
}

func updateAPIConfigHandler(svc *service.Container) gin.HandlerFunc {
	return func(c *gin.Context) {
		var patch service.APIConfigPatch
		if err := c.ShouldBindJSON(&patch); err != nil {
			c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
			return
		}
		view, err := svc.APIConfig.Update(c.Request.Context(), c.Param("provider"), patch)
		if err != nil {
			c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
			return
		}
		c.JSON(http.StatusOK, view)
	}
}

func deleteAPIConfigHandler(svc *service.Container) gin.HandlerFunc {
	return func(c *gin.Context) {
		if err := svc.APIConfig.Delete(c.Request.Context(), c.Param("provider")); err != nil {
			c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
			return
		}
		c.Status(http.StatusNoContent)
	}
}

func testAPIConfigHandler(svc *service.Container) gin.HandlerFunc {
	return func(c *gin.Context) {
		provider := c.Param("provider")
		result, err := svc.ApiConfig.TestConnection(c.Request.Context(), provider)
		if err != nil {
			c.JSON(http.StatusOK, gin.H{"result": result, "error": err.Error()})
			return
		}
		c.JSON(http.StatusOK, gin.H{"result": result})
	}
}
