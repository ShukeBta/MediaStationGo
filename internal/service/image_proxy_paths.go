package service

import (
	"errors"
	"net"
	"net/url"
	"path/filepath"
	"strings"
)

// validateURL parses raw and ensures the scheme is http/https and the
// target host is not a private/loopback/link-local address (SSRF guard).
func (p *ImageProxy) validateURL(raw string) (*url.URL, error) {
	if raw == "" {
		return nil, errors.New("missing url")
	}
	u, err := url.Parse(raw)
	if err != nil || u.Host == "" {
		return nil, errors.New("invalid url")
	}
	scheme := strings.ToLower(u.Scheme)
	if scheme != "http" && scheme != "https" {
		return nil, errors.New("unsupported scheme")
	}
	if isPrivateHost(u.Hostname()) {
		return nil, errors.New("requests to private/internal hosts are not allowed")
	}
	return u, nil
}

// isPrivateHost returns true only when host is a literal loopback, private,
// link-local or unspecified IP address. Hostnames are not resolved here because
// DNS poisoning can map public image CDNs to bogus private addresses.
func isPrivateHost(host string) bool {
	if host == "" {
		return true
	}
	ip := net.ParseIP(host)
	if ip != nil {
		return ip.IsLoopback() || ip.IsPrivate() || ip.IsLinkLocalUnicast() || ip.IsLinkLocalMulticast() || ip.IsUnspecified()
	}
	return false
}

// isAllowedLocalPath restricts local file reads to known-safe roots.
func (p *ImageProxy) isAllowedLocalPath(abs string) bool {
	roots := []string{p.cfg.App.DataDir, p.cfg.Cache.CacheDir, p.cfg.Media.MoviesDir, p.cfg.Media.TVDir, p.cfg.Media.AnimeDir}
	roots = append(roots, p.libraryRoots()...)
	for _, root := range roots {
		if strings.TrimSpace(root) == "" {
			continue
		}
		rootAbs, err := filepath.Abs(root)
		if err != nil {
			continue
		}
		if strings.HasPrefix(abs, rootAbs+string(filepath.Separator)) || abs == rootAbs {
			return true
		}
	}
	return false
}

func isLocalImagePath(raw string) bool {
	raw = strings.TrimSpace(raw)
	if raw == "" || isHTTPish(raw) {
		return false
	}
	ext := strings.ToLower(filepath.Ext(raw))
	switch ext {
	case ".jpg", ".jpeg", ".png", ".webp", ".gif", ".bmp", ".tbn":
		return true
	default:
		return false
	}
}

func isHTTPish(raw string) bool {
	return strings.HasPrefix(strings.ToLower(raw), "http://") || strings.HasPrefix(strings.ToLower(raw), "https://")
}
