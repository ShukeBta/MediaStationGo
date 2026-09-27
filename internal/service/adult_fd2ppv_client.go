package service

import (
	"context"
	"crypto/sha256"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/http/cookiejar"
	"net/url"
	"regexp"
	"strings"
	"sync"
	"time"

	"github.com/ShukeBta/MediaStationGo/internal/helper"
	"go.uber.org/zap"
)

const (
	fd2PPVHTMLCacheTTL        = 6 * time.Hour
	fd2PPVHTMLCacheMaxEntries = 256
	fd2PPVChrome133UserAgent  = "Mozilla/5.0 (X11; Linux x86_64) AppleWebKit/537.36 (KHTML, like Gecko) Chrome/133.0.0.0 Safari/537.36"
	fd2PPVLoginPageAttempts   = 2
)

var (
	fd2PPVCSRFPattern = regexp.MustCompile(`(?is)<input\b[^>]*\bname=["']_csrf["'][^>]*\bvalue=["']([^"']+)`)
	fd2PPVAuthPattern = regexp.MustCompile(`(?is)(?:data-url=["']logout["']|/fetch/logout\.php|ログアウト)`)
)

type fd2PPVClient struct {
	mu       sync.Mutex
	cache    map[string]fd2PPVHTMLCacheEntry
	inflight map[string]*fd2PPVFetchCall

	sessionMu           sync.Mutex
	cookies             []helper.FlareSolverrCookie
	userAgent           string
	credentialSignature [sha256.Size]byte
	direct              fd2PPVDirectFetcher
}

type fd2PPVHTMLCacheEntry struct {
	body     string
	storedAt time.Time
}

type fd2PPVFetchCall struct {
	done chan struct{}
	body string
	err  error
}

type fd2PPVCredentials struct {
	username  string
	password  string
	signature [sha256.Size]byte
}

type fd2PPVLoginPage struct {
	body      string
	cookies   []helper.FlareSolverrCookie
	userAgent string
}

func newFD2PPVClient() *fd2PPVClient {
	return &fd2PPVClient{
		cache:    make(map[string]fd2PPVHTMLCacheEntry),
		inflight: make(map[string]*fd2PPVFetchCall),
		direct:   &fd2PPVTLSFetcher{},
	}
}

func (p *AdultProvider) ForgetFD2PPVCache() {
	if p == nil || p.fd2ppv == nil {
		return
	}
	p.fd2ppv.mu.Lock()
	clear(p.fd2ppv.cache)
	p.fd2ppv.mu.Unlock()
}

func (p *AdultProvider) fetchAuthenticatedFD2PPVText(ctx context.Context, targetURL string) (string, error) {
	if p == nil || p.fd2ppv == nil {
		return "", errors.New("fd2ppv client is unavailable")
	}
	return p.fd2ppv.fetch(ctx, p, targetURL)
}

func (c *fd2PPVClient) fetch(ctx context.Context, provider *AdultProvider, targetURL string) (string, error) {
	now := time.Now()
	c.mu.Lock()
	if entry, ok := c.cache[targetURL]; ok && now.Sub(entry.storedAt) <= fd2PPVHTMLCacheTTL {
		body := entry.body
		c.mu.Unlock()
		return body, nil
	} else if ok {
		delete(c.cache, targetURL)
	}
	if call, ok := c.inflight[targetURL]; ok {
		c.mu.Unlock()
		select {
		case <-ctx.Done():
			return "", ctx.Err()
		case <-call.done:
			return call.body, call.err
		}
	}
	call := &fd2PPVFetchCall{done: make(chan struct{})}
	c.inflight[targetURL] = call
	c.mu.Unlock()

	body, err := c.fetchFresh(ctx, provider, targetURL)
	if err == nil {
		storedAt := time.Now()
		c.mu.Lock()
		c.pruneCacheLocked(storedAt)
		c.cache[targetURL] = fd2PPVHTMLCacheEntry{body: body, storedAt: storedAt}
		c.mu.Unlock()
	}

	c.mu.Lock()
	call.body = body
	call.err = err
	delete(c.inflight, targetURL)
	close(call.done)
	c.mu.Unlock()
	return body, err
}

func (c *fd2PPVClient) pruneCacheLocked(now time.Time) {
	for key, entry := range c.cache {
		if now.Sub(entry.storedAt) > fd2PPVHTMLCacheTTL {
			delete(c.cache, key)
		}
	}
	for len(c.cache) >= fd2PPVHTMLCacheMaxEntries {
		oldestKey := ""
		oldestAt := now
		for key, entry := range c.cache {
			if oldestKey == "" || entry.storedAt.Before(oldestAt) {
				oldestKey = key
				oldestAt = entry.storedAt
			}
		}
		if oldestKey == "" {
			return
		}
		delete(c.cache, oldestKey)
	}
}

