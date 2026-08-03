package service

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/url"
	"sort"
	"strconv"
	"strings"
	"sync"
	"time"

	"go.uber.org/zap"

	"github.com/ShukeBta/MediaStationGo/internal/model"
)

type SiteCategory struct {
	ID          string `json:"id"`
	Name        string `json:"name"`
	Group       string `json:"group"`
	ParentID    string `json:"parent_id,omitempty"`
	SiteID      string `json:"site_id,omitempty"`
	SiteName    string `json:"site_name,omitempty"`
	SiteType    string `json:"site_type,omitempty"`
	Adult       bool   `json:"adult"`
	Description string `json:"description,omitempty"`
}

type SiteBrowseParams struct {
	SiteID       string
	Keyword      string
	Category     string
	Page         int
	IncludeAdult bool
}

type SiteBrowseResult struct {
	Items      []SearchResult `json:"items"`
	Total      int            `json:"total"`
	Page       int            `json:"page"`
	PageSize   int            `json:"page_size"`
	TotalPages int            `json:"total_pages"`
	Category   string         `json:"category,omitempty"`
	Keyword    string         `json:"keyword,omitempty"`
}

type siteCategorySearcher interface {
	SearchWithCategory(ctx context.Context, cfg SiteConfig, keyword, category string, page int) (*SiteSearchResult, error)
}

type siteCategoryModeSearcher interface {
	SearchWithCategoryMode(ctx context.Context, cfg SiteConfig, keyword, category string, page int, includeAdult bool) (*SiteSearchResult, error)
}

type siteBrowseModeAdapter interface {
	BrowseWithMode(ctx context.Context, cfg SiteConfig, category string, page int, includeAdult bool) (*SiteSearchResult, error)
}

type siteCategoryProvider interface {
	Categories(ctx context.Context, cfg SiteConfig) ([]SiteCategory, error)
}

const siteBrowsePageSize = 50

const (
	sitePortalDefaultCacheTTL    = 45 * time.Second
	sitePortalRateLimitCacheTTL  = 2 * time.Minute
	sitePortalDetailCacheTTL     = 10 * time.Minute
	sitePortalDetailStaleTTL     = 2 * time.Hour
	sitePortalStaleCacheTTL      = 15 * time.Minute
	sitePortalRateLimitCooldown  = 3 * time.Minute
	sitePortalMTeamMinInterval   = 10 * time.Second
	sitePortalMTeamMinTimeout    = 60 * time.Second
	sitePortalGenericMinInterval = 1500 * time.Millisecond
)

type sitePortalCacheEntry struct {
	Result    *SiteSearchResult
	Cats      []SiteCategory
	Detail    *TorrentDetail
	ExpiresAt time.Time
	StaleAt   time.Time
}

type sitePortalCooldownEntry struct {
	Message string
	Until   time.Time
}

func (s *SiteService) Categories(ctx context.Context, siteID string) ([]SiteCategory, error) {
	sites, err := s.List(ctx)
	if err != nil {
		return nil, err
	}
	byKey := map[string]SiteCategory{}
	for _, site := range sites {
		if siteID != "" && site.ID != siteID {
			continue
		}
		if !site.Enabled {
			continue
		}
		cats := s.originalCategoriesForSite(ctx, site)
		for _, cat := range cats {
			key := strings.ToLower(strings.Join([]string{cat.SiteType, cat.ID, cat.Name}, "\x00"))
			if _, ok := byKey[key]; !ok {
				byKey[key] = cat
			}
		}
	}
	out := make([]SiteCategory, 0, len(byKey))
	for _, cat := range byKey {
		out = append(out, cat)
	}
	sort.SliceStable(out, func(i, j int) bool {
		if out[i].Adult != out[j].Adult {
			return !out[i].Adult
		}
		if out[i].Group != out[j].Group {
			return out[i].Group < out[j].Group
		}
		return out[i].Name < out[j].Name
	})
	return out, nil
}

func (s *SiteService) originalCategoriesForSite(ctx context.Context, site model.Site) []SiteCategory {
	if cats, ok := s.getCachedSiteCategories(site, false); ok {
		return cats
	}
	adapter := NewSiteAdapter(&site)
	if provider, ok := adapter.(siteCategoryProvider); ok {
		cfg := s.siteModelToConfig(&site)
		timeout := sitePortalRequestTimeout(site)
		cfg.Timeout = timeout
		if err := s.waitSitePortalRequest(ctx, site); err != nil {
			s.log.Warn("site categories throttled", zap.String("site", site.Name), zap.String("type", site.Type), zap.Error(err))
			if cats, ok := s.getCachedSiteCategories(site, true); ok {
				return cats
			}
			cats := normalizeSiteCategories(site, siteCategoriesForSite(site))
			return cats
		}
		reqCtx, cancel := context.WithTimeout(ctx, timeout)
		defer cancel()
		cats, err := provider.Categories(reqCtx, cfg)
		if err == nil && len(cats) > 0 {
			cats = normalizeSiteCategories(site, cats)
			s.storeSiteCategories(site, cats)
			return cats
		}
		if err != nil {
			s.log.Warn("site categories failed", zap.String("site", site.Name), zap.String("type", site.Type), zap.Error(err))
			if cats, ok := s.getCachedSiteCategories(site, true); ok {
				return cats
			}
		}
	}
	cats := normalizeSiteCategories(site, siteCategoriesForSite(site))
	s.storeSiteCategories(site, cats)
	return cats
}

