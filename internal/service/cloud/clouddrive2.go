package cloud

import (
	"bytes"
	"context"
	"encoding/base64"
	"encoding/json"
	"encoding/xml"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"path"
	"sort"
	"strconv"
	"strings"
)

// cloudDrive2Provider bridges CloudDrive2 through its WebDAV endpoint.
//
// CloudDrive2 already integrates many cloud disks (115 / 123 / Aliyun / Quark
// and more). Treating it as a WebDAV-backed cloud provider lets MediaStationGo
// browse, mount and upload to those disks without carrying every provider's
// private chunk-upload protocol in this project.
type cloudDrive2Provider struct {
	typ      string
	name     string
	base     *url.URL
	username string
	password string
	token    string
	ua       string
	apiBase  *url.URL
	client   *http.Client
	proxy    bool
}

func newCloudDrive2(cfg map[string]any, client *http.Client) *cloudDrive2Provider {
	return newCloudDAVProvider(TypeCloudDrive2, "clouddrive2", cfg, client, "/dav")
}

func newOpenList(cfg map[string]any, client *http.Client) *cloudDrive2Provider {
	return newCloudDAVProvider(TypeOpenList, "openlist", cfg, client, "/dav")
}

func newCloudDAVProvider(typ, name string, cfg map[string]any, client *http.Client, defaultDAVPath string) *cloudDrive2Provider {
	rawURL := webDAVURLFromConfig(cfg, defaultDAVPath)
	u, _ := url.Parse(strings.TrimRight(rawURL, "/"))
	var apiBase *url.URL
	if typ == TypeOpenList {
		apiBase = openListAPIBaseFromConfig(cfg, rawURL, defaultDAVPath)
	}
	ua := str(cfg["ua"])
	if ua == "" {
		ua = defaultUA
	}
	proxy := true
	return &cloudDrive2Provider{
		typ:      typ,
		name:     name,
		base:     u,
		username: str(cfg["username"]),
		password: str(cfg["password"]),
		token:    str(cfg["token"]),
		ua:       ua,
		apiBase:  apiBase,
		client:   client,
		proxy:    proxy,
	}
}

func (p *cloudDrive2Provider) Type() string { return p.typ }

func (p *cloudDrive2Provider) Ping(ctx context.Context) error {
	_, err := p.List(ctx, "")
	return err
}

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

