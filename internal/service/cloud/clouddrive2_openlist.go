package cloud

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"path"
	"strings"
	"sync"
	"time"
)

const (
	openListParentWarmupReuseWindow = time.Second
	openListParentWarmupMaxDuration = 10 * time.Second
)

type openListAPIResponseError struct {
	provider string
	action   string
	target   string
	code     int
	message  string
}

func (e *openListAPIResponseError) Error() string {
	return fmt.Sprintf("%s: api %s %s failed: %s", e.provider, e.action, e.target, e.message)
}

type openListParentWarmupFlight struct {
	done chan struct{}
	err  error
}

var openListParentWarmups = struct {
	sync.Mutex
	flights map[string]*openListParentWarmupFlight
}{flights: make(map[string]*openListParentWarmupFlight)}

func (p *cloudDrive2Provider) listOpenListAPI(ctx context.Context, dir string) ([]FileEntry, error) {
	return p.listOpenListAPIWithRefresh(ctx, dir, false)
}

func (p *cloudDrive2Provider) ListRefresh(ctx context.Context, dir string) ([]FileEntry, error) {
	if err := p.validate(); err != nil {
		return nil, err
	}
	if p.typ != TypeOpenList {
		return p.List(ctx, dir)
	}
	if p.apiBase == nil || !p.hasOpenListAPICredentials() {
		return nil, fmt.Errorf("%s: api credentials are required to refresh OpenList directory %s", p.name, normalizeCloudDAVPath(dir))
	}
	return p.listOpenListAPIWithRefresh(ctx, dir, true)
}

func (p *cloudDrive2Provider) listOpenListAPIWithRefresh(ctx context.Context, dir string, refresh bool) ([]FileEntry, error) {
	token, err := p.openListAPIToken(ctx)
	if err != nil {
		return nil, err
	}
	const pageSize = 500
	target := normalizeCloudDAVPath(dir)
	out := make([]FileEntry, 0, pageSize)
	for pageNum := 1; ; pageNum++ {
		payload := map[string]any{
			"path":     target,
			"password": "",
			"page":     pageNum,
			"per_page": pageSize,
			"refresh":  refresh && pageNum == 1,
		}
		body, _ := json.Marshal(payload)
		req, err := http.NewRequestWithContext(ctx, http.MethodPost, p.openListAPIURL("/api/fs/list"), bytes.NewReader(body))
		if err != nil {
			return nil, err
		}
		req.Header.Set("Content-Type", "application/json")
		req.Header.Set("Accept", "application/json")
		req.Header.Set("User-Agent", p.ua)
		if token != "" {
			req.Header.Set("Authorization", token)
		}
		resp, err := p.client.Do(req)
		if err != nil {
			return nil, decorateDAVTransportError(p.name, p.openListAPIURL("/api/fs/list"), err)
		}
		var decoded openListListResponse
		decodeErr := json.NewDecoder(io.LimitReader(resp.Body, 32<<20)).Decode(&decoded)
		resp.Body.Close()
		if resp.StatusCode < 200 || resp.StatusCode >= 300 {
			return nil, p.openListAPIStatusError("list", target, resp.StatusCode)
		}
		if decodeErr != nil {
			return nil, fmt.Errorf("%s: decode api list: %w", p.name, decodeErr)
		}
		if decoded.Code != 0 && decoded.Code != 200 {
			msg := strings.TrimSpace(decoded.Message)
			if msg == "" {
				msg = fmt.Sprintf("code %d", decoded.Code)
			}
			return nil, fmt.Errorf("%s: api list %s failed: %s", p.name, target, msg)
		}
		for _, item := range decoded.Data.Content {
			name := strings.TrimSpace(item.Name)
			if name == "" || name == "." || name == "/" {
				continue
			}
			out = append(out, FileEntry{
				ID:    joinOpenListAPIPath(target, name),
				Name:  name,
				IsDir: item.IsDir,
				Size:  item.Size,
			})
		}
		total := decoded.Data.Total
		if total > 0 {
			if len(out) >= total || len(decoded.Data.Content) == 0 {
				break
			}
			continue
		}
		if len(decoded.Data.Content) == 0 || len(decoded.Data.Content) < pageSize {
			break
		}
	}
	return out, nil
}

