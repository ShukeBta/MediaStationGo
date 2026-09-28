package service

import (
	"context"
	"errors"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"go.uber.org/zap"

	"github.com/ShukeBta/MediaStationGo/internal/config"
)

type discoverSearchRoundTripFunc func(*http.Request) (*http.Response, error)

func (f discoverSearchRoundTripFunc) RoundTrip(req *http.Request) (*http.Response, error) {
	return f(req)
}

func TestExternalMediaResultsFromMatchesPreservesCatalogIdentity(t *testing.T) {
	matches := []*Match{
		{Title: "电影一", OriginalName: "Movie One", TMDbID: 101, Year: 2026, ReleaseDate: "2026-07-18"},
		{Title: "电影二", TMDbID: 102},
	}
	items := externalMediaResultsFromMatches("tmdb", "movie", matches, 1)
	if len(items) != 1 {
		t.Fatalf("items = %#v", items)
	}
	item := items[0]
	if item.Source != "tmdb" || item.MediaType != "movie" || item.TMDbID != 101 {
		t.Fatalf("identity = %#v", item)
	}
	if item.SubscribeKeyword != "电影一 2026" || item.ReleaseDate != "2026-07-18" {
		t.Fatalf("metadata = %#v", item)
	}
}

func TestSearchDiscoverCatalogWithNoProvidersIsExplicitlyEmpty(t *testing.T) {
	result := SearchDiscoverCatalog(t.Context(), "测试", nil, nil, nil, nil)
	if len(result.Items) != 0 || len(result.Errors) != 0 {
		t.Fatalf("result = %#v", result)
	}
}

func TestRunDiscoverCatalogSearchTasksReturnsCompletedSourcesAtDeadline(t *testing.T) {
	releaseSlow := make(chan struct{})
	defer close(releaseSlow)
	ctx, cancel := context.WithTimeout(t.Context(), 100*time.Millisecond)
	defer cancel()

	started := time.Now()
	result := runDiscoverCatalogSearchTasks(ctx, []discoverCatalogSearchTask{
		{
			key: "fast",
			run: func() ([]ExternalMediaResult, error) {
				return []ExternalMediaResult{{Source: "fast", MediaType: "movie", Title: "Fast result"}}, nil
			},
		},
		{
			key: "slow",
			run: func() ([]ExternalMediaResult, error) {
				<-releaseSlow
				return []ExternalMediaResult{{Source: "slow", MediaType: "movie", Title: "Late result"}}, nil
			},
		},
	})
	if elapsed := time.Since(started); elapsed > time.Second {
		t.Fatalf("search waited %s for a source beyond the caller deadline", elapsed)
	}
	if len(result.Items) != 1 || result.Items[0].Title != "Fast result" {
		t.Fatalf("items = %#v, want only the completed source", result.Items)
	}
	if !errors.Is(result.Errors["slow"], context.DeadlineExceeded) {
		t.Fatalf("slow source error = %v, want deadline exceeded", result.Errors["slow"])
	}
	if _, exists := result.Errors["fast"]; exists {
		t.Fatalf("fast source unexpectedly failed: %v", result.Errors["fast"])
	}
}

func TestSearchDiscoverCatalogOnlySearchesAdultMoviesForAdultCode(t *testing.T) {
	var catalogRequests atomic.Int32
	var performerRequests atomic.Int32
	var movieRequests atomic.Int32
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Query().Get("f") {
		case "actor":
			performerRequests.Add(1)
		case "all":
			movieRequests.Add(1)
			_, _ = w.Write([]byte(`<a class="box" href="/v/mizd534" title="Sample title"><strong>MIZD-534</strong></a>`))
		default:
			http.NotFound(w, r)
		}
	}))
	defer server.Close()
	withAdultDefaultBases(t, []string{server.URL})

	client := &http.Client{Transport: discoverSearchRoundTripFunc(func(req *http.Request) (*http.Response, error) {
		catalogRequests.Add(1)
		body := `{"results":[],"list":[]}`
		if strings.Contains(req.URL.Host, "douban") {
			body = `[]`
		}
		return &http.Response{
			StatusCode: http.StatusOK,
			Header:     make(http.Header),
			Body:       io.NopCloser(strings.NewReader(body)),
			Request:    req,
		}, nil
	})}
	cfg := &config.Config{Secrets: config.SecretsConfig{
		TMDbAPIKey:   "test-key",
		TMDbAPIProxy: "https://tmdb.invalid",
	}}
	tmdb := NewTMDbProvider(cfg, zap.NewNop(), nil)
	tmdb.client = client
	douban := NewDoubanProvider(cfg, zap.NewNop())
	douban.client = client
	bangumi := NewBangumiProvider(cfg, zap.NewNop())
	bangumi.base = "https://bangumi.invalid"
	bangumi.client = client
	adult := NewAdultProvider(zap.NewNop(), nil)
	result := SearchDiscoverCatalog(t.Context(), "MIZD-534", tmdb, douban, bangumi, adult)
	if len(result.Errors) != 0 || len(result.Items) != 1 {
		t.Fatalf("result = %#v", result)
	}
	if got := catalogRequests.Load(); got != 0 {
		t.Fatalf("non-adult catalog requests = %d, want 0", got)
	}
	if got := performerRequests.Load(); got != 0 {
		t.Fatalf("performer requests = %d, want 0", got)
	}
	if got := movieRequests.Load(); got != 1 {
		t.Fatalf("movie requests = %d, want 1", got)
	}
}

func TestSearchDiscoverCatalogRoutesFC2CodeOnlyToFD2PPV(t *testing.T) {
	var javDBRequests atomic.Int32
	javDB := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		javDBRequests.Add(1)
		http.NotFound(w, r)
	}))
	defer javDB.Close()
	withAdultDefaultBases(t, []string{javDB.URL})

	flareRequests := atomic.Int32{}
	flare := newFD2PPVFlareServer(t, func(target string) string {
		flareRequests.Add(1)
		if target != "https://fd2ppv.cc/articles/3780016" {
			t.Fatalf("target URL = %q", target)
		}
		return `<h1 class="work-title">3780016</h1>
			<div class="work-brief">FD2 code result</div>
			<div class="work-original-photos">https://xximgs.cc/uploads/fd2.webp</div>`
	})
	defer flare.Close()

	adult := NewAdultProvider(zap.NewNop(), nil)
	adult.SetFlareSolverr(flare.URL, 5)
	result := SearchDiscoverCatalog(t.Context(), "FC2-PPV-3780016", nil, nil, nil, adult)
	if len(result.Errors) != 0 || len(result.Items) != 1 || result.Items[0].Source != "fd2ppv" {
		t.Fatalf("result = %#v", result)
	}
	if javDBRequests.Load() != 0 {
		t.Fatalf("JavDB requests = %d, want 0", javDBRequests.Load())
	}
	if flareRequests.Load() != 1 {
		t.Fatalf("FD2PPV requests = %d, want 1", flareRequests.Load())
	}
}
