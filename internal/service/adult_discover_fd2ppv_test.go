package service

import (
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync/atomic"
	"testing"

	"go.uber.org/zap"
)

func TestParseFD2PPVMovieList(t *testing.T) {
	body := fd2PPVMovieCards(2)
	items := parseFD2PPVMovieList(body, "https://fd2ppv.cc")
	if len(items) != 2 {
		t.Fatalf("items = %#v", items)
	}
	item := items[0]
	if item.Source != "fd2ppv" || item.MediaType != "adult" || item.OriginalName != "FC2-PPV-4900001" {
		t.Fatalf("identity = %#v", item)
	}
	if item.Title != "FC2 sample 1" || item.ReleaseDate != "2026-07-01" || item.Year != 2026 {
		t.Fatalf("metadata = %#v", item)
	}
	if item.PosterURL != "https://xximgs.cc/uploads/4900001.webp" || item.ProviderID != "4900001" {
		t.Fatalf("artwork/provider = %#v", item)
	}
}

func TestDiscoverFD2PPVWindowUsesRequestedSortAndSourcePageSize(t *testing.T) {
	var requestedURL string
	flare := newFD2PPVFlareServer(t, func(target string) string {
		requestedURL = target
		return fd2PPVMovieCards(20)
	})
	defer flare.Close()

	provider := NewAdultProvider(zap.NewNop(), nil)
	provider.SetFlareSolverr(flare.URL, 5)
	items, err := provider.DiscoverFD2PPVWindow(t.Context(), "views", 1, 18)
	if err != nil {
		t.Fatal(err)
	}
	if len(items) != 19 {
		t.Fatalf("window items = %d, want 19", len(items))
	}
	for _, expected := range []string{"sort=views", "size=48", "page=1"} {
		if !strings.Contains(requestedURL, expected) {
			t.Fatalf("target URL %q is missing %q", requestedURL, expected)
		}
	}
}

func TestDiscoverFD2PPVWindowFillsAcrossSparseSourcePages(t *testing.T) {
	var requestedURLs []string
	flare := newFD2PPVFlareServer(t, func(target string) string {
		requestedURLs = append(requestedURLs, target)
		switch {
		case strings.Contains(target, "page=1"):
			return fd2PPVMovieCardsFrom(4900001, 36) + `<nav class="pagination" data-total="2" data-param="page"></nav>`
		case strings.Contains(target, "page=2"):
			return fd2PPVMovieCardsFrom(4900037, 48) + `<nav class="pagination" data-total="2" data-param="page"></nav>`
		default:
			t.Fatalf("unexpected target URL = %q", target)
			return ""
		}
	})
	defer flare.Close()

	provider := NewAdultProvider(zap.NewNop(), nil)
	provider.SetFlareSolverr(flare.URL, 5)
	items, err := provider.DiscoverFD2PPVWindow(t.Context(), "release", 3, 18)
	if err != nil {
		t.Fatal(err)
	}
	if len(items) != 19 {
		t.Fatalf("window items = %d, want 19", len(items))
	}
	if items[0].ProviderID != "4900037" || items[18].ProviderID != "4900055" {
		t.Fatalf("window boundaries = %s ... %s", items[0].ProviderID, items[18].ProviderID)
	}
	if len(requestedURLs) != 2 {
		t.Fatalf("requests = %#v, want two source pages", requestedURLs)
	}
}

func TestSearchFD2PPVPerformersKeepsAliasMatches(t *testing.T) {
	flare := newFD2PPVFlareServer(t, func(target string) string {
		if !strings.Contains(target, "/actresses/?keyword=Yuna") {
			t.Fatalf("target URL = %q", target)
		}
		return `<div class="artist-card"><div class="inner"><h3><a href="/actresses/1669">中田ゆめ</a></h3><div class="aliases-container"><span class="alias-item">Yuna</span></div></div></div>`
	})
	defer flare.Close()

	provider := NewAdultProvider(zap.NewNop(), nil)
	provider.SetFlareSolverr(flare.URL, 5)
	items, err := provider.SearchFD2PPVPerformers(t.Context(), "Yuna")
	if err != nil {
		t.Fatal(err)
	}
	if len(items) != 1 || items[0].Source != "fd2ppv" || items[0].ProviderID != "1669" || items[0].Title != "中田ゆめ" {
		t.Fatalf("performers = %#v", items)
	}
}

