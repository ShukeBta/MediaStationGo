package handler

import (
	"context"
	"encoding/json"
	"net/http"

	"github.com/ShukeBta/MediaStationGo/internal/service"
	"github.com/gin-gonic/gin"
	"github.com/gin-gonic/gin/binding"
)

func bindSubscriptionCreation(c *gin.Context, req any) (context.Context, error) {
	if err := c.ShouldBindBodyWith(req, binding.JSON); err != nil {
		return nil, err
	}
	var fields map[string]json.RawMessage
	if err := c.ShouldBindBodyWith(&fields, binding.JSON); err != nil {
		return nil, err
	}
	return service.WithSubscriptionRuleOverrides(c.Request.Context(), fields), nil
}

func subscriptionDefaultRulesHandler(svc *service.Container, save bool) gin.HandlerFunc {
	return func(c *gin.Context) {
		if save {
			var rules service.SubscriptionRuleDefaults
			if err := c.ShouldBindJSON(&rules); err != nil {
				c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
				return
			}
			if err := svc.Subscription.SaveDefaultRules(c.Request.Context(), rules); err != nil {
				c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
				return
			}
		}
		rules, err := svc.Subscription.DefaultRules(c.Request.Context())
		if err != nil {
			c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
			return
		}
		c.JSON(http.StatusOK, rules)
	}
}
