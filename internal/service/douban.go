// Package service — Douban (豆瓣) metadata provider.
//
// Douban is the dominant Chinese movie/TV rating and metadata site. Its
// unofficial API returns rich Chinese-language titles, overviews, ratings
// and poster URLs. A valid Douban cookie is required to avoid IP bans.
//
// We use the search endpoint at:
//
//	https://movie.douban.com/j/subject_suggest?q=...
//
// And the detail endpoint at:
//
//	https://m.douban.com/rexxar/api/v2/movie/...
//
// The provider is used as a supplemental source: after TMDb matches we
// attempt a Douban lookup to grab a localized Chinese title + overview.
package service

import (
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

	"go.uber.org/zap"

	"github.com/ShukeBta/MediaStationGo/internal/config"
)

const (
	doubanRequestTimeout = 15 * time.Second
	doubanDirectRoute    = -1
)

// DoubanProvider talks to the unofficial Douban movie API.
type DoubanProvider struct {
	cfg          *config.Config
	apiConfig    *APIConfigService
	proxyPool    *ProxyPoolService
	client       *http.Client
	directClient *http.Client
	resinClient  *http.Client

	routeMu             sync.Mutex
	reselectMu          sync.Mutex
	routeInitialized    bool
	route               int
	poolGeneration      uint64
	proxyConfigRevision uint64
	configRevision      uint64
}

// NewDoubanProvider is the constructor.
func NewDoubanProvider(cfg *config.Config, _ *zap.Logger) *DoubanProvider {
	return &DoubanProvider{
		cfg:          cfg,
		client:       NewExternalHTTPClient(doubanRequestTimeout),
		directClient: &http.Client{Timeout: doubanRequestTimeout, Transport: NewInternalTransport()},
		resinClient:  NewExternalHTTPClient(doubanRequestTimeout),
		route:        doubanDirectRoute,
	}
}

func (d *DoubanProvider) setProxyPool(proxyPool *ProxyPoolService) {
	d.proxyPool = proxyPool
}

// Enabled reports whether Douban lookup is available. Public movie.douban.com
// suggest endpoints work without an API key; a cookie is optional and only
// helps when Douban applies stricter anti-scraping rules.
func (d *DoubanProvider) Enabled() bool {
	return true
}

// userAgents for anti-scraping randomization.
var userAgents = []string{
	"Mozilla/5.0 (Windows NT 10.0; Win64; x64) AppleWebKit/537.36 (KHTML, like Gecko) Chrome/120.0.0.0 Safari/537.36",
	"Mozilla/5.0 (Macintosh; Intel Mac OS X 10_15_7) AppleWebKit/537.36 (KHTML, like Gecko) Chrome/120.0.0.0 Safari/537.36",
	"Mozilla/5.0 (Windows NT 10.0; Win64; x64; rv:121.0) Gecko/20100101 Firefox/121.0",
}

// ErrDoubanTemporarilyUnavailable 表示移动详情接口应稍后重试。
var ErrDoubanTemporarilyUnavailable = errors.New("douban temporarily unavailable")

// ErrDoubanSubjectNotFound 表示豆瓣已明确确认条目不存在。
var ErrDoubanSubjectNotFound = errors.New("douban subject not found")

// DoubanMatch is the result of a Douban search hit.
type DoubanMatch struct {
	DoubanID string  `json:"douban_id"`
	Title    string  `json:"title"`
	Year     string  `json:"year"`
	Img      string  `json:"img"`
	Rating   float32 `json:"rating"`
	Type     string  `json:"type,omitempty"`
}

// Search runs a Douban subject_suggest query and returns the top match.
func (d *DoubanProvider) Search(ctx context.Context, query string) (*DoubanMatch, error) {
	results, err := d.SearchCandidates(ctx, query)
	if err != nil || len(results) == 0 {
		return nil, err
	}
	return results[0], nil
}

