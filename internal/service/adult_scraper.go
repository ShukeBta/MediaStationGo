package service

import (
	"context"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"regexp"
	"strings"
	"sync"
	"time"

	"go.uber.org/zap"

	"github.com/ShukeBta/MediaStationGo/internal/helper"
)

var (
	adultFC2Pattern                    = regexp.MustCompile(`(?i)\bFC2[-_\s]?(?:PPV[-_\s]?)?(\d{5,8})\b`)
	adultHEYZOPattern                  = regexp.MustCompile(`(?i)\bHEYZO[-_\s]?(\d{3,6})\b`)
	adultUncensoredPattern             = regexp.MustCompile(`(?i)\b(\d{6})[-_](\d{3,5})\b`)
	adultStandardPattern               = regexp.MustCompile(`(?i)(?:^|[^A-Z0-9])([A-Z]{2,10})[-_\s]?(\d{2,8})(?:[^A-Z0-9]|$)`)
	adultTitlePattern                  = regexp.MustCompile(`(?is)<h[123][^>]*>(.*?)</h[123]>`)
	adultTagPattern                    = regexp.MustCompile(`(?is)<[^>]+>`)
	adultAnchorPattern                 = regexp.MustCompile(`(?is)<a\b([^>]*)>(.*?)</a>`)
	adultImagePattern                  = regexp.MustCompile(`(?is)<img\b([^>]*)>`)
	adultJavBusCoverPattern            = regexp.MustCompile(`(?is)class="bigImage"[^>]*href="([^"]+)"`)
	adultJSONLDPattern                 = regexp.MustCompile(`(?is)<script\b[^>]*type=["']application/ld\+json["'][^>]*>(.*?)</script>`)
	adultAttrPattern                   = regexp.MustCompile(`(?is)([a-zA-Z_:][-a-zA-Z0-9_:.]*)\s*=\s*["']([^"']*)["']`)
	adultPanelBlockPattern             = regexp.MustCompile(`(?is)<div\b[^>]*class=["'][^"']*\bpanel-block\b[^"']*["'][^>]*>(.*?)</div>`)
	adultJavDBFemaleAfterAnchorPattern = regexp.MustCompile(`(?is)^\s*<strong\b[^>]*class=["'][^"']*\bfemale\b[^"']*["'][^>]*>`)
)

var adultExcludedPrefixes = map[string]struct{}{
	"AC": {}, "AAC": {}, "AVC": {}, "BD": {}, "CD": {}, "DDP": {}, "DTS": {},
	"FHD": {}, "HD": {}, "HEVC": {}, "HDR": {}, "MP": {}, "SD": {}, "UHD": {},
	"WEB": {}, "X264": {}, "X265": {},
}

const adultFlareSolverrConcurrency = 2

var defaultAdultBases = []string{
	"https://javdb.com",
	"https://onejav.com",
	"https://javbus.sbs",
	"https://www.javbus.com",
	"https://www.cdnbus.cyou",
	"https://www.javsee.cyou",
	"https://www.busjav.cyou",
}

type AdultProvider struct {
	log                    *zap.Logger
	client                 *http.Client
	apiConfig              *APIConfigService
	flareSolverrURL        string
	flareSolverrTimeout    int
	flareSolverrGateMu     sync.Mutex
	flareSolverrGate       chan struct{}
	javDBPerformerSections adultJavDBPerformerSectionCache
	javDBBrowserMu         sync.Mutex
	javDBBrowserIdentities map[string]adultJavDBBrowserIdentity
	javDBBrowserFlights    map[string]*adultJavDBBrowserFlight
	fd2ppv                 *fd2PPVClient
}

type adultJavDBBrowserIdentity struct {
	userAgent string
	cookies   []helper.FlareSolverrCookie
}

type adultJavDBBrowserFlight struct {
	done chan struct{}
	err  error
}

func NewAdultProvider(log *zap.Logger, apiConfig *APIConfigService) *AdultProvider {
	return &AdultProvider{
		log:                    log,
		apiConfig:              apiConfig,
		client:                 NewExternalHTTPClient(12 * time.Second),
		fd2ppv:                 newFD2PPVClient(),
		javDBBrowserIdentities: make(map[string]adultJavDBBrowserIdentity),
		javDBBrowserFlights:    make(map[string]*adultJavDBBrowserFlight),
		flareSolverrGate:       make(chan struct{}, adultFlareSolverrConcurrency),
	}
}

