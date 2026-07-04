package handler

import (
	"net/http"

	"github.com/gin-gonic/gin"

	"github.com/ShukeBta/MediaStationGo/internal/service"
	"github.com/ShukeBta/MediaStationGo/internal/service/cloud"
)

// cloud115QRStartHandler begins a 115 QR-code login and returns the session +
// QR image URL for the frontend to render.
func cloud115QRStartHandler(svc *service.Container) gin.HandlerFunc {
	return func(c *gin.Context) {
		if c.Param("type") != cloud.Type115 {
			c.JSON(http.StatusBadRequest, gin.H{"error": "qr login is only supported for 115"})
			return
		}
		sess, err := cloud.QRStart(c.Request.Context(), nil)
		if err != nil {
			c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
			return
		}
		c.JSON(http.StatusOK, sess)
	}
}

// cloud115QRPollHandler polls a 115 QR session; on confirmation it returns the
// session cookie so the frontend can save it as the storage credential.
func cloud115QRPollHandler(svc *service.Container) gin.HandlerFunc {
	return func(c *gin.Context) {
		if c.Param("type") != cloud.Type115 {
			c.JSON(http.StatusBadRequest, gin.H{"error": "qr login is only supported for 115"})
			return
		}
		var sess cloud.QRSession
		if err := c.ShouldBindJSON(&sess); err != nil {
			c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
			return
		}
		st, err := cloud.QRPoll(c.Request.Context(), nil, &sess)
		if err != nil {
			c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
			return
		}
		c.JSON(http.StatusOK, st)
	}
}