// SearchCandidates 返回豆瓣联想搜索的全部候选，供需要人工选择的流程使用。
func (d *DoubanProvider) SearchCandidates(ctx context.Context, query string) ([]*DoubanMatch, error) {
	if !d.Enabled() || query == "" {
		return nil, nil
	}
	u := "https://movie.douban.com/j/subject_suggest?q=" + url.QueryEscape(query)
	rawJSON, status, err := d.requestJSON(ctx, u, "https://movie.douban.com/")
	if status >= 400 {
		return nil, fmt.Errorf("douban search: %d", status)
	}
	if err != nil {
		return nil, err
	}

	type suggestion struct {
		ID    string `json:"id"`
		Title string `json:"title"`
		Year  string `json:"year"`
		Img   string `json:"img"`
		Type  string `json:"type"`
	}
	var results []suggestion
	if err := json.Unmarshal(rawJSON, &results); err != nil {
		return nil, err
	}
	matches := make([]*DoubanMatch, 0, len(results))
	for _, result := range results {
		matches = append(matches, &DoubanMatch{
			DoubanID: result.ID,
			Title:    result.Title,
			Year:     result.Year,
			Img:      d.ResolveArtworkURL(ctx, result.Img),
			Type:     result.Type,
		})
	}
	return matches, nil
}

func (d *DoubanProvider) SearchMatch(ctx context.Context, query string) (*Match, error) {
	got, err := d.Search(ctx, query)
	if err != nil || got == nil {
		return nil, err
	}
	return doubanSearchMatch(got), nil
}

func doubanSearchMatch(got *DoubanMatch) *Match {
	mediaType := ""
	if strings.TrimSpace(got.Type) != "" {
		mediaType = normalizeMediaType(got.Type, got.Title, "")
	}
	match := &Match{

		DoubanID:  got.DoubanID,
		MediaType: mediaType,
		Title:     got.Title,
		PosterURL: got.Img,
		Rating:    got.Rating,
	}
	if len(got.Year) >= 4 {
		_, _ = fmt.Sscanf(got.Year[:4], "%d", &match.Year)
	}
	return match
}

func (d *DoubanProvider) GetMatchByID(ctx context.Context, doubanID string) (*Match, error) {
	doubanID = strings.TrimSpace(doubanID)
	if doubanID == "" {
		return nil, nil
	}
	rawJSON, err := d.getDetailRawJSON(ctx, doubanID)
	if err != nil {
		return nil, err
	}
	match, err := doubanMatchFromRawJSON(doubanID, rawJSON)
	if match != nil {
		match.PosterURL = d.ResolveArtworkURL(ctx, match.PosterURL)
	}
	return match, err
}

// GetEnrichmentMatchByID 获取可持久化的完整详情；权限受限时显式回退 subject。
func (d *DoubanProvider) GetEnrichmentMatchByID(ctx context.Context, doubanID, entityKind string) (*Match, bool, error) {
	doubanID = strings.TrimSpace(doubanID)
	if doubanID == "" {
		return nil, false, nil
	}
	rawJSON, degraded, err := d.getEnrichmentDetailRawJSON(ctx, doubanID, entityKind)
	if err != nil {
		return nil, false, err
	}
	match, err := doubanMatchFromRawJSON(doubanID, rawJSON)
	if err != nil {
		return nil, false, fmt.Errorf("%w: invalid detail payload: %w", ErrDoubanTemporarilyUnavailable, err)
	}
	match.PosterURL = d.ResolveArtworkURL(ctx, match.PosterURL)
	return match, degraded, nil
}

func (d *DoubanProvider) getDetailRawJSON(ctx context.Context, doubanID string) ([]byte, error) {
	if rawJSON, err := d.getMobileDetailRawJSON(ctx, doubanID); err == nil {
		return rawJSON, nil
	}
	rawJSON, _, err := d.requestDetailRawJSON(
		ctx,
		"https://movie.douban.com/j/subject_abstract?subject_id="+url.QueryEscape(doubanID),
		"https://movie.douban.com/",
	)
	return rawJSON, err
}