func (s *SiteService) Browse(ctx context.Context, p SiteBrowseParams) (*SiteBrowseResult, error) {
	if p.Page <= 0 {
		p.Page = 1
	}
	p.Keyword = strings.TrimSpace(p.Keyword)
	p.Category = strings.TrimSpace(p.Category)
	sites, err := s.List(ctx)
	if err != nil {
		return nil, err
	}

	var (
		mu         sync.Mutex
		wg         sync.WaitGroup
		results    []SearchResult
		total      int
		matched    int
		failed     int
		lastErrMsg string
	)
	for i := range sites {
		site := sites[i]
		if !site.Enabled {
			continue
		}
		if p.SiteID != "" && site.ID != p.SiteID {
			continue
		}
		matched++
		wg.Add(1)
		go func(site model.Site) {
			defer wg.Done()
			adapter := NewSiteAdapter(&site)
			if adapter == nil {
				return
			}
			cfg := s.siteModelToConfig(&site)
			cfg.Timeout = sitePortalRequestTimeout(site)

			result, err := s.cachedBrowseSiteResources(ctx, site, adapter, cfg, p.Keyword, p.Category, p.Page, p.IncludeAdult)
			if err != nil {
				mu.Lock()
				failed++
				lastErrMsg = err.Error()
				mu.Unlock()
				s.log.Warn("site browse failed",
					zap.String("site", site.Name),
					zap.String("type", site.Type),
					zap.String("category", p.Category),
					zap.String("keyword", p.Keyword),
					zap.Error(err))
				return
			}
			if result == nil {
				return
			}
			cats := s.cachedOrFallbackSiteCategories(site)
			catAdult := siteCategoryIsAdultFromCategories(cats, p.Category)
			items := result.Items
			if items == nil {
				items = []TorrentItem{}
			}
			local := make([]SearchResult, 0, len(items))
			for _, item := range items {
				rawCategory := item.Category
				item.Category = siteCategoryDisplayName(cats, item.Category)
				row := siteSearchResultFromItemWithCategories(site, item, catAdult, cats)
				if !siteCategoryMatchesCategories(cats, rawCategory, p.Category) {
					continue
				}
				if !p.IncludeAdult && row.Adult {
					continue
				}
				local = append(local, row)
			}
			mu.Lock()
			results = append(results, local...)
			total += result.Total
			mu.Unlock()
		}(site)
	}
	wg.Wait()
	sort.SliceStable(results, func(i, j int) bool {
		if results[i].Seeders != results[j].Seeders {
			return results[i].Seeders > results[j].Seeders
		}
		if results[i].UploadTime != results[j].UploadTime {
			return results[i].UploadTime > results[j].UploadTime
		}
		return results[i].Size > results[j].Size
	})
	if results == nil {
		results = []SearchResult{}
	}
	if len(results) == 0 && failed > 0 && failed >= matched {
		return nil, fmt.Errorf("site browse failed: %s", lastErrMsg)
	}
	if total == 0 {
		total = len(results)
	}
	totalPages := 0
	if total > 0 {
		totalPages = (total + siteBrowsePageSize - 1) / siteBrowsePageSize
	}
	return &SiteBrowseResult{
		Items:      results,
		Total:      total,
		Page:       p.Page,
		PageSize:   siteBrowsePageSize,
		TotalPages: totalPages,
		Category:   p.Category,
		Keyword:    p.Keyword,
	}, nil
}

func browseSiteResources(ctx context.Context, adapter SiteAdapter, cfg SiteConfig, keyword, category string, page int, includeAdult bool) (*SiteSearchResult, error) {
	if strings.TrimSpace(keyword) != "" {
		if searcher, ok := adapter.(siteCategoryModeSearcher); ok {
			return searcher.SearchWithCategoryMode(ctx, cfg, keyword, category, page, includeAdult)
		}
		if strings.TrimSpace(category) != "" {
			if searcher, ok := adapter.(siteCategorySearcher); ok {
				return searcher.SearchWithCategory(ctx, cfg, keyword, category, page)
			}
		}
		return adapter.Search(ctx, cfg, keyword, page)
	}
	if browser, ok := adapter.(siteBrowseModeAdapter); ok {
		return browser.BrowseWithMode(ctx, cfg, category, page, includeAdult)
	}
	return adapter.Browse(ctx, cfg, category, page)
}

