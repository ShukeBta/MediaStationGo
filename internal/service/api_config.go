// Package service — third-party API key store.
//
// APIConfigService is a small CRUD layer over the api_configs table. It
// transparently encrypts the api_key column on write and decrypts it on
// read so values stored on disk are useless without the JWT secret.
//
// On first read it seeds the table with the providers supported by
// MediaStationGo today (TMDb / Bangumi / TheTVDB / Fanart / OpenAI / Douban).
package service

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"sort"
	"strings"
	"sync/atomic"
	"time"

	"go.uber.org/zap"
	"gorm.io/gorm"

	"github.com/ShukeBta/MediaStationGo/internal/model"
	"github.com/ShukeBta/MediaStationGo/internal/repository"
)

// APIConfigService coordinates third-party API key storage.
type APIConfigService struct {
	revision atomic.Uint64
	log      *zap.Logger
	repo     *repository.Container
	crypto   *CryptoService
}

// NewAPIConfigService is the constructor.
func NewAPIConfigService(log *zap.Logger, repo *repository.Container, crypto *CryptoService) *APIConfigService {
	return &APIConfigService{log: log, repo: repo, crypto: crypto}
}

// SeedDefaults inserts a row for every well-known provider on first run.
func (s *APIConfigService) SeedDefaults(ctx context.Context) error {
	defaults := []model.APIConfig{
		{Provider: "tmdb", BaseURL: "https://api.themoviedb.org/3", Description: "TMDb (movies + tv)", Enabled: true},
		{Provider: "bangumi", BaseURL: "https://api.bgm.tv", Description: "Bangumi (anime)", Enabled: true},
		{Provider: "thetvdb", BaseURL: "https://api4.thetvdb.com/v4", Description: "TheTVDB (tv)", Enabled: true},
		{Provider: "fanart", BaseURL: "https://webservice.fanart.tv/v3", Description: "Fanart.tv (artwork)", Enabled: true},
		{Provider: "douban", Description: "Douban cookie (zh metadata)", Enabled: true},
		{Provider: "adult", BaseURL: "https://javdb.com", Extra: "https://javbus.sbs,https://www.javbus.com,https://www.cdnbus.cyou,https://www.javsee.cyou,https://www.busjav.cyou", Description: "Adult / 番号元数据（JavDB/JavBus）", Enabled: true},
		{Provider: "fd2ppv", BaseURL: "https://fd2ppv.cc", Description: "FD2PPV 会员账号（FC2 元数据与图片）", Enabled: true},
		{Provider: "openai", BaseURL: "https://api.openai.com/v1", Description: "AI / 大语言模型（智能搜索与对话）", Enabled: true},
		{Provider: "deepseek", BaseURL: "https://api.deepseek.com/v1", Description: "DeepSeek（AI 字幕翻译）", Enabled: true},
		{Provider: "siliconflow", BaseURL: "https://api.siliconflow.cn/v1", Description: "硅基流动（AI 字幕翻译）", Enabled: true},
	}
	for i := range defaults {
		var existing model.APIConfig
		err := s.repo.DB.WithContext(ctx).
			Where("provider = ?", defaults[i].Provider).
			First(&existing).Error
		if err == nil {
			continue
		}
		if !errors.Is(err, gorm.ErrRecordNotFound) {
			return err
		}
		if err := s.repo.DB.WithContext(ctx).Create(&defaults[i]).Error; err != nil {
			return err
		}
	}
	return nil
}

// PublicView is the safe-to-display projection of an API config row.
// The plaintext key is never returned — only a mask.
type PublicView struct {
	ImageDirect  bool      `json:"image_direct"`
	UseProxyPool bool      `json:"use_proxy_pool"`
	ID           string    `json:"id"`
	Provider     string    `json:"provider"`
	BaseURL      string    `json:"base_url,omitempty"`
	Extra        string    `json:"extra,omitempty"`
	Model        string    `json:"model,omitempty"`
	Enabled      bool      `json:"enabled"`
	Description  string    `json:"description,omitempty"`
	HasKey       bool      `json:"has_key"`
	MaskedKey    string    `json:"masked_key,omitempty"`
	CreatedAt    time.Time `json:"created_at"`
	UpdatedAt    time.Time `json:"updated_at"`
}

