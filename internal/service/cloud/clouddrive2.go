package cloud

import (
	"context"
	"encoding/base64"
	"fmt"
	"net/http"
	"net/url"
	"path"
	"strings"
)

// cloudDrive2Provider bridges CloudDrive2 through its WebDAV endpoint.
//
// CloudDrive2 integrates many cloud disks (115 / 123 / Aliyun and more).
// Treating it as a WebDAV-backed cloud provider lets MediaStationGo
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
			return nil, fmt.Errorf("%s: pure 302 playback requires an OpenList API server address; configure server/api_url so /api/fs/get can return raw_url", p.name)
		}
		link, err := p.resolveOpenListAPIDirect(ctx, ref)
		if err != nil {
			return nil, fmt.Errorf("%s: pure 302 playback requires OpenList raw_url for %s: %w", p.name, ref, err)
		}
		return link, nil
	}
	if p.typ == TypeCloudDrive2 && isCloudVideoPlaybackCandidate(ref) {
		link, err := p.resolveCloudDAVRedirectDirect(ctx, ref)
		if err != nil {
			return nil, fmt.Errorf("%s: pure 302 playback requires CloudDrive2/WebDAV to return a CDN Location for %s: %w", p.name, ref, err)
		}
		return link, nil
	}
	headers := map[string]string{
		"User-Agent": p.ua,
	}
	if p.token != "" {
		headers["Authorization"] = p.token
	} else if p.username != "" {
		headers["Authorization"] = "Basic " + base64.StdEncoding.EncodeToString([]byte(p.username+":"+p.password))
	}
	return &DirectLink{URL: p.urlFor(ref), Headers: headers, Proxy: p.proxy}, nil
}

func (p *cloudDrive2Provider) validate() error {
	if p.base == nil || p.base.Scheme == "" || p.base.Host == "" {
		return fmt.Errorf("%s: missing WebDAV URL", p.name)
	}
	return nil
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

func firstNonEmpty(values ...string) string {
	for _, v := range values {
		if strings.TrimSpace(v) != "" {
			return strings.TrimSpace(v)
		}
	}
	return ""
}
