package helper

import (
	"fmt"
	"io"
	"net/http"

	"github.com/ShukeBta/MediaStationGo/internal/model"
	"go.uber.org/zap"
)

// TestSiteConnectivity performs a site connectivity test with browser-like headers.
// If flareSolverrURL is non-empty AND the site has BrowserEmulation turned on,
// it will attempt to use FlareSolverr first.
// Returns (ok, message, error).
func TestSiteConnectivity(site *model.Site, flareSolverrURL string, timeout int, log *zap.Logger) (bool, string, error) {
	// Try FlareSolverr first when (a) globally enabled and (b) the site
	// asked for browser emulation. This matches the contract used by the
	// search path (see service.SiteService.siteModelToConfig).
	useFlare := flareSolverrURL != "" && site.BrowserEmulation
	if useFlare {
		log.Info("Trying FlareSolverr for site test", zap.String("url", site.URL))
		body, err := FetchURLWithFlareSolverr(flareSolverrURL, site.URL, site.Cookie, timeout, "", log)
		if err == nil {
			// Successfully got page via FlareSolverr
			if IsCloudflareChallenge(body) {
				return false, "站点被 Cloudflare/WAF 拦截，但 FlareSolverr 未能完全解决", nil
			}
			return true, "连接成功 (via FlareSolverr)", nil
		}
		log.Warn("FlareSolverr failed, falling back to direct request", zap.Error(err))
		// Fall through to direct request
	}

	// Direct HTTP request with browser-like headers. Honors HTTP(S)_PROXY
	// when the site has UseProxy enabled — this makes the "use proxy"
	// checkbox in the UI actually do something.
	client := NewSiteHTTPClient(timeout, site.UseProxy)
	client.CheckRedirect = func(req *http.Request, via []*http.Request) error {
		if len(via) >= 10 {
			return fmt.Errorf("too many redirects")
		}
		return nil
	}

	req, err := http.NewRequest("GET", site.URL, nil)
	if err != nil {
		return false, err.Error(), nil
	}

	headers := HTTPHeaderPresets()
	for k, v := range headers {
		req.Header.Set(k, v)
	}

	ApplySiteAuthHeaders(req, site)

	resp, err := client.Do(req)
	if err != nil {
		return false, err.Error(), nil
	}
	defer resp.Body.Close()

	body, _ := io.ReadAll(resp.Body)
	bodyStr := string(body)

	if IsCloudflareChallenge(bodyStr) {
		log.Warn("Cloudflare challenge detected", zap.String("url", site.URL))
		return false, "站点被 Cloudflare/WAF 拦截，请配置 FlareSolverr 或浏览器模拟", nil
	}

	// Evaluate status code (mirror the reference Python project's semantics:
	// 200 → success, 3xx → success/redirect-to-self, 401/403 → failure with
	// hint to check credentials, 4xx/5xx → failure with raw status text).
	switch {
	case resp.StatusCode >= 200 && resp.StatusCode < 300:
		return true, fmt.Sprintf("连接成功 (%s)", resp.Status), nil
	case resp.StatusCode == 301 || resp.StatusCode == 302 || resp.StatusCode == 307 || resp.StatusCode == 308:
		loc := resp.Header.Get("Location")
		if loc == "" {
			loc = "(unknown)"
		}
		// Most PT sites redirect logged-out users to login; treat as failure.
		return false, fmt.Sprintf("未登录或 Cookie 失效（重定向至 %s）", loc), nil
	case resp.StatusCode == 401:
		return false, "未授权（HTTP 401），请检查 API Key / Cookie", nil
	case resp.StatusCode == 403:
		return false, "认证失败（HTTP 403），请检查 Cookie / API Key 或站点是否需要浏览器模拟", nil
	case resp.StatusCode == 429:
		return false, "请求被限流（HTTP 429），请稍后再试", nil
	case resp.StatusCode == 503:
		return false, "服务暂时不可用（HTTP 503）", nil
	default:
		return false, resp.Status, nil
	}
}
