package handler

import (
	"context"
	"errors"
	"net/http"
	"strconv"
	"time"

	"github.com/ShukeBta/MediaStationGo/internal/middleware"
	"github.com/ShukeBta/MediaStationGo/internal/service"
	"github.com/gin-gonic/gin"
	"gorm.io/gorm"
)

func registerPeopleRoutes(authed *gin.RouterGroup, svc *service.Container) {
	authed.GET("/people", requirePermission(svc, "can_play_media"), listPeopleHandler(svc))
	authed.GET("/people/:id", requirePermission(svc, "can_play_media"), getPersonHandler(svc))
	authed.GET("/people/:id/works", requirePermission(svc, "can_play_media"), personWorksHandler(svc))
	authed.POST("/people/:id/refresh", middleware.AdminRequired(), refreshPersonHandler(svc, false))
	authed.POST("/people/:id/translate", middleware.AdminRequired(), refreshPersonHandler(svc, true))
	authed.GET("/people/translation-settings", middleware.AdminRequired(), peopleTranslationSettingsHandler(svc))
	authed.PUT("/people/translation-settings", middleware.AdminRequired(), peopleTranslationSettingsHandler(svc))
	authed.GET("/media/:id/people", requirePermission(svc, "can_play_media"), mediaPeopleHandler(svc, false))
	authed.POST("/media/:id/people/translate", middleware.AdminRequired(), mediaPeopleHandler(svc, true))
	authed.POST("/media/:id/people/refresh", middleware.AdminRequired(), refreshMediaPeopleHandler(svc))
}

func peopleRequestContext(c *gin.Context, svc *service.Container) context.Context {
	return svc.Emby.PeopleContext(c.Request.Context(), mediaVisibilityForRequest(c, svc))
}

func peopleError(c *gin.Context, err error) {
	status := http.StatusBadGateway
	if errors.Is(err, gorm.ErrRecordNotFound) {
		status = http.StatusNotFound
	}
	c.JSON(status, gin.H{"error": err.Error()})
}

func listPeopleHandler(svc *service.Container) gin.HandlerFunc {
	return func(c *gin.Context) {
		limit, _ := strconv.Atoi(c.DefaultQuery("limit", "24"))
		offset, _ := strconv.Atoi(c.DefaultQuery("offset", "0"))
		result, err := svc.Emby.Persons(peopleRequestContext(c, svc), service.ItemsParams{UserID: currentUserID(c), SearchTerm: c.Query("q"), ParentID: c.Query("library_id"), StartIndex: offset, Limit: limit})
		if err != nil {
			peopleError(c, err)
			return
		}
		c.JSON(http.StatusOK, result)
	}
}

func getPersonHandler(svc *service.Container) gin.HandlerFunc {
	return func(c *gin.Context) {
		result, err := svc.Emby.Item(peopleRequestContext(c, svc), c.Param("id"), currentUserID(c))
		if err != nil {
			peopleError(c, err)
			return
		}
		if result == nil || result["Type"] != "Person" {
			c.JSON(http.StatusNotFound, gin.H{"error": "人物不存在"})
			return
		}
		c.JSON(http.StatusOK, result)
	}
}

func personWorksHandler(svc *service.Container) gin.HandlerFunc {
	return func(c *gin.Context) {
		offset, _ := strconv.Atoi(c.DefaultQuery("offset", "0"))
		result, err := svc.Emby.PersonWorks(peopleRequestContext(c, svc), c.Param("id"), currentUserID(c), offset, 50)
		if err != nil {
			peopleError(c, err)
			return
		}
		c.JSON(http.StatusOK, result)
	}
}

func refreshPersonHandler(svc *service.Container, translate bool) gin.HandlerFunc {
	return func(c *gin.Context) {
		ctx, cancel := context.WithTimeout(c.Request.Context(), 2*time.Minute)
		defer cancel()
		var err error
		if translate {
			err = svc.TranslatePerson(ctx, c.Param("id"))
		} else {
			err = svc.RefreshPerson(ctx, c.Param("id"))
		}
		if err != nil {
			peopleError(c, err)
			return
		}
		c.JSON(http.StatusOK, gin.H{"ok": true})
	}
}

func mediaPeopleHandler(svc *service.Container, translate bool) gin.HandlerFunc {
	return func(c *gin.Context) {
		ctx := peopleRequestContext(c, svc)
		people, err := svc.Emby.MediaPeople(ctx, c.Param("id"), currentUserID(c))
		if err != nil {
			peopleError(c, err)
			return
		}
		if translate {
			ctx, cancel := context.WithTimeout(ctx, 2*time.Minute)
			defer cancel()
			count, err := svc.TranslateMediaPeople(ctx, c.Param("id"))
			if err != nil {
				peopleError(c, err)
				return
			}
			c.JSON(http.StatusOK, gin.H{"translated": count})
			return
		}
		c.JSON(http.StatusOK, gin.H{"items": people})
	}
}

func peopleTranslationSettingsHandler(svc *service.Container) gin.HandlerFunc {
	return func(c *gin.Context) {
		if c.Request.Method == http.MethodPut {
			var input struct {
				Enabled bool `json:"enabled"`
			}
			if err := c.ShouldBindJSON(&input); err != nil {
				c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
				return
			}
			if err := svc.Repo.Setting.Set(c.Request.Context(), service.PeopleTranslationSetting, strconv.FormatBool(input.Enabled)); err != nil {
				peopleError(c, err)
				return
			}
		}
		value, err := svc.Repo.Setting.Get(c.Request.Context(), service.PeopleTranslationSetting)
		if err != nil {
			peopleError(c, err)
			return
		}
		c.JSON(http.StatusOK, gin.H{"enabled": value == "true"})
	}
}

func refreshMediaPeopleHandler(svc *service.Container) gin.HandlerFunc {
	return func(c *gin.Context) {
		ctx, cancel := context.WithTimeout(peopleRequestContext(c, svc), 2*time.Minute)
		defer cancel()
		if _, err := svc.Emby.MediaPeople(ctx, c.Param("id"), currentUserID(c)); err != nil {
			peopleError(c, err)
			return
		}
		if err := svc.RefreshMediaPeople(ctx, c.Param("id")); err != nil {
			peopleError(c, err)
			return
		}
		c.JSON(http.StatusOK, gin.H{"ok": true})
	}
}