func (s *SiteService) cachedBrowseSiteResources(ctx context.Context, site model.Site, adapter SiteAdapter, cfg SiteConfig, keyword, category string, page int, includeAdult bool) (*SiteSearchResult, error) {
	modeCategory := category
	if includeAdult {
		modeCategory = modeCategory + "\x00adult"
	}
	key := sitePortalCacheKey("browse", site, keyword, modeCategory, page)
	if result, ok := s.getCachedSiteBrowse(key, false); ok {
		return result, nil
	}
	if err := s.sitePortalCooldownError(site); err != nil {
		if result, ok := s.getCachedSiteBrowse(key, true); ok {
			return result, nil
		}
		return nil, err
	}
	if err := s.waitSitePortalRequest(ctx, site); err != nil {
		if result, ok := s.getCachedSiteBrowse(key, true); ok {
			return result, nil
		}
		return nil, err
	}

	timeout := sitePortalRequestTimeout(site)
	cfg.Timeout = timeout
	reqCtx, cancel := context.WithTimeout(ctx, timeout)
	defer cancel()
	result, err := browseSiteResources(reqCtx, adapter, cfg, keyword, category, page, includeAdult)
	if err != nil {
		if isSitePortalRateLimitError(err) {
			s.markSitePortalCooldown(site, err)
			if result, ok := s.getCachedSiteBrowse(key, true); ok {
				return result, nil
			}
		}
		return nil, err
	}
	s.storeSiteBrowse(key, site, result)
	return result, nil
}

func sitePortalCacheKey(kind string, site model.Site, keyword, category string, page int) string {
	siteKey := site.ID
	if siteKey == "" {
		siteKey = site.Type + ":" + site.URL
	}
	return strings.Join([]string{
		kind,
		siteKey,
		strings.ToLower(strings.TrimSpace(keyword)),
		strings.ToLower(strings.TrimSpace(category)),
		strconv.Itoa(page),
	}, "\x00")
}

func (s *SiteService) getCachedSiteBrowse(key string, allowStale bool) (*SiteSearchResult, bool) {
	if s == nil {
		return nil, false
	}
	now := time.Now()
	s.portalMu.Lock()
	defer s.portalMu.Unlock()
	entry, ok := s.portalCache[key]
	if !ok || entry.Result == nil {
		return nil, false
	}
	if now.Before(entry.ExpiresAt) || (allowStale && now.Before(entry.StaleAt)) {
		return cloneSiteSearchResult(entry.Result), true
	}
	return nil, false
}

func (s *SiteService) storeSiteBrowse(key string, site model.Site, result *SiteSearchResult) {
	if s == nil || result == nil {
		return
	}
	now := time.Now()
	ttl := sitePortalDefaultCacheTTL
	if site.RateLimit || strings.EqualFold(site.Type, "mteam") {
		ttl = sitePortalRateLimitCacheTTL
	}
	s.portalMu.Lock()
	defer s.portalMu.Unlock()
	s.ensureSitePortalStateLocked()
	s.portalCache[key] = sitePortalCacheEntry{
		Result:    cloneSiteSearchResult(result),
		ExpiresAt: now.Add(ttl),
		StaleAt:   now.Add(sitePortalStaleCacheTTL),
	}
	s.pruneSitePortalCacheLocked(now)
}

func (s *SiteService) getCachedSiteCategories(site model.Site, allowStale bool) ([]SiteCategory, bool) {
	if s == nil {
		return nil, false
	}
	key := sitePortalCacheKey("categories", site, "", "", 0)
	now := time.Now()
	s.portalMu.Lock()
	defer s.portalMu.Unlock()
	entry, ok := s.portalCache[key]
	if !ok || len(entry.Cats) == 0 {
		return nil, false
	}
	if now.After(entry.ExpiresAt) && (!allowStale || now.After(entry.StaleAt)) {
		return nil, false
	}
	return cloneSiteCategories(entry.Cats), true
}

func (s *SiteService) getCachedSiteDetail(site model.Site, torrentID string, allowStale bool) (*TorrentDetail, bool) {
	if s == nil {
		return nil, false
	}
	key := sitePortalCacheKey("detail", site, "", torrentID, 0)
	now := time.Now()
	s.portalMu.Lock()
	defer s.portalMu.Unlock()
	entry, ok := s.portalCache[key]
	if !ok || entry.Detail == nil {
		return nil, false
	}
	if now.After(entry.ExpiresAt) && (!allowStale || now.After(entry.StaleAt)) {
		return nil, false
	}
	return cloneTorrentDetail(entry.Detail), true
}

func (s *SiteService) storeSiteDetail(site model.Site, torrentID string, detail *TorrentDetail) {
	if s == nil || detail == nil {
		return
	}
	key := sitePortalCacheKey("detail", site, "", torrentID, 0)
	now := time.Now()
	s.portalMu.Lock()
	defer s.portalMu.Unlock()
	s.ensureSitePortalStateLocked()
	s.portalCache[key] = sitePortalCacheEntry{
		Detail:    cloneTorrentDetail(detail),
		ExpiresAt: now.Add(sitePortalDetailCacheTTL),
		StaleAt:   now.Add(sitePortalDetailStaleTTL),
	}
	s.pruneSitePortalCacheLocked(now)
}

func (s *SiteService) storeSiteCategories(site model.Site, cats []SiteCategory) {
	if s == nil || len(cats) == 0 {
		return
	}
	key := sitePortalCacheKey("categories", site, "", "", 0)
	now := time.Now()
	s.portalMu.Lock()
	defer s.portalMu.Unlock()
	s.ensureSitePortalStateLocked()
	s.portalCache[key] = sitePortalCacheEntry{
		Cats:      cloneSiteCategories(cats),
		ExpiresAt: now.Add(6 * time.Hour),
		StaleAt:   now.Add(48 * time.Hour),
	}
	s.pruneSitePortalCacheLocked(now)
}