func (c *fd2PPVClient) fetchFresh(ctx context.Context, provider *AdultProvider, targetURL string) (string, error) {
	c.sessionMu.Lock()
	defer c.sessionMu.Unlock()

	credentials, err := provider.resolveFD2PPVCredentials(ctx)
	if err != nil {
		return "", err
	}
	if credentials.signature != c.credentialSignature || !fd2PPVHasCookie(c.cookies, "member") {
		if err := c.login(ctx, provider, credentials); err != nil {
			return "", err
		}
	}

	var lastErr error
	for loginAttempt := 0; loginAttempt < 2; loginAttempt++ {
		direct, directErr := c.fetchDirect(ctx, targetURL)
		if directErr != nil {
			return "", fmt.Errorf("fd2ppv direct request: %w", directErr)
		}
		if fd2PPVAuthenticatedHTMLUsable(direct.body) {
			return direct.body, nil
		}
		if direct.statusCode >= http.StatusBadRequest && direct.statusCode != http.StatusForbidden {
			return "", fmt.Errorf("fd2ppv direct request returned HTTP %d", direct.statusCode)
		}

		solution, fetchErr := provider.fetchWithFlareSolverrResultContext(
			ctx,
			targetURL,
			cloneFD2PPVCookies(c.cookies),
		)
		if fetchErr != nil {
			lastErr = fetchErr
		} else {
			c.cookies = mergeFD2PPVCookies(c.cookies, solution.Cookies)
			if strings.TrimSpace(solution.UserAgent) != "" {
				c.userAgent = strings.TrimSpace(solution.UserAgent)
			}
			if fd2PPVAuthenticatedHTMLUsable(solution.Response) {
				return solution.Response, nil
			}

			direct, directErr = c.fetchDirect(ctx, targetURL)
			if directErr != nil {
				return "", fmt.Errorf("fd2ppv direct request after challenge: %w", directErr)
			}
			if fd2PPVAuthenticatedHTMLUsable(direct.body) {
				return direct.body, nil
			}
			if direct.statusCode >= http.StatusBadRequest && direct.statusCode != http.StatusForbidden {
				return "", fmt.Errorf("fd2ppv direct request after challenge returned HTTP %d", direct.statusCode)
			}
			lastErr = errors.New("fd2ppv returned incomplete authenticated HTML after challenge refresh")
		}

		c.cookies = nil
		c.userAgent = ""
		c.credentialSignature = [sha256.Size]byte{}
		if err := c.login(ctx, provider, credentials); err != nil {
			return "", err
		}
	}
	if lastErr == nil {
		lastErr = errors.New("fd2ppv authenticated request failed")
	}
	return "", lastErr
}

func (c *fd2PPVClient) fetchDirect(ctx context.Context, targetURL string) (fd2PPVDirectFetchResult, error) {
	if c.direct == nil {
		return fd2PPVDirectFetchResult{}, errors.New("browser fingerprint client is unavailable")
	}
	result, err := c.direct.fetch(
		ctx,
		targetURL,
		firstNonEmpty(strings.TrimSpace(c.userAgent), fd2PPVChrome133UserAgent),
		cloneFD2PPVCookies(c.cookies),
	)
	if err != nil {
		return fd2PPVDirectFetchResult{}, err
	}
	c.cookies = mergeFD2PPVCookies(c.cookies, result.cookies)
	return result, nil
}

// CheckFD2PPVSession verifies the single shared FD2PPV session without using
// the six-hour HTML cache. Disabled or unconfigured credentials are a normal
// no-op; enabled but incomplete credentials remain an explicit error.
func (p *AdultProvider) CheckFD2PPVSession(ctx context.Context) error {
	if p == nil || p.fd2ppv == nil || !p.FD2PPVEnabled() || p.apiConfig == nil {
		return nil
	}
	credentials, configured, err := p.fd2PPVSessionCredentials(ctx)
	if err != nil {
		return err
	}
	if !configured {
		return nil
	}
	return p.fd2ppv.checkSession(ctx, p, credentials)
}

