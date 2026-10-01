package service

import (
	"context"
	"errors"
	"fmt"
	"sort"
	"strings"
	"sync"

	"go.uber.org/zap"

	"github.com/ShukeBta/MediaStationGo/internal/model"
)

// SearchResult is one torrent returned by a site adapter search.
type SearchResult struct {
	SiteName      string `json:"site_name"`
	ID            string `json:"id,omitempty"`
	SiteID        string `json:"site_id"`
	Title         string `json:"title"`
	Subtitle      string `json:"subtitle,omitempty"`
	PosterURL     string `json:"poster_url,omitempty"`
	BackdropURL   string `json:"backdrop_url,omitempty"`
	Labels        string `json:"labels,omitempty"`
	TorrentURL    string `json:"torrent_url"`
	DownloadURL   string `json:"download_url"`
	Category      string `json:"category,omitempty"`
	SearchKeyword string `json:"search_keyword,omitempty"`
	Size          int64  `json:"size"`
	Seeders       int    `json:"seeders"`
	Leechers      int    `json:"leechers"`
	Snatched      int    `json:"snatched,omitempty"`
	Free          bool   `json:"free"`
	Adult         bool   `json:"adult,omitempty"`
	UploadTime    string `json:"upload_time,omitempty"`
}

// Search fans out a keyword query to every enabled site and returns
// merged results sorted by seeders descending.
// Uses concurrent search with sync.WaitGroup for performance.
func (s *SiteService) Search(ctx context.Context, keyword string) ([]SearchResult, error) {
	if strings.TrimSpace(keyword) == "" {
		return []SearchResult{}, nil
	}
	sites, err := s.List(ctx)
	if err != nil {
		return nil, err
	}

	var (
		mu           sync.Mutex
		wg           sync.WaitGroup
		enabledCount int
		failedCount  int
		failureErrs  []error
		failures     []string
		results      []SearchResult
	)

	for i := range sites {
		if !sites[i].Enabled {
			continue
		}
		enabledCount++
		wg.Add(1)
		go func(site model.Site) {
			defer wg.Done()

			adapter := NewSiteAdapter(&site)
			if adapter == nil {
				mu.Lock()
				failedCount++
				err := fmt.Errorf("%s: unsupported site type %s", site.Name, site.Type)
				failureErrs = append(failureErrs, err)
				failures = append(failures, err.Error())
				mu.Unlock()
				return
			}

			cfg := s.siteModelToConfig(&site)
			timeout := cfg.Timeout
			ctxWithTimeout, cancel := context.WithTimeout(ctx, timeout)
			defer cancel()

			result, err := adapter.Search(ctxWithTimeout, cfg, keyword, 1)
			if err != nil {
				mu.Lock()
				failedCount++
				failureErr := fmt.Errorf("%s: %w", site.Name, err)
				failureErrs = append(failureErrs, failureErr)
				failures = append(failures, failureErr.Error())
				mu.Unlock()
				s.log.Warn("site search failed",
					zap.String("site", site.Name),
					zap.String("type", site.Type),
					zap.String("url", site.URL),
					zap.String("keyword", keyword),
					zap.Duration("timeout", timeout),
					zap.Error(err))
				return
			}
			if result == nil {
				return
			}
			siteResults := siteSearchResultsFromItems(site, result, keyword)
			mu.Lock()
			results = append(results, siteResults...)
			mu.Unlock()
		}(sites[i])
	}
	wg.Wait()

	// Ensure results is never nil (return [] instead of null in JSON)
	if results == nil {
		results = []SearchResult{}
	}

	// Sort by seeders desc.
	sort.Slice(results, func(i, j int) bool {
		return results[i].Seeders > results[j].Seeders
	})
	if s.log != nil {
		s.log.Info("site search completed",
			zap.String("keyword", keyword),
			zap.Int("enabled_sites", enabledCount),
			zap.Int("failed_sites", failedCount),
			zap.Int("results_count", len(results)))
	}
	if enabledCount > 0 && failedCount >= enabledCount && len(results) == 0 {
		if len(failureErrs) > 0 {
			return results, fmt.Errorf("all enabled sites failed while searching %q: %w", keyword, errors.Join(failureErrs...))
		}
		return results, fmt.Errorf("all enabled sites failed while searching %q: %s", keyword, strings.Join(failures, "; "))
	}
	return results, nil
}

// SearchSite runs a keyword search against one configured site, regardless of
// whether the site is enabled globally. This is used by per-site diagnostics in
// the management UI, where the user expects the selected site to be tested
// directly instead of a full fan-out followed by filtering.
func (s *SiteService) SearchSite(ctx context.Context, siteID, keyword string, page int) ([]SearchResult, error) {
	if strings.TrimSpace(keyword) == "" {
		return []SearchResult{}, nil
	}
	if page <= 0 {
		page = 1
	}
	site, err := s.FindByID(ctx, siteID)
	if err != nil {
		return nil, err
	}
	if site == nil {
		return nil, fmt.Errorf("site not found")
	}
	adapter := NewSiteAdapter(site)
	if adapter == nil {
		return nil, fmt.Errorf("%s: unsupported site type %s", site.Name, site.Type)
	}
	cfg := s.siteModelToConfig(site)
	timeout := cfg.Timeout
	ctxWithTimeout, cancel := context.WithTimeout(ctx, timeout)
	defer cancel()

	result, err := adapter.Search(ctxWithTimeout, cfg, keyword, page)
	if err != nil {
		if s.log != nil {
			s.log.Warn("single site search failed",
				zap.String("site", site.Name),
				zap.String("type", site.Type),
				zap.String("url", site.URL),
				zap.String("keyword", keyword),
				zap.Duration("timeout", timeout),
				zap.Error(err))
		}
		return nil, err
	}
	out := siteSearchResultsFromItems(*site, result, keyword)
	sort.Slice(out, func(i, j int) bool {
		return out[i].Seeders > out[j].Seeders
	})
	if s.log != nil {
		s.log.Info("single site search completed",
			zap.String("site", site.Name),
			zap.String("keyword", keyword),
			zap.Int("results_count", len(out)))
	}
	return out, nil
}

func siteSearchResultsFromItems(site model.Site, result *SiteSearchResult, keyword string) []SearchResult {
	if result == nil || len(result.Items) == 0 {
		return []SearchResult{}
	}
	out := make([]SearchResult, 0, len(result.Items))
	for _, item := range result.Items {
		out = append(out, SearchResult{
			SiteName:      site.Name,
			SiteID:        site.ID,
			ID:            item.ID,
			Title:         item.Title,
			Subtitle:      item.Subtitle,
			PosterURL:     item.PosterURL,
			BackdropURL:   item.BackdropURL,
			Labels:        item.Labels,
			TorrentURL:    item.DetailURL,
			DownloadURL:   item.DownloadURL,
			Category:      item.Category,
			SearchKeyword: keyword,
			Size:          item.Size,
			Seeders:       item.Seeders,
			Leechers:      item.Leechers,
			Free:          item.Free,
		})
	}
	return out
}