func (s *SiteService) waitSitePortalRequest(ctx context.Context, site model.Site) error {
	interval := sitePortalMinInterval(site)
	if interval <= 0 || s == nil {
		return nil
	}
	siteKey := site.ID
	if siteKey == "" {
		siteKey = site.Type + ":" + site.URL
	}
	now := time.Now()
	s.portalMu.Lock()
	s.ensureSitePortalStateLocked()
	waitUntil := s.portalNext[siteKey]
	if waitUntil.Before(now) {
		waitUntil = now
	}
	s.portalNext[siteKey] = waitUntil.Add(interval)
	s.portalMu.Unlock()

	if delay := time.Until(waitUntil); delay > 0 {
		timer := time.NewTimer(delay)
		defer timer.Stop()
		select {
		case <-ctx.Done():
			return ctx.Err()
		case <-timer.C:
		}
	}
	return nil
}

func sitePortalMinInterval(site model.Site) time.Duration {
	if strings.EqualFold(site.Type, "mteam") {
		return sitePortalMTeamMinInterval
	}
	if site.RateLimit {
		return sitePortalGenericMinInterval
	}
	return 0
}

func sitePortalRequestTimeout(site model.Site) time.Duration {
	timeout := time.Duration(site.Timeout) * time.Second
	if timeout <= 0 {
		timeout = 30 * time.Second
	}
	if strings.EqualFold(site.Type, "mteam") && timeout < sitePortalMTeamMinTimeout {
		return sitePortalMTeamMinTimeout
	}
	return timeout
}

func (s *SiteService) sitePortalCooldownError(site model.Site) error {
	if s == nil {
		return nil
	}
	siteKey := site.ID
	if siteKey == "" {
		siteKey = site.Type + ":" + site.URL
	}
	now := time.Now()
	s.portalMu.Lock()
	defer s.portalMu.Unlock()
	entry, ok := s.portalCooldown[siteKey]
	if !ok || now.After(entry.Until) {
		if ok {
			delete(s.portalCooldown, siteKey)
		}
		return nil
	}
	remaining := int(time.Until(entry.Until).Seconds())
	if remaining < 1 {
		remaining = 1
	}
	message := entry.Message
	if message == "" {
		message = "请求过于频繁"
	}
	return fmt.Errorf("%s: %s，已进入冷却，约 %d 秒后再试", site.Name, message, remaining)
}

func (s *SiteService) markSitePortalCooldown(site model.Site, err error) {
	if s == nil || err == nil {
		return
	}
	siteKey := site.ID
	if siteKey == "" {
		siteKey = site.Type + ":" + site.URL
	}
	s.portalMu.Lock()
	defer s.portalMu.Unlock()
	s.ensureSitePortalStateLocked()
	s.portalCooldown[siteKey] = sitePortalCooldownEntry{
		Message: strings.TrimSpace(err.Error()),
		Until:   time.Now().Add(sitePortalRateLimitCooldown),
	}
}

func isSitePortalRateLimitError(err error) bool {
	if err == nil {
		return false
	}
	text := strings.ToLower(err.Error())
	for _, needle := range []string{
		"429",
		"too many",
		"too frequent",
		"rate limit",
		"请求过于频繁",
		"请求太频繁",
		"請求過於頻繁",
		"頻繁",
		"频繁",
	} {
		if strings.Contains(text, strings.ToLower(needle)) {
			return true
		}
	}
	return false
}

func (s *SiteService) ensureSitePortalStateLocked() {
	if s.portalNext == nil {
		s.portalNext = map[string]time.Time{}
	}
	if s.portalCache == nil {
		s.portalCache = map[string]sitePortalCacheEntry{}
	}
	if s.portalCooldown == nil {
		s.portalCooldown = map[string]sitePortalCooldownEntry{}
	}
}

func (s *SiteService) pruneSitePortalCacheLocked(now time.Time) {
	if len(s.portalCache) < 256 {
		return
	}
	for key, entry := range s.portalCache {
		if now.After(entry.StaleAt) {
			delete(s.portalCache, key)
		}
	}
}

func cloneSiteSearchResult(in *SiteSearchResult) *SiteSearchResult {
	if in == nil {
		return nil
	}
	out := *in
	if in.Items != nil {
		out.Items = append([]TorrentItem(nil), in.Items...)
	}
	return &out
}

func cloneSiteCategories(in []SiteCategory) []SiteCategory {
	if in == nil {
		return nil
	}
	return append([]SiteCategory(nil), in...)
}

func cloneTorrentDetail(in *TorrentDetail) *TorrentDetail {
	if in == nil {
		return nil
	}
	out := *in
	if in.FreeEndAt != nil {
		t := *in.FreeEndAt
		out.FreeEndAt = &t
	}
	if in.Genres != nil {
		out.Genres = append([]string(nil), in.Genres...)
	}
	if in.Tags != nil {
		out.Tags = append([]string(nil), in.Tags...)
	}
	if in.Images != nil {
		out.Images = append([]string(nil), in.Images...)
	}
	if in.Files != nil {
		out.Files = append([]string(nil), in.Files...)
	}
	return &out
}

