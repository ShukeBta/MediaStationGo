package service

import (
	"fmt"
	"net/http"
	"net/http/httptest"
	"sync/atomic"
	"testing"

	"go.uber.org/zap"

	"github.com/ShukeBta/MediaStationGo/internal/model"
)

func TestParseJavDBMovieList(t *testing.T) {
	body := `<div class="movie-list">
<div class="item"><a href="/v/1Axvvw" class="box" title="Sample title">
<div class="cover"><img src="https://img.example/MVSD-696.jpg"></div>
<div class="video-title"><strong>MVSD-696</strong> Sample title</div>
<div class="score"><span class="value">4.73分, 由181人评价</span></div>
<div class="meta">2026-07-21</div></a></div></div>`
	items := parseJavDBMovieList(body, "https://javdb.example")
	if len(items) != 1 {
		t.Fatalf("items = %#v", items)
	}
	item := items[0]
	if item.Source != "javdb" || item.MediaType != "adult" || item.OriginalName != "MVSD-696" {
		t.Fatalf("unexpected identity: %#v", item)
	}
	if item.Title != "Sample title" || item.PosterURL != "https://img.example/MVSD-696.jpg" {
		t.Fatalf("unexpected display data: %#v", item)
	}
	if item.ReleaseDate != "2026-07-21" || item.Year != 2026 || item.Rating != 4.73 {
		t.Fatalf("unexpected release metadata: %#v", item)
	}
	if item.ProviderID != "1Axvvw" || item.ProviderURL != "https://javdb.example/v/1Axvvw" {
		t.Fatalf("unexpected provider link: %#v", item)
	}
}

func TestParseJavDBMovieListKeepsOriginalCover(t *testing.T) {
	body := `<a href="/v/1Axvvw" class="box" title="Sample title">
<div class="cover"><img loading="lazy" src="https://c0.jdbstatic.com/covers/1a/1Axvvw.jpg"></div>
<div class="video-title"><strong>MVSD-696</strong> Sample title</div></a>`
	items := parseJavDBMovieList(body, "https://javdb.com")
	if len(items) != 1 {
		t.Fatalf("items = %#v", items)
	}
	if got := items[0].PosterURL; got != "https://c0.jdbstatic.com/covers/1a/1Axvvw.jpg" {
		t.Fatalf("poster = %q, want original cover", got)
	}
}

func TestParseJavDBMovieListAcceptsCurrentLinkMarkup(t *testing.T) {
	body := `<a href="/v/current" title="Current markup">
<strong>ABF-123</strong><span class="meta">2026-08-07</span></a>`
	items := parseJavDBMovieList(body, "https://javdb.com")
	if len(items) != 1 || items[0].ProviderID != "current" || items[0].OriginalName != "ABF-123" {
		t.Fatalf("items = %#v", items)
	}
}

func TestParseJavDBMovieListKeepsExtendedDiscoveryCodes(t *testing.T) {
	body := `<a href="/v/extended" class="box" title="Extended">
<strong>EXVR-781jo01</strong></a>
<a href="/v/edition" class="box" title="Edition">
<strong>JKSR-631-01</strong></a>`
	items := parseJavDBMovieList(body, "https://javdb.example")
	if len(items) != 2 {
		t.Fatalf("items = %#v", items)
	}
	if items[0].OriginalName != "EXVR-781JO01" || items[1].OriginalName != "JKSR-631-01" {
		t.Fatalf("extended codes = %q, %q", items[0].OriginalName, items[1].OriginalName)
	}
}

func TestAdultProviderDiscoverPerformerWorksPageUsesSourcePagination(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/actors/Actor1" || r.URL.Query().Get("page") != "2" {
			http.NotFound(w, r)
			return
		}
		for index := 0; index < 39; index++ {
			_, _ = fmt.Fprintf(w, `<a href="/v/item%d" class="box"><strong>TEST-%03d</strong></a>`, index, index)
		}
		_, _ = w.Write([]byte(`<a rel="next" class="pagination-next" href="/actors/Actor1?page=3">下一页</a>`))
	}))
	defer server.Close()
	withAdultDefaultBases(t, []string{server.URL})

	provider := NewAdultProvider(zap.NewNop(), nil)
	items, hasNext, err := provider.DiscoverPerformerWorksPage(t.Context(), "javdb", "Actor1", 2)
	if err != nil {
		t.Fatal(err)
	}
	if len(items) != 39 || !hasNext {
		t.Fatalf("items=%d hasNext=%v", len(items), hasNext)
	}
}

