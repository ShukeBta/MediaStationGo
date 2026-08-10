// Package service — 下载管理器，管理多个下载客户端适配器。
//
// DownloadManager 提供多客户端分发能力，支持运行时热插拔。
// 调用方通过 GetDefault() 或 GetClient(id) 获取适配器来执行下载操作。
package service

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"sync"

	"go.uber.org/zap"

	"github.com/ShukeBta/MediaStationGo/internal/model"
	"github.com/ShukeBta/MediaStationGo/internal/repository"
)

// DownloadManager 管理多个下载客户端适配器实例。
type DownloadManager struct {
	log    *zap.Logger
	repo   *repository.Container
	crypto *CryptoService

	mu      sync.RWMutex
	clients map[string]DownloadAdapter // clientID -> adapter
	configs map[string]DownloadClientConfig
	models  map[string]model.DownloadClient
	order   []string
}

// NewDownloadManager 创建新的下载管理器。
func NewDownloadManager(log *zap.Logger, repo *repository.Container, crypto *CryptoService) *DownloadManager {
	return &DownloadManager{
		log:     log,
		repo:    repo,
		crypto:  crypto,
		clients: make(map[string]DownloadAdapter),
		configs: make(map[string]DownloadClientConfig),
		models:  make(map[string]model.DownloadClient),
	}
}

// LoadAll 从数据库加载所有已启用的客户端并初始化适配器。
func (m *DownloadManager) LoadAll(ctx context.Context) error {
	dbClients, err := m.repo.DownloadClient.ListEnabled(ctx)
	if err != nil {
		return err
	}
	if err := m.ensureEnabledDefault(ctx, dbClients); err != nil {
		return err
	}

	clients := make(map[string]DownloadAdapter, len(dbClients))
	configs := make(map[string]DownloadClientConfig, len(dbClients))
	models := make(map[string]model.DownloadClient, len(dbClients))
	order := make([]string, 0, len(dbClients))

	for _, dc := range dbClients {
		adapter, cfg, ok := m.initializeClient(ctx, dc)
		if !ok {
			continue
		}
		clients[dc.ID] = adapter
		configs[dc.ID] = cfg
		models[dc.ID] = dc
		order = append(order, dc.ID)
		m.log.Info("download client registered",
			zap.String("id", dc.ID),
			zap.String("name", dc.Name),
			zap.String("type", dc.Type),
		)
	}
	m.mu.Lock()
	m.clients = clients
	m.configs = configs
	m.models = models
	m.order = order
	m.mu.Unlock()
	return nil
}

func (m *DownloadManager) ensureEnabledDefault(ctx context.Context, clients []model.DownloadClient) error {
	if len(clients) == 0 {
		return nil
	}
	for i := range clients {
		if clients[i].IsDefault {
			return nil
		}
	}
	if err := m.repo.DownloadClient.SetDefault(ctx, clients[0].ID); err != nil {
		return err
	}
	clients[0].IsDefault = true
	return nil
}

func (m *DownloadManager) initializeClient(ctx context.Context, dc model.DownloadClient) (DownloadAdapter, DownloadClientConfig, bool) {
	cfg, err := m.buildConfig(&dc)
	if err != nil {
		m.log.Warn("failed to build config for download client",
			zap.String("id", dc.ID),
			zap.String("name", dc.Name),
			zap.Error(err))
		return nil, DownloadClientConfig{}, false
	}
	adapter := AdapterFactory(dc.Type)
	if adapter == nil {
		m.log.Warn("unknown download client type",
			zap.String("type", dc.Type),
			zap.String("id", dc.ID))
		return nil, DownloadClientConfig{}, false
	}
	if initErr := adapter.Initialize(ctx, cfg); initErr != nil {
		// Register configured clients even when the external process is still
		// starting; each operation can reconnect once it becomes reachable.
		m.log.Warn("download client init failed; registered for lazy reconnect",
			zap.String("id", dc.ID),
			zap.String("name", dc.Name),
			zap.Error(initErr))
	}
	return adapter, cfg, true
}

// GetDefault 返回默认下载客户端适配器。
// 如果没有设置默认客户端，返回第一个可用的客户端。
func (m *DownloadManager) GetDefault(_ context.Context) (*model.DownloadClient, DownloadAdapter, error) {
	m.mu.RLock()
	defer m.mu.RUnlock()

	for _, id := range m.order {
		client := m.models[id]
		if client.IsDefault {
			if adapter, ok := m.clients[id]; ok {
				copy := client
				return &copy, adapter, nil
			}
		}
	}
	for _, id := range m.order {
		if adapter, ok := m.clients[id]; ok {
			client := m.models[id]
			copy := client
			return &copy, adapter, nil
		}
	}

	return nil, nil, errors.New("no download client available")
}

// GetClient 返回指定 ID 的下载客户端适配器。
func (m *DownloadManager) GetClient(id string) (DownloadAdapter, error) {
	m.mu.RLock()
	defer m.mu.RUnlock()
	adapter, ok := m.clients[id]
	if !ok {
		return nil, errors.New("download client not found or not initialized")
	}
	return adapter, nil
}

type managedDownloadTarget struct {
	client  model.DownloadClient
	adapter DownloadAdapter
}