// List returns every API config row (with masked keys).
func (s *APIConfigService) List(ctx context.Context) ([]PublicView, error) {
	var rows []model.APIConfig
	if err := s.repo.DB.WithContext(ctx).Order("provider asc").Find(&rows).Error; err != nil {
		return nil, err
	}
	out := make([]PublicView, 0, len(rows))
	for _, r := range rows {
		out = append(out, s.toPublic(&r))
	}
	return out, nil
}

// Get returns the public view for a single provider, or nil.
func (s *APIConfigService) Get(ctx context.Context, provider string) (*PublicView, error) {
	row, err := s.findByProvider(ctx, provider)
	if err != nil || row == nil {
		return nil, err
	}
	v := s.toPublic(row)
	return &v, nil
}

// Resolve returns the decrypted key + base url ready for use by an HTTP
// client. Empty struct (with no error) when the provider is unknown or
// the API key is empty.
type Resolved struct {
	ImageDirect  bool
	UseProxyPool bool
	Revision     uint64
	APIKey       string
	BaseURL      string
	Extra        string
	Model        string
	Enabled      bool
}

// Resolve fetches the live configuration for a provider, decrypting the
// API key. Callers can use Resolved.APIKey != "" as the "configured" check.
func (s *APIConfigService) Resolve(ctx context.Context, provider string) (Resolved, error) {
	row, err := s.findByProvider(ctx, provider)
	if err != nil {
		s.log.Warn("api_config.resolve: query failed", zap.String("provider", provider), zap.Error(err))
		return Resolved{}, err
	}
	if row == nil {
		s.log.Warn("api_config.resolve: provider not found", zap.String("provider", provider))
		return Resolved{}, nil
	}
	resolved := Resolved{
		ImageDirect: row.ImageDirect, UseProxyPool: row.UseProxyPool, Revision: s.revision.Load(),
		APIKey:  s.crypto.Decrypt(row.APIKey),
		BaseURL: row.BaseURL,
		Extra:   row.Extra,
		Model:   apiConfigModel(row.Provider, row.Extra),
		Enabled: row.Enabled,
	}
	s.log.Debug("api_config.resolve: success",
		zap.String("provider", provider),
		zap.Bool("has_key", resolved.APIKey != ""),
		zap.Bool("enabled", resolved.Enabled))
	return resolved, nil
}

// Update upserts a single provider's config. An empty patch.APIKey leaves
// the existing key untouched; pass "<clear>" sentinel to wipe it.
type APIConfigPatch struct {
	ImageDirect  *bool   `json:"image_direct,omitempty"`
	UseProxyPool *bool   `json:"use_proxy_pool,omitempty"`
	APIKey       *string `json:"api_key,omitempty"`
	BaseURL      *string `json:"base_url,omitempty"`
	Extra        *string `json:"extra,omitempty"`
	Model        *string `json:"model,omitempty"`
	Enabled      *bool   `json:"enabled,omitempty"`
	Description  *string `json:"description,omitempty"`
}