func (d *DoubanProvider) getMobileDetailRawJSON(ctx context.Context, doubanID string) ([]byte, error) {
	escapedID := url.PathEscape(doubanID)
	rawJSON, status, err := d.requestDetailRawJSON(
		ctx,
		"https://m.douban.com/rexxar/api/v2/movie/"+escapedID,
		"https://m.douban.com/subject/"+escapedID+"/",
	)
	permissionDenied, err := classifyDoubanDetailResponse(rawJSON, status, err)
	if permissionDenied {
		return nil, err
	}
	return rawJSON, err
}

func (d *DoubanProvider) getEnrichmentDetailRawJSON(ctx context.Context, doubanID, entityKind string) ([]byte, bool, error) {
	detailType := "movie"
	if entityKind == "series" {
		detailType = "tv"
	} else if entityKind != "movie" {
		return nil, false, fmt.Errorf("unsupported douban entity kind %q", entityKind)
	}
	escapedID := url.PathEscape(doubanID)
	referer := "https://m.douban.com/subject/" + escapedID + "/"
	rawJSON, status, requestErr := d.requestDetailRawJSON(
		ctx,
		"https://m.douban.com/rexxar/api/v2/"+detailType+"/"+escapedID,
		referer,
	)
	permissionDenied, err := classifyDoubanDetailResponse(rawJSON, status, requestErr)
	if !permissionDenied {
		return rawJSON, false, err
	}
	rawJSON, status, requestErr = d.requestDetailRawJSON(
		ctx,
		"https://m.douban.com/rexxar/api/v2/subject/"+escapedID,
		referer,
	)
	permissionDenied, err = classifyDoubanDetailResponse(rawJSON, status, requestErr)
	if err != nil {
		return nil, false, err
	}
	return rawJSON, true, nil
}

func (d *DoubanProvider) requestDetailRawJSON(ctx context.Context, requestURL, referer string) ([]byte, int, error) {
	rawJSON, status, readErr := d.requestJSON(ctx, requestURL, referer)
	switch {
	case status >= 400:
		return nil, status, fmt.Errorf("douban detail: %d", status)
	case readErr != nil:
		return nil, status, readErr
	case !json.Valid(rawJSON):
		return nil, status, errors.New("douban detail: invalid json")
	default:
		return rawJSON, status, nil
	}
}

type doubanHTTPResult struct {
	body   []byte
	status int
	err    error
}

type doubanTaskRouteReset struct{ once sync.Once }
type doubanTaskRouteResetKey struct{}

// withDoubanTaskRouteReset 保证同一补齐任务的首个豆瓣请求从直连开始。
func withDoubanTaskRouteReset(ctx context.Context) context.Context {
	return context.WithValue(ctx, doubanTaskRouteResetKey{}, &doubanTaskRouteReset{})
}

func (d *DoubanProvider) requestJSON(ctx context.Context, requestURL, referer string) ([]byte, int, error) {
	resolved := d.resolveConfig(ctx)
	attempt := func(client *http.Client, config Resolved) doubanHTTPResult {
		return d.doJSONRequest(ctx, client, config, requestURL, referer)
	}
	if !resolved.UseProxyPool {
		d.resetRoute(resolved.Revision)
		result := attempt(d.client, resolved)
		return result.body, result.status, result.err
	}

	snapshot, err := d.proxySnapshot(ctx, resolved)
	if err != nil {
		return nil, 0, err
	}
	route := d.currentRoute(snapshot.generation, snapshot.configRevision, resolved.Revision)
	if reset, _ := ctx.Value(doubanTaskRouteResetKey{}).(*doubanTaskRouteReset); reset != nil {
		reset.once.Do(func() {
			route = doubanDirectRoute
			d.setRoute(snapshot.generation, snapshot.configRevision, resolved.Revision, route)
		})
	}
	result := d.doJSONRequestForRoute(ctx, snapshot, route, resolved, requestURL, referer)
	if !shouldReselectDoubanRoute(route, result) {
		return result.body, result.status, result.err
	}
	result = d.reselectAfterRouteFailure(ctx, requestURL, referer, route, snapshot.generation, snapshot.configRevision, resolved.Revision)
	return result.body, result.status, result.err
}