func (p *cloudDrive2Provider) resolveOpenListAPIDirect(ctx context.Context, fileRef string) (*DirectLink, error) {
	target := normalizeCloudDAVPath(fileRef)
	parent := path.Dir(target)
	link, err := p.resolveOpenListAPIDirectOnce(ctx, target)
	if err == nil || !isOpenListObjectCacheMiss(err) {
		return link, err
	}

	if parent == "." || parent == "/" {
		return nil, err
	}
	warmCtx, cancel := context.WithTimeout(ctx, openListParentWarmupMaxDuration)
	warmErr := p.warmOpenListParent(warmCtx, parent)
	cancel()
	if warmErr != nil {
		return nil, fmt.Errorf("%s: list parent %s after api get cache miss failed: %w (initial get: %v)", p.name, parent, warmErr, err)
	}
	link, retryErr := p.resolveOpenListAPIDirectOnce(ctx, target)
	if retryErr != nil {
		return nil, fmt.Errorf("%s: api get %s still failed after listing parent %s: %w", p.name, target, parent, retryErr)
	}
	return link, nil
}

func (p *cloudDrive2Provider) resolveOpenListAPIDirectOnce(ctx context.Context, fileRef string) (*DirectLink, error) {
	token, err := p.openListAPIToken(ctx)
	if err != nil {
		return nil, err
	}
	payload, _ := json.Marshal(map[string]string{"path": normalizeCloudDAVPath(fileRef), "password": ""})
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, p.openListAPIURL("/api/fs/get"), bytes.NewReader(payload))
	if err != nil {
		return nil, err
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Accept", "application/json")
	req.Header.Set("User-Agent", p.ua)
	if token != "" {
		req.Header.Set("Authorization", token)
	}
	resp, err := p.client.Do(req)
	if err != nil {
		return nil, decorateDAVTransportError(p.name, p.openListAPIURL("/api/fs/get"), err)
	}
	defer resp.Body.Close()
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return nil, p.openListAPIStatusError("get", fileRef, resp.StatusCode)
	}
	var decoded openListGetResponse
	if err := json.NewDecoder(io.LimitReader(resp.Body, 4<<20)).Decode(&decoded); err != nil {
		return nil, fmt.Errorf("%s: decode api get: %w", p.name, err)
	}
	if decoded.Code != 0 && decoded.Code != 200 {
		msg := strings.TrimSpace(decoded.Message)
		if msg == "" {
			msg = fmt.Sprintf("code %d", decoded.Code)
		}
		return nil, &openListAPIResponseError{
			provider: p.name,
			action:   "get",
			target:   fileRef,
			code:     decoded.Code,
			message:  msg,
		}
	}
	raw := firstNonEmpty(decoded.Data.RawURL, decoded.Data.URL)
	if raw == "" {
		return nil, fmt.Errorf("%s: api get %s returned empty raw_url", p.name, fileRef)
	}
	resolved, err := p.resolveOpenListPlaybackURL(raw)
	if err != nil {
		return nil, err
	}
	headers := normalizeOpenListPlaybackHeaders(decoded.Data.Header)
	if len(headers) > 0 {
		return nil, fmt.Errorf("%s: api get %s returned raw_url that requires headers (%s); refusing WebDAV/proxy fallback for pure 302 playback", p.name, fileRef, strings.Join(sortedHeaderNames(headers), ","))
	}
	resolved, err = p.resolveOpenListCDNRedirect(ctx, fileRef, resolved)
	if err != nil {
		return nil, err
	}
	return &DirectLink{URL: resolved, Headers: nil, Proxy: false}, nil
}

func isOpenListObjectCacheMiss(err error) bool {
	var responseErr *openListAPIResponseError
	return errors.As(err, &responseErr) && responseErr.code == 500 && strings.Contains(responseErr.message, "430004")
}

func (p *cloudDrive2Provider) warmOpenListParent(ctx context.Context, parent string) error {
	key := p.openListAPIURL("") + "\x00" + parent
	openListParentWarmups.Lock()
	if flight, ok := openListParentWarmups.flights[key]; ok {
		done := flight.done
		openListParentWarmups.Unlock()
		select {
		case <-done:
			return flight.err
		case <-ctx.Done():
			return ctx.Err()
		}
	}
	flight := &openListParentWarmupFlight{done: make(chan struct{})}
	openListParentWarmups.flights[key] = flight
	openListParentWarmups.Unlock()

	_, flight.err = p.listOpenListAPI(ctx, parent)
	close(flight.done)
	time.AfterFunc(openListParentWarmupReuseWindow, func() {
		openListParentWarmups.Lock()
		if openListParentWarmups.flights[key] == flight {
			delete(openListParentWarmups.flights, key)
		}
		openListParentWarmups.Unlock()
	})
	return flight.err
}