func (s *SiteService) Detail(ctx context.Context, siteID, torrentID string) (*TorrentDetail, error) {
	siteID = strings.TrimSpace(siteID)
	torrentID = strings.TrimSpace(torrentID)
	if siteID == "" || torrentID == "" {
		return nil, errors.New("site_id and id required")
	}
	site, err := s.FindByID(ctx, siteID)
	if err != nil || site == nil {
		if err != nil {
			return nil, err
		}
		return nil, errors.New("site not found")
	}
	if detail, ok := s.getCachedSiteDetail(*site, torrentID, false); ok {
		return detail, nil
	}
	if err := s.sitePortalCooldownError(*site); err != nil {
		if detail, ok := s.getCachedSiteDetail(*site, torrentID, true); ok {
			return detail, nil
		}
		return nil, err
	}
	adapter := NewSiteAdapter(site)
	if adapter == nil {
		return nil, errors.New("site adapter unavailable")
	}
	cfg := s.siteModelToConfig(site)
	timeout := sitePortalRequestTimeout(*site)
	cfg.Timeout = timeout
	reqCtx, cancel := context.WithTimeout(ctx, timeout)
	defer cancel()
	detail, err := adapter.GetDetail(reqCtx, cfg, torrentID)
	if err != nil || detail == nil {
		if isSitePortalRateLimitError(err) {
			s.markSitePortalCooldown(*site, err)
		}
		if cached, ok := s.getCachedSiteDetail(*site, torrentID, true); ok {
			return cached, nil
		}
		return detail, err
	}
	detail.Category = siteCategoryDisplayName(s.cachedOrFallbackSiteCategories(*site), detail.Category)
	s.storeSiteDetail(*site, torrentID, detail)
	return detail, nil
}

func (s *SiteService) DownloadURL(ctx context.Context, siteID, torrentID, fallback string) (string, error) {
	fallback = strings.TrimSpace(fallback)
	siteID = strings.TrimSpace(siteID)
	torrentID = strings.TrimSpace(torrentID)
	if siteID == "" || torrentID == "" {
		if fallback != "" {
			return s.ResolveDownloadURL(ctx, fallback), nil
		}
		return "", errors.New("site_id and id required")
	}
	site, err := s.FindByID(ctx, siteID)
	if err != nil || site == nil {
		if fallback != "" {
			return s.ResolveDownloadURL(ctx, fallback), nil
		}
		if err != nil {
			return "", err
		}
		return "", errors.New("site not found")
	}
	adapter := NewSiteAdapter(site)
	if adapter == nil {
		if fallback != "" {
			return s.ResolveDownloadURL(ctx, fallback), nil
		}
		return "", errors.New("site adapter unavailable")
	}
	cfg := s.siteModelToConfig(site)
	timeout := sitePortalRequestTimeout(*site)
	cfg.Timeout = timeout
	reqCtx, cancel := context.WithTimeout(ctx, timeout)
	defer cancel()
	resolved, err := adapter.GetDownloadURL(reqCtx, cfg, torrentID)
	if err != nil || strings.TrimSpace(resolved) == "" {
		if fallback != "" {
			return s.ResolveDownloadURL(ctx, fallback), nil
		}
		return "", err
	}
	return resolved, nil
}

func siteSearchResultFromItem(site model.Site, item TorrentItem, forcedAdult bool) SearchResult {
	return siteSearchResultFromItemWithCategories(site, item, forcedAdult, siteCategoriesForSite(site))
}

func siteSearchResultFromItemWithCategories(site model.Site, item TorrentItem, forcedAdult bool, cats []SiteCategory) SearchResult {
	upload := ""
	if !item.UploadTime.IsZero() {
		upload = item.UploadTime.UTC().Format(time.RFC3339)
	}
	adult := forcedAdult || siteCategoryIsAdultFromCategories(cats, item.Category) || looksAdultPTResource(item.Title+" "+item.Subtitle+" "+item.Category)
	return SearchResult{
		SiteName:    site.Name,
		SiteID:      site.ID,
		ID:          item.ID,
		Title:       item.Title,
		Subtitle:    item.Subtitle,
		PosterURL:   item.PosterURL,
		BackdropURL: item.BackdropURL,
		TorrentURL:  item.DetailURL,
		DownloadURL: item.DownloadURL,
		Category:    item.Category,
		Size:        item.Size,
		Seeders:     item.Seeders,
		Leechers:    item.Leechers,
		Snatched:    item.Snatched,
		Free:        item.Free,
		Adult:       adult,
		UploadTime:  upload,
	}
}

func siteCategoriesForSite(site model.Site) []SiteCategory {
	out := []SiteCategory{{
		ID:       "",
		Name:     "全部",
		Group:    "全部",
		SiteID:   site.ID,
		SiteName: site.Name,
		SiteType: site.Type,
	}}
	for _, cat := range defaultSiteCategories(site.Type) {
		if strings.TrimSpace(cat.ID) == "" && strings.TrimSpace(cat.Name) == "全部" {
			continue
		}
		out = append(out, cat)
	}
	out = append(out, extraSiteCategories(site)...)
	seen := map[string]struct{}{}
	deduped := make([]SiteCategory, 0, len(out))
	for i := range out {
		if out[i].SiteID == "" {
			out[i].SiteID = site.ID
		}
		if out[i].SiteName == "" {
			out[i].SiteName = site.Name
		}
		out[i].SiteType = site.Type
		key := strings.ToLower(strings.Join([]string{out[i].SiteID, out[i].ID, out[i].Name}, "\x00"))
		if _, ok := seen[key]; ok {
			continue
		}
		seen[key] = struct{}{}
		deduped = append(deduped, out[i])
	}
	return deduped
}