func shouldReselectDoubanRoute(route int, result doubanHTTPResult) bool {
	if route != doubanDirectRoute {
		return result.status >= http.StatusBadRequest || result.err != nil
	}
	return result.status == http.StatusBadRequest || errors.Is(result.err, io.ErrUnexpectedEOF)
}

func (d *DoubanProvider) reselectAfterRouteFailure(
	ctx context.Context,
	requestURL string,
	referer string,
	failedRoute int,
	failedGeneration uint64,
	failedProxyConfigRevision uint64,
	failedRevision uint64,
) doubanHTTPResult {
	d.reselectMu.Lock()
	defer d.reselectMu.Unlock()

	resolved := d.resolveConfig(ctx)
	if !resolved.UseProxyPool {
		d.resetRoute(resolved.Revision)
		return d.doJSONRequest(ctx, d.client, resolved, requestURL, referer)
	}
	snapshot, err := d.proxySnapshot(ctx, resolved)
	if err != nil {
		return doubanHTTPResult{err: err}
	}
	current := d.currentRoute(snapshot.generation, snapshot.configRevision, resolved.Revision)
	if current != failedRoute || snapshot.generation != failedGeneration || snapshot.configRevision != failedProxyConfigRevision || resolved.Revision != failedRevision {
		result := d.doJSONRequestForRoute(ctx, snapshot, current, resolved, requestURL, referer)
		if failedRoute != doubanDirectRoute && current == doubanDirectRoute {
			return result
		}
		if !shouldReselectDoubanRoute(current, result) {
			return result
		}
		failedRoute = current
	}

	if failedRoute != doubanDirectRoute {
		d.setRoute(snapshot.generation, snapshot.configRevision, resolved.Revision, doubanDirectRoute)
		return d.doJSONRequest(ctx, d.directClient, resolved, requestURL, referer)
	}

	if len(snapshot.clients) == 0 {
		d.setRoute(snapshot.generation, snapshot.configRevision, resolved.Revision, doubanDirectRoute)
		return d.doJSONRequest(ctx, d.directClient, resolved, requestURL, referer)
	}

	result := d.doJSONRequestForRoute(ctx, snapshot, 0, resolved, requestURL, referer)
	if shouldReselectDoubanRoute(0, result) {
		d.setRoute(snapshot.generation, snapshot.configRevision, resolved.Revision, doubanDirectRoute)
		return d.doJSONRequest(ctx, d.directClient, resolved, requestURL, referer)
	}
	if result.status != 0 {
		d.setRoute(snapshot.generation, snapshot.configRevision, resolved.Revision, 0)
	}
	return result
}

func (d *DoubanProvider) doJSONRequestForRoute(
	ctx context.Context,
	snapshot proxyPoolSnapshot,
	route int,
	resolved Resolved,
	requestURL string,
	referer string,
) doubanHTTPResult {
	client := d.clientForRoute(snapshot, route)
	if route < 0 || snapshot.resin == nil {
		return d.doJSONRequest(ctx, client, resolved, requestURL, referer)
	}
	reverseURL, err := buildResinReverseURL(*snapshot.resin, requestURL)
	if err != nil {
		return doubanHTTPResult{err: err}
	}
	result := d.doJSONRequest(ctx, client, resolved, reverseURL, referer)
	result.err = redactResinRequestError(result.err)
	return result
}

