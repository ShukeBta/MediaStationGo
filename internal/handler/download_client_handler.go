// Package handler — 下载客户端管理 HTTP 端点。
package handler

import (
	"context"
	"encoding/json"
	"net/http"

	"github.com/gin-gonic/gin"
	"go.uber.org/zap"

	"github.com/ShukeBta/MediaStationGo/internal/model"
	"github.com/ShukeBta/MediaStationGo/internal/service"
)

// DownloadClientHandler 处理下载客户端的 CRUD 操作。
type DownloadClientHandler struct {
	svc *service.Container
	log *zap.Logger
}

// NewDownloadClientHandler 创建下载客户端处理器。
func NewDownloadClientHandler(svc *service.Container, log *zap.Logger) *DownloadClientHandler {
	return &DownloadClientHandler{svc: svc, log: log}
}

// downloadClientCreateRequest 创建下载客户端请求体。
type downloadClientCreateRequest struct {
	Name      string            `json:"name" binding:"required"`
	Type      string            `json:"type" binding:"required,oneof=qbittorrent transmission aria2"`
	Host      string            `json:"host" binding:"required"`
	Username  string            `json:"username"`
	Password  string            `json:"password"`
	IsDefault bool              `json:"is_default"`
	Extra     map[string]string `json:"extra,omitempty"`
}

// downloadClientUpdateRequest 更新下载客户端请求体。
type downloadClientUpdateRequest struct {
	Name      string            `json:"name"`
	Type      string            `json:"type" binding:"omitempty,oneof=qbittorrent transmission aria2"`
	Host      string            `json:"host"`
	Username  string            `json:"username"`
	Password  string            `json:"password"`
	IsDefault *bool             `json:"is_default"`
	Enabled   *bool             `json:"enabled"`
	Extra     map[string]string `json:"extra,omitempty"`
}

// Create 创建新的下载客户端。
func (h *DownloadClientHandler) Create(c *gin.Context) {
	var req downloadClientCreateRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		Error(c, http.StatusBadRequest, ErrInvalidParams, err.Error())
		return
	}

	ctx := c.Request.Context()
	_ = h.svc.Repo.Setting.Set(ctx, "download_clients.managed", "true")
	normalizedHost, err := service.NormalizeDownloadClientHost(req.Type, req.Host)
	if err != nil {
		Error(c, http.StatusBadRequest, ErrInvalidParams, err.Error())
		return
	}

	// 加密密码
	password := req.Password
	if password != "" && h.svc.Crypto != nil {
		password = h.svc.Crypto.Encrypt(password)
	}

	// 加密 Extra 配置
	extraStr := ""
	if len(req.Extra) > 0 {
		extraJSON, _ := json.Marshal(req.Extra)
		extraStr = string(extraJSON)
		if h.svc.Crypto != nil {
			extraStr = h.svc.Crypto.Encrypt(extraStr)
		}
	}

	// 如果设为默认，先清除其他默认
	if req.IsDefault {
		_ = h.svc.Repo.DownloadClient.ClearDefault(ctx)
	}

	client := &model.DownloadClient{
		Name:      req.Name,
		Type:      req.Type,
		Host:      normalizedHost,
		Username:  req.Username,
		Password:  password,
		IsDefault: req.IsDefault,
		Enabled:   true,
		Extra:     extraStr,
	}

	if err := h.svc.Repo.DownloadClient.Create(ctx, client); err != nil {
		Error(c, http.StatusInternalServerError, ErrInternal, "创建失败: "+err.Error())
		return
	}

	if err := h.svc.Downloads.ReloadConfig(ctx); err != nil {
		h.log.Warn("failed to reload download clients after create", zap.Error(err))
		Error(c, http.StatusInternalServerError, ErrInternal, "客户端已保存，但运行时重载失败: "+err.Error())
		return
	}

	Success(c, client)
}

// List 返回所有下载客户端。
func (h *DownloadClientHandler) List(c *gin.Context) {
	ctx := c.Request.Context()
	clients, err := h.svc.Repo.DownloadClient.List(ctx)
	if err != nil {
		Error(c, http.StatusInternalServerError, ErrInternal, "查询失败")
		return
	}
	Success(c, clients)
}

// Get 返回指定下载客户端详情。
func (h *DownloadClientHandler) Get(c *gin.Context) {
	id := c.Param("id")
	ctx := c.Request.Context()
	client, err := h.svc.Repo.DownloadClient.FindByID(ctx, id)
	if err != nil || client == nil {
		Error(c, http.StatusNotFound, ErrNotFound, "客户端不存在")
		return
	}
	Success(c, client)
}

