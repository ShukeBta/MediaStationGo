package handler

import (
	"crypto/sha256"
	"encoding/hex"
	"io"
	"net/http"
	"net/url"
	"path"
	"sort"
	"strings"
	"time"

	"github.com/gin-gonic/gin"
	"go.uber.org/zap"

	"github.com/ShukeBta/MediaStationGo/internal/service"
	"github.com/ShukeBta/MediaStationGo/internal/service/cloud"
)

type cloudPlaybackRequest struct {
	svc          *service.Container
	c            *gin.Context
	typ          string
	ref          string
	link         *cloud.DirectLink
	resolveStart time.Time
	resolveDur   time.Duration
}

// cloudPlayHandler resolves a cloud file to its direct link and either issues a
// 302 redirect (true offload — host does not stream the bytes) or, when the
// provider requires authenticated headers, reverse-proxies the response.
func cloudPlayHandler(svc *service.Container) gin.HandlerFunc {
	return func(c *gin.Context) {
		typ := c.Param("type")
		ref := c.Query("ref")
		if !service.IsAdminCloudConfigurable(typ) {
			c.JSON(http.StatusBadRequest, gin.H{"error": "unsupported cloud provider"})
			return
		}
		if ref == "" {
			c.JSON(http.StatusBadRequest, gin.H{"error": "ref required"})
			return
		}
		if !enforceScopedCloudPlaybackToken(c, svc, typ, ref) {
			return
		}
		serveCloudResolvedLink(svc, c, typ, ref)
	}
}

func serveCloudResolvedLink(svc *service.Container, c *gin.Context, typ, ref string) {
	if isCloudImageRef(ref) && svc != nil && svc.ImageProxy != nil {
		if svc.ImageProxy.ServeCloudCached(c.Writer, c.Request, typ+":"+ref) {
			return
		}
	}
	if svc == nil || svc.StorageCfg == nil {
		c.JSON(http.StatusServiceUnavailable, gin.H{"error": "cloud storage service unavailable"})
		return
	}
	resolveStart := time.Now()
	link, err := svc.StorageCfg.CloudResolve(c.Request.Context(), typ, ref, c.Request.UserAgent())
	resolveDur := time.Since(resolveStart)
	if err != nil {
		logCloudPlayback(svc, "cloud playback resolve failed",
			append(cloudPlaybackLogFields(typ, ref, nil, resolveDur), zap.Error(err))...)
		c.JSON(http.StatusBadGateway, gin.H{"error": err.Error()})
		return
	}
	if isCloudImageRef(ref) && svc.ImageProxy != nil {
		if err := svc.ImageProxy.ServeCloudResolved(c.Request.Context(), c.Writer, c.Request, typ+":"+ref, link); err != nil {
			c.JSON(http.StatusBadGateway, gin.H{"error": err.Error()})
		}
		return
	}
	if isCloudImageRef(ref) {
		c.Header("Cache-Control", "public, max-age=2592000, immutable")
	}
	if !link.Proxy {
		// Pure offload: send the client straight to the cloud CDN.
		setRedirectNoStoreHeaders(c)
		logCloudPlayback(svc, "cloud playback redirect",
			append(cloudPlaybackLogFields(typ, ref, link, resolveDur),
				zap.String("mode", "redirect"),
				zap.Int("status", http.StatusFound),
				zap.String("method", c.Request.Method),
				zap.String("range", c.GetHeader("Range")),
			)...)
		c.Redirect(http.StatusFound, link.URL)
		return
	}
	proxyCloudResolvedLink(cloudPlaybackRequest{
		svc:          svc,
		c:            c,
		typ:          typ,
		ref:          ref,
		link:         link,
		resolveStart: resolveStart,
		resolveDur:   resolveDur,
	})
}

func proxyCloudResolvedLink(playback cloudPlaybackRequest) {
	c := playback.c
	clientMethod := playback.c.Request.Method
	if clientMethod == "" {
		clientMethod = http.MethodGet
	}
	upstreamMethod := clientMethod
	req, err := http.NewRequestWithContext(c.Request.Context(), upstreamMethod, playback.link.URL, nil)
	if err != nil {
		c.JSON(http.StatusBadGateway, gin.H{"error": err.Error()})
		return
	}
	for k, v := range playback.link.Headers {
		req.Header.Set(k, v)
	}
	if rng := c.GetHeader("Range"); rng != "" {
		req.Header.Set("Range", rng)
	}
	if accept := c.GetHeader("Accept"); accept != "" {
		req.Header.Set("Accept", accept)
	}
	if c.GetHeader("Accept-Encoding") == "" {
		req.Header.Set("Accept-Encoding", "identity")
	}
	upstreamStart := time.Now()
	resp, err := http.DefaultClient.Do(req)
	upstreamHeaderDur := time.Since(upstreamStart)
	if err != nil {
		logCloudPlayback(playback.svc, "cloud playback proxy upstream failed",
			append(cloudPlaybackLogFields(playback.typ, playback.ref, playback.link, playback.resolveDur),
				zap.String("mode", "proxy"),
				zap.String("method", clientMethod),
				zap.String("upstream_method", upstreamMethod),
				zap.String("range", c.GetHeader("Range")),
				zap.Int64("upstream_header_ms", durationMilliseconds(upstreamHeaderDur)),
				zap.Error(err),
			)...)
		c.JSON(http.StatusBadGateway, gin.H{"error": err.Error()})
		return
	}
	defer resp.Body.Close()
	for _, h := range []string{"Content-Type", "Content-Length", "Content-Range", "Accept-Ranges", "ETag", "Last-Modified"} {
		if v := resp.Header.Get(h); v != "" {
			c.Header(h, v)
		}
	}
	if c.Writer.Header().Get("Accept-Ranges") == "" {
		c.Header("Accept-Ranges", "bytes")
	}
	if resp.StatusCode >= 400 {
		handleCloudProxyError(playback, req, resp, clientMethod, upstreamMethod, upstreamHeaderDur)
		return
	}
	streamCloudProxyResponse(playback, req, resp, clientMethod, upstreamMethod, upstreamHeaderDur)
}