func (p *cloudDrive2Provider) listOpenListAPI(ctx context.Context, dir string) ([]FileEntry, error) {
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
			"refresh":  false,
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
			return nil, fmt.Errorf("%s: api list %s returned http %d", p.name, target, resp.StatusCode)
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

func (p *cloudDrive2Provider) Resolve(ctx context.Context, fileRef string) (*DirectLink, error) {
	if err := p.validate(); err != nil {
		return nil, err
	}
	ref := normalizeCloudDAVPath(fileRef)
	if ref == "/" {
		return nil, fmt.Errorf("%s: file reference required", p.name)
	}
	if p.typ == TypeOpenList && isCloudVideoPlaybackCandidate(ref) {
		if p.apiBase == nil {
			return p.webDAVProxyLink(ref), nil
		}
		link, err := p.resolveOpenListAPIDirect(ctx, ref)
		if err != nil {
			return p.webDAVProxyLink(ref), nil
		}
		return link, nil
	}
	if p.typ == TypeCloudDrive2 && isCloudVideoPlaybackCandidate(ref) {
		link, err := p.resolveCloudDAVRedirectDirect(ctx, ref)
		if err != nil {
			return p.webDAVProxyLink(ref), nil
		}
		return link, nil
	}
	return p.webDAVProxyLink(ref), nil
}

func (p *cloudDrive2Provider) webDAVProxyLink(ref string) *DirectLink {
	headers := map[string]string{"User-Agent": p.ua}
	if p.token != "" {
		headers["Authorization"] = p.token
	} else if p.username != "" {
		headers["Authorization"] = "Basic " + base64.StdEncoding.EncodeToString([]byte(p.username+":"+p.password))
	}
	return &DirectLink{URL: p.urlFor(ref), Headers: headers, Proxy: p.proxy}
}

func (p *cloudDrive2Provider) resolveOpenListAPIDirect(ctx context.Context, fileRef string) (*DirectLink, error) {
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
		return nil, fmt.Errorf("%s: api get %s returned http %d", p.name, fileRef, resp.StatusCode)
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
		return nil, fmt.Errorf("%s: api get %s failed: %s", p.name, fileRef, msg)
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
		return &DirectLink{URL: resolved, Headers: headers, Proxy: true}, nil
	}
	cdnURL, err := p.resolveOpenListCDNRedirect(ctx, fileRef, resolved)
	if err != nil {
		return &DirectLink{URL: resolved, Headers: nil, Proxy: true}, nil
	}
	return &DirectLink{URL: cdnURL, Headers: nil, Proxy: false}, nil
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

func sortedHeaderNames(headers map[string]string) []string {
	if len(headers) == 0 {
		return nil
	}
	out := make([]string, 0, len(headers))
	for key := range headers {
		key = strings.TrimSpace(key)
		if key != "" {
			out = append(out, key)
		}
	}
	sort.Strings(out)
	return out
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

func sameURLHost(raw string, base *url.URL) bool {
	if base == nil {
		return false
	}
	u, err := url.Parse(strings.TrimSpace(raw))
	if err != nil {
		return false
	}
	if !u.IsAbs() {
		return true
	}
	return strings.EqualFold(u.Host, base.Host)
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

func normalizeOpenListPlaybackHeaders(raw json.RawMessage) map[string]string {
	if len(raw) == 0 || string(raw) == "null" {
		return nil
	}
	var obj map[string]any
	if err := json.Unmarshal(raw, &obj); err != nil {
		return nil
	}
	out := make(map[string]string, len(obj))
	for k, v := range obj {
		key := strings.TrimSpace(k)
		if key == "" {
			continue
		}
		switch value := v.(type) {
		case string:
			if strings.TrimSpace(value) != "" {
				out[key] = strings.TrimSpace(value)
			}
		case []any:
			parts := make([]string, 0, len(value))
			for _, item := range value {
				if s, ok := item.(string); ok && strings.TrimSpace(s) != "" {
					parts = append(parts, strings.TrimSpace(s))
				}
			}
			if len(parts) > 0 {
				out[key] = strings.Join(parts, ", ")
			}
		}
	}
	if len(out) == 0 {
		return nil
	}
	return out
}

func isCloudVideoPlaybackCandidate(fileRef string) bool {
	switch strings.ToLower(path.Ext(strings.TrimSpace(fileRef))) {
	case ".mkv", ".mp4", ".m4v", ".avi", ".mov", ".webm", ".ts", ".rmvb", ".rm", ".3gp", ".mpg", ".mpeg":
		return true
	default:
		return false
	}
}

func (p *cloudDrive2Provider) validate() error {
	if p.base == nil || p.base.Scheme == "" || p.base.Host == "" {
		return fmt.Errorf("%s: missing WebDAV URL", p.name)
	}
	return nil
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

func webDAVURLFromConfig(cfg map[string]any, defaultDAVPath string) string {
	rawURL := str(cfg["url"])
	if rawURL == "" {
		rawURL = str(cfg["webdav_url"])
	}
	if rawURL != "" {
		return ensureDefaultDAVPath(rawURL, defaultDAVPath)
	}
	return defaultWebDAVURL(str(cfg["server"]), defaultDAVPath)
}

func defaultWebDAVURL(server, defaultDAVPath string) string {
	server = strings.TrimRight(strings.TrimSpace(server), "/")
	if server == "" {
		return ""
	}
	davPath := strings.TrimSpace(defaultDAVPath)
	if davPath == "" {
		return server
	}
	if !strings.HasPrefix(davPath, "/") {
		davPath = "/" + davPath
	}
	return server + davPath
}

func openListAPIBaseFromConfig(cfg map[string]any, webDAVURL, defaultDAVPath string) *url.URL {
	raw := str(cfg["server"])
	if raw == "" {
		raw = firstNonEmpty(str(cfg["api_url"]), webDAVURL)
	}
	raw = strings.TrimRight(strings.TrimSpace(raw), "/")
	if raw == "" {
		return nil
	}
	u, err := url.Parse(raw)
	if err != nil || u.Scheme == "" || u.Host == "" {
		return nil
	}
	davPath := strings.Trim(strings.TrimSpace(defaultDAVPath), "/")
	if davPath != "" {
		pathParts := strings.Split(strings.TrimRight(u.Path, "/"), "/")
		if len(pathParts) > 0 && strings.EqualFold(pathParts[len(pathParts)-1], davPath) {
			u.Path = strings.Join(pathParts[:len(pathParts)-1], "/")
			if u.Path == "" {
				u.Path = "/"
			}
		}
	}
	u.RawPath = ""
	u.RawQuery = ""
	u.Fragment = ""
	return u
}

func (p *cloudDrive2Provider) openListAPIURL(apiPath string) string {
	if p.apiBase == nil {
		return ""
	}
	u := *p.apiBase
	u.RawPath = ""
	basePath := strings.TrimRight(u.Path, "/")
	apiPath = "/" + strings.TrimLeft(apiPath, "/")
	if basePath == "" || basePath == "/" {
		u.Path = apiPath
	} else {
		u.Path = basePath + apiPath
	}
	return u.String()
}

func ensureDefaultDAVPath(rawURL, defaultDAVPath string) string {
	rawURL = strings.TrimRight(strings.TrimSpace(rawURL), "/")
	if rawURL == "" {
		return ""
	}
	u, err := url.Parse(rawURL)
	if err != nil || u.Scheme == "" || u.Host == "" {
		return rawURL
	}
	if strings.TrimSpace(defaultDAVPath) == "" {
		return rawURL
	}
	if u.Path == "" || u.Path == "/" {
		davPath := strings.TrimSpace(defaultDAVPath)
		if !strings.HasPrefix(davPath, "/") {
			davPath = "/" + davPath
		}
		u.Path = davPath
		u.RawPath = ""
		return strings.TrimRight(u.String(), "/")
	}
	return rawURL
}

func (p *cloudDrive2Provider) decorateDAVStatusError(resp *http.Response, target string) error {
	body, _ := io.ReadAll(io.LimitReader(resp.Body, 4096))
	detail := compactDAVErrorBody(string(body))
	if detail == "" {
		return fmt.Errorf("%s: list %s returned http %d", p.name, target, resp.StatusCode)
	}
	if resp.StatusCode == http.StatusMethodNotAllowed {
		return fmt.Errorf("%s: list %s returned http %d：%s；请确认填写的是 WebDAV 地址（通常以 /dav 结尾），并且桥接网盘已在 OpenList/CloudDrive2 内完成登录或 Cookie 保存", p.name, target, resp.StatusCode, detail)
	}
	if resp.StatusCode == http.StatusUnauthorized || resp.StatusCode == http.StatusForbidden {
		return fmt.Errorf("%s: list %s returned http %d：%s；请检查 WebDAV 用户名/密码、Authorization Token，或先在 OpenList/CloudDrive2 中保存对应网盘 Cookie", p.name, target, resp.StatusCode, detail)
	}
	return fmt.Errorf("%s: list %s returned http %d：%s", p.name, target, resp.StatusCode, detail)
}

func compactDAVErrorBody(raw string) string {
	raw = strings.TrimSpace(strings.ReplaceAll(raw, "\x00", ""))
	if raw == "" {
		return ""
	}
	raw = strings.Join(strings.Fields(raw), " ")
	if len([]rune(raw)) > 180 {
		return string([]rune(raw)[:180]) + "…"
	}
	return raw
}

func decorateDAVTransportError(name, target string, err error) error {
	if err == nil {
		return nil
	}
	message := err.Error()
	if strings.Contains(message, "server gave HTTP response to HTTPS client") {
		return fmt.Errorf("%s: %w；当前地址使用 https://，但服务端返回 HTTP。请改用 http:// 地址，例如 OpenList 默认 WebDAV 通常是 http://host:5244/dav/；如果必须使用 https，请在 OpenList 前配置反向代理和证书", name, err)
	}
	if strings.Contains(message, "first record does not look like a TLS handshake") {
		return fmt.Errorf("%s: %w；疑似把 HTTP 服务配置成了 https://，请检查 %s 的协议头", name, err, target)
	}
	return err
}

func (p *cloudDrive2Provider) urlFor(remotePath string) string {
	u := *p.base
	u.RawPath = ""
	basePath := strings.TrimRight(u.Path, "/")
	remote := strings.Trim(normalizeCloudDAVPath(remotePath), "/")
	switch {
	case basePath == "" || basePath == "/":
		if remote == "" {
			u.Path = "/"
		} else {
			u.Path = "/" + remote
		}
	case remote == "":
		u.Path = basePath
	default:
		u.Path = basePath + "/" + remote
	}
	return u.String()
}

func (p *cloudDrive2Provider) entryIDFromHref(href, basePath string) (string, error) {
	if href == "" {
		return "", nil
	}
	parsed, err := url.Parse(href)
	if err != nil {
		return "", err
	}
	hrefPath := parsed.EscapedPath()
	if hrefPath == "" {
		hrefPath = href
	}
	if basePath != "" && basePath != "/" {
		hrefPath = strings.TrimPrefix(hrefPath, basePath)
	}
	if decoded, err := url.PathUnescape(hrefPath); err == nil {
		hrefPath = decoded
	}
	return normalizeCloudDAVPath(hrefPath), nil
}

const cloudDAVPropfindBody = `<?xml version="1.0" encoding="utf-8"?>
<d:propfind xmlns:d="DAV:">
  <d:prop>
    <d:displayname/>
    <d:getcontentlength/>
    <d:resourcetype/>
  </d:prop>
</d:propfind>`

type cloudDAVMultiStatus struct {
	Responses []cloudDAVResponse `xml:"response"`
}

type cloudDAVResponse struct {
	Href     string           `xml:"href"`
	PropStat cloudDAVPropStat `xml:"propstat"`
}

type cloudDAVPropStat struct {
	Prop cloudDAVProp `xml:"prop"`
}

type cloudDAVProp struct {
	DisplayName   string               `xml:"displayname"`
	ContentLength string               `xml:"getcontentlength"`
	ResourceType  cloudDAVResourceType `xml:"resourcetype"`
}

type cloudDAVResourceType struct {
	Collection *struct{} `xml:"collection"`
}

type openListListResponse struct {
	Code    int    `json:"code"`
	Message string `json:"message"`
	Data    struct {
		Content []openListListItem `json:"content"`
		Total   int                `json:"total"`
	} `json:"data"`
}

type openListListItem struct {
	Name  string `json:"name"`
	Size  int64  `json:"size"`
	IsDir bool   `json:"is_dir"`
}

type openListGetResponse struct {
	Code    int    `json:"code"`
	Message string `json:"message"`
	Data    struct {
		RawURL string          `json:"raw_url"`
		URL    string          `json:"url"`
		Header json.RawMessage `json:"header"`
	} `json:"data"`
}

type openListLoginResponse struct {
	Code    int    `json:"code"`
	Message string `json:"message"`
	Data    struct {
		Token string `json:"token"`
	} `json:"data"`
}

func normalizeCloudDAVPath(p string) string {
	p = strings.ReplaceAll(strings.TrimSpace(p), "\\", "/")
	if p == "" || p == "." {
		return "/"
	}
	if !strings.HasPrefix(p, "/") {
		p = "/" + p
	}
	cleaned := path.Clean(p)
	if cleaned == "." {
		return "/"
	}
	return cleaned
}

func sameCloudDAVPath(a, b string) bool {
	return strings.TrimRight(normalizeCloudDAVPath(a), "/") == strings.TrimRight(normalizeCloudDAVPath(b), "/")
}

func joinOpenListAPIPath(dir, name string) string {
	dir = strings.TrimRight(normalizeCloudDAVPath(dir), "/")
	name = strings.Trim(strings.ReplaceAll(name, "\\", "/"), "/")
	if dir == "" || dir == "/" {
		return normalizeCloudDAVPath(name)
	}
	return normalizeCloudDAVPath(dir + "/" + name)
}

func parseDAVSize(raw string) int64 {
	n, _ := strconv.ParseInt(strings.TrimSpace(raw), 10, 64)
	return n
}

func firstNonEmpty(values ...string) string {
	for _, v := range values {
		if strings.TrimSpace(v) != "" {
			return strings.TrimSpace(v)
		}
	}
	return ""
}
