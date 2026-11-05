package service

import (
	"context"
	"net/http"
	"net/url"
	"strings"
)

type embyPlaybackRequestContextKey struct{}

func WithEmbyPlaybackRequest(ctx context.Context, request *http.Request) context.Context {
	return context.WithValue(ctx, embyPlaybackRequestContextKey{}, request)
}

func embyPlaybackRequestFromContext(ctx context.Context) *http.Request {
	request, _ := ctx.Value(embyPlaybackRequestContextKey{}).(*http.Request)
	return request
}

// normalizeCloudPlayTarget 把存库的云盘播放 URL 规范化为相对路径。
//
// STRMURL 是扫描时根据当时的 server_url/请求地址生成并固化进数据库的。
// 在 Windows 开发机上扫描、再部署到 Docker（或更换了内网 IP/域名）后，
// 这些绝对 URL 会指向已失效的旧地址，第三方播放器跟随 302 就会拿到
// 连接失败/404。这里只要能从 URL 中解析出 provider+ref，就重建为相对
// /api/cloud/play 路径，由 absoluteInternalRedirect 基于「当前请求」补全
// host，从而对历史脏数据免疫。
func normalizeCloudPlayTarget(raw string) string {
	typ, ref, ok := parseCloudMediaPlaybackURL(raw)
	if !ok {
		return raw
	}
	return BuildRelativeCloudPlayURL(typ, ref)
}

// BuildRelativeCloudPlayURL 构造相对的云盘播放 API 路径。
func BuildRelativeCloudPlayURL(typ, ref string) string {
	return "/api/cloud/play/" + url.PathEscape(strings.TrimSpace(typ)) + "?" + url.Values{"ref": []string{ref}}.Encode()
}

// withAuthToken propagates the caller's auth token onto an internal redirect
// target. A browser <video> element cannot send Authorization headers or
// cookies when it follows a 302, so the cloud 302 chain
// (/api/stream?token=… → /api/cloud/play → CDN) would otherwise hit
// /api/cloud/play unauthenticated and 401. We only attach the token to our
// own relative API endpoints — never to an absolute external direct link —
// so the JWT is never leaked off-site (e.g. to the cloud CDN).
func withAuthToken(target string, r *http.Request) string {
	return withAuthTokenForInternalRedirect(target, r, "")
}

func withAuthTokenForInternalRedirect(target string, r *http.Request, publicBase string) string {
	if r == nil {
		return target
	}
	if strings.HasPrefix(target, "//") {
		return target
	}
	u, err := url.Parse(target)
	if err != nil {
		return target
	}
	if u.IsAbs() && !isInternalAPIURL(u, r, publicBase) {
		return target
	}
	if !strings.HasPrefix(strings.ToLower(u.Path), "/api/") {
		return target
	}
	tok := requestToken(r)
	if tok == "" {
		return target
	}
	q := u.Query()
	if q.Get("token") == "" {
		q.Set("token", tok)
	}
	if q.Get("media_id") == "" && strings.HasPrefix(strings.ToLower(u.Path), "/api/cloud/play/") {
		if mediaID := playbackMediaIDFromRequestPath(r.URL.Path); mediaID != "" {
			q.Set("media_id", mediaID)
		}
	}
	u.RawQuery = q.Encode()
	return u.String()
}

func playbackMediaIDFromRequestPath(pathValue string) string {
	pathValue = strings.TrimSpace(pathValue)
	if pathValue == "" {
		return ""
	}
	segments := strings.Split(strings.Trim(pathValue, "/"), "/")
	lower := make([]string, len(segments))
	for i, segment := range segments {
		lower[i] = strings.ToLower(segment)
	}
	var mediaID string
	switch {
	case len(segments) >= 3 && lower[0] == "api" && lower[1] == "stream":
		mediaID = segments[2]
	case len(segments) >= 4 && lower[0] == "emby" && lower[1] == "api" && lower[2] == "stream":
		mediaID = segments[3]
	case len(segments) >= 3 && lower[0] == "videos":
		mediaID = segments[1]
	}
	if decoded, err := url.PathUnescape(mediaID); err == nil {
		mediaID = decoded
	}
	return strings.TrimSpace(mediaID)
}

