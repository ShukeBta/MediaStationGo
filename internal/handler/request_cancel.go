package handler

import (
	"context"
	"errors"
	"net/http"

	"github.com/gin-gonic/gin"
)

const statusClientClosedRequest = 499

func requestContextCanceled(c *gin.Context, err error) bool {
	if errors.Is(err, context.Canceled) {
		return true
	}
	if c == nil || c.Request == nil {
		return false
	}
	return errors.Is(c.Request.Context().Err(), context.Canceled)
}

func writeInternalOrCanceled(c *gin.Context, err error) {
	if requestContextCanceled(c, err) {
		c.AbortWithStatus(statusClientClosedRequest)
		return
	}
	c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
}