func (c *fd2PPVClient) checkSession(
	ctx context.Context,
	provider *AdultProvider,
	credentials fd2PPVCredentials,
) error {
	c.sessionMu.Lock()
	defer c.sessionMu.Unlock()

	baseURL := strings.TrimRight(fd2PPVBaseURL, "/")
	if credentials.signature == c.credentialSignature && fd2PPVHasCookie(c.cookies, "member") {
		direct, err := c.fetchDirect(ctx, baseURL+"/")
		if err != nil {
			return fmt.Errorf("fd2ppv session health request: %w", err)
		}
		if fd2PPVAuthenticatedHTMLUsable(direct.body) {
			return nil
		}
		if direct.statusCode >= http.StatusInternalServerError {
			return fmt.Errorf("fd2ppv session health request returned HTTP %d", direct.statusCode)
		}
	}

	c.cookies = nil
	c.userAgent = ""
	c.credentialSignature = [sha256.Size]byte{}
	if err := c.login(ctx, provider, credentials); err != nil {
		return fmt.Errorf("fd2ppv session refresh: %w", err)
	}
	direct, err := c.fetchDirect(ctx, baseURL+"/")
	if err != nil {
		return fmt.Errorf("fd2ppv session verification request: %w", err)
	}
	if direct.statusCode >= http.StatusBadRequest {
		return fmt.Errorf("fd2ppv session verification returned HTTP %d", direct.statusCode)
	}
	if !fd2PPVAuthenticatedHTMLUsable(direct.body) {
		return errors.New("fd2ppv session verification did not return authenticated HTML")
	}
	return nil
}

func (p *AdultProvider) fd2PPVSessionCredentials(ctx context.Context) (fd2PPVCredentials, bool, error) {
	resolved, err := p.apiConfig.Resolve(ctx, "fd2ppv")
	if err != nil {
		return fd2PPVCredentials{}, false, fmt.Errorf("resolve fd2ppv credentials: %w", err)
	}
	if !resolved.Enabled {
		return fd2PPVCredentials{}, false, nil
	}
	username := strings.TrimSpace(resolved.Extra)
	password := strings.TrimSpace(resolved.APIKey)
	if username == "" || password == "" {
		return fd2PPVCredentials{}, true, errors.New("fd2ppv username or password is not configured")
	}
	return fd2PPVCredentials{
		username:  username,
		password:  password,
		signature: sha256.Sum256([]byte(username + "\x00" + password)),
	}, true, nil
}

func (p *AdultProvider) resolveFD2PPVCredentials(ctx context.Context) (fd2PPVCredentials, error) {
	if p == nil || p.apiConfig == nil {
		return fd2PPVCredentials{}, errors.New("fd2ppv credentials service is unavailable")
	}
	credentials, configured, err := p.fd2PPVSessionCredentials(ctx)
	if err != nil {
		return fd2PPVCredentials{}, err
	}
	if !configured {
		return fd2PPVCredentials{}, errors.New("fd2ppv credentials are disabled")
	}
	return credentials, nil
}

