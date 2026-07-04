package service

import (
	"context"
	"time"

	"go.uber.org/zap"

	"github.com/ShukeBta/MediaStationGo/internal/helper"
	"github.com/ShukeBta/MediaStationGo/internal/model"
)

// TestConnection tries to reach the site's base URL with the configured
// credentials and reports success/failure.
//
// 测试逻辑（与旧版参考实现对齐）：
//
//  1. 优先调用对应站点适配器的 Authenticate()，让 PT 站点（M-Team / UNIT3D /
//     Gazelle 等）使用各自的开放 API 验证，而不是去拉首页 HTML——后者通常
//     被 Cloudflare 直接 403 但 API 能正常访问。
//  2. 适配器不可用或站点类型未知时，回退到 helper.TestSiteConnectivity 的
//     通用浏览器头 GET 方案。
//  3. helper.TestSiteConnectivity 在全局 FlareSolverr 启用且站点开启了
//     BrowserEmulation 时，会自动走 FlareSolverr。
func (s *SiteService) TestConnection(ctx context.Context, id string) (bool, string, error) {
	site, err := s.FindByID(ctx, id)
	if err != nil || site == nil {
		return false, "site not found", err
	}

	flareSolverrURL := s.flareSolverrURL

	// ── Path 1: site-aware adapter Authenticate ────────────────────────
	// custom_rss 没有真适配器，跳过；其它类型先尝试针对性认证端点。
	if adapter := NewSiteAdapter(site); adapter != nil && site.Type != "" && site.Type != "custom_rss" {
		cfg := s.siteModelToConfig(site)
		actx, cancel := context.WithTimeout(ctx, cfg.Timeout)
		defer cancel()
		if authErr := adapter.Authenticate(actx, cfg); authErr == nil {
			now := time.Now()
			_ = s.repo.DB.WithContext(ctx).Model(&model.Site{}).Where("id = ?", id).
				Updates(map[string]any{
					"login_status":  "ok",
					"last_error":    "",
					"last_check_at": &now,
				}).Error
			return true, "连接成功", nil
		} else {
			if site.Type == "mteam" || site.Type == "yemapt" || isYemaPTURL(site.URL) {
				s.log.Warn("site adapter authenticate failed",
					zap.String("site", site.Name),
					zap.String("type", site.Type),
					zap.Error(authErr))
				now := time.Now()
				_ = s.repo.DB.WithContext(ctx).Model(&model.Site{}).Where("id = ?", id).
					Updates(map[string]any{
						"login_status":  "fail",
						"last_error":    authErr.Error(),
						"last_check_at": &now,
					}).Error
				return false, authErr.Error(), nil
			}
			s.log.Warn("site adapter authenticate failed, falling back to generic test",
				zap.String("site", site.Name),
				zap.String("type", site.Type),
				zap.Error(authErr))
			// 回退到通用 GET 测试 — 给 Cookie/RSS 类站点一个机会
		}
	}

	// ── Path 2: generic GET with browser headers / FlareSolverr ───────
	timeout := int(siteRequestTimeout(site.Type, site.Timeout).Seconds())
	ok, msg, err := helper.TestSiteConnectivity(site, flareSolverrURL, timeout, s.log)
	if err != nil {
		now := time.Now()
		_ = s.repo.DB.WithContext(ctx).Model(&model.Site{}).Where("id = ?", id).
			Updates(map[string]any{
				"login_status":  "fail",
				"last_error":    err.Error(),
				"last_check_at": &now,
			}).Error
		return false, err.Error(), nil
	}

	loginStatus := "ok"
	storedError := ""
	if !ok {
		loginStatus = "fail"
		storedError = msg
	}
	now := time.Now()
	_ = s.repo.DB.WithContext(ctx).Model(&model.Site{}).Where("id = ?", id).
		Updates(map[string]any{
			"login_status":  loginStatus,
			"last_error":    storedError,
			"last_check_at": &now,
		}).Error
	return ok, msg, nil
}