// Update 更新下载客户端。
func (h *DownloadClientHandler) Update(c *gin.Context) {
	id := c.Param("id")
	var req downloadClientUpdateRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		Error(c, http.StatusBadRequest, ErrInvalidParams, err.Error())
		return
	}

	ctx := c.Request.Context()
	client, err := h.svc.Repo.DownloadClient.FindByID(ctx, id)
	if err != nil || client == nil {
		Error(c, http.StatusNotFound, ErrNotFound, "客户端不存在")
		return
	}

	if req.Name != "" {
		client.Name = req.Name
	}
	if req.Type != "" {
		client.Type = req.Type
	}
	if req.Host != "" {
		clientType := client.Type
		if req.Type != "" {
			clientType = req.Type
		}
		normalizedHost, err := service.NormalizeDownloadClientHost(clientType, req.Host)
		if err != nil {
			Error(c, http.StatusBadRequest, ErrInvalidParams, err.Error())
			return
		}
		client.Host = normalizedHost
	}
	if req.Username != "" {
		client.Username = req.Username
	}
	if req.Password != "" {
		if h.svc.Crypto != nil {
			client.Password = h.svc.Crypto.Encrypt(req.Password)
		} else {
			client.Password = req.Password
		}
	}
	if req.IsDefault != nil && *req.IsDefault {
		_ = h.svc.Repo.DownloadClient.ClearDefault(ctx)
		client.IsDefault = *req.IsDefault
	}
	if req.Enabled != nil {
		client.Enabled = *req.Enabled
	}
	if len(req.Extra) > 0 {
		extraJSON, _ := json.Marshal(req.Extra)
		extraStr := string(extraJSON)
		if h.svc.Crypto != nil {
			client.Extra = h.svc.Crypto.Encrypt(extraStr)
		} else {
			client.Extra = extraStr
		}
	}
	normalizedHost, err := service.NormalizeDownloadClientHost(client.Type, client.Host)
	if err != nil {
		Error(c, http.StatusBadRequest, ErrInvalidParams, err.Error())
		return
	}
	client.Host = normalizedHost

	if err := h.svc.Repo.DownloadClient.Update(ctx, client); err != nil {
		Error(c, http.StatusInternalServerError, ErrInternal, "更新失败")
		return
	}
	clearLegacyQBitSettingsIfNoDefault(c.Request.Context(), h.svc)

	if err := h.svc.Downloads.ReloadConfig(ctx); err != nil {
		h.log.Warn("failed to reload download clients after update", zap.Error(err))
		Error(c, http.StatusInternalServerError, ErrInternal, "客户端已更新，但运行时重载失败: "+err.Error())
		return
	}

	Success(c, client)
}

// Delete 删除下载客户端。
func (h *DownloadClientHandler) Delete(c *gin.Context) {
	id := c.Param("id")
	ctx := c.Request.Context()
	_ = h.svc.Repo.Setting.Set(ctx, "download_clients.managed", "true")

	_, err := h.svc.Repo.DownloadClient.FindByID(ctx, id)
	if err != nil {
		Error(c, http.StatusNotFound, ErrNotFound, "客户端不存在")
		return
	}

	if delErr := h.svc.Repo.DownloadClient.Delete(ctx, id); delErr != nil {
		Error(c, http.StatusInternalServerError, ErrInternal, "删除失败")
		return
	}

	clearLegacyQBitSettingsIfNoDefault(c.Request.Context(), h.svc)
	if err := h.svc.Downloads.ReloadConfig(ctx); err != nil {
		h.log.Warn("failed to reload download clients after delete", zap.Error(err))
		Error(c, http.StatusInternalServerError, ErrInternal, "客户端已删除，但运行时重载失败: "+err.Error())
		return
	}

	SuccessWithMessage(c, "已删除", nil)
}

func clearLegacyQBitSettingsIfNoDefault(ctx context.Context, svc *service.Container) {
	if svc == nil || svc.Repo == nil || svc.Repo.DownloadClient == nil || svc.Repo.Setting == nil {
		return
	}
	defaultClient, err := svc.Repo.DownloadClient.FindDefault(ctx)
	if err != nil || defaultClient != nil {
		return
	}
	_ = svc.Repo.Setting.Set(ctx, "qbittorrent.url", "")
	_ = svc.Repo.Setting.Set(ctx, "qbittorrent.username", "")
	_ = svc.Repo.Setting.Set(ctx, "qbittorrent.password", "")
}

// Test 测试下载客户端连接。
func (h *DownloadClientHandler) Test(c *gin.Context) {
	id := c.Param("id")
	ctx := c.Request.Context()

	client, err := h.svc.Repo.DownloadClient.FindByID(ctx, id)
	if err != nil || client == nil {
		Error(c, http.StatusNotFound, ErrNotFound, "客户端不存在")
		return
	}

	if err := h.svc.DownloadMgr.TestConnection(ctx, client); err != nil {
		Error(c, http.StatusBadRequest, ErrExternal, "连接测试失败: "+err.Error())
		return
	}

	SuccessWithMessage(c, "连接成功", nil)
}
