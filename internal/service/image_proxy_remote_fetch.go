package service

import (
	"context"
	"errors"
	"io"
	"net"
	"net/http"
	"net/url"
	"strings"
	"time"

	"go.uber.org/zap"
)

type remoteImageFetchClient struct {
	name   string
	client *http.Client
}

func (p *ImageProxy) remoteImageFetchClients() []remoteImageFetchClient {
	client := p.client
	if client == nil {
		client = NewExternalHTTPClient(30 * time.Second)
	}
	clients := []remoteImageFetchClient{{name: "default", client: p.securedImageClient(client)}}
	if _, ok := client.Transport.(*http.Transport); ok {
		timeout := client.Timeout
		if timeout <= 0 {
			timeout = 30 * time.Second
		}
		clients = append(clients, remoteImageFetchClient{
			name:   "direct",
			client: p.securedImageClient(&http.Client{Timeout: timeout, Transport: NewInternalTransport()}),
		})
	}
	return clients
}

func (p *ImageProxy) fetchRemoteImageOnce(ctx context.Context, raw, host string, candidate remoteImageFetchClient) ([]byte, string, string, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, raw, nil)
	if err != nil {
		p.log.Warn("imageproxy: build request failed", zap.String("url", raw), zap.Error(err))
		return nil, "", "", errImageProxyRequestSetup
	}
	applyRemoteImageHeaders(req, host)

	resp, err := candidate.client.Do(req)
	if err != nil {
		p.log.Warn("imageproxy: upstream fetch failed", zap.String("host", host), zap.String("client", candidate.name), zap.Error(err))
		return nil, "", "", err
	}
	defer resp.Body.Close()
	if resp.StatusCode >= 400 {
		p.log.Warn("imageproxy: upstream returned non-OK", zap.String("host", host), zap.String("client", candidate.name), zap.String("status", resp.Status))
		return nil, "", "", errors.New("upstream returned " + resp.Status)
	}
	data, err := io.ReadAll(io.LimitReader(resp.Body, 32<<20))
	if err != nil || len(data) == 0 {
		p.log.Warn("imageproxy: read upstream body failed", zap.String("host", host), zap.String("client", candidate.name), zap.Error(err))
		if err == nil {
			err = errors.New("upstream image body is empty")
		}
		return nil, "", "", err
	}
	ctype, ok := validRemoteImageContentType(host, data)
	if !ok {
		p.log.Warn("imageproxy: upstream returned unusable image content", zap.String("host", host), zap.String("client", candidate.name), zap.String("content_type", resp.Header.Get("Content-Type")))
		return nil, "", "", errImageProxyNonImageContent
	}
	return data, ctype, resp.Header.Get("Content-Length"), nil
}

func applyRemoteImageHeaders(req *http.Request, host string) {
	req.Header.Set("User-Agent", "Mozilla/5.0 (Windows NT 10.0; Win64; x64) AppleWebKit/537.36 (KHTML, like Gecko) Chrome/125.0 Safari/537.36")
	req.Header.Set("Accept", "image/avif,image/webp,image/apng,image/svg+xml,image/*,*/*;q=0.8")
	req.Header.Set("Accept-Language", "zh-CN,zh;q=0.9,en;q=0.8")
	req.Header.Set("Cache-Control", "no-cache")
	req.Header.Set("Pragma", "no-cache")
	if referer := remoteImageReferer(host); referer != "" {
		req.Header.Set("Referer", referer)
	}
}

func remoteImageReferer(host string) string {
	host = strings.ToLower(strings.TrimSpace(host))
	switch {
	case strings.Contains(host, "doubanio.com"):
		return "https://movie.douban.com/"
	case strings.Contains(host, "bgm.tv"):
		return "https://bgm.tv/"
	case strings.Contains(host, "javbus.com"):
		return "https://www.javbus.com/"
	case host == "xximgs.cc" || strings.HasSuffix(host, ".xximgs.cc"):
		return "https://fd2ppv.cc/"
	}
	return ""
}

func isDoubanImageHost(host string) bool {
	host = strings.TrimSuffix(strings.ToLower(strings.TrimSpace(host)), ".")
	if name, _, err := net.SplitHostPort(host); err == nil {
		host = name
	}
	return host == "doubanio.com" || strings.HasSuffix(host, ".doubanio.com")
}

func (p *ImageProxy) useDoubanImageDirect(ctx context.Context, host string) bool {
	if p == nil || p.apiConfig == nil {
		return false
	}
	resolved, err := p.apiConfig.Resolve(ctx, "douban")
	if err != nil || !resolved.Enabled || !resolved.ImageDirect {
		return false
	}
	if isDoubanImageHost(host) {
		return true
	}
	origin, err := url.Parse(resolved.BaseURL)
	return err == nil && origin.Host != "" && strings.EqualFold(origin.Host, host)
}
func (p *ImageProxy) imageClientsForHost(ctx context.Context, host string) []remoteImageFetchClient {
	clients := p.remoteImageFetchClients()
	if !p.useDoubanImageDirect(ctx, host) {
		return clients
	}
	for _, client := range clients {
		if client.name == "direct" {
			return []remoteImageFetchClient{client}
		}
	}
	// Custom transports in tests/integrations remain injectable.
	return clients
}
