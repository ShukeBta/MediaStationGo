package service

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"net/url"
	"strings"
	"time"

	"github.com/ShukeBta/MediaStationGo/internal/model"
)

// TestConnection 测试 API 连接。
func (s *ApiConfigService) TestConnection(ctx context.Context, provider string) (string, error) {
	cfg, err := s.GetByProvider(ctx, provider)
	if err != nil {
		return "error", err
	}

	// 根据不同提供者执行不同的测试逻辑
	switch provider {
	case "tmdb":
		return s.testTMDb(cfg)
	case "openai":
		return s.testOpenAI(cfg)
	case "deepseek":
		return s.testDeepSeek(cfg)
	case "siliconflow":
		return s.testSiliconFlow(cfg)
	default:
		return "unknown", fmt.Errorf("no test implemented for provider: %s", provider)
	}
}

// testTMDb 测试 TMDb API 连接。
func (s *ApiConfigService) testTMDb(cfg *model.ApiConfig) (string, error) {
	if cfg.APIKey == "" {
		return "error", errors.New("API key is required")
	}

	baseURL := strings.TrimRight(cfg.BaseURL, "/")
	if baseURL == "" {
		baseURL = strings.TrimRight(s.cfg.Secrets.TMDbAPIProxy, "/")
	}
	if baseURL == "" {
		baseURL = "https://api.themoviedb.org/3"
	}
	testURL := baseURL + "/configuration?api_key=" + url.QueryEscape(cfg.APIKey)
	req, err := http.NewRequestWithContext(context.Background(), http.MethodGet, testURL, nil)
	if err != nil {
		return "error", err
	}
	client := NewExternalHTTPClient(10 * time.Second)
	resp, err := client.Do(req)
	if err != nil {
		return "error", fmt.Errorf("TMDb connection failed: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode == 200 {
		return "success", nil
	}
	if resp.StatusCode == 401 {
		return "invalid", errors.New("invalid API key")
	}
	return "error", fmt.Errorf("TMDb API returned status %d", resp.StatusCode)
}

// testOpenAI 测试 OpenAI API 连接。
func (s *ApiConfigService) testOpenAI(cfg *model.ApiConfig) (string, error) {
	if cfg.APIKey == "" {
		return "error", errors.New("API key is required")
	}

	baseURL := cfg.BaseURL
	if baseURL == "" {
		baseURL = "https://api.openai.com/v1"
	}

	testURL := baseURL + "/models"
	req, err := http.NewRequest("GET", testURL, nil)
	if err != nil {
		return "error", err
	}
	req.Header.Set("Authorization", "Bearer "+cfg.APIKey)

	client := &http.Client{Timeout: 10 * time.Second}
	resp, err := client.Do(req.WithContext(context.Background()))
	if err != nil {
		return "error", fmt.Errorf("OpenAI connection failed: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode == 200 {
		return "success", nil
	}
	if resp.StatusCode == 401 {
		return "invalid", errors.New("invalid API key")
	}
	return "error", fmt.Errorf("OpenAI API returned status %d", resp.StatusCode)
}

// testDeepSeek 测试 DeepSeek API 连接。
func (s *ApiConfigService) testDeepSeek(cfg *model.ApiConfig) (string, error) {
	if cfg.APIKey == "" {
		return "error", errors.New("API key is required")
	}

	baseURL := cfg.BaseURL
	if baseURL == "" {
		baseURL = "https://api.deepseek.com"
	}

	testURL := baseURL + "/models"
	req, err := http.NewRequest("GET", testURL, nil)
	if err != nil {
		return "error", err
	}
	req.Header.Set("Authorization", "Bearer "+cfg.APIKey)

	client := &http.Client{Timeout: 10 * time.Second}
	resp, err := client.Do(req.WithContext(context.Background()))
	if err != nil {
		return "error", fmt.Errorf("DeepSeek connection failed: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode == 200 {
		return "success", nil
	}
	if resp.StatusCode == 401 {
		return "invalid", errors.New("invalid API key")
	}
	return "error", fmt.Errorf("DeepSeek API returned status %d", resp.StatusCode)
}

// testSiliconFlow 测试 SiliconFlow API 连接。
func (s *ApiConfigService) testSiliconFlow(cfg *model.ApiConfig) (string, error) {
	if cfg.APIKey == "" {
		return "error", errors.New("API key is required")
	}

	baseURL := cfg.BaseURL
	if baseURL == "" {
		baseURL = "https://api.siliconflow.cn/v1"
	}

	testURL := baseURL + "/models"
	req, err := http.NewRequest("GET", testURL, nil)
	if err != nil {
		return "error", err
	}
	req.Header.Set("Authorization", "Bearer "+cfg.APIKey)

	client := &http.Client{Timeout: 10 * time.Second}
	resp, err := client.Do(req.WithContext(context.Background()))
	if err != nil {
		return "error", fmt.Errorf("SiliconFlow connection failed: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode == 200 {
		return "success", nil
	}
	if resp.StatusCode == 401 {
		return "invalid", errors.New("invalid API key")
	}
	return "error", fmt.Errorf("SiliconFlow API returned status %d", resp.StatusCode)
}
