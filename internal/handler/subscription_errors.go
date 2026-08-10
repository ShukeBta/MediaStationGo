package handler

import (
	"errors"
	"net/http"

	"github.com/gin-gonic/gin"

	"github.com/ShukeBta/MediaStationGo/internal/service"
)

func writeSubscriptionConflict(c *gin.Context, err error) bool {
	if !errors.Is(err, service.ErrSubscriptionAlreadyExists) {
		return false
	}
	body := gin.H{"error": "subscription already exists"}
	if existingID := service.SubscriptionAlreadyExistsID(err); existingID != "" {
		body["existing_id"] = existingID
	}
	c.JSON(http.StatusConflict, body)
	return true
}