func (c *fd2PPVClient) login(ctx context.Context, provider *AdultProvider, credentials fd2PPVCredentials) error {
	select {
	case <-ctx.Done():
		return ctx.Err()
	default:
	}

	baseURL := strings.TrimRight(fd2PPVBaseURL, "/")
	loginPage, err := c.resolveLoginPage(ctx, provider, baseURL+"/")
	if err != nil {
		return err
	}
	csrf := fd2PPVCSRFToken(loginPage.body)

	jar, err := cookiejar.New(nil)
	if err != nil {
		return fmt.Errorf("create fd2ppv cookie jar: %w", err)
	}
	rootURL, err := url.Parse(baseURL + "/")
	if err != nil {
		return fmt.Errorf("parse fd2ppv base URL: %w", err)
	}
	jar.SetCookies(rootURL, netHTTPCookies(loginPage.cookies))

	payload, err := json.Marshal(map[string]string{
		"user":     credentials.username,
		"password": credentials.password,
		"_csrf":    csrf,
	})
	if err != nil {
		return fmt.Errorf("encode fd2ppv login request: %w", err)
	}
	request, err := http.NewRequestWithContext(ctx, http.MethodPost, baseURL+"/fetch/login.php", strings.NewReader(string(payload)))
	if err != nil {
		return fmt.Errorf("create fd2ppv login request: %w", err)
	}
	request.Header.Set("Accept", "application/json")
	request.Header.Set("Content-Type", "application/json")
	request.Header.Set("Origin", baseURL)
	request.Header.Set("Referer", baseURL+"/")
	request.Header.Set("User-Agent", firstNonEmpty(strings.TrimSpace(loginPage.userAgent), fd2PPVChrome133UserAgent))

	client := &http.Client{
		Transport: provider.client.Transport,
		Timeout:   15 * time.Second,
		Jar:       jar,
	}
	response, err := client.Do(request)
	if err != nil {
		return fmt.Errorf("fd2ppv login request failed: %w", err)
	}
	defer response.Body.Close()
	if response.StatusCode >= http.StatusBadRequest {
		_, _ = io.Copy(io.Discard, io.LimitReader(response.Body, 4096))
		return fmt.Errorf("fd2ppv login returned HTTP %d", response.StatusCode)
	}
	var loginResponse struct {
		Script string `json:"script"`
	}
	if err := json.NewDecoder(io.LimitReader(response.Body, 64<<10)).Decode(&loginResponse); err != nil {
		return fmt.Errorf("fd2ppv login returned invalid JSON: %w", err)
	}
	if strings.TrimSpace(loginResponse.Script) == "" {
		return errors.New("fd2ppv login response did not confirm success")
	}
	decodedScript, err := base64.StdEncoding.DecodeString(loginResponse.Script)
	if err != nil {
		return errors.New("fd2ppv login response contained an invalid success script")
	}
	if !strings.Contains(strings.ReplaceAll(string(decodedScript), " ", ""), "location.reload()") {
		return errors.New("fd2ppv login response did not contain the expected success action")
	}

	memberCookies := make([]helper.FlareSolverrCookie, 0, 1)
	for _, cookie := range jar.Cookies(rootURL) {
		if cookie.Name != "member" || strings.TrimSpace(cookie.Value) == "" {
			continue
		}
		memberCookies = append(memberCookies, helper.FlareSolverrCookie{
			Name:   cookie.Name,
			Value:  cookie.Value,
			Domain: rootURL.Hostname(),
			Path:   "/",
		})
	}
	if len(memberCookies) == 0 {
		return errors.New("fd2ppv login did not return a member cookie")
	}

	c.cookies = mergeFD2PPVCookies(loginPage.cookies, memberCookies)
	c.userAgent = firstNonEmpty(strings.TrimSpace(loginPage.userAgent), fd2PPVChrome133UserAgent)
	c.credentialSignature = credentials.signature
	return nil
}

func (c *fd2PPVClient) resolveLoginPage(
	ctx context.Context,
	provider *AdultProvider,
	targetURL string,
) (fd2PPVLoginPage, error) {
	directPage, directErr := fetchFD2PPVLoginPageDirect(ctx, provider, targetURL)
	if directErr == nil {
		if fd2PPVCSRFToken(directPage.body) != "" {
			return directPage, nil
		}
		directErr = errors.New("direct response did not contain a CSRF token")
	}
	if err := ctx.Err(); err != nil {
		return fd2PPVLoginPage{}, err
	}
	if provider.log != nil {
		provider.log.Info("fd2ppv direct login page unavailable; using FlareSolverr",
			zap.Error(directErr))
	}

	for attempt := 0; attempt < fd2PPVLoginPageAttempts; attempt++ {
		solution, err := provider.fetchWithFlareSolverrResultContext(
			ctx,
			targetURL,
			nil,
		)
		if err != nil {
			return fd2PPVLoginPage{}, fmt.Errorf("fd2ppv login page direct request failed (%v); FlareSolverr request: %w", directErr, err)
		}
		if fd2PPVCSRFToken(solution.Response) == "" {
			continue
		}
		return fd2PPVLoginPage{
			body:      solution.Response,
			cookies:   cloneFD2PPVCookies(solution.Cookies),
			userAgent: firstNonEmpty(strings.TrimSpace(solution.UserAgent), fd2PPVChrome133UserAgent),
		}, nil
	}
	return fd2PPVLoginPage{}, fmt.Errorf(
		"fd2ppv login page direct request failed (%v); FlareSolverr response did not contain a CSRF token after %d attempts",
		directErr,
		fd2PPVLoginPageAttempts,
	)
}