func normalizeSiteCategories(site model.Site, cats []SiteCategory) []SiteCategory {
	out := make([]SiteCategory, 0, len(cats)+1)
	hasAll := false
	seen := map[string]struct{}{}
	for _, cat := range cats {
		cat.ID = strings.TrimSpace(cat.ID)
		cat.Name = strings.TrimSpace(cat.Name)
		if cat.Name == "" {
			cat.Name = cat.ID
		}
		if cat.Name == cat.ID && cat.ID != "" {
			if name := defaultSiteCategoryName(site.Type, cat.ID); name != "" {
				cat.Name = name
			} else {
				cat.Name = "原站分类 " + cat.ID
			}
		}
		if cat.ID == "" && (cat.Name == "" || cat.Name == "全部") {
			hasAll = true
			cat.Name = "全部"
		}
		if cat.ID == "" && cat.Name == "" {
			continue
		}
		if strings.EqualFold(site.Type, "mteam") && !mteamVisibleVideoCategory(cat) {
			continue
		}
		if cat.Group == "" {
			cat.Group = inferSiteCategoryGroup(cat.Name, cat.ID)
		}
		cat.SiteID = site.ID
		cat.SiteName = site.Name
		cat.SiteType = site.Type
		cat.Adult = cat.Adult || looksAdultPTResource(cat.Name+" "+cat.ID+" "+cat.Group)
		key := strings.ToLower(strings.Join([]string{cat.SiteID, cat.ID, cat.Name}, "\x00"))
		if _, ok := seen[key]; ok {
			continue
		}
		seen[key] = struct{}{}
		out = append(out, cat)
	}
	if !hasAll {
		out = append([]SiteCategory{{
			ID:       "",
			Name:     "全部",
			Group:    "全部",
			SiteID:   site.ID,
			SiteName: site.Name,
			SiteType: site.Type,
		}}, out...)
	}
	return out
}

func (s *SiteService) cachedOrFallbackSiteCategories(site model.Site) []SiteCategory {
	if cats, ok := s.getCachedSiteCategories(site, true); ok {
		return cats
	}
	return normalizeSiteCategories(site, siteCategoriesForSite(site))
}

func defaultSiteCategoryName(siteType, id string) string {
	id = strings.TrimSpace(id)
	if id == "" {
		return ""
	}
	for _, cat := range defaultSiteCategories(siteType) {
		if strings.EqualFold(strings.TrimSpace(cat.ID), id) && strings.TrimSpace(cat.Name) != "" && !strings.EqualFold(cat.Name, id) {
			return strings.TrimSpace(cat.Name)
		}
	}
	return ""
}

func siteCategoryDisplayName(cats []SiteCategory, raw string) string {
	raw = strings.TrimSpace(raw)
	if raw == "" {
		return ""
	}
	if strings.HasPrefix(raw, "原站分类 ") {
		raw = strings.TrimSpace(strings.TrimPrefix(raw, "原站分类 "))
	}
	for _, cat := range cats {
		if strings.EqualFold(strings.TrimSpace(cat.ID), raw) || strings.EqualFold(strings.TrimSpace(cat.Name), raw) {
			name := strings.TrimSpace(cat.Name)
			if name != "" && !strings.EqualFold(name, cat.ID) {
				return name
			}
		}
	}
	siteType := ""
	for _, cat := range cats {
		if strings.TrimSpace(cat.SiteType) != "" {
			siteType = cat.SiteType
			break
		}
	}
	if name := defaultSiteCategoryName(siteType, raw); name != "" {
		return name
	}
	if strings.EqualFold(siteType, "mteam") && looksCategoryID(raw) {
		return ""
	}
	return raw
}

func inferSiteCategoryGroup(name, id string) string {
	text := strings.ToLower(strings.TrimSpace(name + " " + id))
	switch {
	case looksAdultPTResource(text):
		return "成人"
	case strings.Contains(text, "movie") || strings.Contains(text, "电影") || strings.Contains(text, "剧") || strings.Contains(text, "tv") || strings.Contains(text, "anime") || strings.Contains(text, "动漫") || strings.Contains(text, "综艺") || strings.Contains(text, "纪录"):
		return "影视"
	case strings.Contains(text, "music") || strings.Contains(text, "音乐") || strings.Contains(text, "mv"):
		return "音乐"
	case strings.Contains(text, "game") || strings.Contains(text, "游戏") || strings.Contains(text, "software") || strings.Contains(text, "软件") || strings.Contains(text, "book") || strings.Contains(text, "书"):
		return "其他"
	default:
		return "原站"
	}
}