func (p *cloudDrive2Provider) resolveOpenListCDNRedirect(ctx context.Context, fileRef, rawURL string) (string, error) {
	if p.apiBase == nil || !sameURLHost(rawURL, p.apiBase) {
		return rawURL, nil
	}
	location, status, err := p.firstHTTPRedirectLocation(ctx, rawURL, nil)
	if err != nil {
		return "", fmt.Errorf("%s: probe raw_url %s failed: %w", p.name, fileRef, err)
	}
	if location != "" {
		return location, nil
	}
	return "", fmt.Errorf("%s: api get %s returned an OpenList-hosted raw_url with http %d and no CDN Location; refusing OpenList/WebDAV proxy fallback for pure 302 playback", p.name, fileRef, status)
}

func (p *cloudDrive2Provider) openListAPIStatusError(action, target string, status int) error {
	if status == http.StatusUnauthorized || status == http.StatusForbidden {
		return fmt.Errorf("%s: api %s %s returned http %d；请检查 OpenList Token 或用户名密码，并确认填写的是 OpenList 服务地址而不是 /dav 地址", p.name, action, target, status)
	}
	return fmt.Errorf("%s: api %s %s returned http %d", p.name, action, target, status)
}

func (p *cloudDrive2Provider) hasOpenListAPICredentials() bool {
	return strings.TrimSpace(p.token) != "" || (strings.TrimSpace(p.username) != "" && p.password != "")
}

func (p *cloudDrive2Provider) openListAPIToken(ctx context.Context) (string, error) {
	if token := strings.TrimSpace(p.token); token != "" {
		return token, nil
	}
	if strings.TrimSpace(p.username) == "" || p.password == "" {
		return "", nil
	}
	payload, _ := json.Marshal(map[string]string{
		"username": p.username,
		"password": p.password,
	})
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, p.openListAPIURL("/api/auth/login"), bytes.NewReader(payload))
	if err != nil {
		return "", err
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Accept", "application/json")
	req.Header.Set("User-Agent", p.ua)
	resp, err := p.client.Do(req)
	if err != nil {
		return "", decorateDAVTransportError(p.name, p.openListAPIURL("/api/auth/login"), err)
	}
	defer resp.Body.Close()
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return "", fmt.Errorf("%s: api login returned http %d", p.name, resp.StatusCode)
	}
	var decoded openListLoginResponse
	if err := json.NewDecoder(io.LimitReader(resp.Body, 4<<20)).Decode(&decoded); err != nil {
		return "", fmt.Errorf("%s: decode api login: %w", p.name, err)
	}
	if decoded.Code != 0 && decoded.Code != 200 {
		msg := strings.TrimSpace(decoded.Message)
		if msg == "" {
			msg = fmt.Sprintf("code %d", decoded.Code)
		}
		return "", fmt.Errorf("%s: api login failed: %s", p.name, msg)
	}
	token := strings.TrimSpace(decoded.Data.Token)
	if token == "" {
		return "", fmt.Errorf("%s: api login returned empty token", p.name)
	}
	p.token = token
	return token, nil
}

func (p *cloudDrive2Provider) resolveOpenListPlaybackURL(raw string) (string, error) {
	raw = strings.TrimSpace(raw)
	if raw == "" {
		return "", fmt.Errorf("%s: empty playback URL", p.name)
	}
	if strings.HasPrefix(raw, "//") {
		if p.apiBase == nil || p.apiBase.Scheme == "" {
			return "", fmt.Errorf("%s: protocol-relative playback URL without API base", p.name)
		}
		raw = p.apiBase.Scheme + ":" + raw
	}
	u, err := url.Parse(raw)
	if err != nil {
		return "", fmt.Errorf("%s: invalid playback URL: %w", p.name, err)
	}
	if u.IsAbs() {
		if u.Scheme != "http" && u.Scheme != "https" {
			return "", fmt.Errorf("%s: unsupported playback URL scheme %q", p.name, u.Scheme)
		}
		return u.String(), nil
	}
	if p.apiBase == nil {
		return "", fmt.Errorf("%s: relative playback URL without API base", p.name)
	}
	base := *p.apiBase
	base.RawPath = ""
	base.RawQuery = ""
	base.Fragment = ""
	return base.ResolveReference(u).String(), nil
}