func fetchFD2PPVLoginPageDirect(
	ctx context.Context,
	provider *AdultProvider,
	targetURL string,
) (fd2PPVLoginPage, error) {
	if provider == nil || provider.client == nil {
		return fd2PPVLoginPage{}, errors.New("fd2ppv direct HTTP client is unavailable")
	}
	request, err := http.NewRequestWithContext(ctx, http.MethodGet, targetURL, nil)
	if err != nil {
		return fd2PPVLoginPage{}, fmt.Errorf("create fd2ppv direct login page request: %w", err)
	}
	request.Header.Set("Accept", "text/html,application/xhtml+xml,application/xml;q=0.9,image/avif,image/webp,*/*;q=0.8")
	request.Header.Set("Accept-Language", "ja,en-US;q=0.9,en;q=0.8")
	request.Header.Set("User-Agent", fd2PPVChrome133UserAgent)

	response, err := provider.client.Do(request)
	if err != nil {
		return fd2PPVLoginPage{}, fmt.Errorf("fd2ppv direct login page request failed: %w", err)
	}
	defer response.Body.Close()
	if response.StatusCode >= http.StatusBadRequest {
		_, _ = io.Copy(io.Discard, io.LimitReader(response.Body, 4096))
		return fd2PPVLoginPage{}, fmt.Errorf("fd2ppv direct login page returned HTTP %d", response.StatusCode)
	}
	body, err := io.ReadAll(io.LimitReader(response.Body, 8<<20))
	if err != nil {
		return fd2PPVLoginPage{}, fmt.Errorf("read fd2ppv direct login page: %w", err)
	}
	host := ""
	if response.Request != nil && response.Request.URL != nil {
		host = response.Request.URL.Hostname()
	}
	return fd2PPVLoginPage{
		body:      string(body),
		cookies:   fd2PPVNetHTTPCookies(response.Cookies(), host),
		userAgent: fd2PPVChrome133UserAgent,
	}, nil
}

func fd2PPVCSRFToken(body string) string {
	match := fd2PPVCSRFPattern.FindStringSubmatch(body)
	if len(match) < 2 {
		return ""
	}
	return strings.TrimSpace(match[1])
}

func fd2PPVAuthenticatedHTMLUsable(body string) bool {
	body = strings.TrimSpace(body)
	return len(body) >= 4096 &&
		strings.Contains(strings.ToLower(body), "</html>") &&
		!helper.IsCloudflareChallenge(body) &&
		fd2PPVAuthPattern.MatchString(body)
}

func netHTTPCookies(cookies []helper.FlareSolverrCookie) []*http.Cookie {
	out := make([]*http.Cookie, 0, len(cookies))
	for _, cookie := range cookies {
		if strings.TrimSpace(cookie.Name) == "" {
			continue
		}
		out = append(out, &http.Cookie{
			Name:   cookie.Name,
			Value:  cookie.Value,
			Domain: cookie.Domain,
			Path:   firstNonEmpty(strings.TrimSpace(cookie.Path), "/"),
			Secure: true,
		})
	}
	return out
}

func fd2PPVNetHTTPCookies(cookies []*http.Cookie, fallbackDomain string) []helper.FlareSolverrCookie {
	out := make([]helper.FlareSolverrCookie, 0, len(cookies))
	for _, cookie := range cookies {
		if cookie == nil || strings.TrimSpace(cookie.Name) == "" {
			continue
		}
		out = append(out, helper.FlareSolverrCookie{
			Name:   cookie.Name,
			Value:  cookie.Value,
			Domain: firstNonEmpty(strings.TrimPrefix(strings.TrimSpace(cookie.Domain), "."), fallbackDomain),
			Path:   firstNonEmpty(strings.TrimSpace(cookie.Path), "/"),
		})
	}
	return out
}

func cloneFD2PPVCookies(cookies []helper.FlareSolverrCookie) []helper.FlareSolverrCookie {
	if len(cookies) == 0 {
		return nil
	}
	out := make([]helper.FlareSolverrCookie, len(cookies))
	copy(out, cookies)
	return out
}

func mergeFD2PPVCookies(current, fresh []helper.FlareSolverrCookie) []helper.FlareSolverrCookie {
	merged := make(map[string]helper.FlareSolverrCookie, len(current)+len(fresh))
	order := make([]string, 0, len(current)+len(fresh))
	add := func(cookie helper.FlareSolverrCookie) {
		if strings.TrimSpace(cookie.Name) == "" {
			return
		}
		domain := strings.TrimPrefix(strings.ToLower(strings.TrimSpace(cookie.Domain)), ".")
		path := firstNonEmpty(strings.TrimSpace(cookie.Path), "/")
		key := strings.ToLower(strings.TrimSpace(cookie.Name)) + "\x00" + domain + "\x00" + path
		if _, exists := merged[key]; !exists {
			order = append(order, key)
		}
		merged[key] = cookie
	}
	for _, cookie := range current {
		add(cookie)
	}
	for _, cookie := range fresh {
		add(cookie)
	}
	out := make([]helper.FlareSolverrCookie, 0, len(order))
	for _, key := range order {
		out = append(out, merged[key])
	}
	return out
}

func fd2PPVHasCookie(cookies []helper.FlareSolverrCookie, name string) bool {
	for _, cookie := range cookies {
		if strings.EqualFold(strings.TrimSpace(cookie.Name), name) && strings.TrimSpace(cookie.Value) != "" {
			return true
		}
	}
	return false
}