func TestFD2PPVPerformerWorksAndDetailUseExistingFlareSolverrPath(t *testing.T) {
	var requests atomic.Int32
	flare := newFD2PPVFlareServer(t, func(target string) string {
		requests.Add(1)
		switch {
		case strings.Contains(target, "/actresses/1669?page=2"):
			return fd2PPVMovieCards(2) + `<nav class="pagination" data-total="3" data-param="page"></nav>`
		case strings.Contains(target, "/articles/3780016"):
			return `<h1 class="work-title">3780016</h1>
				<div class="work-brief">FD2 detail</div>
				<div class="work-original-photos">https://xximgs.cc/uploads/detail.webp https://xximgs.cc/uploads/preview.webp</div>
				<div class="work-meta-label">配信日</div><div class="work-meta-value">2023-09-07</div>
				<a class="artistUrl" data-actress="1669" href="/actresses/1669">中田ゆめ</a>`
		default:
			t.Fatalf("unexpected target URL = %q", target)
			return ""
		}
	})
	defer flare.Close()

	provider := NewAdultProvider(zap.NewNop(), nil)
	provider.SetFlareSolverr(flare.URL, 5)
	works, hasNext, err := provider.DiscoverPerformerWorksPage(t.Context(), "fd2ppv", "1669", 2)
	if err != nil {
		t.Fatal(err)
	}
	if len(works) != 2 || !hasNext || works[0].Source != "fd2ppv" {
		t.Fatalf("works = %#v hasNext=%v", works, hasNext)
	}
	detail, err := provider.DiscoverMovieDetail(t.Context(), "fd2ppv", "3780016", "FC2-PPV-3780016")
	if err != nil {
		t.Fatal(err)
	}
	if detail.Source != "fd2ppv" || detail.Title != "FD2 detail" || detail.ReleaseDate != "2023-09-07" {
		t.Fatalf("detail = %#v", detail)
	}
	if len(detail.People) != 1 || detail.People[0].SourceID != "1669" || len(detail.PreviewImages) != 1 {
		t.Fatalf("detail people/previews = %#v", detail)
	}
	if requests.Load() != 2 {
		t.Fatalf("FlareSolverr requests = %d, want 2", requests.Load())
	}
}

func TestNormalizeAdultPerformerImageURLAllowsFD2ArtworkHosts(t *testing.T) {
	for _, raw := range []string{
		"https://xximgs.cc/uploads/avatar.webp",
		"https://storage201000.contents.fc2.com/file/avatar.jpg",
		"https://contents-thumbnail2.fc2.com/w480/avatar.jpg",
	} {
		if got, ok := NormalizeAdultPerformerImageURL(raw); !ok || got != raw {
			t.Fatalf("NormalizeAdultPerformerImageURL(%q) = %q, %v", raw, got, ok)
		}
	}
}

func newFD2PPVFlareServer(t *testing.T, response func(target string) string) *httptest.Server {
	t.Helper()
	return httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		var request struct {
			Cmd string `json:"cmd"`
			URL string `json:"url"`
		}
		if err := json.NewDecoder(r.Body).Decode(&request); err != nil {
			t.Fatal(err)
		}
		if request.Cmd != "request.get" {
			t.Fatalf("FlareSolverr cmd = %q", request.Cmd)
		}
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(map[string]any{
			"status": "ok",
			"solution": map[string]any{
				"status":   200,
				"response": response(request.URL),
			},
		})
	}))
}

func fd2PPVMovieCards(count int) string {
	return fd2PPVMovieCardsFrom(4900001, count)
}

func fd2PPVMovieCardsFrom(firstNumber, count int) string {
	var body strings.Builder
	for index := 0; index < count; index++ {
		number := firstNumber + index
		_, _ = fmt.Fprintf(&body, `<div class="artist-card">
			<a href="/articles/%d"><div class="work-photos hidden">https://xximgs.cc/uploads/%d.webp https://xximgs.cc/uploads/%d-preview.webp</div></a>
			<div class="inner"><h3><a href="/articles/%d">%d</a></h3>
			<div class="artist-content"><a href="/articles/%d">FC2 sample %d</a></div>
			<div class="artist-release flex"><span>2026-07-%02d</span></div></div></div>`,
			number, number, number, number, number, number, index+1, index+1)
	}
	return body.String()
}