func (p *AdultProvider) SetFlareSolverr(rawURL string, timeout int) {
	if p == nil {
		return
	}
	p.flareSolverrURL = strings.TrimSpace(rawURL)
	p.flareSolverrTimeout = timeout
	p.flareSolverrGateMu.Lock()
	if p.flareSolverrGate == nil {
		p.flareSolverrGate = make(chan struct{}, adultFlareSolverrConcurrency)
	}
	p.flareSolverrGateMu.Unlock()
}

func (p *AdultProvider) Enabled() bool {
	return p != nil
}

// CheckJavDBSession keeps the shared Cloudflare browser identity warm without
// forcing a new solve while the current cookie and user agent still work.
func (p *AdultProvider) CheckJavDBSession(ctx context.Context) error {
	if p == nil || strings.TrimSpace(p.flareSolverrURL) == "" {
		return nil
	}
	items, err := p.DiscoverJavDBPopular(ctx)
	if err == nil && p.log != nil {
		p.log.Info("javdb session check succeeded", zap.Int("items", len(items)))
	}
	return err
}

func (p *AdultProvider) Search(ctx context.Context, code string) (*Match, error) {
	matches, err := p.SearchAll(ctx, code)
	if len(matches) > 0 {
		return bestAdultMatch(matches), nil
	}
	return nil, err
}

func (p *AdultProvider) SearchAll(ctx context.Context, code string) ([]*Match, error) {
	code = normalizeAdultCode(code)
	if code == "" {
		return nil, errors.New("empty adult code")
	}
	bases := p.resolveBases(ctx)
	if len(bases) == 0 {
		return nil, nil
	}
	var lastErr error
	if adultFC2Number(code) != "" && p.flareSolverrURL != "" {
		match, err := p.scrapeFD2PPV(ctx, code)
		if err != nil {
			if p.log != nil {
				p.log.Warn("fd2ppv adult scrape failed", zap.String("code", code), zap.Error(err))
			}
			return nil, err
		} else if match != nil {
			match.OriginalName = code
			match.NSFW = true
			return []*Match{match}, nil
		}
		return nil, nil
	}
	out := make([]*Match, 0, len(bases))
	seen := map[string]struct{}{}
	for _, base := range bases {
		base = strings.TrimRight(base, "/")
		var match *Match
		var err error
		switch adultSourceKind(base) {
		case "javbus":
			match, err = p.scrapeJavBus(ctx, base, code)
		case "onejav":
			match, err = p.scrapeOneJav(ctx, base, code)
		default:
			match, err = p.scrapeJavDB(ctx, base, code)
		}
		if err != nil {
			lastErr = err
			if p.log != nil {
				p.log.Debug("adult scrape source failed", zap.String("base", base), zap.String("code", code), zap.Error(err))
			}
			continue
		}
		if match != nil {
			match.OriginalName = code
			match.NSFW = true
			key := adultMatchDedupeKey(match)
			if _, ok := seen[key]; ok {
				continue
			}
			seen[key] = struct{}{}
			out = append(out, match)
		}
	}
	if len(out) > 0 {
		return out, nil
	}
	return nil, lastErr
}

func bestAdultMatch(matches []*Match) *Match {
	var best *Match
	bestScore := -1
	for _, match := range matches {
		score := adultMatchScore(match)
		if best == nil || score > bestScore {
			best = match
			bestScore = score
		}
	}
	return best
}

func adultMatchScore(match *Match) int {
	if match == nil {
		return -1
	}
	score := 0
	if strings.TrimSpace(match.Title) != "" && !strings.EqualFold(strings.TrimSpace(match.Title), strings.TrimSpace(match.OriginalName)) {
		score += 20
	}
	if strings.TrimSpace(match.PosterURL) != "" {
		score += 10
	}
	if strings.TrimSpace(match.BackdropURL) != "" {
		score += 10
	}
	if match.Year > 0 {
		score += 2
	}
	if match.Rating > 0 {
		score += 1
	}
	if len(match.Actors) > 0 {
		score += 8
	}
	return score
}

func adultMatchDedupeKey(match *Match) string {
	if match == nil {
		return ""
	}
	source := adultMatchSource(match)
	for _, value := range []string{match.OriginalName, match.Title, match.PosterURL, match.BackdropURL} {
		if key := adultCodeKey(value); key != "" {
			return source + "\x00" + key + "\x00" + strings.ToUpper(strings.TrimSpace(match.Title))
		}
	}
	return source + "\x00" + strings.ToUpper(strings.TrimSpace(match.Title+"\x00"+match.PosterURL+"\x00"+match.BackdropURL))
}

func adultMatchSource(match *Match) string {
	if match == nil {
		return ""
	}
	for _, genre := range match.Genres {
		value := strings.ToLower(strings.TrimSpace(genre))
		if value != "" && value != "adult" {
			return value
		}
	}
	return "adult"
}