func handleCloudProxyError(playback cloudPlaybackRequest, req *http.Request, resp *http.Response, clientMethod, upstreamMethod string, upstreamHeaderDur time.Duration) {
	c := playback.c
	c.Header("Cache-Control", "no-store")
	body, _ := io.ReadAll(io.LimitReader(resp.Body, 4096))
	fields := append(cloudPlaybackLogFields(playback.typ, playback.ref, playback.link, playback.resolveDur),
		zap.String("mode", "proxy"),
		zap.String("method", clientMethod),
		zap.String("upstream_method", upstreamMethod),
		zap.String("range", c.GetHeader("Range")),
		zap.String("upstream_range", req.Header.Get("Range")),
		zap.Int("status", resp.StatusCode),
		zap.String("content_range", resp.Header.Get("Content-Range")),
		zap.String("content_length", resp.Header.Get("Content-Length")),
		zap.String("upstream_error_body", strings.TrimSpace(string(body))),
		zap.Int64("upstream_header_ms", durationMilliseconds(upstreamHeaderDur)),
		zap.Int64("total_ms", durationMilliseconds(time.Since(playback.resolveStart))),
	)
	logCloudPlayback(playback.svc, "cloud playback proxy upstream returned error", fields...)
	c.Status(resp.StatusCode)
	if clientMethod != http.MethodHead && len(body) > 0 {
		_, _ = c.Writer.Write(body)
	}
}

func streamCloudProxyResponse(playback cloudPlaybackRequest, req *http.Request, resp *http.Response, clientMethod, upstreamMethod string, upstreamHeaderDur time.Duration) {
	c := playback.c
	c.Status(resp.StatusCode)
	var copied int64
	var copyErr error
	streamStart := time.Now()
	if c.Request.Method != http.MethodHead {
		copied, copyErr = io.Copy(c.Writer, resp.Body)
	}
	fields := append(cloudPlaybackLogFields(playback.typ, playback.ref, playback.link, playback.resolveDur),
		zap.String("mode", "proxy"),
		zap.String("method", clientMethod),
		zap.String("upstream_method", upstreamMethod),
		zap.String("range", c.GetHeader("Range")),
		zap.String("upstream_range", req.Header.Get("Range")),
		zap.Int("status", resp.StatusCode),
		zap.String("content_range", resp.Header.Get("Content-Range")),
		zap.String("content_length", resp.Header.Get("Content-Length")),
		zap.Int64("upstream_header_ms", durationMilliseconds(upstreamHeaderDur)),
		zap.Int64("stream_ms", durationMilliseconds(time.Since(streamStart))),
		zap.Int64("total_ms", durationMilliseconds(time.Since(playback.resolveStart))),
		zap.Int64("bytes", copied),
	)
	if copyErr != nil {
		logCloudPlayback(playback.svc, "cloud playback proxy copy failed", append(fields, zap.Error(copyErr))...)
		return
	}
	logCloudPlayback(playback.svc, "cloud playback proxy finished", fields...)
}

func isCloudImageRef(ref string) bool {
	ref = strings.ToLower(strings.TrimSpace(ref))
	for _, suffix := range []string{".jpg", ".jpeg", ".png", ".webp", ".gif", ".bmp", ".tbn"} {
		if strings.HasSuffix(ref, suffix) {
			return true
		}
	}
	return false
}

func logCloudPlayback(svc *service.Container, msg string, fields ...zap.Field) {
	if svc == nil || svc.Log == nil {
		return
	}
	svc.Log.Info(msg, fields...)
}

func cloudPlaybackLogFields(typ, ref string, link *cloud.DirectLink, resolveDur time.Duration) []zap.Field {
	refHash, refExt := cloudPlaybackRefFingerprint(ref)
	fields := []zap.Field{
		zap.String("provider", strings.TrimSpace(typ)),
		zap.String("ref_hash", refHash),
		zap.String("ref_ext", refExt),
		zap.Int64("resolve_ms", durationMilliseconds(resolveDur)),
	}
	if link != nil {
		fields = append(fields,
			zap.String("target_host", cloudPlaybackLinkHost(link.URL)),
			zap.Bool("headers_required", len(link.Headers) > 0),
			zap.Strings("header_names", cloudPlaybackHeaderNames(link.Headers)),
		)
	}
	return fields
}

func cloudPlaybackRefFingerprint(ref string) (string, string) {
	ref = strings.TrimSpace(ref)
	sum := sha256.Sum256([]byte(ref))
	ext := strings.ToLower(path.Ext(strings.Trim(strings.ReplaceAll(ref, "\\", "/"), "/")))
	return hex.EncodeToString(sum[:])[:12], ext
}

func cloudPlaybackLinkHost(raw string) string {
	u, err := url.Parse(strings.TrimSpace(raw))
	if err != nil || u.Host == "" {
		return ""
	}
	return u.Host
}

func cloudPlaybackHeaderNames(headers map[string]string) []string {
	if len(headers) == 0 {
		return nil
	}
	out := make([]string, 0, len(headers))
	for key := range headers {
		if key = strings.TrimSpace(key); key != "" {
			out = append(out, key)
		}
	}
	sort.Strings(out)
	return out
}

func durationMilliseconds(d time.Duration) int64 {
	if d <= 0 {
		return 0
	}
	return d.Milliseconds()
}