func internalSTRMStreamMediaID(raw string, r *http.Request, publicBase string) (string, bool) {
	raw = strings.TrimSpace(raw)
	if raw == "" {
		return "", false
	}
	u, err := url.Parse(raw)
	if err != nil {
		return "", false
	}
	lowerPath := strings.ToLower(u.Path)
	if !strings.HasPrefix(lowerPath, "/api/stream/") && !strings.HasPrefix(lowerPath, "/emby/api/stream/") {
		return "", false
	}
	if u.IsAbs() {
		if !isInternalAPIURL(u, r, publicBase) && !isPrivatePlaybackHost(u.Hostname()) {
			return "", false
		}
	} else if u.Host != "" || !strings.HasPrefix(u.Path, "/") {
		return "", false
	}
	id := playbackMediaIDFromRequestPath(u.Path)
	return id, id != ""
}

func isPrivatePlaybackHost(host string) bool {
	host = strings.TrimSuffix(strings.ToLower(strings.TrimSpace(host)), ".")
	if host == "" || isPrivateHost(host) || !strings.Contains(host, ".") {
		return true
	}
	for _, suffix := range []string{".local", ".lan", ".internal", ".home", ".home.arpa"} {
		if strings.HasSuffix(host, suffix) {
			return true
		}
	}
	return false
}

func absoluteInternalRedirect(target string, r *http.Request) string {
	if r == nil || target == "" || strings.HasPrefix(target, "//") {
		return target
	}
	u, err := url.Parse(target)
	if err != nil || u.IsAbs() || !strings.HasPrefix(target, "/") {
		return target
	}
	scheme := strings.TrimSpace(r.Header.Get("X-Forwarded-Proto"))
	if scheme == "" {
		if r.TLS != nil {
			scheme = "https"
		} else {
			scheme = "http"
		}
	}
	host := strings.TrimSpace(r.Header.Get("X-Forwarded-Host"))
	if host == "" {
		host = r.Host
	}
	if host == "" {
		return target
	}
	u.Scheme = scheme
	u.Host = host
	return u.String()
}

func isInternalAPIURL(u *url.URL, r *http.Request, publicBase string) bool {
	if u == nil || !strings.HasPrefix(strings.ToLower(u.Path), "/api/") {
		return false
	}
	targetHost := strings.ToLower(strings.TrimSpace(u.Host))
	if targetHost == "" {
		return true
	}
	if r != nil {
		if host := strings.ToLower(strings.TrimSpace(r.Host)); host != "" && targetHost == host {
			return true
		}
		if host := strings.ToLower(strings.TrimSpace(r.Header.Get("X-Forwarded-Host"))); host != "" && targetHost == host {
			return true
		}
	}
	if publicBase != "" {
		if base, err := url.Parse(publicBase); err == nil && strings.EqualFold(strings.TrimSpace(base.Host), targetHost) {
			return true
		}
	}
	return false
}

// requestToken extracts the bearer JWT from the incoming request the same way
// the auth middleware does (Authorization header, Emby token headers, or the
// token / api_key query params used by <video>.src).
func requestToken(r *http.Request) string {
	if h := r.Header.Get("Authorization"); strings.HasPrefix(h, "Bearer ") {
		return strings.TrimSpace(strings.TrimPrefix(h, "Bearer "))
	}
	for _, hk := range []string{"X-Emby-Token", "X-MediaBrowser-Token"} {
		if v := strings.TrimSpace(r.Header.Get(hk)); v != "" {
			return v
		}
	}
	for _, hk := range []string{"X-Emby-Authorization", "X-MediaBrowser-Authorization"} {
		if token := streamTokenFromAuthHeader(r.Header.Get(hk)); token != "" {
			return token
		}
	}
	if token := streamTokenFromAuthHeader(r.Header.Get("Authorization")); token != "" {
		return token
	}
	for _, k := range []string{"token", "api_key", "apiKey", "ApiKey"} {
		if v := strings.TrimSpace(r.URL.Query().Get(k)); v != "" {
			return v
		}
	}
	return ""
}

func streamTokenFromAuthHeader(value string) string {
	value = strings.TrimSpace(value)
	if value == "" {
		return ""
	}
	for _, prefix := range []string{"Bearer ", "Emby "} {
		if strings.HasPrefix(value, prefix) {
			return strings.TrimSpace(strings.TrimPrefix(value, prefix))
		}
	}
	if strings.HasPrefix(value, "MediaBrowser ") || strings.Contains(value, "Token=") {
		for _, part := range strings.Split(value, ",") {
			part = strings.TrimSpace(strings.TrimPrefix(strings.TrimSpace(part), "MediaBrowser "))
			if !strings.HasPrefix(part, "Token=") {
				continue
			}
			token := strings.TrimSpace(strings.TrimPrefix(part, "Token="))
			return strings.Trim(token, `"`)
		}
		return ""
	}
	return value
}