func defaultSiteCategories(siteType string) []SiteCategory {
	switch strings.ToLower(strings.TrimSpace(siteType)) {
	case "mteam":
		return []SiteCategory{
			{ID: "", Name: "全部", Group: "全部", SiteType: siteType},
			{ID: "100", Name: "电影", Group: "影视", SiteType: siteType},
			{ID: "105", Name: "剧集", Group: "影视", SiteType: siteType},
			{ID: "110", Name: "综艺", Group: "影视", SiteType: siteType},
			{ID: "115", Name: "动漫", Group: "影视", SiteType: siteType},
			{ID: "120", Name: "纪录片", Group: "影视", SiteType: siteType},
			{ID: "419", Name: "成人", Group: "成人", SiteType: siteType, Adult: true},
			{ID: "420", Name: "成人视频", Group: "成人", SiteType: siteType, Adult: true},
			{ID: "421", Name: "成人写真", Group: "成人", SiteType: siteType, Adult: true},
			{ID: "422", Name: "成人动漫", Group: "成人", SiteType: siteType, Adult: true},
			{ID: "423", Name: "成人三级", Group: "成人", SiteType: siteType, Adult: true},
			{ID: "424", Name: "成人高清", Group: "成人", SiteType: siteType, Adult: true},
			{ID: "425", Name: "成人无码", Group: "成人", SiteType: siteType, Adult: true},
			{ID: "426", Name: "成人有码", Group: "成人", SiteType: siteType, Adult: true},
			{ID: "427", Name: "成人素人", Group: "成人", SiteType: siteType, Adult: true},
			{ID: "429", Name: "成人写真", Group: "成人", SiteType: siteType, Adult: true},
			{ID: "430", Name: "成人影像", Group: "成人", SiteType: siteType, Adult: true},
			{ID: "431", Name: "成人图片", Group: "成人", SiteType: siteType, Adult: true},
			{ID: "432", Name: "成人动漫", Group: "成人", SiteType: siteType, Adult: true},
			{ID: "433", Name: "成人游戏", Group: "成人", SiteType: siteType, Adult: true},
			{ID: "434", Name: "成人电子书", Group: "成人", SiteType: siteType, Adult: true},
			{ID: "435", Name: "成人软件", Group: "成人", SiteType: siteType, Adult: true},
			{ID: "436", Name: "成人音乐", Group: "成人", SiteType: siteType, Adult: true},
			{ID: "437", Name: "成人MV", Group: "成人", SiteType: siteType, Adult: true},
			{ID: "438", Name: "成人纪录片", Group: "成人", SiteType: siteType, Adult: true},
			{ID: "439", Name: "成人综艺", Group: "成人", SiteType: siteType, Adult: true},
			{ID: "440", Name: "成人其他", Group: "成人", SiteType: siteType, Adult: true},
			{ID: "442", Name: "写真", Group: "成人", SiteType: siteType, Adult: true},
			{ID: "444", Name: "成人影像", Group: "成人", SiteType: siteType, Adult: true},
			{ID: "445", Name: "成人图片", Group: "成人", SiteType: siteType, Adult: true},
			{ID: "446", Name: "成人写真", Group: "成人", SiteType: siteType, Adult: true},
			{ID: "447", Name: "成人视频", Group: "成人", SiteType: siteType, Adult: true},
			{ID: "448", Name: "成人动漫", Group: "成人", SiteType: siteType, Adult: true},
			{ID: "449", Name: "成人游戏", Group: "成人", SiteType: siteType, Adult: true},
			{ID: "450", Name: "成人小说", Group: "成人", SiteType: siteType, Adult: true},
			{ID: "451", Name: "成人其他", Group: "成人", SiteType: siteType, Adult: true},
			{ID: "401", Name: "电影", Group: "影视", SiteType: siteType},
			{ID: "402", Name: "剧集", Group: "影视", SiteType: siteType},
			{ID: "403", Name: "综艺", Group: "影视", SiteType: siteType},
			{ID: "404", Name: "动漫", Group: "影视", SiteType: siteType},
			{ID: "405", Name: "纪录片", Group: "影视", SiteType: siteType},
		}
	}
	base := []SiteCategory{
		{ID: "", Name: "全部", Group: "全部"},
		{ID: "401", Name: "电影", Group: "影视"},
		{ID: "402", Name: "剧集", Group: "影视"},
		{ID: "403", Name: "综艺", Group: "影视"},
		{ID: "404", Name: "动漫", Group: "影视"},
		{ID: "405", Name: "纪录片", Group: "影视"},
		{ID: "406", Name: "音乐", Group: "音乐"},
		{ID: "407", Name: "MV", Group: "音乐"},
		{ID: "408", Name: "体育", Group: "其他"},
		{ID: "409", Name: "软件", Group: "其他"},
		{ID: "410", Name: "游戏", Group: "其他"},
		{ID: "411", Name: "电子书", Group: "其他"},
		{ID: "412", Name: "教育", Group: "其他"},
		{ID: "413", Name: "其他", Group: "其他"},
		{ID: "420", Name: "成人视频", Group: "成人", Adult: true},
		{ID: "421", Name: "成人写真", Group: "成人", Adult: true},
		{ID: "422", Name: "成人动漫", Group: "成人", Adult: true},
		{ID: "adult", Name: "成人", Group: "成人", Adult: true},
	}
	switch strings.ToLower(strings.TrimSpace(siteType)) {
	case "unit3d":
		base = append(base,
			SiteCategory{ID: "movie", Name: "电影", Group: "影视"},
			SiteCategory{ID: "tv", Name: "剧集", Group: "影视"},
			SiteCategory{ID: "anime", Name: "动漫", Group: "影视"},
			SiteCategory{ID: "xxx", Name: "成人", Group: "成人", Adult: true},
		)
	case "gazelle":
		base = append(base,
			SiteCategory{ID: "music", Name: "音乐", Group: "音乐"},
			SiteCategory{ID: "applications", Name: "软件", Group: "其他"},
			SiteCategory{ID: "ebooks", Name: "电子书", Group: "其他"},
		)
	case "custom_rss":
		return []SiteCategory{{ID: "", Name: "全部", Group: "全部", SiteType: siteType}}
	}
	for i := range base {
		base[i].SiteType = siteType
	}
	return base
}

