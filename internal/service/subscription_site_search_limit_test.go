package service

import (
	"errors"
	"net/http"
	"net/http/httptest"
	"sync/atomic"
	"testing"
	"time"

	"go.uber.org/zap"

	"github.com/ShukeBta/MediaStationGo/internal/model"
	"github.com/ShukeBta/MediaStationGo/internal/repository"
)

func TestSearchSubscriptionSitesStopsAfterRateLimit(t *testing.T) {
	var requests atomic.Int32
	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		requests.Add(1)
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"code":"0","message":"SUCCESS","data":{"total":"0","data":[]}}`))
	}))
	defer upstream.Close()

	db := newServiceTestDB(t, &model.Site{}, &model.Setting{})
	repos := repository.New(db)
	siteSvc := NewSiteService(zap.NewNop(), repos, "")
	limiter := &staticSiteAPIRateLimiter{err: &siteAPIRateLimitError{
		Bucket:     "torrent_search_24h",
		Limit:      1500,
		Window:     24 * time.Hour,
		RetryAfter: time.Hour,
	}}
	siteSvc.apiRateLimiter = limiter
	if err := siteSvc.Create(t.Context(), &model.Site{
		Name:     "馒头",
		Type:     "mteam",
		URL:      upstream.URL,
		AuthType: "api_key",
		APIKey:   "token-123",
		Enabled:  true,
		Timeout:  5,
	}); err != nil {
		t.Fatal(err)
	}
	svc := NewSubscriptionService(nil, zap.NewNop(), repos, nil, siteSvc, NewHub(zap.NewNop()))
	sub := &model.Subscription{Name: "问心2 自动订阅", Filter: "问心2", MediaType: "tv"}

	_, err := svc.searchSubscriptionSites(t.Context(), sub, []string{"问心2", "问心", "问心2 2023"})
	var limited *siteAPIRateLimitError
	if !errors.As(err, &limited) {
		t.Fatalf("searchSubscriptionSites error = %v, want siteAPIRateLimitError", err)
	}
	if limiter.calls != 1 {
		t.Fatalf("rate limiter calls = %d, want 1 keyword attempt", limiter.calls)
	}
	if got := requests.Load(); got != 0 {
		t.Fatalf("HTTP requests = %d, want 0 after local rate limit", got)
	}
}

func TestSubscriptionRunAllStopsSweepAfterRateLimit(t *testing.T) {
	var requests atomic.Int32
	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		requests.Add(1)
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"code":"0","message":"SUCCESS","data":{"total":"0","data":[]}}`))
	}))
	defer upstream.Close()

	db := newServiceTestDB(t, &model.Site{}, &model.Setting{}, &model.Subscription{})
	repos := repository.New(db)
	siteSvc := NewSiteService(zap.NewNop(), repos, "")
	limiter := &staticSiteAPIRateLimiter{err: &siteAPIRateLimitError{
		Bucket:     "torrent_search_24h",
		Limit:      1500,
		Window:     24 * time.Hour,
		RetryAfter: time.Hour,
	}}
	siteSvc.apiRateLimiter = limiter
	if err := siteSvc.Create(t.Context(), &model.Site{
		Name:     "馒头",
		Type:     "mteam",
		URL:      upstream.URL,
		AuthType: "api_key",
		APIKey:   "token-123",
		Enabled:  true,
		Timeout:  5,
	}); err != nil {
		t.Fatal(err)
	}
	for _, name := range []string{"问心2 自动订阅", "南部档案 自动订阅"} {
		sub := &model.Subscription{
			Name:    name,
			FeedURL: "site-search://search?keyword=" + name,
			Filter:  name,
			Enabled: true,
		}
		if err := repos.Subscription.Create(t.Context(), sub); err != nil {
			t.Fatal(err)
		}
	}
	svc := NewSubscriptionService(nil, zap.NewNop(), repos, nil, siteSvc, NewHub(zap.NewNop()))

	svc.runAll(t.Context())
	if limiter.calls != 1 {
		t.Fatalf("rate limiter calls = %d, want sweep to stop after first quota failure", limiter.calls)
	}
	if got := requests.Load(); got != 0 {
		t.Fatalf("HTTP requests = %d, want 0 after local rate limit", got)
	}
}

func TestSubscriptionSiteSearchStopsAfterTransientSiteErrors(t *testing.T) {
	for _, errText := range []string{
		`search: Post "https://api.m-team.cc/api/torrent/search": context deadline exceeded`,
		`search: Post "https://api.m-team.cc/api/torrent/search": net/http: TLS handshake timeout`,
		`search: Post "https://api.m-team.cc/api/torrent/search": unexpected EOF`,
		`search: Post "https://api.m-team.cc/api/torrent/search": read tcp 127.0.0.1: connection reset by peer`,
	} {
		if !subscriptionSiteSearchShouldStopOnError(errors.New(errText)) {
			t.Fatalf("subscriptionSiteSearchShouldStopOnError(%q) = false, want true", errText)
		}
	}
	if subscriptionSiteSearchShouldStopOnError(errors.New("temporary parser warning: no matching torrent rows")) {
		t.Fatal("non-upstream-failure errors should not stop alias search")
	}
}