func (d *DoubanProvider) doJSONRequest(
	ctx context.Context,
	client *http.Client,
	resolved Resolved,
	requestURL string,
	referer string,
) doubanHTTPResult {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, requestURL, nil)
	if err != nil {
		return doubanHTTPResult{err: err}
	}
	d.setHeaders(req, resolved)
	if referer != "" {
		req.Header.Set("Referer", referer)
	}
	if client == nil {
		return doubanHTTPResult{err: errors.New("douban http client unavailable")}
	}
	requestClient := *client
	if requestClient.Timeout <= 0 {
		requestClient.Timeout = doubanRequestTimeout
	}
	resp, err := requestClient.Do(req)
	if err != nil {
		return doubanHTTPResult{err: err}
	}
	body, readErr := io.ReadAll(io.LimitReader(resp.Body, 8<<20))
	_ = resp.Body.Close()
	return doubanHTTPResult{body: body, status: resp.StatusCode, err: readErr}
}

func (d *DoubanProvider) resolveConfig(ctx context.Context) Resolved {
	if d.apiConfig == nil {
		if d.cfg == nil {
			return Resolved{Enabled: true}
		}
		return Resolved{Enabled: true, APIKey: d.cfg.Secrets.DoubanCookie}
	}
	resolved, err := d.apiConfig.Resolve(ctx, "douban")
	if err != nil {
		return Resolved{}
	}
	return resolved
}

func (d *DoubanProvider) proxySnapshot(ctx context.Context, _ Resolved) (proxyPoolSnapshot, error) {
	if d.proxyPool == nil {
		return proxyPoolSnapshot{}, nil
	}
	config, err := d.proxyPool.resolveConfig(ctx)
	if err != nil {
		return proxyPoolSnapshot{}, err
	}
	switch config.ProxyPoolType {
	case ProxyPoolTypeNormal:
		snapshot, err := d.proxyPool.snapshot(ctx)
		if err != nil {
			return proxyPoolSnapshot{}, err
		}
		snapshot.configRevision = config.Revision
		return snapshot, nil
	case ProxyPoolTypeResin:
		origin, err := normalizeResinProxyOrigin(config.ResinProxyURL)
		if err != nil || origin == "" || config.ResinProxyToken == "" || d.resinClient == nil {
			return proxyPoolSnapshot{}, errors.New("invalid resin proxy configuration")
		}
		config.ResinProxyURL = origin
		return proxyPoolSnapshot{
			configRevision: config.Revision,
			clients:        []*http.Client{d.resinClient},
			resin:          &config,
		}, nil
	default:
		return proxyPoolSnapshot{}, errors.New("invalid douban proxy pool type")
	}
}

func buildResinReverseURL(config resolvedProxyPoolConfig, requestURL string) (string, error) {
	target, err := url.Parse(requestURL)
	if err != nil || (target.Scheme != "http" && target.Scheme != "https") || target.Host == "" {
		return "", errors.New("invalid resin proxy target")
	}
	identity := "."
	if account := strings.TrimSpace(config.ResinAccount); account != "" {
		identity = "Default." + account
	}
	reverseURL := config.ResinProxyURL + "/" + url.PathEscape(config.ResinProxyToken) + "/" + url.PathEscape(identity) + "/" + target.Scheme + "/" + url.PathEscape(target.Host)
	if escapedPath := target.EscapedPath(); escapedPath != "" {
		reverseURL += escapedPath
	}
	if target.RawQuery != "" || target.ForceQuery {
		reverseURL += "?" + target.RawQuery
	}
	return reverseURL, nil
}

func redactResinRequestError(err error) error {
	if err == nil {
		return nil
	}
	var urlErr *url.Error
	if !errors.As(err, &urlErr) {
		return err
	}
	redacted := *urlErr
	redacted.URL = "[redacted-url]"
	return &redacted
}

