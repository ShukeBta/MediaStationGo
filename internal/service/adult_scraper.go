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

	"github.com/ShukeBta/MediaStationGo/internal/repository"
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
	"WEB": {}, "X264": {}, "X265": {}, "H264": {}, "H265": {}, "AV1": {},
	"SEASON": {}, "EPISODE": {}, "EP": {}, "SP": {}, "OVA": {}, "OAD": {},
	"TMDB": {}, "TMDBID": {}, "DOUBAN": {}, "BANGUMI": {}, "BGM": {}, "TVDB": {}, "THETVDB": {},
	"PART": {}, "VOL": {}, "DISC": {}, "DISK": {}, "TRACK": {}, "CHAPTER": {}, "SAMPLE": {}, "TRAILER": {},
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
	repo      *repository.Container
	metatube  *MetaTubeProvider
}

func NewAdultProvider(log *zap.Logger, apiConfig *APIConfigService, repos ...*repository.Container) *AdultProvider {
	var repo *repository.Container
	if len(repos) > 0 {
		repo = repos[0]
	}
	return &AdultProvider{
		log:       log,
		apiConfig: apiConfig,
		repo:      repo,
		metatube:  NewMetaTubeProvider(log),
		client:    NewExternalHTTPClient(12 * time.Second),
	}
}

func (p *AdultProvider) Enabled() bool {
	return p != nil
}

// MetaTubeClient 返回 MetaTube 客户端实例。
func (p *AdultProvider) MetaTubeClient() *MetaTubeProvider {
	if p == nil {
		return nil
	}
	return p.metatube
}

// ResolveMetaTubeConfig 解析当前生效的 MetaTube 配置。
func (p *AdultProvider) ResolveMetaTubeConfig(ctx context.Context) MetaTubeConfig {
	serverURL := p.getSetting(ctx, "adult.scraper.metatube_server", "")
	token := p.getSetting(ctx, "adult.scraper.metatube_token", "")
	provider := p.getSetting(ctx, "adult.scraper.metatube_provider", "")
	enableActor := p.getSettingBool(ctx, "adult.scraper.enable_actor", true)
	enableTrailer := p.getSettingBool(ctx, "adult.scraper.enable_trailer", false)
	cropCover := p.getSettingBool(ctx, "adult.scraper.crop_cover", true)
	badge := p.getSettingBool(ctx, "adult.scraper.badge", false)

	if serverURL == "" && p.apiConfig != nil {
		if resolved, err := p.apiConfig.Resolve(ctx, "metatube"); err == nil && resolved.BaseURL != "" {
			serverURL = resolved.BaseURL
			if token == "" {
				token = resolved.APIKey
			}
		}
	}

	return MetaTubeConfig{
		ServerURL:       serverURL,
		Token:           token,
		DefaultProvider: provider,
		EnableActor:     enableActor,
		EnableTrailer:   enableTrailer,
		CropCover:       cropCover,
		Badge:           badge,
	}
}

// Search 执行单个番号刮削（根据配置自动在 MetaTube、内置源或智能混合之间路由）。
func (p *AdultProvider) Search(ctx context.Context, code string) (*Match, error) {
	code = normalizeAdultCode(code)
	if code == "" {
		return nil, errors.New("empty adult code")
	}

	engine := strings.ToLower(p.getSetting(ctx, "adult.scraper.engine", "builtin"))
	switch engine {
	case "metatube":
		mtCfg := p.ResolveMetaTubeConfig(ctx)
		if mtCfg.ServerURL != "" {
			match, err := p.metatube.SearchAndGetBestMatch(ctx, mtCfg, code)
			if err != nil {
				if p.log != nil {
					p.log.Warn("metatube scrape failed", zap.String("code", code), zap.Error(err))
				}
				return nil, err
			}
			if match != nil {
				match.OriginalName = code
				match.Title = FormatAdultTitle(code, match.Title)
				match.NSFW = true
				return match, nil
			}
		}
		return nil, nil

	case "auto":
		mtCfg := p.ResolveMetaTubeConfig(ctx)
		if mtCfg.ServerURL != "" {
			match, err := p.metatube.SearchAndGetBestMatch(ctx, mtCfg, code)
			if err == nil && match != nil {
				match.OriginalName = code
				match.Title = FormatAdultTitle(code, match.Title)
				match.NSFW = true
				return match, nil
			}
			if p.log != nil && err != nil {
				p.log.Debug("metatube search in auto mode failed, falling back to builtin", zap.String("code", code), zap.Error(err))
			}
		}
		return p.searchBuiltin(ctx, code)

	default: // "builtin"
		return p.searchBuiltin(ctx, code)
	}
}

