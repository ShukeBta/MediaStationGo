package service

import (
	"context"
	"encoding/json"
	"errors"
	"strings"
	"time"

	"gorm.io/gorm"

	"github.com/ShukeBta/MediaStationGo/internal/model"
)

// Create persists a new site.
func (s *SiteService) Create(ctx context.Context, site *model.Site) error {
	if strings.TrimSpace(site.Name) == "" || strings.TrimSpace(site.URL) == "" {
		return errors.New("name and url required")
	}
	site.URL = strings.TrimRight(site.URL, "/")
	if site.Type == "" {
		site.Type = "nexusphp"
	}
	if site.AuthType == "" {
		site.AuthType = "cookie"
	}
	return s.repo.DB.WithContext(ctx).Create(site).Error
}

// List returns every site ordered by created_at.
func (s *SiteService) List(ctx context.Context) ([]model.Site, error) {
	var sites []model.Site
	err := s.repo.DB.WithContext(ctx).Order("created_at asc").Find(&sites).Error
	if sites == nil {
		sites = []model.Site{}
	}
	return sites, err
}

// FindByID returns a single site or nil.
func (s *SiteService) FindByID(ctx context.Context, id string) (*model.Site, error) {
	var site model.Site
	err := s.repo.DB.WithContext(ctx).Where("id = ?", id).First(&site).Error
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return nil, nil
	}
	return &site, err
}

// siteUpdatableFields is the whitelist of columns that may be patched via
// the update endpoint. Fields like id, created_at, deleted_at, login_status,
// upload_bytes, download_bytes are excluded to prevent injection.
var siteUpdatableFields = map[string]bool{
	"name":              true,
	"url":               true,
	"type":              true,
	"auth_type":         true,
	"api_key":           true,
	"cookie":            true,
	"auth_header":       true,
	"user_agent":        true,
	"rss_url":           true,
	"timeout":           true,
	"priority":          true,
	"use_proxy":         true,
	"rate_limit":        true,
	"browser_emulation": true,
	"downloader":        true,
	"enabled":           true,
	"is_default":        true,
	"extra":             true,
}

// Update applies a partial patch to an existing site.
func (s *SiteService) Update(ctx context.Context, id string, updates map[string]any) error {
	if id == "" {
		return errors.New("site id required")
	}
	filtered := make(map[string]any, len(updates))
	for k, v := range updates {
		if siteUpdatableFields[k] {
			filtered[k] = v
		}
	}
	if len(filtered) == 0 {
		return errors.New("no valid fields to update")
	}
	if raw, ok := filtered["url"].(string); ok {
		filtered["url"] = strings.TrimRight(strings.TrimSpace(raw), "/")
	}
	for _, key := range []string{"api_key", "cookie", "auth_header"} {
		if raw, ok := filtered[key].(string); ok && strings.TrimSpace(raw) == "" {
			delete(filtered, key)
		}
	}
	return s.repo.DB.WithContext(ctx).Model(&model.Site{}).Where("id = ?", id).Updates(filtered).Error
}

// Delete removes a site.
func (s *SiteService) Delete(ctx context.Context, id string) error {
	return s.repo.DB.WithContext(ctx).Where("id = ?", id).Delete(&model.Site{}).Error
}

// siteModelToConfig 将 model.Site 转换为适配器使用的 SiteConfig。
// 当全局 FlareSolverr 已启用且此站点开启了 BrowserEmulation 时，填充 FlareSolverrURL。
func (svc *SiteService) siteModelToConfig(s *model.Site) SiteConfig {
	timeout := siteRequestTimeout(s.Type, s.Timeout)
	userAgent := s.UserAgent
	if userAgent == "" {
		userAgent = model.DefaultUserAgent
	}
	var extra map[string]string
	if s.Extra != "" {
		_ = json.Unmarshal([]byte(s.Extra), &extra)
	}

	// Per-site FlareSolverr opt-in: only when global FlareSolverr is enabled
	// AND this site has BrowserEmulation turned on.
	flareSolverrURL := ""
	if svc.flareSolverrURL != "" && s.BrowserEmulation {
		flareSolverrURL = svc.flareSolverrURL
	}

	return SiteConfig{
		SiteID:          s.ID,
		Name:            s.Name,
		Type:            s.Type,
		URL:             s.URL,
		AuthType:        s.AuthType,
		Cookie:          s.Cookie,
		APIKey:          s.APIKey,
		AuthHeader:      s.AuthHeader,
		UserAgent:       userAgent,
		Timeout:         timeout,
		Extra:           extra,
		FlareSolverrURL: flareSolverrURL,
		UseProxy:        s.UseProxy,
		RateLimit:       s.RateLimit,
		rateLimiter:     svc.apiRateLimiter,
	}
}

func siteRequestTimeout(siteType string, timeoutSeconds int) time.Duration {
	timeout := time.Duration(timeoutSeconds) * time.Second
	if timeout <= 0 {
		timeout = 15 * time.Second
	}
	if isAPISiteType(siteType) && timeout <= 15*time.Second {
		return 45 * time.Second
	}
	return timeout
}

func isAPISiteType(siteType string) bool {
	switch strings.ToLower(strings.TrimSpace(siteType)) {
	case "mteam", "yemapt":
		return true
	default:
		return false
	}
}
