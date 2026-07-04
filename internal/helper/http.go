// Package helper provides shared HTTP client utilities
// with browser-like headers and Cloudflare/WAF bypass support.
package helper

import (
	"net/http"
	"net/url"
	"time"

	"github.com/ShukeBta/MediaStationGo/internal/model"
)

// NewSiteHTTPClient builds an http.Client honoring per-site policies:
//   - timeout (seconds, defaults to 15)
//   - proxy via HTTP(S)_PROXY environment variables when site.UseProxy is on
//
// When useProxy is false, the client is created without proxy plumbing so
// the request goes out direct, matching the user's checkbox intent.
func NewSiteHTTPClient(timeoutSeconds int, useProxy bool) *http.Client {
	if timeoutSeconds <= 0 {
		timeoutSeconds = 15
	}
	tr := &http.Transport{
		MaxIdleConns:          16,
		MaxIdleConnsPerHost:   4,
		IdleConnTimeout:       60 * time.Second,
		TLSHandshakeTimeout:   15 * time.Second,
		ExpectContinueTimeout: 1 * time.Second,
	}
	if useProxy {
		tr.Proxy = func(r *http.Request) (*url.URL, error) {
			return ProxyFromEnvironmentOrSystem(r)
		}
	}
	return &http.Client{
		Timeout:   time.Duration(timeoutSeconds) * time.Second,
		Transport: tr,
	}
}

// HTTPHeaderPresets returns a map of realistic browser HTTP headers.
// These mimic a real Chrome browser to avoid WAF/bot detection.
func HTTPHeaderPresets() map[string]string {
	return map[string]string{
		"User-Agent":                model.DefaultUserAgent,
		"Accept":                    "text/html,application/xhtml+xml,application/xml;q=0.9,image/avif,image/webp,image/apng,*/*;q=0.8,application/signed-exchange;v=b3;q=0.7",
		"Accept-Language":           "zh-CN,zh;q=0.9,en;q=0.8",
		"Accept-Encoding":           "gzip, deflate, br",
		"Connection":                "keep-alive",
		"Upgrade-Insecure-Requests": "1",
		"Sec-Fetch-Dest":            "document",
		"Sec-Fetch-Mode":            "navigate",
		"Sec-Fetch-Site":            "none",
		"Sec-Fetch-User":            "?1",
		"Cache-Control":             "max-age=0",
	}
}