func (p *AdultProvider) resolveBases(ctx context.Context) []string {
	out := append([]string{}, defaultAdultBases...)
	if p.apiConfig == nil {
		return out
	}
	resolved, err := p.apiConfig.Resolve(ctx, "adult")
	if err != nil {
		return out
	}
	if !resolved.Enabled && (resolved.BaseURL != "" || resolved.Extra != "" || resolved.APIKey != "") {
		return nil
	}
	configured := []string{}
	configured = append(configured, adultConfiguredBases(resolved.BaseURL)...)
	configured = append(configured, adultConfiguredBases(resolved.Extra)...)
	if len(configured) > 0 {
		out = append(configured, out...)
	}
	return dedupeStrings(out)
}

func (p *AdultProvider) scrapeJavDB(ctx context.Context, base, code string) (*Match, error) {
	searchURL := base + "/search?q=" + url.QueryEscape(code) + "&f=all"
	body, err := p.fetchText(ctx, searchURL, base)
	if err != nil {
		return nil, err
	}
	detail := ""
	for _, found := range adultAnchorPattern.FindAllStringSubmatch(body, -1) {
		if len(found) < 3 {
			continue
		}
		attrs := adultAttrs(found[1])
		if !strings.Contains(" "+attrs["class"]+" ", " box ") || attrs["href"] == "" {
			continue
		}
		if strings.Contains(strings.ToUpper(stripAdultHTML(found[2])), code) {
			detail = absolutizeURL(base, attrs["href"])
			break
		}
	}
	if detail == "" {
		return nil, nil
	}
	body, err = p.fetchText(ctx, detail, base)
	if err != nil {
		return nil, err
	}
	return parseAdultDetailHTML(body, code, "javdb", detail), nil
}

func (p *AdultProvider) scrapeJavBus(ctx context.Context, base, code string) (*Match, error) {
	body, err := p.fetchText(ctx, base+"/"+url.PathEscape(code), base)
	if err != nil {
		return nil, err
	}
	return parseAdultDetailHTML(body, code, "javbus", base+"/"+url.PathEscape(code)), nil
}

func (p *AdultProvider) scrapeOneJav(ctx context.Context, base, code string) (*Match, error) {
	expected := oneJavCodeKey(code)
	if expected == "" {
		return nil, nil
	}
	seen := map[string]struct{}{}
	for _, slug := range oneJavSlugs(code) {
		seen[slug] = struct{}{}
		detail := base + "/torrent/" + url.PathEscape(slug)
		body, err := p.fetchText(ctx, detail, base)
		if err != nil {
			return nil, err
		}
		if match := parseOneJavDetailHTML(body, code, detail); match != nil {
			return match, nil
		}
	}

	body, err := p.fetchText(ctx, base+"/search/"+url.PathEscape(code), base)
	if err != nil {
		return nil, err
	}
	for _, found := range adultAnchorPattern.FindAllStringSubmatch(body, -1) {
		if len(found) < 3 {
			continue
		}
		attrs := adultAttrs(found[1])
		href := attrs["href"]
		if !strings.Contains(href, "/torrent/") {
			continue
		}
		slug := strings.ToLower(strings.Trim(strings.TrimRight(href, "/")[strings.LastIndex(strings.TrimRight(href, "/"), "/")+1:], " "))
		if slug == "" {
			continue
		}
		if _, ok := seen[slug]; ok {
			continue
		}
		if oneJavCodeKey(stripAdultHTML(found[2])) != expected && oneJavCodeKey(slug) != expected {
			continue
		}
		detail := absolutizeURL(base, href)
		body, err := p.fetchText(ctx, detail, base)
		if err != nil {
			return nil, err
		}
		if match := parseOneJavDetailHTML(body, code, detail); match != nil {
			return match, nil
		}
	}
	return nil, nil
}