func TestParseJavDBPerformerList(t *testing.T) {
	body := `<div id="actors" class="actors">
<div class="box actor-box"><a href="/actors/BzpA" title="Actor Alias">
<figure class="image"><img class="avatar" src="https://img.example/BzpA.jpg"></figure>
<strong>Actor Name</strong></a></div>
<a href="/actors/censored">Category</a></div>`
	items := parseJavDBPerformerList(body, "https://javdb.example")
	if len(items) != 1 {
		t.Fatalf("items = %#v", items)
	}
	item := items[0]
	if item.MediaType != "person" || item.Title != "Actor Name" || item.ProviderID != "BzpA" {
		t.Fatalf("unexpected performer: %#v", item)
	}
	if item.PosterURL != "https://img.example/BzpA.jpg" || item.ProviderURL != "https://javdb.example/actors/BzpA" {
		t.Fatalf("unexpected performer links: %#v", item)
	}
	if len(item.People) != 1 || item.People[0].SourceID != "BzpA" {
		t.Fatalf("unexpected performer metadata: %#v", item.People)
	}
}

func TestParseJavDBPerformerSections(t *testing.T) {
	body := `<h3 class="title is-4 mb-4">新人</h3>
<div id="actors" class="actors"><a href="/actors/New1"><img src="https://c0.jdbstatic.com/avatars/new1.jpg"><strong>新人女优</strong></a></div>
<h3 class="title is-4 mb-4">月榜</h3>
<div id="actors" class="actors"><a href="/actors/Month1"><img src="https://c0.jdbstatic.com/avatars/month1.jpg"><strong>月榜女优</strong></a></div>
<h3 class="title is-4 mb-4">Fanza(DMM)推薦</h3>
<div id="actors" class="actors"><a href="/actors/Fanza1"><img src="https://c0.jdbstatic.com/avatars/fanza1.jpg"><strong>推荐女优</strong></a></div>`
	sections, err := parseJavDBPerformerSections(body, "https://javdb.com")
	if err != nil {
		t.Fatal(err)
	}
	if got := sections[adultJavDBPerformerNew]; len(got) != 1 || got[0].ProviderID != "New1" {
		t.Fatalf("new performers = %#v", got)
	}
	if got := sections[adultJavDBPerformerMonthly]; len(got) != 1 || got[0].ProviderID != "Month1" {
		t.Fatalf("monthly performers = %#v", got)
	}
	if got := sections[adultJavDBPerformerFanza]; len(got) != 1 || got[0].ProviderID != "Fanza1" {
		t.Fatalf("fanza performers = %#v", got)
	}
}

func TestAdultProviderJavDBPerformerSectionsShareOneFetch(t *testing.T) {
	var requests atomic.Int32
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/actors" {
			http.NotFound(w, r)
			return
		}
		requests.Add(1)
		_, _ = w.Write([]byte(`<h3>新人</h3><a href="/actors/New1"><strong>新人女优</strong></a>
<h3>月榜</h3><a href="/actors/Month1"><strong>月榜女优</strong></a>
<h3>Fanza(DMM)推薦</h3><a href="/actors/Fanza1"><strong>推荐女优</strong></a>`))
	}))
	defer server.Close()
	withAdultDefaultBases(t, []string{server.URL})

	provider := NewAdultProvider(zap.NewNop(), nil)
	for _, section := range []string{adultJavDBPerformerNew, adultJavDBPerformerMonthly, adultJavDBPerformerFanza} {
		items, err := provider.DiscoverJavDBPerformerSection(t.Context(), section)
		if err != nil || len(items) != 1 {
			t.Fatalf("section %s items=%#v err=%v", section, items, err)
		}
	}
	if got := requests.Load(); got != 1 {
		t.Fatalf("actors requests = %d, want 1", got)
	}
}

func TestFollowedAdultPerformerItems(t *testing.T) {
	items := FollowedAdultPerformerItems([]model.AdultPerformerFollow{{
		Name: "关注女优", Source: "javdb", SourceID: "Actor1",
		ImageURL:   "https://c0.jdbstatic.com/avatars/actor1.jpg",
		ProfileURL: "https://javdb.com/actors/Actor1",
	}})
	if len(items) != 1 || !items[0].Followed || items[0].MediaType != "person" {
		t.Fatalf("items = %#v", items)
	}
	if items[0].ProviderID != "Actor1" || len(items[0].People) != 1 {
		t.Fatalf("performer identity = %#v", items[0])
	}
}

func TestNormalizeAdultPerformerImageURL(t *testing.T) {
	if got, ok := NormalizeAdultPerformerImageURL("https://c0.jdbstatic.com/avatars/a.jpg"); !ok || got == "" {
		t.Fatalf("expected jdbstatic image, got %q ok=%v", got, ok)
	}
	if _, ok := NormalizeAdultPerformerImageURL("http://127.0.0.1/private"); ok {
		t.Fatal("local or non-HTTPS image URL must be rejected")
	}
	if _, ok := NormalizeAdultPerformerImageURL("https://example.com/avatar.jpg"); ok {
		t.Fatal("untrusted image host must be rejected")
	}
}

