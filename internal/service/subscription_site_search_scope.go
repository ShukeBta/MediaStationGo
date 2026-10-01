package service

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"sync"

	"github.com/ShukeBta/MediaStationGo/internal/model"
)

// Search the saved tracker/category scope with fresh results. Portal caches
// serve interactive browsing; a scheduled run must see newly published files.
func (s *SubscriptionService) searchSubscriptionSiteScope(ctx context.Context, sub *model.Subscription, keyword string) ([]SearchResult, error) {
	scope := siteSearchParamsFromURL(sub.FeedURL)
	scope.Keyword = keyword
	sites, err := s.site.List(ctx)
	if err != nil {
		return nil, err
	}
	var mu sync.Mutex
	var wg sync.WaitGroup
	var results []SearchResult
	var failures []error
	matched := 0
	for _, site := range sites {
		if !site.Enabled || (scope.SiteID != "" && site.ID != scope.SiteID) {
			continue
		}
		matched++
		wg.Add(1)
		go func(site model.Site) {
			defer wg.Done()
			items, searchErr := s.searchSubscriptionTracker(ctx, site, scope)
			mu.Lock()
			defer mu.Unlock()
			if searchErr != nil {
				failures = append(failures, fmt.Errorf("%s: %w", site.Name, searchErr))
				return
			}
			results = append(results, items...)
		}(site)
	}
	wg.Wait()
	if matched == 0 {
		return nil, errors.New("订阅没有可用的 PT 站点，请检查所选站点是否已启用")
	}
	if len(failures) == matched {
		return nil, errors.Join(failures...)
	}
	return results, nil
}

func (s *SubscriptionService) searchSubscriptionTracker(ctx context.Context, site model.Site, scope SiteBrowseParams) ([]SearchResult, error) {
	adapter := NewSiteAdapter(&site)
	if adapter == nil {
		return nil, fmt.Errorf("unsupported site type %s", site.Type)
	}
	cfg := s.site.siteModelToConfig(&site)
	requestCtx, cancel := context.WithTimeout(ctx, cfg.Timeout)
	defer cancel()
	result, err := browseSiteResources(requestCtx, adapter, cfg, scope.Keyword, scope.Category, 1, scope.IncludeAdult)
	if err != nil || result == nil {
		return nil, err
	}
	categories := s.site.cachedOrFallbackSiteCategories(site)
	categoryAdult := siteCategoryIsAdultFromCategories(categories, scope.Category)
	items := make([]SearchResult, 0, len(result.Items))
	for _, item := range result.Items {
		if !subscriptionSiteCategoryMatches(categories, item.Category, scope.Category) {
			continue
		}
		item.Category = siteCategoryDisplayName(categories, item.Category)
		row := siteSearchResultFromItemWithCategories(site, item, categoryAdult, categories)
		if row.Adult && !scope.IncludeAdult {
			continue
		}
		row.Labels = item.Labels
		row.SearchKeyword = scope.Keyword
		items = append(items, row)
	}
	return items, nil
}

func subscriptionSiteCategoryMatches(categories []SiteCategory, actual, selected string) bool {
	actual, selected = strings.TrimSpace(actual), strings.TrimSpace(selected)
	if selected == "" {
		return true
	}
	if actual == "" {
		return false
	}
	if strings.EqualFold(actual, selected) {
		return true
	}
	for _, category := range categories {
		if strings.EqualFold(category.ID, selected) || strings.EqualFold(category.Name, selected) {
			return strings.EqualFold(actual, category.ID) || strings.EqualFold(actual, category.Name)
		}
	}
	return false
}