func (d *DoubanProvider) currentRoute(generation, proxyConfigRevision, revision uint64) int {
	d.routeMu.Lock()
	defer d.routeMu.Unlock()
	if !d.routeInitialized || d.poolGeneration != generation || d.proxyConfigRevision != proxyConfigRevision || d.configRevision != revision {
		d.routeInitialized = true
		d.route = doubanDirectRoute
		d.poolGeneration = generation
		d.proxyConfigRevision = proxyConfigRevision
		d.configRevision = revision
	}
	return d.route
}

func (d *DoubanProvider) setRoute(generation, proxyConfigRevision, revision uint64, route int) {
	d.routeMu.Lock()
	d.routeInitialized = true
	d.route = route
	d.poolGeneration = generation
	d.proxyConfigRevision = proxyConfigRevision
	d.configRevision = revision
	d.routeMu.Unlock()
}

func (d *DoubanProvider) resetRoute(revision uint64) {
	d.setRoute(0, 0, revision, doubanDirectRoute)
}

func (d *DoubanProvider) clientForRoute(snapshot proxyPoolSnapshot, route int) *http.Client {
	if route >= 0 && route < len(snapshot.clients) {
		return snapshot.clients[route]
	}
	return d.directClient
}

func classifyDoubanDetailResponse(rawJSON []byte, status int, requestErr error) (permissionDenied bool, err error) {
	if status == http.StatusNotFound {
		return false, ErrDoubanSubjectNotFound
	}
	if requestErr != nil {
		if errors.Is(requestErr, context.Canceled) {
			return false, requestErr
		}
		if status == http.StatusForbidden {
			return true, fmt.Errorf("%w: HTTP %d", ErrDoubanTemporarilyUnavailable, status)
		}
		if status == http.StatusTooManyRequests || status >= 500 {
			return false, fmt.Errorf("%w: HTTP %d", ErrDoubanTemporarilyUnavailable, status)
		}
		if status == 0 || status < 400 {
			return false, fmt.Errorf("%w: %w", ErrDoubanTemporarilyUnavailable, requestErr)
		}
		return false, requestErr
	}
	if notFound, permissionDenied, reason := doubanDetailResponseFailure(rawJSON); reason != "" {
		if notFound {
			return false, ErrDoubanSubjectNotFound
		}
		err := fmt.Errorf("%w: %s", ErrDoubanTemporarilyUnavailable, reason)
		if permissionDenied {
			return true, err
		}
		return false, err
	}
	return false, nil
}

func doubanDetailResponseFailure(rawJSON []byte) (notFound, permissionDenied bool, reason string) {
	var raw map[string]any
	if err := json.Unmarshal(rawJSON, &raw); err != nil || raw == nil {
		return false, false, "invalid response"
	}
	code := ""
	for _, key := range []string{"code", "error_code"} {
		switch value := raw[key].(type) {
		case string:
			code = strings.TrimSpace(value)
		case float64:
			code = fmt.Sprint(value)
		}
		if code != "" {
			break
		}
	}
	code = strings.ToLower(code)
	if code != "" {
		return strings.Contains(code, "not_found") || strings.Contains(code, "not found") || strings.Contains(code, "not_exist"), code == "1000", fmt.Sprintf("response code %q", code)
	}
	value, ok := raw["error"]
	if !ok {
		return false, false, ""
	}
	switch typed := value.(type) {
	case nil:
		return false, false, ""
	case bool:
		if !typed {
			return false, false, ""
		}
		return false, false, "error response"
	case string:
		typed = strings.ToLower(strings.TrimSpace(typed))
		if typed == "" {
			return false, false, ""
		}
		return strings.Contains(typed, "not found") || strings.Contains(typed, "not_exist"), false, fmt.Sprintf("response error %q", typed)
	default:
		return false, false, "error response"
	}
}