func TestAdultProviderSearchPerformersUsesDirectActorResult(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/search" || r.URL.Query().Get("q") != "Actor Name" || r.URL.Query().Get("f") != "actor" {
			http.NotFound(w, r)
			return
		}
		_, _ = w.Write([]byte(`<a href="/actors/BzpA"><img src="https://c0.jdbstatic.com/actors/BzpA.jpg"><strong>Actor Name</strong></a>`))
	}))
	defer server.Close()
	withAdultDefaultBases(t, []string{server.URL})

	provider := NewAdultProvider(zap.NewNop(), nil)
	items, err := provider.SearchPerformers(t.Context(), "Actor Name")
	if err != nil {
		t.Fatal(err)
	}
	if len(items) != 1 || items[0].ProviderID != "BzpA" || items[0].Title != "Actor Name" {
		t.Fatalf("items = %#v", items)
	}
}

func TestAdultProviderSearchPerformersResolvesMovieDetails(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/search":
			_, _ = w.Write([]byte(`<a class="box" href="/v/abc" title="Sample"><strong>ABF-001</strong> Sample</a>`))
		case "/v/abc":
			_, _ = w.Write([]byte(`<h2>ABF-001 Sample</h2><a href="/actors/BzpA"><img src="https://c0.jdbstatic.com/actors/BzpA.jpg">Actor Name</a><strong class="symbol female">♀</strong>`))
		default:
			http.NotFound(w, r)
		}
	}))
	defer server.Close()
	withAdultDefaultBases(t, []string{server.URL})

	provider := NewAdultProvider(zap.NewNop(), nil)
	items, err := provider.SearchPerformers(t.Context(), "Actor Name")
	if err != nil {
		t.Fatal(err)
	}
	if len(items) != 1 || items[0].ProviderID != "BzpA" || items[0].PosterURL == "" {
		t.Fatalf("items = %#v", items)
	}
}

func TestAdultProviderSearchMoviesKeepsOnlyExactAdultCode(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/search" || r.URL.Query().Get("q") != "MIZD-534" || r.URL.Query().Get("f") != "all" {
			http.NotFound(w, r)
			return
		}
		_, _ = w.Write([]byte(`<a class="box" href="/v/mizd534" title="Sample title">
<img src="https://c0.jdbstatic.com/covers/mi/mizd534.jpg">
<strong>MIZD-534</strong><span>2026-08-01</span></a>
<a class="box" href="/v/unrelated" title="Unrelated title">
<strong>MIAA-001</strong><span>2026-07-01</span></a>`))
	}))
	defer server.Close()
	withAdultDefaultBases(t, []string{server.URL})

	provider := NewAdultProvider(zap.NewNop(), nil)
	items, err := provider.SearchMovies(t.Context(), "MIZD-534")
	if err != nil {
		t.Fatal(err)
	}
	if len(items) != 1 || items[0].ProviderID != "mizd534" || items[0].OriginalName != "MIZD-534" {
		t.Fatalf("items = %#v", items)
	}
	if items[0].ReleaseDate != "2026-08-01" || items[0].MediaType != "adult" {
		t.Fatalf("release metadata = %#v", items[0])
	}
}

func TestAdultProviderSearchMoviesReturnsEmptyWithoutExactAdultCode(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_, _ = w.Write([]byte(`<a class="box" href="/v/unrelated" title="Unrelated title">
<strong>MIAA-001</strong><span>2026-07-01</span></a>`))
	}))
	defer server.Close()
	withAdultDefaultBases(t, []string{server.URL})

	provider := NewAdultProvider(zap.NewNop(), nil)
	items, err := provider.SearchMovies(t.Context(), "MIZD-534")
	if err != nil {
		t.Fatal(err)
	}
	if len(items) != 0 {
		t.Fatalf("items = %#v, want empty exact-code result", items)
	}
}

func TestAdultProviderSearchMoviesFiltersUnrelatedTitles(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_, _ = w.Write([]byte(`<a class="box" href="/v/match" title="Sample title extended">
<strong>MIAA-001</strong></a>
<a class="box" href="/v/unrelated" title="Different title">
<strong>MIAA-002</strong></a>`))
	}))
	defer server.Close()
	withAdultDefaultBases(t, []string{server.URL})

	provider := NewAdultProvider(zap.NewNop(), nil)
	items, err := provider.SearchMovies(t.Context(), "Sample title")
	if err != nil {
		t.Fatal(err)
	}
	if len(items) != 1 || items[0].ProviderID != "match" {
		t.Fatalf("items = %#v", items)
	}
}