// Update applies the patch and returns the new public view.
func (s *APIConfigService) Update(ctx context.Context, provider string, patch APIConfigPatch) (*PublicView, error) {
	provider = strings.TrimSpace(strings.ToLower(provider))
	if provider == "" {
		return nil, errors.New("provider required")
	}
	var aiOptions *aiProviderOptions
	if isAIConfigProvider(provider) && patch.Extra != nil {
		parsed, err := parseAIProviderOptions(*patch.Extra)
		if err != nil {
			return nil, err
		}
		aiOptions = &parsed
	}

	row, err := s.findByProvider(ctx, provider)
	if err != nil {
		return nil, err
	}
	if row == nil {
		row = &model.APIConfig{Provider: provider, Enabled: true}
		if err := s.repo.DB.WithContext(ctx).Create(row).Error; err != nil {
			return nil, err
		}
	}

	updates := map[string]any{}
	if patch.ImageDirect != nil {
		updates["image_direct"] = *patch.ImageDirect
	}
	if patch.UseProxyPool != nil {
		updates["use_proxy_pool"] = *patch.UseProxyPool
	}
	if aiOptions != nil {
		previous, _ := parseAIProviderOptions(row.Extra)
		oldProvider, newProvider := provider, provider
		if previous.Provider != "" {
			oldProvider = normalizeAIProvider(previous.Provider)
		}
		if aiOptions.Provider != "" {
			newProvider = normalizeAIProvider(aiOptions.Provider)
		}
		oldProtocol := normalizeAIProtocol(oldProvider, previous.Protocol)
		newProtocol := normalizeAIProtocol(newProvider, aiOptions.Protocol)
		oldFamily := aiCredentialFamily(oldProtocol)
		newFamily := aiCredentialFamily(newProtocol)
		if oldFamily != newFamily || oldProvider != newProvider {
			// A key for one provider must not follow a switch to another API.
			if patch.APIKey == nil {
				updates["api_key"] = ""
			}
			if patch.BaseURL == nil && row.BaseURL == defaultAIBase(oldProvider, oldProtocol) {
				updates["base_url"] = defaultAIBase(newProvider, newProtocol)
			}
		}
	}
	if patch.APIKey != nil {
		v := strings.TrimSpace(*patch.APIKey)
		if v == "" || v == "<clear>" {
			updates["api_key"] = ""
		} else {
			updates["api_key"] = s.crypto.Encrypt(v)
		}
	}
	if patch.BaseURL != nil {
		value := *patch.BaseURL
		if provider == "douban" {
			var err error
			value, err = normalizeResinProxyOrigin(value)
			if err != nil {
				return nil, errors.New("douban image address must be an HTTP(S) origin")
			}
		}
		updates["base_url"] = value
	}
	if patch.Extra != nil {
		updates["extra"] = *patch.Extra
	}
	if patch.Model != nil {
		if !isAICompatibleProvider(provider) {
			return nil, errors.New("model is only supported for openai-compatible config")
		}
		// Extra 是 AI 选项 JSON({"provider","protocol","model"}),模型写进
		// 其中的 model 键,避免覆盖服务商/协议选项(纯字符串会被运行时
		// 解析失败并禁用 AI)。
		base := row.Extra
		if raw, ok := updates["extra"].(string); ok {
			base = raw
		}
		merged, err := setAIOptionsModel(base, strings.TrimSpace(*patch.Model))
		if err != nil {
			return nil, err
		}
		updates["extra"] = merged
	}
	if patch.Enabled != nil {
		updates["enabled"] = *patch.Enabled
	}
	if patch.Description != nil {
		updates["description"] = *patch.Description
	}
	if len(updates) > 0 {
		if err := s.repo.DB.WithContext(ctx).
			Model(&model.APIConfig{}).
			Where("id = ?", row.ID).
			Updates(updates).Error; err != nil {
			return nil, err
		}
	}
	s.revision.Add(1)
	row, _ = s.findByProvider(ctx, provider)
	v := s.toPublic(row)
	return &v, nil
}

// Delete clears a provider's API key (the row stays so the masked
// description is still useful). Non-existent providers are a no-op.
func (s *APIConfigService) Delete(ctx context.Context, provider string) error {
	row, err := s.findByProvider(ctx, provider)
	if err != nil || row == nil {
		return err
	}
	err = s.repo.DB.WithContext(ctx).Model(&model.APIConfig{}).Where("id = ?", row.ID).Update("api_key", "").Error
	if err == nil {
		s.revision.Add(1)
	}
	return err
}

func (s *APIConfigService) findByProvider(ctx context.Context, provider string) (*model.APIConfig, error) {
	var row model.APIConfig
	err := s.repo.DB.WithContext(ctx).Where("provider = ?", provider).First(&row).Error
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	return &row, nil
}

func (s *APIConfigService) toPublic(r *model.APIConfig) PublicView {
	plain := s.crypto.Decrypt(r.APIKey)
	pv := PublicView{
		ImageDirect: r.ImageDirect, UseProxyPool: r.UseProxyPool,
		ID:          r.ID,
		Provider:    r.Provider,
		BaseURL:     r.BaseURL,
		Extra:       r.Extra,
		Model:       apiConfigModel(r.Provider, r.Extra),
		Enabled:     r.Enabled,
		Description: r.Description,
		HasKey:      plain != "",
		CreatedAt:   r.CreatedAt,
		UpdatedAt:   r.UpdatedAt,
	}
	if pv.HasKey {
		if strings.EqualFold(r.Provider, "fd2ppv") || strings.EqualFold(r.Provider, "douban") {
			pv.MaskedKey = "••••••••"
		} else {
			pv.MaskedKey = MaskAPIKey(plain)
		}
	}
	return pv
}

type AIModelInfo struct {
	ID      string `json:"id"`
	OwnedBy string `json:"owned_by,omitempty"`
}