// SearchCandidates 执行搜索并返回所有候选 Match 列表（用于手动刮削弹窗）。
func (p *AdultProvider) SearchCandidates(ctx context.Context, query string) ([]*Match, error) {
	code := normalizeAdultCode(query)
	if code == "" {
		code = strings.TrimSpace(query)
	}
	if code == "" {
		return nil, errors.New("empty adult query")
	}

	engine := strings.ToLower(p.getSetting(ctx, "adult.scraper.engine", "builtin"))
	switch engine {
	case "metatube":
		mtCfg := p.ResolveMetaTubeConfig(ctx)
		if mtCfg.ServerURL != "" {
			matches, err := p.metatube.Search(ctx, mtCfg, code)
			if err != nil {
				return nil, err
			}
			for _, m := range matches {
				m.OriginalName = code
				m.Title = FormatAdultTitle(code, m.Title)
				m.NSFW = true
			}
			return matches, nil
		}
		return nil, nil

	case "auto":
		mtCfg := p.ResolveMetaTubeConfig(ctx)
		if mtCfg.ServerURL != "" {
			matches, err := p.metatube.Search(ctx, mtCfg, code)
			if err == nil && len(matches) > 0 {
				for _, m := range matches {
					m.OriginalName = code
					m.Title = FormatAdultTitle(code, m.Title)
					m.NSFW = true
				}
				return matches, nil
			}
		}
		m, err := p.searchBuiltin(ctx, code)
		if err != nil || m == nil {
			return nil, err
		}
		return []*Match{m}, nil

	default:
		m, err := p.searchBuiltin(ctx, code)
		if err != nil || m == nil {
			return nil, err
		}
		return []*Match{m}, nil
	}
}

func (p *AdultProvider) searchBuiltin(ctx context.Context, code string) (*Match, error) {
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
			match.Title = FormatAdultTitle(code, match.Title)
			match.NSFW = true
			return match, nil
		}
	}
	return nil, lastErr
}

func (p *AdultProvider) resolveBases(ctx context.Context) []string {
	customJavDB := p.getSetting(ctx, "adult.scraper.builtin_javdb_url", "")
	customJavBus := p.getSetting(ctx, "adult.scraper.builtin_javbus_url", "")

	configured := []string{}
	if customJavDB != "" {
		configured = append(configured, adultConfiguredBases(customJavDB)...)
	}
	if customJavBus != "" {
		configured = append(configured, adultConfiguredBases(customJavBus)...)
	}

	out := append([]string{}, defaultAdultBases...)
	if p.apiConfig != nil {
		if resolved, err := p.apiConfig.Resolve(ctx, "adult"); err == nil {
			if !resolved.Enabled && (resolved.BaseURL != "" || resolved.Extra != "" || resolved.APIKey != "") {
				return nil
			}
			configured = append(configured, adultConfiguredBases(resolved.BaseURL)...)
			configured = append(configured, adultConfiguredBases(resolved.Extra)...)
		}
	}
	if len(configured) > 0 {
		out = append(configured, out...)
	}
	return dedupeStrings(out)
}

const defaultAdultCookies = "age=verified; existmag=all"

func (p *AdultProvider) resolveCookie(ctx context.Context) string {
	customCookie := p.getSetting(ctx, "adult.scraper.builtin_cookie", "")
	if customCookie != "" {
		if !strings.Contains(customCookie, "age=") {
			customCookie = customCookie + "; age=verified"
		}
		if !strings.Contains(customCookie, "existmag=") {
			customCookie = customCookie + "; existmag=all"
		}
		return customCookie
	}
	if p == nil || p.apiConfig == nil {
		return defaultAdultCookies
	}
	resolved, err := p.apiConfig.Resolve(ctx, "adult")
	if err != nil || strings.TrimSpace(resolved.APIKey) == "" {
		return defaultAdultCookies
	}
	userCookie := strings.TrimSpace(resolved.APIKey)
	if !strings.Contains(userCookie, "age=") {
		userCookie = userCookie + "; age=verified"
	}
	if !strings.Contains(userCookie, "existmag=") {
		userCookie = userCookie + "; existmag=all"
	}
	return userCookie
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
	cookie := p.resolveCookie(ctx)
	applyAdultHeaders(req, referer, cookie)
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
	text := string(body)
	if strings.Contains(text, "driver-verify") || strings.Contains(text, "Age Verification") || strings.Contains(text, "你是否已經成年") {
		return "", fmt.Errorf("adult source %s intercepted by age verification", targetURL)
	}
	return text, nil
}

func applyAdultHeaders(req *http.Request, referer, cookie string) {
	req.Header.Set("User-Agent", "Mozilla/5.0 (Windows NT 10.0; Win64; x64) AppleWebKit/537.36 (KHTML, like Gecko) Chrome/125 Safari/537.36")
	req.Header.Set("Accept", "text/html,application/xhtml+xml,application/xml;q=0.9,image/avif,image/webp,*/*;q=0.8")
	req.Header.Set("Accept-Language", "zh-CN,zh;q=0.9,ja;q=0.8,en;q=0.7")
	if cookie != "" {
		req.Header.Set("Cookie", cookie)
	}
	if referer != "" {
		req.Header.Set("Referer", referer)
	}
}