func doubanMatchFromRawJSON(doubanID string, rawJSON []byte) (*Match, error) {
	var raw map[string]any
	if err := json.Unmarshal(rawJSON, &raw); err != nil {
		return nil, err
	}
	subject := raw
	if nested, ok := raw["subject"].(map[string]any); ok {
		subject = nested
	} else if nested, ok := raw["data"].(map[string]any); ok {
		subject = nested
	}
	if id := firstStringFromMap(subject, "id"); id != "" && id != strings.TrimSpace(doubanID) {
		return nil, errors.New("douban subject identity mismatch")
	}
	title := firstStringFromMap(subject, "title", "name")
	if strings.TrimSpace(title) == "" {
		title = firstStringFromMap(raw, "title")
	}
	if strings.TrimSpace(title) == "" {
		return nil, ErrDoubanSubjectNotFound
	}
	year := 0
	if y := firstStringFromMap(subject, "year"); len(y) >= 4 {
		_, _ = fmt.Sscanf(y[:4], "%d", &year)
	}
	m := &Match{

		DoubanID:     strings.TrimSpace(doubanID),
		TMDbID:       positiveIntFromMap(subject, "tmdb_id", "tmdbid"),
		Title:        title,
		OriginalName: firstStringFromMap(subject, "original_title", "original_name"),
		Overview:     firstStringFromMap(subject, "intro", "summary", "abstract", "short_comment"),
		PosterURL:    doubanPosterURL(subject),
		Year:         year,
		ReleaseDate:  normalizeReleaseDate(firstStringFromMap(subject, "release_date", "pubdate")),
		Rating:       float32FromMap(subject, "rate", "rating"),
		Languages:    stringsFromMap(subject, "languages", "language"),
		Countries:    stringsFromMap(subject, "countries", "country", "regions"),
		Genres:       stringsFromMap(subject, "genres", "genre"),
		RawJSON:      rawJSON,
	}
	if m.TMDbID == 0 {
		m.TMDbID = positiveIntFromMap(raw, "tmdb_id", "tmdbid")
	}
	m.MediaType = firstStringFromMap(subject, "type")
	if tv, _ := subject["is_tv"].(bool); tv {
		m.MediaType = "tv"
	}
	m.Actors = stringsFromMap(subject, "actors", "casts")
	m.Directors = stringsFromMap(subject, "directors")
	m.Writers = stringsFromMap(subject, "writers")
	m.Aliases = stringsFromMap(subject, "aka")
	m.People = peopleFromNames(m.Actors)
	if m.ReleaseDate == "" {
		m.ReleaseDate = firstDoubanReleaseDate(stringsFromMap(subject, "pubdate", "pubdates"))
	}
	m.DurationMinutes = firstDoubanDuration(stringsFromMap(subject, "durations"))
	return m, nil
}

func doubanPosterURL(subject map[string]any) string {
	for _, path := range [][]string{
		{"cover", "image", "large", "url"},
		{"pic", "large"},
	} {
		if value := stringFromMapPath(subject, path...); validDoubanArtworkURL(value) {
			return value
		}
	}
	for _, key := range []string{"cover_url", "pic", "img", "cover"} {
		if v := firstStringFromMap(subject, key); validDoubanArtworkURL(v) {
			return v
		}
	}
	return ""
}

func stringFromMapPath(values map[string]any, path ...string) string {
	var current any = values
	for _, key := range path {
		nested, ok := current.(map[string]any)
		if !ok {
			return ""
		}
		current = nested[key]
	}
	value, _ := current.(string)
	return strings.TrimSpace(value)
}

// ResolveArtworkURL 返回可稳定持久化的豆瓣官方大图地址。
func (d *DoubanProvider) ResolveArtworkURL(_ context.Context, raw string) string {
	sourceURL := strings.TrimSpace(raw)
	if sourceURL == "" {
		return ""
	}
	if largeURL := deriveDoubanLargePosterURL(sourceURL); largeURL != "" {
		sourceURL = largeURL
	}
	target, err := url.Parse(sourceURL)
	if err != nil || !strings.HasPrefix(target.RawQuery, "imageView2/") || strings.Contains(target.RawQuery, "&") {
		return sourceURL
	}
	target.RawQuery = ""
	target.ForceQuery = false
	return target.String()
}

