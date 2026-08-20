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
	"errors"
	"strings"
	"time"

	"go.uber.org/zap"
	"gorm.io/gorm"

	"github.com/ShukeBta/MediaStationGo/internal/model"
	"github.com/ShukeBta/MediaStationGo/internal/repository"
)

// APIConfigService coordinates third-party API key storage.
type APIConfigService struct {
	log    *zap.Logger
	repo   *repository.Container
	crypto *CryptoService
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
			{Provider: "metatube", BaseURL: "http://127.0.0.1:7700", Description: "MetaTube Server 番号刮削后端", Enabled: true},
			{Provider: "openai", BaseURL: "https://api.openai.com/v1", Description: "OpenAI-compatible (smart search)", Enabled: true},
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
	ID          string    `json:"id"`
	Provider    string    `json:"provider"`
	BaseURL     string    `json:"base_url,omitempty"`
	Extra       string    `json:"extra,omitempty"`
	Enabled     bool      `json:"enabled"`
	Description string    `json:"description,omitempty"`
	HasKey      bool      `json:"has_key"`
	MaskedKey   string    `json:"masked_key,omitempty"`
	CreatedAt   time.Time `json:"created_at"`
	UpdatedAt   time.Time `json:"updated_at"`
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
	APIKey  string
	BaseURL string
	Extra   string
	Enabled bool
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
		APIKey:  s.crypto.Decrypt(row.APIKey),
		BaseURL: row.BaseURL,
		Extra:   row.Extra,
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
	APIKey      *string `json:"api_key,omitempty"`
	BaseURL     *string `json:"base_url,omitempty"`
	Extra       *string `json:"extra,omitempty"`
	Enabled     *bool   `json:"enabled,omitempty"`
	Description *string `json:"description,omitempty"`
}

// Update applies the patch and returns the new public view.
func (s *APIConfigService) Update(ctx context.Context, provider string, patch APIConfigPatch) (*PublicView, error) {
	provider = strings.TrimSpace(strings.ToLower(provider))
	if provider == "" {
		return nil, errors.New("provider required")
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
	if patch.APIKey != nil {
		v := strings.TrimSpace(*patch.APIKey)
		if v == "" || v == "<clear>" {
			updates["api_key"] = ""
		} else {
			updates["api_key"] = s.crypto.Encrypt(v)
		}
	}
	if patch.BaseURL != nil {
		updates["base_url"] = *patch.BaseURL
	}
	if patch.Extra != nil {
		updates["extra"] = *patch.Extra
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
	return s.repo.DB.WithContext(ctx).
		Model(&model.APIConfig{}).
		Where("id = ?", row.ID).
		Update("api_key", "").Error
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
		ID:          r.ID,
		Provider:    r.Provider,
		BaseURL:     r.BaseURL,
		Extra:       r.Extra,
		Enabled:     r.Enabled,
		Description: r.Description,
		HasKey:      plain != "",
		CreatedAt:   r.CreatedAt,
		UpdatedAt:   r.UpdatedAt,
	}
	if pv.HasKey {
		pv.MaskedKey = MaskAPIKey(plain)
	}
	return pv
}