func (p *AdultProvider) fetchText(ctx context.Context, targetURL, referer string) (string, error) {
	host, isJavDB := adultJavDBRequestHost(targetURL)
	var identity adultJavDBBrowserIdentity
	if isJavDB {
		identity = p.javDBBrowserIdentity(host)
	}
	body, status, err := p.fetchTextDirect(ctx, targetURL, referer, identity)
	if err != nil {
		return "", err
	}
	if status == http.StatusNotFound {
		return "", nil
	}
	if p.shouldRetryAdultFetchWithFlareSolverr(status, body) {
		if isJavDB {
			if p.log != nil {
				p.log.Warn("javdb response requires Cloudflare refresh",
					zap.String("host", host),
					zap.Int("status", status),
					zap.Bool("challenge", helper.IsCloudflareChallenge(body)))
			}
			return p.refreshJavDBTextWithFlareSolverr(ctx, targetURL, referer, host)
		}
		return p.fetchTextWithFlareSolverr(ctx, targetURL)
	}
	if status >= 400 {
		return "", fmt.Errorf("adult source %s returned %d", targetURL, status)
	}
	if isJavDB && isJavDBPopularRankingURL(targetURL) && !strings.Contains(body, "/v/") {
		if p.log != nil {
			p.log.Warn("javdb ranking response has no discovery links; refreshing Cloudflare clearance",
				zap.String("host", host), zap.Int("status", status))
		}
		return p.refreshJavDBTextWithFlareSolverr(ctx, targetURL, referer, host)
	}
	return body, nil
}

func (p *AdultProvider) fetchTextDirect(
	ctx context.Context,
	targetURL string,
	referer string,
	identity adultJavDBBrowserIdentity,
) (string, int, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, targetURL, nil)
	if err != nil {
		return "", 0, err
	}
	applyAdultHeaders(req, referer)
	applyAdultBrowserIdentity(req, identity)
	resp, err := p.client.Do(req)
	if err != nil {
		return "", 0, err
	}
	defer resp.Body.Close()
	body, err := io.ReadAll(io.LimitReader(resp.Body, 4<<20))
	if err != nil {
		return "", resp.StatusCode, err
	}
	return string(body), resp.StatusCode, nil
}

func (p *AdultProvider) shouldRetryAdultFetchWithFlareSolverr(status int, body string) bool {
	if p == nil || strings.TrimSpace(p.flareSolverrURL) == "" {
		return false
	}
	return status == http.StatusForbidden || helper.IsCloudflareChallenge(body)
}

func (p *AdultProvider) fetchTextWithFlareSolverr(ctx context.Context, targetURL string) (string, error) {
	solution, err := p.fetchWithFlareSolverrResultContext(
		ctx,
		targetURL,
		nil,
	)
	if err != nil {
		return "", err
	}
	if solution == nil {
		return "", errors.New("FlareSolverr returned no solution")
	}
	if helper.IsCloudflareChallenge(solution.Response) {
		return "", errors.New("adult source returned Cloudflare challenge")
	}
	return solution.Response, nil
}

func (p *AdultProvider) refreshJavDBTextWithFlareSolverr(ctx context.Context, targetURL, referer, host string) (string, error) {
	p.clearJavDBBrowserIdentity(host)
	return p.fetchJavDBTextWithFlareSolverr(ctx, targetURL, referer, host)
}

func (p *AdultProvider) fetchJavDBTextWithFlareSolverr(
	ctx context.Context,
	targetURL string,
	referer string,
	host string,
) (string, error) {
	for attempt := 0; attempt < 2; attempt++ {
		flight, leader := p.beginJavDBBrowserFlight(host)
		if !leader {
			select {
			case <-ctx.Done():
				return "", ctx.Err()
			case <-flight.done:
			}
			if flight.err != nil {
				return "", flight.err
			}
			body, status, err := p.fetchTextDirect(ctx, targetURL, referer, p.javDBBrowserIdentity(host))
			if err != nil {
				return "", err
			}
			if status == http.StatusNotFound {
				return "", nil
			}
			if status < 400 && !p.shouldRetryAdultFetchWithFlareSolverr(status, body) {
				if isJavDBPopularRankingURL(targetURL) && !strings.Contains(body, "/v/") {
					continue
				}
				return body, nil
			}
			continue
		}

		solution, err := p.fetchWithFlareSolverrResultContext(ctx, targetURL, nil)
		if err == nil && solution == nil {
			err = errors.New("FlareSolverr returned no solution")
		}
		if err == nil && helper.IsCloudflareChallenge(solution.Response) {
			err = errors.New("adult source returned Cloudflare challenge")
		}
		if err == nil {
			p.rememberJavDBBrowserIdentity(host, solution)
		}
		p.finishJavDBBrowserFlight(host, flight, err)
		if err != nil {
			return "", err
		}
		return solution.Response, nil
	}
	return "", errors.New("JavDB Cloudflare identity was not reusable")
}

func adultJavDBRequestHost(targetURL string) (string, bool) {
	host := adultSourceHost(targetURL)
	return host, host != "" && adultSourceKind(targetURL) == "javdb"
}

func (p *AdultProvider) javDBBrowserIdentity(host string) adultJavDBBrowserIdentity {
	p.javDBBrowserMu.Lock()
	defer p.javDBBrowserMu.Unlock()
	identity := p.javDBBrowserIdentities[host]
	identity.cookies = cloneFD2PPVCookies(identity.cookies)
	return identity
}