type AIModelDiscoveryInput struct {
	APIKey  string `json:"api_key,omitempty"`
	BaseURL string `json:"base_url,omitempty"`
}

func (s *APIConfigService) DiscoverModels(ctx context.Context, provider string, input AIModelDiscoveryInput) ([]AIModelInfo, error) {
	provider = strings.ToLower(strings.TrimSpace(provider))
	if !isAICompatibleProvider(provider) {
		return nil, errors.New("model discovery is only supported for openai-compatible config")
	}
	resolved, err := s.Resolve(ctx, provider)
	if err != nil {
		return nil, err
	}
	apiKey := strings.TrimSpace(input.APIKey)
	if apiKey == "" {
		apiKey = strings.TrimSpace(resolved.APIKey)
	}
	baseURL := strings.TrimSpace(input.BaseURL)
	if baseURL == "" {
		baseURL = strings.TrimSpace(resolved.BaseURL)
	}
	if apiKey == "" {
		return nil, errors.New("API Key 未配置")
	}
	if baseURL == "" {
		return nil, errors.New("Base URL 未配置")
	}

	endpoint := aiModelsEndpoint(baseURL)
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, endpoint, nil)
	if err != nil {
		return nil, err
	}
	req.Header.Set("Accept", "application/json")
	req.Header.Set("Authorization", "Bearer "+apiKey)
	resp, err := NewExternalHTTPClient(20 * time.Second).Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	if resp.StatusCode >= http.StatusBadRequest {
		body, _ := io.ReadAll(io.LimitReader(resp.Body, 4096))
		return nil, fmt.Errorf("模型探测失败 (%d): %s", resp.StatusCode, strings.TrimSpace(string(body)))
	}
	var payload struct {
		Data []AIModelInfo `json:"data"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&payload); err != nil {
		return nil, fmt.Errorf("模型列表响应不是有效 JSON: %w", err)
	}
	seen := map[string]struct{}{}
	items := make([]AIModelInfo, 0, len(payload.Data))
	for _, item := range payload.Data {
		item.ID = strings.TrimSpace(item.ID)
		if item.ID == "" {
			continue
		}
		key := strings.ToLower(item.ID)
		if _, ok := seen[key]; ok {
			continue
		}
		seen[key] = struct{}{}
		items = append(items, item)
	}
	if len(items) == 0 {
		return nil, errors.New("服务未返回可用模型")
	}
	sort.SliceStable(items, func(i, j int) bool {
		return strings.ToLower(items[i].ID) < strings.ToLower(items[j].ID)
	})
	return items, nil
}

func aiModelsEndpoint(baseURL string) string {
	baseURL = strings.TrimRight(strings.TrimSpace(baseURL), "/")
	baseURL = strings.TrimSuffix(baseURL, "/chat/completions")
	return strings.TrimRight(baseURL, "/") + "/models"
}

func apiConfigModel(provider, extra string) string {
	if !isAICompatibleProvider(provider) {
		return ""
	}
	extra = strings.TrimSpace(extra)
	if strings.HasPrefix(extra, "{") {
		var opts aiProviderOptions
		if err := json.Unmarshal([]byte(extra), &opts); err == nil {
			return strings.TrimSpace(opts.Model)
		}
		return ""
	}
	// 兼容早期直接把模型名存进 extra 的配置。
	return extra
}

// setAIOptionsModel 把模型写入 AI 选项 JSON,保留其余键;旧的纯字符串 extra
// 视为模型名并被替换。
func setAIOptionsModel(extra, model string) (string, error) {
	opts := map[string]any{}
	extra = strings.TrimSpace(extra)
	if strings.HasPrefix(extra, "{") {
		if err := json.Unmarshal([]byte(extra), &opts); err != nil {
			return "", fmt.Errorf("AI options must be a JSON object: %w", err)
		}
	}
	if model == "" {
		delete(opts, "model")
	} else {
		opts["model"] = model
	}
	if len(opts) == 0 {
		return "", nil
	}
	data, err := json.Marshal(opts)
	if err != nil {
		return "", err
	}
	return string(data), nil
}

func isAICompatibleProvider(provider string) bool {
	switch strings.ToLower(strings.TrimSpace(provider)) {
	case "openai", "deepseek", "siliconflow":
		return true
	default:
		return false
	}
}