func (m *DownloadManager) getTarget(id string) (managedDownloadTarget, error) {
	m.mu.RLock()
	defer m.mu.RUnlock()
	adapter, ok := m.clients[id]
	if !ok {
		return managedDownloadTarget{}, errors.New("download client not found or not initialized")
	}
	client, ok := m.models[id]
	if !ok {
		return managedDownloadTarget{}, errors.New("download client metadata not found")
	}
	return managedDownloadTarget{client: client, adapter: adapter}, nil
}

func (m *DownloadManager) targets() []managedDownloadTarget {
	if m == nil {
		return nil
	}
	m.mu.RLock()
	defer m.mu.RUnlock()
	out := make([]managedDownloadTarget, 0, len(m.order))
	for _, id := range m.order {
		adapter, ok := m.clients[id]
		if !ok {
			continue
		}
		client, ok := m.models[id]
		if !ok {
			continue
		}
		out = append(out, managedDownloadTarget{client: client, adapter: adapter})
	}
	return out
}

func (m *DownloadManager) hasClients() bool {
	if m == nil {
		return false
	}
	m.mu.RLock()
	defer m.mu.RUnlock()
	return len(m.clients) > 0
}

// AddClient 动态添加并初始化一个下载客户端。
func (m *DownloadManager) AddClient(ctx context.Context, dc *model.DownloadClient) error {
	cfg, err := m.buildConfig(dc)
	if err != nil {
		return err
	}

	adapter := AdapterFactory(dc.Type)
	if adapter == nil {
		return errors.New("unknown download client type: " + dc.Type)
	}

	if err := adapter.Initialize(ctx, cfg); err != nil {
		return err
	}

	m.mu.Lock()
	defer m.mu.Unlock()
	for i, current := range m.order {
		if current == dc.ID {
			m.order = append(m.order[:i], m.order[i+1:]...)
			break
		}
	}
	m.clients[dc.ID] = adapter
	m.configs[dc.ID] = cfg
	m.models[dc.ID] = *dc
	m.order = append(m.order, dc.ID)
	return nil
}

// RemoveClient 移除一个下载客户端（停止适配器，不删除数据库记录）。
func (m *DownloadManager) RemoveClient(id string) {
	m.mu.Lock()
	defer m.mu.Unlock()
	delete(m.clients, id)
	delete(m.configs, id)
	delete(m.models, id)
	for i, current := range m.order {
		if current == id {
			m.order = append(m.order[:i], m.order[i+1:]...)
			break
		}
	}
}

// UpdateClient 更新已有客户端的配置并重新初始化。
func (m *DownloadManager) UpdateClient(ctx context.Context, dc *model.DownloadClient) error {
	m.RemoveClient(dc.ID)
	return m.AddClient(ctx, dc)
}

// TestConnection 测试客户端连接。
func (m *DownloadManager) TestConnection(ctx context.Context, dc *model.DownloadClient) error {
	cfg, err := m.buildConfig(dc)
	if err != nil {
		return err
	}

	adapter := AdapterFactory(dc.Type)
	if adapter == nil {
		return errors.New("unknown download client type: " + dc.Type)
	}

	return adapter.Initialize(ctx, cfg)
}

// ListAll 获取所有已加载客户端的种子列表。
func (m *DownloadManager) ListAll(ctx context.Context, filter string) (map[string][]TorrentInfo, error) {
	result := make(map[string][]TorrentInfo)
	var listErrs []error
	for _, target := range m.targets() {
		list, err := target.adapter.List(ctx, filter)
		if err != nil {
			m.log.Warn("failed to list torrents from client",
				zap.String("id", target.client.ID),
				zap.Error(err),
			)
			listErrs = append(listErrs, fmt.Errorf("%s (%s): %w", target.client.Name, target.client.Type, err))
		}
		if len(list) > 0 || err == nil {
			result[target.client.ID] = list
		}
	}
	return result, errors.Join(listErrs...)
}

// GetAdapterTypes 返回支持的下载客户端类型列表。
func (m *DownloadManager) GetAdapterTypes() []AdapterTypeInfo {
	return []AdapterTypeInfo{
		{Type: "qbittorrent", Name: "qBittorrent", Description: "qBittorrent WebUI API (v2)"},
		{Type: "transmission", Name: "Transmission", Description: "Transmission RPC API"},
		{Type: "aria2", Name: "Aria2", Description: "Aria2 JSON-RPC API"},
	}
}

// AdapterTypeInfo 描述下载客户端类型信息。
type AdapterTypeInfo struct {
	Type        string `json:"type"`
	Name        string `json:"name"`
	Description string `json:"description"`
}

// buildConfig 从数据库模型构建适配器配置。
func (m *DownloadManager) buildConfig(dc *model.DownloadClient) (DownloadClientConfig, error) {
	password := dc.Password
	if m.crypto != nil && password != "" {
		password = m.crypto.Decrypt(password)
	}
	host, err := normalizeDownloadClientEndpoint(dc.Type, dc.Host)
	if err != nil {
		return DownloadClientConfig{}, err
	}

	cfg := DownloadClientConfig{
		Host:     host,
		Username: dc.Username,
		Password: password,
	}

	// 解析 Extra JSON 配置
	if dc.Extra != "" {
		extraStr := dc.Extra
		if m.crypto != nil {
			extraStr = m.crypto.Decrypt(extraStr)
		}
		var extra map[string]string
		if err := json.Unmarshal([]byte(extraStr), &extra); err == nil {
			cfg.Extra = extra
		}
	}

	return cfg, nil
}
