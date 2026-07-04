// Package service — API 配置管理服务。
package service

import (
	"context"
	"errors"

	"go.uber.org/zap"

	"github.com/ShukeBta/MediaStationGo/internal/config"
	"github.com/ShukeBta/MediaStationGo/internal/model"
	"github.com/ShukeBta/MediaStationGo/internal/repository"
)

// ApiConfigService 负责第三方 API 配置的 CRUD 和加密管理。
type ApiConfigService struct {
	cfg    *config.Config
	log    *zap.Logger
	repo   *repository.Container
	crypto *CryptoService
}

// NewApiConfigService 创建 API 配置服务实例。
func NewApiConfigService(cfg *config.Config, log *zap.Logger, repo *repository.Container, crypto *CryptoService) *ApiConfigService {
	return &ApiConfigService{cfg: cfg, log: log, repo: repo, crypto: crypto}
}

// ApiConfigService 错误定义。
var (
	ErrApiConfigNotFound = errors.New("API configuration not found")
	ErrInvalidProvider   = errors.New("invalid provider")
	ErrTestFailed        = errors.New("connection test failed")
)

// GetByProvider 获取指定提供者的 API 配置。
func (s *ApiConfigService) GetByProvider(ctx context.Context, provider string) (*model.ApiConfig, error) {
	cfg, err := s.repo.ApiConfig.FindByProvider(ctx, provider)
	if err != nil {
		return nil, err
	}
	if cfg == nil {
		return nil, ErrApiConfigNotFound
	}
	// 解密敏感字段
	if cfg.APIKey != "" && s.crypto.IsEncrypted(cfg.APIKey) {
		cfg.APIKey = s.crypto.Decrypt(cfg.APIKey)
	}
	return cfg, nil
}

// List 返回所有 API 配置。
func (s *ApiConfigService) List(ctx context.Context) ([]model.ApiConfig, error) {
	configs, err := s.repo.ApiConfig.List(ctx)
	if err != nil {
		return nil, err
	}
	// 解密敏感字段
	for i := range configs {
		if configs[i].APIKey != "" && s.crypto.IsEncrypted(configs[i].APIKey) {
			configs[i].APIKey = s.crypto.Decrypt(configs[i].APIKey)
		}
	}
	return configs, nil
}

// GetProviders 返回预定义的提供者列表。
func (s *ApiConfigService) GetProviders() []model.ApiProvider {
	return model.PredefinedProviders()
}

// Upsert 创建或更新 API 配置，自动加密敏感字段。
func (s *ApiConfigService) Upsert(ctx context.Context, provider string, apiKey, baseURL, extra string, enabled bool) (*model.ApiConfig, error) {
	// 验证提供者是否有效
	if !s.isValidProvider(provider) {
		return nil, ErrInvalidProvider
	}

	// 加密 API Key
	encryptedKey := apiKey
	if apiKey != "" && !s.crypto.IsEncrypted(apiKey) {
		encryptedKey = s.crypto.Encrypt(apiKey)
	}

	cfg := &model.ApiConfig{
		Provider:    provider,
		APIKey:      encryptedKey,
		BaseURL:     baseURL,
		Extra:       extra,
		Enabled:     enabled,
		Description: s.getProviderDescription(provider),
	}

	if err := s.repo.ApiConfig.Upsert(ctx, cfg); err != nil {
		return nil, err
	}

	// 返回解密后的配置
	cfg.APIKey = apiKey
	return cfg, nil
}

// Delete 删除 API 配置。
func (s *ApiConfigService) Delete(ctx context.Context, provider string) error {
	return s.repo.ApiConfig.Delete(ctx, provider)
}

// Update 更新 API 配置。
func (s *ApiConfigService) Update(ctx context.Context, provider string, apiKey, baseURL, extra string, enabled bool) error {
	// 加密 API Key
	encryptedKey := apiKey
	if apiKey != "" && !s.crypto.IsEncrypted(apiKey) {
		encryptedKey = s.crypto.Encrypt(apiKey)
	}

	cfg := &model.ApiConfig{
		Provider: provider,
		APIKey:   encryptedKey,
		BaseURL:  baseURL,
		Extra:    extra,
		Enabled:  enabled,
	}

	return s.repo.ApiConfig.Update(ctx, cfg)
}
