package service

import (
	"net/http"
	"net/url"
	"strings"
	"time"
)

// ProxyFromEnvironmentOrSystem mirrors http.ProxyFromEnvironment, then falls
// back to the OS user proxy. On Windows this means the current user's Internet
// Settings proxy, e.g. the proxy configured by Clash/V2RayN/系统设置.
func ProxyFromEnvironmentOrSystem(req *http.Request) (*url.URL, error) {
	if proxy, err := http.ProxyFromEnvironment(req); proxy != nil || err != nil {
		return proxy, err
	}
	return systemProxyForRequest(req)
}

// NewExternalHTTPClient builds an HTTP client for third-party APIs. It uses
// environment proxies first, then the local OS proxy configuration.
func NewExternalHTTPClient(timeout time.Duration) *http.Client {
	return &http.Client{
		Timeout:   timeout,
		Transport: NewExternalTransport(),
	}
}

// NewInternalHTTPClient builds an HTTP client for LAN / Docker-internal
// services such as qBittorrent, Transmission and Aria2. These endpoints are
// usually 127.0.0.1, host.docker.internal, 172.17.0.1 or a NAS LAN IP; sending
// them through HTTP_PROXY/SOCKS proxies makes local WebUI logins hang or fail.
func NewInternalHTTPClient(timeout time.Duration) *http.Client {
	return &http.Client{
		Timeout:   timeout,
		Transport: NewInternalTransport(),
	}
}

func NewExternalTransport() *http.Transport {
	return &http.Transport{
		Proxy:                 ProxyFromEnvironmentOrSystem,
		ForceAttemptHTTP2:     true,
		MaxIdleConns:          32,
		MaxIdleConnsPerHost:   8,
		IdleConnTimeout:       90 * time.Second,
		TLSHandshakeTimeout:   15 * time.Second,
		ExpectContinueTimeout: 1 * time.Second,
	}
}

func NewInternalTransport() *http.Transport {
	transport := http.DefaultTransport.(*http.Transport).Clone()
	transport.Proxy = nil
	return transport
}

func proxyURLFromProxyServer(proxyServer, requestScheme string) (*url.URL, error) {
	proxyServer = strings.TrimSpace(proxyServer)
	if proxyServer == "" {
		return nil, nil
	}

	if !strings.Contains(proxyServer, "=") {
		return normalizeProxyURL(proxyServer, "http")
	}

	entries := strings.Split(proxyServer, ";")
	values := map[string]string{}
	first := ""
	for _, entry := range entries {
		key, value, ok := strings.Cut(strings.TrimSpace(entry), "=")
		if !ok {
			continue
		}
		key = strings.ToLower(strings.TrimSpace(key))
		value = strings.TrimSpace(value)
		if key == "" || value == "" {
			continue
		}
		if first == "" {
			first = value
		}
		values[key] = value
	}

	if value := values[strings.ToLower(requestScheme)]; value != "" {
		return normalizeProxyURL(value, strings.ToLower(requestScheme))
	}
	if value := values["http"]; value != "" {
		return normalizeProxyURL(value, "http")
	}
	if value := values["https"]; value != "" {
		return normalizeProxyURL(value, "http")
	}
	if value := values["socks"]; value != "" {
		return normalizeProxyURL(value, "socks")
	}
	return normalizeProxyURL(first, "http")
}

func normalizeProxyURL(raw, proxyKind string) (*url.URL, error) {
	raw = strings.TrimSpace(raw)
	if raw == "" {
		return nil, nil
	}
	if !strings.Contains(raw, "://") {
		scheme := "http"
		if strings.EqualFold(proxyKind, "socks") {
			scheme = "socks5"
		}
		raw = scheme + "://" + raw
	}
	return url.Parse(raw)
}