func (p *AdultProvider) clearJavDBBrowserIdentity(host string) {
	p.javDBBrowserMu.Lock()
	defer p.javDBBrowserMu.Unlock()
	delete(p.javDBBrowserIdentities, host)
}

func isJavDBPopularRankingURL(rawURL string) bool {
	u, err := url.Parse(rawURL)
	return err == nil && strings.Contains(strings.ToLower(u.Path), "/rankings/movies")
}

func (p *AdultProvider) rememberJavDBBrowserIdentity(host string, solution *helper.FlareSolverrSolution) {
	if solution == nil {
		return
	}
	p.javDBBrowserMu.Lock()
	defer p.javDBBrowserMu.Unlock()
	if p.javDBBrowserIdentities == nil {
		p.javDBBrowserIdentities = make(map[string]adultJavDBBrowserIdentity)
	}
	current := p.javDBBrowserIdentities[host]
	current.cookies = mergeFD2PPVCookies(current.cookies, solution.Cookies)
	if strings.TrimSpace(solution.UserAgent) != "" {
		current.userAgent = strings.TrimSpace(solution.UserAgent)
	}
	p.javDBBrowserIdentities[host] = current
}

func (p *AdultProvider) beginJavDBBrowserFlight(host string) (*adultJavDBBrowserFlight, bool) {
	p.javDBBrowserMu.Lock()
	defer p.javDBBrowserMu.Unlock()
	if p.javDBBrowserFlights == nil {
		p.javDBBrowserFlights = make(map[string]*adultJavDBBrowserFlight)
	}
	if flight := p.javDBBrowserFlights[host]; flight != nil {
		return flight, false
	}
	flight := &adultJavDBBrowserFlight{done: make(chan struct{})}
	p.javDBBrowserFlights[host] = flight
	return flight, true
}

func (p *AdultProvider) finishJavDBBrowserFlight(host string, flight *adultJavDBBrowserFlight, err error) {
	p.javDBBrowserMu.Lock()
	defer p.javDBBrowserMu.Unlock()
	flight.err = err
	if p.javDBBrowserFlights[host] == flight {
		delete(p.javDBBrowserFlights, host)
	}
	close(flight.done)
}

func (p *AdultProvider) fetchWithFlareSolverrResultContext(
	ctx context.Context,
	targetURL string,
	cookies []helper.FlareSolverrCookie,
) (*helper.FlareSolverrSolution, error) {
	if p == nil {
		return nil, errors.New("adult provider is unavailable")
	}
	if p.flareSolverrGate != nil {
		select {
		case p.flareSolverrGate <- struct{}{}:
			defer func() { <-p.flareSolverrGate }()
		case <-ctx.Done():
			return nil, ctx.Err()
		}
	}
	return helper.FetchURLWithFlareSolverrResultContext(
		ctx,
		p.flareSolverrURL,
		targetURL,
		cookies,
		p.flareSolverrTimeout,
		"",
		p.log,
	)
}

func applyAdultHeaders(req *http.Request, referer string) {
	req.Header.Set("User-Agent", "Mozilla/5.0 (Windows NT 10.0; Win64; x64) AppleWebKit/537.36 (KHTML, like Gecko) Chrome/125 Safari/537.36")
	req.Header.Set("Accept", "text/html,application/xhtml+xml,application/xml;q=0.9,image/avif,image/webp,*/*;q=0.8")
	req.Header.Set("Accept-Language", "zh-CN,zh;q=0.9,ja;q=0.8,en;q=0.7")
	if referer != "" {
		req.Header.Set("Referer", referer)
	}
}

func applyAdultBrowserIdentity(req *http.Request, identity adultJavDBBrowserIdentity) {
	if strings.TrimSpace(identity.userAgent) != "" {
		req.Header.Set("User-Agent", identity.userAgent)
	}
	host := strings.ToLower(req.URL.Hostname())
	for _, cookie := range identity.cookies {
		if strings.TrimSpace(cookie.Name) == "" || !adultCookieDomainMatchesHost(cookie.Domain, host) {
			continue
		}
		req.AddCookie(&http.Cookie{Name: cookie.Name, Value: cookie.Value})
	}
}

func adultCookieDomainMatchesHost(domain, host string) bool {
	domain = strings.TrimPrefix(strings.ToLower(strings.TrimSpace(domain)), ".")
	host = strings.ToLower(strings.TrimSpace(host))
	return domain == "" || domain == host || strings.HasSuffix(host, "."+domain)
}