func projectDoubanArtworkURL(raw, baseURL string) string {
	target, err := url.Parse(strings.TrimSpace(raw))
	if err != nil || !isDoubanImageHost(target.Host) {
		return ""
	}
	origin, err := url.Parse(strings.TrimSpace(baseURL))
	if err != nil || origin.Host == "" || (origin.Scheme != "http" && origin.Scheme != "https") {
		return ""
	}
	target.Scheme, target.Host, target.User = origin.Scheme, origin.Host, nil
	if ext := path.Ext(target.Path); ext != "" {
		target.Path = strings.TrimSuffix(target.Path, ext) + ".webp"
		target.RawPath = ""
	}
	return target.String()
}

func deriveDoubanLargePosterURL(raw string) string {
	u, err := url.Parse(strings.TrimSpace(raw))
	if err != nil || (u.Scheme != "http" && u.Scheme != "https") || u.Host == "" {
		return ""
	}
	const prefix = "/view/photo/"
	if !strings.HasPrefix(u.Path, prefix) {
		return ""
	}
	rest := strings.TrimPrefix(u.Path, prefix)
	publicAt := strings.Index(rest, "/public/")
	if publicAt <= 0 || strings.TrimSpace(rest[publicAt+len("/public/"):]) == "" {
		return ""
	}
	u.Path = prefix + "l" + rest[publicAt:]
	u.RawPath = ""
	return u.String()
}

func (d *DoubanProvider) GetEpisodeCount(ctx context.Context, query string) (int, error) {
	match, err := d.Search(ctx, query)
	if err != nil || match == nil || strings.TrimSpace(match.DoubanID) == "" {
		return 0, err
	}
	return d.GetEpisodeCountByID(ctx, match.DoubanID)
}

func (d *DoubanProvider) GetEpisodeCountByID(ctx context.Context, doubanID string) (int, error) {
	doubanID = strings.TrimSpace(doubanID)
	if doubanID == "" {
		return 0, nil
	}
	rawJSON, err := d.getDetailRawJSON(ctx, doubanID)
	if err != nil {
		return 0, err
	}
	var raw map[string]any
	if err := json.Unmarshal(rawJSON, &raw); err != nil {
		return 0, err
	}
	for _, key := range []string{"episode_count", "episodes_count", "episodes", "eps"} {
		if count := doubanEpisodeCountFromValue(raw[key]); count > 0 {
			return count, nil
		}
	}
	for _, key := range []string{"subject", "data"} {
		if nested, ok := raw[key].(map[string]any); ok {
			for _, field := range []string{"episode_count", "episodes_count", "episodes", "eps"} {
				if count := doubanEpisodeCountFromValue(nested[field]); count > 0 {
					return count, nil
				}
			}
		}
	}
	return 0, nil
}

func (d *DoubanProvider) setHeaders(req *http.Request, resolved Resolved) {
	req.Header.Set("User-Agent", userAgents[secureRandomIntn(len(userAgents))])
	req.Header.Set("Referer", "https://movie.douban.com/")
	req.Header.Set("Accept", "application/json, text/plain, */*")
	req.Header.Set("Accept-Language", "zh-CN,zh;q=0.9,en;q=0.8")
	if !resolved.Enabled {
		return
	}
	if cookie := strings.TrimSpace(resolved.APIKey); cookie != "" {
		req.Header.Set("Cookie", cookie)
	}
}

func validDoubanArtworkURL(raw string) bool {
	u, err := url.Parse(raw)
	return err == nil && (u.Scheme == "http" || u.Scheme == "https") && u.Host != ""
}
