package service

import (
	"context"
	"errors"
	"fmt"
	"io"
	"mime"
	"net/http"
	"net/url"
	"path"
	"strings"
	"time"

	"go.uber.org/zap"

	"github.com/ShukeBta/MediaStationGo/internal/model"
)

// ResolveDownloadURL converts tracker-specific search result URLs into a URL
// that a downloader can fetch directly. M-Team, NexusPHP and similar sites
// often expose a signed/detail endpoint in search results; qBittorrent cannot
// call those APIs with the configured site credentials, so subscriptions need
// the same resolution path as the manual download button.
func (s *SiteService) ResolveDownloadURL(ctx context.Context, raw string) string {
	if strings.TrimSpace(raw) == "" {
		return raw
	}
	matched := s.matchSiteForURL(ctx, raw)
	if matched == nil {
		return raw
	}

	u, err := url.Parse(raw)
	if err != nil || u.Host == "" {
		return raw
	}
	id := u.Query().Get("id")
	if id == "" {
		return raw
	}
	adapter := GetAdapterForType(matched.Type)
	if adapter == nil {
		return raw
	}
	cfg := s.siteModelToConfig(matched)
	timeout := cfg.Timeout
	if timeout <= 0 {
		timeout = 15 * time.Second
	}
	resolveCtx, cancel := context.WithTimeout(ctx, timeout)
	defer cancel()
	resolved, err := adapter.GetDownloadURL(resolveCtx, cfg, id)
	if err != nil || resolved == "" {
		if s.log != nil {
			s.log.Warn("resolve PT download URL failed",
				zap.String("site", matched.Name),
				zap.String("raw", redactSensitiveDownloadURL(raw)),
				zap.Error(err))
		}
		return raw
	}
	return resolved
}

func redactSensitiveDownloadURL(raw string) string {
	raw = strings.TrimSpace(raw)
	if raw == "" {
		return ""
	}
	if strings.HasPrefix(strings.ToLower(raw), "magnet:") {
		return "magnet:?xt=***"
	}
	u, err := url.Parse(raw)
	if err != nil || u.Host == "" {
		return "[redacted-download-url]"
	}
	u.RawQuery = ""
	u.Fragment = ""
	return u.String()
}

func (s *SiteService) FetchTorrentFile(ctx context.Context, raw string) ([]byte, string, error) {
	matched := s.matchSiteForURL(ctx, raw)
	if matched == nil {
		return nil, "", errors.New("no matching PT site for torrent URL")
	}
	cfg := s.siteModelToConfig(matched)
	timeout := cfg.Timeout
	if timeout <= 0 {
		timeout = 30 * time.Second
	}
	req, err := buildRequest(ctx, http.MethodGet, raw, cfg, nil)
	if err != nil {
		return nil, "", err
	}
	req.Header.Set("Accept", "application/x-bittorrent,application/octet-stream,*/*")
	client := newHTTPClient(cfg, timeout)
	resp, err := client.Do(req)
	if err != nil {
		return nil, "", err
	}
	defer resp.Body.Close()
	if resp.StatusCode >= 400 {
		return nil, "", fmt.Errorf("torrent fetch: HTTP %d", resp.StatusCode)
	}
	const maxTorrentSize = 32 << 20
	data, err := io.ReadAll(io.LimitReader(resp.Body, maxTorrentSize+1))
	if err != nil {
		return nil, "", err
	}
	if len(data) == 0 {
		return nil, "", errors.New("torrent fetch: empty body")
	}
	if len(data) > maxTorrentSize {
		return nil, "", errors.New("torrent fetch: body too large")
	}
	if strings.Contains(strings.ToLower(resp.Header.Get("Content-Type")), "text/html") {
		return nil, "", errors.New("torrent fetch: upstream returned HTML")
	}
	if torrentInfoHash(data) == "" {
		return nil, "", errors.New("torrent fetch: upstream did not return a valid torrent")
	}
	return data, torrentFilename(raw, resp.Header.Get("Content-Disposition")), nil
}

func (s *SiteService) matchSiteForURL(ctx context.Context, raw string) *model.Site {
	u, err := url.Parse(raw)
	if err != nil || u.Host == "" {
		return nil
	}
	host := strings.ToLower(u.Host)

	sites, err := s.List(ctx)
	if err != nil || len(sites) == 0 {
		return nil
	}
	for i := range sites {
		if siteHostMatches(host, sites[i].URL) || siteHostMatches(host, sites[i].RSSURL) {
			return &sites[i]
		}
	}
	return nil
}

func siteHostMatches(host, raw string) bool {
	if raw == "" {
		return false
	}
	u, err := url.Parse(raw)
	if err != nil || u.Host == "" {
		return false
	}
	siteHost := strings.ToLower(u.Host)
	return strings.EqualFold(siteHost, host) || strings.HasSuffix(host, "."+siteHost)
}

func torrentFilename(rawURL, disposition string) string {
	if disposition != "" {
		if _, params, err := mime.ParseMediaType(disposition); err == nil {
			if filename := strings.TrimSpace(params["filename"]); filename != "" {
				return filename
			}
		}
	}
	if u, err := url.Parse(rawURL); err == nil {
		if name := strings.TrimSpace(path.Base(u.Path)); name != "" && name != "." && name != "/" {
			if !strings.HasSuffix(strings.ToLower(name), ".torrent") {
				name += ".torrent"
			}
			return name
		}
	}
	return "download.torrent"
}
