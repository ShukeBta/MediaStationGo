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
	"time"

	"go.uber.org/zap"
)

var (
	adultFC2Pattern         = regexp.MustCompile(`(?i)\bFC2[-_\s]?(?:PPV[-_\s]?)?(\d{5,8})\b`)
	adultHEYZOPattern       = regexp.MustCompile(`(?i)\bHEYZO[-_\s]?(\d{3,6})\b`)
	adultUncensoredPattern  = regexp.MustCompile(`(?i)\b(\d{6})[-_](\d{3,5})\b`)
	adultStandardPattern    = regexp.MustCompile(`(?i)(?:^|[^A-Z0-9])([A-Z]{2,10})[-_\s]?(\d{2,8})(?:[^A-Z0-9]|$)`)
	adultTitlePattern       = regexp.MustCompile(`(?is)<h[123][^>]*>(.*?)</h[123]>`)
	adultTagPattern         = regexp.MustCompile(`(?is)<[^>]+>`)
	adultAnchorPattern      = regexp.MustCompile(`(?is)<a\b([^>]*)>(.*?)</a>`)
	adultImagePattern       = regexp.MustCompile(`(?is)<img\b([^>]*)>`)
	adultJavBusCoverPattern = regexp.MustCompile(`(?is)class="bigImage"[^>]*href="([^"]+)"`)
	adultSamplePattern      = regexp.MustCompile(`(?is)<a[^>]+class="[^"]*\bsample-box\b[^"]*"[^>]+href="([^"]+)"`)
	adultAttrPattern        = regexp.MustCompile(`(?is)([a-zA-Z_:][-a-zA-Z0-9_:.]*)\s*=\s*["']([^"']*)["']`)
)

var adultExcludedPrefixes = map[string]struct{}{
	"AC": {}, "AAC": {}, "AVC": {}, "BD": {}, "CD": {}, "DDP": {}, "DTS": {},
	"FHD": {}, "HD": {}, "HEVC": {}, "HDR": {}, "MP": {}, "SD": {}, "UHD": {},
	"WEB": {}, "X264": {}, "X265": {},
}

var defaultAdultBases = []string{
	"https://javdb.com",
	"https://javbus.sbs",
	"https://www.javbus.com",
	"https://www.cdnbus.cyou",
	"https://www.javsee.cyou",
	"https://www.busjav.cyou",
}

type AdultProvider struct {
	log       *zap.Logger
	client    *http.Client
	apiConfig *APIConfigService
}

func NewAdultProvider(log *zap.Logger, apiConfig *APIConfigService) *AdultProvider {
	return &AdultProvider{
		log:       log,
		apiConfig: apiConfig,
		client:    NewExternalHTTPClient(12 * time.Second),
	}
}

func (p *AdultProvider) Enabled() bool {
	return p != nil
}

func (p *AdultProvider) Search(ctx context.Context, code string) (*Match, error) {
	code = normalizeAdultCode(code)
	if code == "" {
		return nil, errors.New("empty adult code")
	}
	bases := p.resolveBases(ctx)
	if len(bases) == 0 {
		return nil, nil
	}
	var lastErr error
	for _, base := range bases {
		base = strings.TrimRight(base, "/")
		var match *Match
		var err error
		if adultSourceKind(base) == "javbus" {
			match, err = p.scrapeJavBus(ctx, base, code)
		} else {
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
			return match, nil
		}
	}
	return nil, lastErr
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

func (p *AdultProvider) fetchText(ctx context.Context, targetURL, referer string) (string, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, targetURL, nil)
	if err != nil {
		return "", err
	}
	applyAdultHeaders(req, referer)
	resp, err := p.client.Do(req)
	if err != nil {
		return "", err
	}
	defer resp.Body.Close()
	if resp.StatusCode == http.StatusNotFound {
		return "", nil
	}
	if resp.StatusCode >= 400 {
		return "", fmt.Errorf("adult source %s returned %d", targetURL, resp.StatusCode)
	}
	body, err := io.ReadAll(io.LimitReader(resp.Body, 4<<20))
	if err != nil {
		return "", err
	}
	return string(body), nil
}

func applyAdultHeaders(req *http.Request, referer string) {
	req.Header.Set("User-Agent", "Mozilla/5.0 (Windows NT 10.0; Win64; x64) AppleWebKit/537.36 (KHTML, like Gecko) Chrome/125 Safari/537.36")
	req.Header.Set("Accept", "text/html,application/xhtml+xml,application/xml;q=0.9,image/avif,image/webp,*/*;q=0.8")
	req.Header.Set("Accept-Language", "zh-CN,zh;q=0.9,ja;q=0.8,en;q=0.7")
	if referer != "" {
		req.Header.Set("Referer", referer)
	}
}
