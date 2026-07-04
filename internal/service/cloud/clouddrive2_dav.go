package cloud

import (
	"context"
	"encoding/base64"
	"encoding/xml"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"path"
	"strings"
)

func (p *cloudDrive2Provider) List(ctx context.Context, dir string) ([]FileEntry, error) {
	if err := p.validate(); err != nil {
		return nil, err
	}
	if p.typ == TypeOpenList && p.apiBase != nil && p.hasOpenListAPICredentials() {
		return p.listOpenListAPI(ctx, dir)
	}
	target := normalizeCloudDAVPath(dir)
	req, err := http.NewRequestWithContext(ctx, "PROPFIND", p.urlFor(target), strings.NewReader(cloudDAVPropfindBody))
	if err != nil {
		return nil, err
	}
	p.auth(req)
	req.Header.Set("Depth", "1")
	req.Header.Set("Content-Type", "application/xml; charset=utf-8")
	req.Header.Set("Accept", "application/xml,text/xml,*/*")
	resp, err := p.client.Do(req)
	if err != nil {
		return nil, decorateDAVTransportError(p.name, p.urlFor(target), err)
	}
	defer resp.Body.Close()
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return nil, p.decorateDAVStatusError(resp, target)
	}
	body, _ := io.ReadAll(io.LimitReader(resp.Body, 4<<20))
	var multi cloudDAVMultiStatus
	if err := xml.Unmarshal(body, &multi); err != nil {
		return nil, fmt.Errorf("%s: decode webdav: %w", p.name, err)
	}
	basePath := strings.TrimRight(p.base.EscapedPath(), "/")
	currentID := normalizeCloudDAVPath(target)
	out := make([]FileEntry, 0, len(multi.Responses))
	for _, item := range multi.Responses {
		entryPath, err := p.entryIDFromHref(item.Href, basePath)
		if err != nil || entryPath == "" || sameCloudDAVPath(entryPath, currentID) {
			continue
		}
		name := firstNonEmpty(item.PropStat.Prop.DisplayName, path.Base(strings.TrimRight(entryPath, "/")))
		if decoded, err := url.PathUnescape(name); err == nil {
			name = decoded
		}
		if name == "" || name == "." || name == "/" {
			continue
		}
		out = append(out, FileEntry{
			ID:    entryPath,
			Name:  name,
			IsDir: item.PropStat.Prop.ResourceType.Collection != nil || strings.HasSuffix(item.Href, "/"),
			Size:  parseDAVSize(item.PropStat.Prop.ContentLength),
		})
	}
	return out, nil
}

func (p *cloudDrive2Provider) resolveCloudDAVRedirectDirect(ctx context.Context, fileRef string) (*DirectLink, error) {
	target := p.urlFor(fileRef)
	headers := map[string]string{
		"User-Agent": p.ua,
	}
	if p.token != "" {
		headers["Authorization"] = p.token
	} else if p.username != "" {
		headers["Authorization"] = "Basic " + base64.StdEncoding.EncodeToString([]byte(p.username+":"+p.password))
	}
	location, status, err := p.firstHTTPRedirectLocation(ctx, target, headers)
	if err != nil {
		return nil, decorateDAVTransportError(p.name, target, err)
	}
	if location == "" {
		return nil, fmt.Errorf("%s: WebDAV %s returned http %d without CDN Location; refusing WebDAV/proxy fallback for pure 302 playback", p.name, fileRef, status)
	}
	return &DirectLink{URL: location, Headers: nil, Proxy: false}, nil
}

func (p *cloudDrive2Provider) firstHTTPRedirectLocation(ctx context.Context, target string, headers map[string]string) (string, int, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, target, nil)
	if err != nil {
		return "", 0, err
	}
	req.Header.Set("Accept", "*/*")
	req.Header.Set("Accept-Encoding", "identity")
	req.Header.Set("Range", "bytes=0-0")
	if strings.TrimSpace(p.ua) != "" {
		req.Header.Set("User-Agent", p.ua)
	}
	for key, value := range headers {
		key = strings.TrimSpace(key)
		if key != "" && strings.TrimSpace(value) != "" {
			req.Header.Set(key, value)
		}
	}
	client := p.client
	if client == nil {
		client = http.DefaultClient
	}
	noFollow := *client
	noFollow.CheckRedirect = func(*http.Request, []*http.Request) error {
		return http.ErrUseLastResponse
	}
	resp, err := noFollow.Do(req)
	if err != nil {
		return "", 0, err
	}
	defer resp.Body.Close()
	status := resp.StatusCode
	if status >= 300 && status < 400 {
		rawLocation := strings.TrimSpace(resp.Header.Get("Location"))
		if rawLocation == "" {
			return "", status, fmt.Errorf("%s: upstream returned redirect http %d without Location", p.name, status)
		}
		location, err := resolveHTTPRedirectLocation(target, rawLocation)
		if err != nil {
			return "", status, err
		}
		return location, status, nil
	}
	return "", status, nil
}

func resolveHTTPRedirectLocation(baseURL, rawLocation string) (string, error) {
	rawLocation = strings.TrimSpace(rawLocation)
	if rawLocation == "" {
		return "", fmt.Errorf("empty redirect Location")
	}
	if strings.HasPrefix(rawLocation, "//") {
		base, err := url.Parse(baseURL)
		if err != nil || base.Scheme == "" {
			return "", fmt.Errorf("protocol-relative redirect Location without base scheme")
		}
		rawLocation = base.Scheme + ":" + rawLocation
	}
	location, err := url.Parse(rawLocation)
	if err != nil {
		return "", fmt.Errorf("invalid redirect Location: %w", err)
	}
	if location.IsAbs() {
		if location.Scheme != "http" && location.Scheme != "https" {
			return "", fmt.Errorf("unsupported redirect Location scheme %q", location.Scheme)
		}
		return location.String(), nil
	}
	base, err := url.Parse(baseURL)
	if err != nil {
		return "", fmt.Errorf("invalid redirect base URL: %w", err)
	}
	return base.ResolveReference(location).String(), nil
}

func (p *cloudDrive2Provider) auth(req *http.Request) {
	req.Header.Set("User-Agent", p.ua)
	if p.token != "" {
		req.Header.Set("Authorization", p.token)
		return
	}
	if p.username != "" {
		req.SetBasicAuth(p.username, p.password)
	}
}
