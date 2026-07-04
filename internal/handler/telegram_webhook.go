// Package handler — Telegram Bot Webhook 端点。
//
// 接收 Telegram Bot API 推送的 update 消息，路由到 TelegramBotService 处理。
package handler

import (
	"context"
	"io"
	"net/http"
	"time"

	"github.com/gin-gonic/gin"
	"go.uber.org/zap"

	"github.com/ShukeBta/MediaStationGo/internal/service"
)

// telegramWebhookHandler 处理 Telegram Bot 的 Webhook 回调。
//
// 路由：POST /api/telegram/webhook （无需认证，由 Telegram 服务器调用）
func telegramWebhookHandler(svc *service.Container) gin.HandlerFunc {
	return func(c *gin.Context) {
		body, err := io.ReadAll(c.Request.Body)
		if err != nil {
			c.JSON(http.StatusBadRequest, gin.H{"error": "cannot read body"})
			return
		}

		if err := svc.TelegramBot.HandleWebhook(c.Request.Context(), body); err != nil {
			svc.Log.Error("telegram webhook error", zap.Error(err))
			c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
			return
		}

		c.JSON(http.StatusOK, gin.H{"ok": true})
	}
}

// telegramSetWebhookHandler 管理员手动设置/更新 Webhook URL。
//
// 路由：POST /api/admin/telegram/webhook (需 admin 认证)
func telegramSetWebhookHandler(svc *service.Container) gin.HandlerFunc {
	return func(c *gin.Context) {
		var req struct {
			BotToken   string `json:"bot_token" binding:"required"`
			WebhookURL string `json:"webhook_url" binding:"required"`
		}
		if err := c.ShouldBindJSON(&req); err != nil {
			c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
			return
		}

		if err := svc.TelegramBot.SetWebhook(c.Request.Context(), req.BotToken, req.WebhookURL); err != nil {
			c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
			return
		}

		c.JSON(http.StatusOK, gin.H{"message": "webhook set successfully", "url": req.WebhookURL})
	}
}

// telegramGetWebhookHandler 获取当前 Webhook 配置信息。
//
// 路由：GET /api/admin/telegram/webhook (需 admin 认证)
func telegramGetWebhookHandler(svc *service.Container) gin.HandlerFunc {
	return func(c *gin.Context) {
		botToken := c.Query("bot_token")
		if botToken == "" {
			c.JSON(http.StatusBadRequest, gin.H{"error": "bot_token is required"})
			return
		}

		info, err := svc.TelegramBot.GetWebhookInfo(c.Request.Context(), botToken)
		if err != nil {
			c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
			return
		}

		c.JSON(http.StatusOK, info)
	}
}

// telegramStartPollingHandler 启动 Telegram 长轮询。
//
// 路由：POST /api/admin/telegram/polling/start (需 admin 认证)
func telegramStartPollingHandler(svc *service.Container) gin.HandlerFunc {
	return func(c *gin.Context) {
		setupCtx, cancel := context.WithTimeout(svc.Context(), 30*time.Second)
		defer cancel()

		result := svc.TelegramBot.StartPolling(setupCtx)
		c.JSON(http.StatusOK, result)
	}
}

// telegramStopPollingHandler 停止 Telegram 长轮询。
//
// 路由：POST /api/admin/telegram/polling/stop (需 admin 认证)
func telegramStopPollingHandler(svc *service.Container) gin.HandlerFunc {
	return func(c *gin.Context) {
		stopped := svc.TelegramBot.StopPolling()
		c.JSON(http.StatusOK, gin.H{"message": "polling stopped", "stopped": stopped})
	}
}