func extraSiteCategories(site model.Site) []SiteCategory {
	raw := strings.TrimSpace(site.Extra)
	if raw == "" {
		return nil
	}
	var payload struct {
		Categories []SiteCategory `json:"categories"`
	}
	if err := json.Unmarshal([]byte(raw), &payload); err != nil || len(payload.Categories) == 0 {
		return nil
	}
	out := make([]SiteCategory, 0, len(payload.Categories))
	for _, cat := range payload.Categories {
		cat.ID = strings.TrimSpace(cat.ID)
		cat.Name = strings.TrimSpace(cat.Name)
		if cat.Name == "" {
			cat.Name = cat.ID
		}
		if cat.Name == "" {
			continue
		}
		if cat.Group == "" {
			if cat.Adult || looksAdultPTResource(cat.Name+" "+cat.ID) {
				cat.Group = "成人"
				cat.Adult = true
			} else {
				cat.Group = "自定义"
			}
		}
		cat.SiteType = site.Type
		out = append(out, cat)
	}
	return out
}

func siteCategoryIsAdult(site model.Site, category string) bool {
	return siteCategoryIsAdultFromCategories(siteCategoriesForSite(site), category)
}

func siteCategoryMatches(site model.Site, itemCategory, selected string) bool {
	return siteCategoryMatchesCategories(siteCategoriesForSite(site), itemCategory, selected)
}

func siteCategoryIsAdultFromCategories(cats []SiteCategory, category string) bool {
	category = strings.TrimSpace(category)
	if category == "" {
		return false
	}
	for _, cat := range cats {
		if strings.EqualFold(cat.ID, category) || strings.EqualFold(cat.Name, category) {
			return cat.Adult || looksAdultPTResource(cat.Name+" "+cat.ID)
		}
	}
	return looksAdultPTResource(category)
}

func siteCategoryMatchesCategories(cats []SiteCategory, itemCategory, selected string) bool {
	selected = strings.TrimSpace(selected)
	if selected == "" {
		return true
	}
	itemCategory = strings.TrimSpace(itemCategory)
	if itemCategory == "" {
		return true
	}
	if strings.EqualFold(itemCategory, selected) {
		return true
	}
	for _, cat := range cats {
		if strings.EqualFold(cat.ID, selected) || strings.EqualFold(cat.Name, selected) {
			return strings.EqualFold(itemCategory, cat.ID) || strings.EqualFold(itemCategory, cat.Name)
		}
	}
	return true
}

func looksAdultPTResource(text string) bool {
	text = strings.ToLower(strings.TrimSpace(text))
	if text == "" {
		return false
	}
	for _, needle := range []string{
		"adult", "xxx", "porn", "jav", "av", "18+", "18禁", "成人", "写真", "无码", "有码", "女优", "素人", "裏番",
	} {
		if strings.Contains(text, needle) {
			return true
		}
	}
	return false
}

func BuildSiteSubscriptionKeyword(values ...string) string {
	for _, value := range values {
		value = strings.TrimSpace(value)
		if value != "" {
			return value
		}
	}
	return ""
}

func SiteSearchURL(keyword, siteID, category string, includeAdult bool) string {
	values := url.Values{}
	if strings.TrimSpace(keyword) != "" {
		values.Set("keyword", strings.TrimSpace(keyword))
	}
	if strings.TrimSpace(siteID) != "" {
		values.Set("site_id", strings.TrimSpace(siteID))
	}
	if strings.TrimSpace(category) != "" {
		values.Set("category", strings.TrimSpace(category))
	}
	if includeAdult {
		values.Set("include_adult", "true")
	}
	return "site-search://resources?" + values.Encode()
}

func siteSearchParamsFromURL(feedURL string) SiteBrowseParams {
	p := SiteBrowseParams{Page: 1}
	u, err := url.Parse(strings.TrimSpace(feedURL))
	if err != nil {
		return p
	}
	q := u.Query()
	p.Keyword = strings.TrimSpace(q.Get("keyword"))
	p.SiteID = strings.TrimSpace(q.Get("site_id"))
	p.Category = strings.TrimSpace(q.Get("category"))
	p.IncludeAdult, _ = strconv.ParseBool(q.Get("include_adult"))
	return p
}
