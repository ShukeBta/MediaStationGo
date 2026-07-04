package service

import (
	"context"
	"net/url"
	"strings"

	"github.com/ShukeBta/MediaStationGo/internal/model"
)

// GetEffectiveConfig 获取生效的 API 配置（数据库配置优先于配置文件）。
func (s *ApiConfigService) GetEffectiveConfig(ctx context.Context, provider string) (*model.ApiConfig, error) {
	// 首先尝试从数据库获取
	cfg, err := s.GetByProvider(ctx, provider)
	if err == nil && cfg != nil {
		return cfg, nil
	}

	// 如果数据库没有，尝试从配置文件获取
	return s.getConfigFromFile(provider)
}

// getConfigFromFile 从配置文件获取 API 配置。
func (s *ApiConfigService) getConfigFromFile(provider string) (*model.ApiConfig, error) {
	var apiKey string
	var hasKey bool

	switch provider {
	case "tmdb":
		apiKey = s.cfg.Secrets.TMDbAPIKey
		hasKey = apiKey != ""
	case "bangumi":
		apiKey = s.cfg.Secrets.BangumiToken
		hasKey = apiKey != ""
	case "thetvdb":
		apiKey = s.cfg.Secrets.TheTVDBAPIKey
		hasKey = apiKey != ""
	case "fanart":
		apiKey = s.cfg.Secrets.FanartAPIKey
		hasKey = apiKey != ""
	}

	if !hasKey {
		return nil, ErrApiConfigNotFound
	}

	return &model.ApiConfig{
		Provider: provider,
		APIKey:   apiKey,
		Enabled:  true,
	}, nil
}

// isValidProvider 检查提供者是否有效。
func (s *ApiConfigService) isValidProvider(provider string) bool {
	providers := model.PredefinedProviders()
	for _, p := range providers {
		if p.ID == provider {
			return true
		}
	}
	return false
}

// getProviderDescription 获取提供者描述。
func (s *ApiConfigService) getProviderDescription(provider string) string {
	providers := model.PredefinedProviders()
	for _, p := range providers {
		if p.ID == provider {
			return p.Description
		}
	}
	return ""
}

// UpdateTestResult 更新测试结果。
func (s *ApiConfigService) UpdateTestResult(ctx context.Context, provider, result string) error {
	return s.repo.ApiConfig.UpdateTestResult(ctx, provider, result)
}

// MaskAPIKey 遮蔽 API Key 的中间部分。
func (s *ApiConfigService) MaskAPIKey(apiKey string) string {
	if len(apiKey) <= 8 {
		return "***"
	}
	return apiKey[:4] + "..." + apiKey[len(apiKey)-4:]
}

// ExtractBaseURL 从 URL 中提取域名。
func ExtractBaseURL(rawURL string) string {
	if rawURL == "" {
		return ""
	}
	u, err := url.Parse(rawURL)
	if err != nil {
		return rawURL
	}
	return u.Scheme + "://" + u.Host
}

// ProviderMatches 检查请求的提供者是否与配置的提供者匹配。
func ProviderMatches(requested, configured string) bool {
	return strings.EqualFold(requested, configured)
}
