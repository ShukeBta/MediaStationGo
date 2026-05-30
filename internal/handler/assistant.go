// Package handler — multi-turn AI assistant chat endpoints.
package handler

import (
	"net/http"

	"github.com/gin-gonic/gin"

	"github.com/ShukeBta/MediaStationGo/internal/middleware"
	"github.com/ShukeBta/MediaStationGo/internal/service"
)

func listAssistantSessionsHandler(svc *service.Container) gin.HandlerFunc {
	return func(c *gin.Context) {
		uid, _ := c.Get(middleware.CtxUserID)
		role, _ := c.Get(middleware.CtxUserRole)
		rows, err := svc.Assistant.ListSessions(
			c.Request.Context(), toString(uid), role == "admin",
		)
		if err != nil {
			c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
			return
		}
		c.JSON(http.StatusOK, rows)
	}
}

type createSessionReq struct {
	Title string `json:"title"`
}

func createAssistantSessionHandler(svc *service.Container) gin.HandlerFunc {
	return func(c *gin.Context) {
		var req createSessionReq
		_ = c.ShouldBindJSON(&req)
		uid, _ := c.Get(middleware.CtxUserID)
		sess, err := svc.Assistant.CreateSession(c.Request.Context(), toString(uid), req.Title)
		if err != nil {
			c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
			return
		}
		c.JSON(http.StatusCreated, sess)
	}
}

func getAssistantSessionHandler(svc *service.Container) gin.HandlerFunc {
	return func(c *gin.Context) {
		uid, _ := c.Get(middleware.CtxUserID)
		role, _ := c.Get(middleware.CtxUserRole)
		view, err := svc.Assistant.GetSession(
			c.Request.Context(), c.Param("id"), toString(uid), role == "admin",
		)
		if err != nil {
			c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
			return
		}
		c.JSON(http.StatusOK, view)
	}
}

func deleteAssistantSessionHandler(svc *service.Container) gin.HandlerFunc {
	return func(c *gin.Context) {
		uid, _ := c.Get(middleware.CtxUserID)
		role, _ := c.Get(middleware.CtxUserRole)
		if err := svc.Assistant.DeleteSession(
			c.Request.Context(), c.Param("id"), toString(uid), role == "admin",
		); err != nil {
			c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
			return
		}
		c.Status(http.StatusNoContent)
	}
}

type chatReq struct {
	SessionID string `json:"session_id" binding:"required"`
	Message   string `json:"message" binding:"required"`
}

func assistantChatHandler(svc *service.Container) gin.HandlerFunc {
	return func(c *gin.Context) {
		var req chatReq
		if err := c.ShouldBindJSON(&req); err != nil {
			c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
			return
		}
		uid, _ := c.Get(middleware.CtxUserID)
		role, _ := c.Get(middleware.CtxUserRole)
		view, err := svc.Assistant.Chat(
			c.Request.Context(), req.SessionID, toString(uid), req.Message, role == "admin",
		)
		if err != nil {
			c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
			return
		}
		c.JSON(http.StatusOK, view)
	}
}

type executeReq struct {
	SessionID string                 `json:"session_id" binding:"required"`
	Action    map[string]interface{} `json:"action" binding:"required"`
}

func assistantExecuteHandler(svc *service.Container) gin.HandlerFunc {
	return func(c *gin.Context) {
		var req executeReq
		if err := c.ShouldBindJSON(&req); err != nil {
			c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
			return
		}
		uid, _ := c.Get(middleware.CtxUserID)
		opID, err := svc.Assistant.Execute(
			c.Request.Context(), req.SessionID, toString(uid), req.Action,
		)
		if err != nil {
			c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
			return
		}
		c.JSON(http.StatusOK, gin.H{"op_id": opID})
	}
}

func assistantUndoHandler(svc *service.Container) gin.HandlerFunc {
	return func(c *gin.Context) {
		if err := svc.Assistant.Undo(c.Request.Context(), c.Param("op_id")); err != nil {
			c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
			return
		}
		c.JSON(http.StatusOK, gin.H{"ok": true})
	}
}

func assistantHistoryHandler(svc *service.Container) gin.HandlerFunc {
	return func(c *gin.Context) {
		uid, _ := c.Get(middleware.CtxUserID)
		role, _ := c.Get(middleware.CtxUserRole)
		rows, err := svc.Assistant.History(
			c.Request.Context(), toString(uid), role == "admin",
		)
		if err != nil {
			c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
			return
		}
		c.JSON(http.StatusOK, gin.H{"items": rows})
	}
}
