package service

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"go.uber.org/zap"

	"github.com/ShukeBta/MediaStationGo/internal/model"
	"github.com/ShukeBta/MediaStationGo/internal/repository"
)

func TestNormalizeAdultCode(t *testing.T) {
	cases := map[string]string{
		"SSIS001.mp4":          "SSIS-001",
		"fc2-ppv-1234567.mkv":  "FC2-PPV-1234567",
		"heyzo_1234.mp4":       "HEYZO-1234",
		"120118_001-carib.mp4": "120118-001",
		"movie.1080p.x264.mkv": "",
	}
	for in, want := range cases {
		if got := normalizeAdultCode(in); got != want {
			t.Fatalf("normalizeAdultCode(%q) = %q, want %q", in, got, want)
		}
	}
}

func TestParseAdultDetailHTML(t *testing.T) {
	html := `<html>
<h2 class="title"><strong>SSIS-001 测试标题</strong></h2>
<img class="video-cover" src="/covers/ssis001.jpg">
<a class="sample-box" href="/samples/1.jpg"></a>
<a href="/actors/censored">有碼</a>
<a href="/actors/male">男性演员</a><strong class="symbol male">♂</strong>
<a href="/actors/abc">七沢みあ</a><strong class="symbol female">♀</strong>
<div class="panel-block"><strong>評分:</strong><span class="value">4.7分, 由9人評價</span></div>
<div class="panel-block"><strong>日期:</strong><span class="value">2024-05-01</span></div>
<div class="panel-block"><strong>時長:</strong><span class="value">125 分鍾</span></div>
<div class="panel-block"><strong>片商:</strong><span class="value"><a href="/makers/test">测试片商</a></span></div>
<div class="panel-block"><strong>類別:</strong><span class="value"><a>單體作品</a>, <a>美少女電影</a>, <a>69</a>, <a>999999</a></span></div>
</html>`
	got := parseAdultDetailHTML(html, "SSIS-001", "javdb", "https://javdb.com/v/abc")
	if got == nil {
		t.Fatal("parseAdultDetailHTML returned nil")
	}
	if got.Title != "测试标题" || got.OriginalName != "SSIS-001" || !got.NSFW {
		t.Fatalf("unexpected metadata: %+v", got)
	}
	if got.PosterURL != "https://javdb.com/covers/ssis001.jpg" || got.BackdropURL != "https://javdb.com/samples/1.jpg" {
		t.Fatalf("unexpected artwork: %+v", got)
	}
	if got.Year != 2024 {
		t.Fatalf("year = %d, want 2024", got.Year)
	}
	if got.ReleaseDate != "2024-05-01" || got.DurationMinutes != 125 || got.Maker != "测试片商" || got.Rating != 4.7 {
		t.Fatalf("detail fields = release %q duration %d maker %q rating %.1f", got.ReleaseDate, got.DurationMinutes, got.Maker, got.Rating)
	}
	if len(got.Genres) != 5 || got.Genres[2] != "單體作品" || got.Genres[3] != "美少女電影" || got.Genres[4] != "六九式" {
		t.Fatalf("genres = %#v", got.Genres)
	}
	if len(got.Actors) != 1 || got.Actors[0] != "七沢みあ" {
		t.Fatalf("actors = %#v", got.Actors)
	}
	if len(got.People) != 1 || got.People[0].Source != "javdb" || got.People[0].SourceID != "abc" {
		t.Fatalf("people = %#v", got.People)
	}
}

func TestParseAdultDetailHTMLReadsJavDBTileItemSample(t *testing.T) {
	body := `<html>
	<h2 class="title"><strong>EBWH-348 Sample</strong></h2>
	<img class="video-cover" src="https://c0.jdbstatic.com/covers/eb/EbPpD3.jpg">
	<a data-caption="sample" href="https://c0.jdbstatic.com/samples/eb/EbPpD3_l_0.jpg" class="tile-item"><img src="sample.jpg"></a>
	<a data-caption="sample" href="https://c0.jdbstatic.com/samples/eb/EbPpD3_s_1.jpg" class="tile-item"><img src="small.jpg"></a>
	<a data-caption="sample" href="https://c0.jdbstatic.com/samples/eb/EbPpD3_l_1.jpg" class="tile-item"><img src="sample-2.jpg"></a>
	</html>`

	got := parseAdultDetailHTML(body, "EBWH-348", "javdb", "https://javdb.com/v/EbPpD3")
	if got == nil {
		t.Fatal("parseAdultDetailHTML returned nil")
	}
	if got.BackdropURL != "https://c0.jdbstatic.com/samples/eb/EbPpD3_l_0.jpg" {
		t.Fatalf("BackdropURL = %q", got.BackdropURL)
	}
	if len(got.PreviewImages) != 2 || got.PreviewImages[0] != "https://c0.jdbstatic.com/samples/eb/EbPpD3_l_0.jpg" || got.PreviewImages[1] != "https://c0.jdbstatic.com/samples/eb/EbPpD3_l_1.jpg" {
		t.Fatalf("PreviewImages = %#v", got.PreviewImages)
	}
}

func TestValidAdultActorNameRejectsCategoryLabels(t *testing.T) {
	for _, value := range []string{"有码", "无码", "有码 无码", "Censored", "男性演员", "女优"} {
		if validAdultActorName(value) {
			t.Fatalf("validAdultActorName(%q) = true", value)
		}
	}
	for _, value := range []string{"石川澪", "七沢みあ"} {
		if !validAdultActorName(value) {
			t.Fatalf("validAdultActorName(%q) = false", value)
		}
	}
}

func TestAdultProviderDiscoverMovieDetailUsesRequestedJavDBItem(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/v/QNRVYG" {
			http.NotFound(w, r)
			return
		}
		_, _ = w.Write([]byte(`<html>
			<h2 class="title"><strong>MIZD-534 测试详情标题</strong></h2>
			<div class="panel-block"><strong>日期:</strong><span class="value">2026-08-04</span></div>
			<div class="panel-block"><strong>時長:</strong><span class="value">240 分鍾</span></div>
			<div class="panel-block"><strong>片商:</strong><span class="value">MOODYZ</span></div>
			<a href="/actors/QV0p9">石川澪</a><strong class="symbol female">♀</strong>
		</html>`))
	}))
	defer server.Close()
	withAdultDefaultBases(t, []string{server.URL})

	provider := NewAdultProvider(zap.NewNop(), nil)
	item, err := provider.DiscoverMovieDetail(context.Background(), "javdb", "QNRVYG", "MIZD-534")
	if err != nil {
		t.Fatal(err)
	}
	if item.ProviderID != "QNRVYG" || item.ProviderURL != server.URL+"/v/QNRVYG" {
		t.Fatalf("provider reference = %q %q", item.ProviderID, item.ProviderURL)
	}
	if item.ReleaseDate != "2026-08-04" || item.DurationMinutes != 240 || item.Maker != "MOODYZ" {
		t.Fatalf("detail item = %+v", item)
	}
	if len(item.People) != 1 || item.People[0].Name != "石川澪" || item.People[0].SourceID != "QV0p9" {
		t.Fatalf("people = %#v", item.People)
	}
}

func TestAdultProviderFallsBackToFlareSolverrForCloudflare403(t *testing.T) {
	directCalls := 0
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		directCalls++
		w.WriteHeader(http.StatusForbidden)
		_, _ = w.Write([]byte(`<html><title>Just a moment...</title><div id="cf-challenge-running"></div></html>`))
	}))
	defer server.Close()
	withAdultDefaultBases(t, []string{server.URL})

	requestedURL := ""
	flare := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/v1" {
			t.Fatalf("FlareSolverr path = %q", r.URL.Path)
		}
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
		requestedURL = request.URL
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(map[string]any{
			"status": "ok",
			"solution": map[string]any{
				"status": 200,
				"response": `<a class="box" title="SSIS-001 FlareSolverr title" href="/v/ssis001">
					<img src="/covers/ssis001.jpg">
					<div>2026-08-06</div>
				</a>`,
			},
		})
	}))
	defer flare.Close()

	provider := NewAdultProvider(zap.NewNop(), nil)
	provider.SetFlareSolverr(flare.URL, 5)
	items, err := provider.DiscoverJavDBPopular(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if directCalls != 1 {
		t.Fatalf("direct calls = %d, want 1", directCalls)
	}
	wantURL := server.URL + "/rankings/movies?p=daily&t=censored"
	if requestedURL != wantURL {
		t.Fatalf("FlareSolverr URL = %q, want %q", requestedURL, wantURL)
	}
	if len(items) != 1 || items[0].OriginalName != "SSIS-001" || items[0].Title != "FlareSolverr title" {
		t.Fatalf("items = %+v", items)
	}
}

func TestAdultProviderRefreshesJavDBClearanceAfterEmptyRanking(t *testing.T) {
	var directCalls atomic.Int32
	var flareCalls atomic.Int32
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		directCalls.Add(1)
		_, _ = w.Write([]byte(`<html><title>JavDB</title><p>empty ranking response</p></html>`))
	}))
	defer server.Close()
	withAdultDefaultBases(t, []string{server.URL})

	flare := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		flareCalls.Add(1)
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(map[string]any{
			"status": "ok",
			"solution": map[string]any{
				"status":    200,
				"userAgent": "fresh-browser",
				"response":  `<a href="/v/ssis001" title="SSIS-001 refreshed"><strong>SSIS-001</strong></a>`,
				"cookies":   []map[string]string{{"name": "cf_clearance", "value": "fresh"}},
			},
		})
	}))
	defer flare.Close()

	provider := NewAdultProvider(zap.NewNop(), nil)
	provider.SetFlareSolverr(flare.URL, 5)
	items, err := provider.DiscoverJavDBPopular(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if len(items) != 1 || items[0].OriginalName != "SSIS-001" {
		t.Fatalf("items = %#v", items)
	}
	if got := directCalls.Load(); got != 1 {
		t.Fatalf("direct calls = %d, want 1", got)
	}
	if got := flareCalls.Load(); got != 1 {
		t.Fatalf("FlareSolverr calls = %d, want 1", got)
	}
}

func TestAdultProviderReusesJavDBClearanceIdentity(t *testing.T) {
	var directCalls atomic.Int32
	var flareCalls atomic.Int32
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		directCalls.Add(1)
		cookie, err := r.Cookie("cf_clearance")
		if err != nil || cookie.Value != "fresh" || r.UserAgent() != "test-browser" {
			w.WriteHeader(http.StatusForbidden)
			_, _ = w.Write([]byte(`<title>Just a moment...</title><div id="cf-challenge-running"></div>`))
			return
		}
		_, _ = w.Write([]byte(`<a class="box" title="SSIS-001 reused title" href="/v/ssis001"><img src="/covers/ssis001.jpg"><div>2026-08-06</div></a>`))
	}))
	defer server.Close()
	withAdultDefaultBases(t, []string{server.URL})

	flare := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		flareCalls.Add(1)
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(map[string]any{
			"status": "ok",
			"solution": map[string]any{
				"status":    200,
				"userAgent": "test-browser",
				"response":  `<a class="box" title="SSIS-001 reused title" href="/v/ssis001"><img src="/covers/ssis001.jpg"><div>2026-08-06</div></a>`,
				"cookies":   []map[string]string{{"name": "cf_clearance", "value": "fresh"}},
			},
		})
	}))
	defer flare.Close()

	provider := NewAdultProvider(zap.NewNop(), nil)
	provider.SetFlareSolverr(flare.URL, 5)
	if err := provider.CheckJavDBSession(context.Background()); err != nil {
		t.Fatal(err)
	}
	items, err := provider.DiscoverJavDBPopular(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if len(items) != 1 || items[0].Title != "reused title" {
		t.Fatalf("items = %#v", items)
	}
	if got := flareCalls.Load(); got != 1 {
		t.Fatalf("FlareSolverr calls = %d, want 1", got)
	}
	if got := directCalls.Load(); got != 2 {
		t.Fatalf("direct calls = %d, want 2", got)
	}
}

func TestAdultProviderCoalescesConcurrentJavDBClearanceSolves(t *testing.T) {
	var flareCalls atomic.Int32
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		cookie, err := r.Cookie("cf_clearance")
		if err != nil || cookie.Value != "fresh" || r.UserAgent() != "test-browser" {
			w.WriteHeader(http.StatusForbidden)
			_, _ = w.Write([]byte(`<title>Just a moment...</title><div id="cf-challenge-running"></div>`))
			return
		}
		_, _ = w.Write([]byte(`<a class="box" title="SSIS-001 coalesced title" href="/v/ssis001"><img src="/covers/ssis001.jpg"><div>2026-08-06</div></a>`))
	}))
	defer server.Close()
	withAdultDefaultBases(t, []string{server.URL})

	flareStarted := make(chan struct{}, 1)
	releaseFlare := make(chan struct{})
	flare := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		flareCalls.Add(1)
		select {
		case flareStarted <- struct{}{}:
		default:
		}
		<-releaseFlare
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(map[string]any{
			"status": "ok",
			"solution": map[string]any{
				"status":    200,
				"userAgent": "test-browser",
				"response":  `<a class="box" title="SSIS-001 coalesced title" href="/v/ssis001"><img src="/covers/ssis001.jpg"><div>2026-08-06</div></a>`,
				"cookies":   []map[string]string{{"name": "cf_clearance", "value": "fresh"}},
			},
		})
	}))
	defer flare.Close()

	provider := NewAdultProvider(zap.NewNop(), nil)
	provider.SetFlareSolverr(flare.URL, 5)
	errs := make(chan error, 2)
	var wg sync.WaitGroup
	for i := 0; i < 2; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			items, err := provider.DiscoverJavDBPopular(context.Background())
			if err == nil && (len(items) != 1 || items[0].Title != "coalesced title") {
				err = fmt.Errorf("items = %#v", items)
			}
			errs <- err
		}()
	}
	select {
	case <-flareStarted:
	case <-time.After(2 * time.Second):
		t.Fatal("FlareSolverr solve did not start")
	}
	close(releaseFlare)
	wg.Wait()
	close(errs)
	for err := range errs {
		if err != nil {
			t.Fatal(err)
		}
	}
	if got := flareCalls.Load(); got != 1 {
		t.Fatalf("FlareSolverr calls = %d, want 1", got)
	}
}

func TestAdultProviderLimitsConcurrentFlareSolverrRequests(t *testing.T) {
	var active int32
	var maxActive int32
	started := make(chan struct{}, 3)
	release := make(chan struct{})
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		current := atomic.AddInt32(&active, 1)
		for {
			previous := atomic.LoadInt32(&maxActive)
			if current <= previous || atomic.CompareAndSwapInt32(&maxActive, previous, current) {
				break
			}
		}
		started <- struct{}{}
		<-release
		atomic.AddInt32(&active, -1)
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"status":"ok","solution":{"status":200,"response":"<html>ok</html>"}}`))
	}))
	defer server.Close()

	provider := NewAdultProvider(zap.NewNop(), nil)
	provider.SetFlareSolverr(server.URL, 5)
	var wg sync.WaitGroup
	errs := make(chan error, 3)
	for i := 0; i < 3; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			_, err := provider.fetchWithFlareSolverrResultContext(context.Background(), "https://javdb.com/actors", nil)
			errs <- err
		}()
	}
	for i := 0; i < 2; i++ {
		select {
		case <-started:
		case <-time.After(2 * time.Second):
			t.Fatal("FlareSolverr requests did not start")
		}
	}
	select {
	case <-started:
		t.Fatal("FlareSolverr gate allowed more than two concurrent requests")
	case <-time.After(100 * time.Millisecond):
	}
	close(release)
	wg.Wait()
	close(errs)
	for err := range errs {
		if err != nil {
			t.Fatal(err)
		}
	}
	if got := atomic.LoadInt32(&maxActive); got != 2 {
		t.Fatalf("max concurrent FlareSolverr requests = %d, want 2", got)
	}
}

func TestParseAdultDetailHTMLReadsJSONLDActors(t *testing.T) {
	body := `<html>
<h3>ABF-001 Test</h3>
<script type="application/ld+json">{"@type":"Movie","actor":[{"name":"Actor A"},{"name":"Actor B"}]}</script>
</html>`

	got := parseAdultDetailHTML(body, "ABF-001", "javbus", "https://example.com/ABF-001")
	if got == nil {
		t.Fatal("parseAdultDetailHTML returned nil")
	}
	if len(got.Actors) != 2 || got.Actors[0] != "Actor A" || got.Actors[1] != "Actor B" {
		t.Fatalf("actors = %#v", got.Actors)
	}
}

func TestParseAdultDetailHTMLDerivesDMMPoster(t *testing.T) {
	html := `<html>
	<h3>NACR-833 测试标题</h3>
	<a class="sample-box" href="https://pics.dmm.co.jp/digital/video/h_237nacr00833/h_237nacr00833jp-1.jpg"></a>
	</html>`

	got := parseAdultDetailHTML(html, "NACR-833", "javbus", "https://www.javbus.com/NACR-833")
	if got == nil {
		t.Fatal("parseAdultDetailHTML returned nil")
	}
	if got.PosterURL != "https://pics.dmm.co.jp/digital/video/h_237nacr00833/h_237nacr00833pl.jpg" {
		t.Fatalf("PosterURL = %q", got.PosterURL)
	}
}

func TestParseAdultDetailHTMLClassifiesMGStageArtwork(t *testing.T) {
	html := `<html>
	<h3>ABF-246 测试标题</h3>
	<a class="bigImage" href="https://www.javbus.com/pics/cover/bima_b.jpg"></a>
	<a class="sample-box" href="https://image.mgstage.com/images/prestige/abf/246/cap_e_0_abf-246.jpg"></a>
	</html>`

	got := parseAdultDetailHTML(html, "ABF-246", "javbus", "https://www.javbus.com/ABF-246")
	if got == nil {
		t.Fatal("parseAdultDetailHTML returned nil")
	}
	if got.PosterURL != "https://image.mgstage.com/images/prestige/abf/246/cap_e_0_abf-246.jpg" {
		t.Fatalf("PosterURL = %q", got.PosterURL)
	}
	if got.BackdropURL != "https://image.mgstage.com/images/prestige/abf/246/pb_e_abf-246.jpg" {
		t.Fatalf("BackdropURL = %q", got.BackdropURL)
	}
}

func withAdultDefaultBases(t *testing.T, bases []string) {
	t.Helper()
	original := defaultAdultBases
	defaultAdultBases = append([]string(nil), bases...)
	t.Cleanup(func() {
		defaultAdultBases = original
	})
}

func TestAdultProviderUsesConfiguredMultipleSources(t *testing.T) {
	withAdultDefaultBases(t, nil)

	bad := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		http.Error(w, "temporary failure", http.StatusInternalServerError)
	}))
	defer bad.Close()

	good := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/search":
			w.Header().Set("Content-Type", "text/html; charset=utf-8")
			_, _ = w.Write([]byte(`<a class="box" href="/v/ssis001"><strong>SSIS-001 多源入口</strong></a>`))
		case "/v/ssis001":
			w.Header().Set("Content-Type", "text/html; charset=utf-8")
			_, _ = w.Write([]byte(`<h2 class="title"><strong>SSIS-001 多源命中标题</strong></h2><img class="video-cover" src="/cover.jpg">`))
		default:
			http.NotFound(w, r)
		}
	}))
	defer good.Close()

	db := newServiceTestDB(t, &model.APIConfig{})
	apiConfig := NewAPIConfigService(zap.NewNop(), repository.New(db), NewCryptoService("", zap.NewNop()))
	baseURL := bad.URL + "\n" + good.URL
	if _, err := apiConfig.Update(context.Background(), "adult", APIConfigPatch{BaseURL: &baseURL}); err != nil {
		t.Fatal(err)
	}

	provider := NewAdultProvider(zap.NewNop(), apiConfig)
	match, err := provider.Search(context.Background(), "SSIS-001")
	if err != nil {
		t.Fatal(err)
	}
	if match == nil || match.Title != "多源命中标题" || match.OriginalName != "SSIS-001" || !match.NSFW {
		t.Fatalf("multi-source adult match = %+v", match)
	}
}

func TestAdultProviderSearchAllReturnsMultipleSourceMatches(t *testing.T) {
	withAdultDefaultBases(t, nil)

	first := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/search":
			w.Header().Set("Content-Type", "text/html; charset=utf-8")
			_, _ = w.Write([]byte(`<a class="box" href="/v/adn093"><strong>ADN-093 第一源</strong></a>`))
		case "/v/adn093":
			w.Header().Set("Content-Type", "text/html; charset=utf-8")
			_, _ = w.Write([]byte(`<h2 class="title"><strong>ADN-093 第一源标题</strong></h2><img class="video-cover" src="/cover1.jpg">`))
		default:
			http.NotFound(w, r)
		}
	}))
	defer first.Close()
	second := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/search":
			w.Header().Set("Content-Type", "text/html; charset=utf-8")
			_, _ = w.Write([]byte(`<a class="box" href="/v/adn093"><strong>ADN-093 第二源</strong></a>`))
		case "/v/adn093":
			w.Header().Set("Content-Type", "text/html; charset=utf-8")
			_, _ = w.Write([]byte(`<h2 class="title"><strong>ADN-093 第二源标题</strong></h2><img class="video-cover" src="/cover2.jpg">`))
		default:
			http.NotFound(w, r)
		}
	}))
	defer second.Close()

	db := newServiceTestDB(t, &model.APIConfig{})
	apiConfig := NewAPIConfigService(zap.NewNop(), repository.New(db), NewCryptoService("", zap.NewNop()))
	baseURL := first.URL + "\n" + second.URL
	if _, err := apiConfig.Update(context.Background(), "adult", APIConfigPatch{BaseURL: &baseURL}); err != nil {
		t.Fatal(err)
	}

	provider := NewAdultProvider(zap.NewNop(), apiConfig)
	matches, err := provider.SearchAll(context.Background(), "ADN-093")
	if err != nil {
		t.Fatal(err)
	}
	if len(matches) != 2 {
		t.Fatalf("matches len = %d, want 2: %+v", len(matches), matches)
	}
	if matches[0].Title != "第一源标题" || matches[1].Title != "第二源标题" {
		t.Fatalf("unexpected multi-source titles: %+v", matches)
	}
}

func TestAdultSourceKindRecognizesAdultSources(t *testing.T) {
	cases := map[string]string{
		"https://javdb.com":       "javdb",
		"https://onejav.com":      "onejav",
		"https://javbus.sbs":      "javbus",
		"https://www.javbus.com":  "javbus",
		"https://www.cdnbus.cyou": "javbus",
		"https://www.javsee.cyou": "javbus",
		"https://www.busjav.cyou": "javbus",
		"www.cdnbus.cyou":         "javbus",
		"https://example.invalid": "javdb",
	}
	for in, want := range cases {
		if got := adultSourceKind(in); got != want {
			t.Fatalf("adultSourceKind(%q) = %q, want %q", in, got, want)
		}
	}
}

func TestAdultProviderDefaultBases(t *testing.T) {
	provider := &AdultProvider{}
	got := provider.resolveBases(context.Background())
	want := []string{
		"https://javdb.com",
		"https://onejav.com",
		"https://javbus.sbs",
		"https://www.javbus.com",
		"https://www.cdnbus.cyou",
		"https://www.javsee.cyou",
		"https://www.busjav.cyou",
	}
	if len(got) != len(want) {
		t.Fatalf("resolveBases len = %d, want %d: %v", len(got), len(want), got)
	}
	for i := range want {
		if got[i] != want[i] {
			t.Fatalf("resolveBases[%d] = %q, want %q", i, got[i], want[i])
		}
	}
}

func TestAdultConfiguredBasesSkipsMissAV(t *testing.T) {
	got := adultConfiguredBases("https://missav.live, javdb.com; https://www.javbus.com")
	want := []string{"https://javdb.com", "https://www.javbus.com"}
	if len(got) != len(want) {
		t.Fatalf("adultConfiguredBases len = %d, want %d: %v", len(got), len(want), got)
	}
	for i := range want {
		if got[i] != want[i] {
			t.Fatalf("adultConfiguredBases[%d] = %q, want %q", i, got[i], want[i])
		}
	}
}

func TestParseOneJavDetailHTML(t *testing.T) {
	html := `<html><title>FC2PPV4661145 - OneJAV.com - Free JAV Torrents</title><img class="image" src="/fc2.jpg"></html>`
	got := parseOneJavDetailHTML(html, "FC2-PPV-4661145", "https://onejav.com/torrent/fc2ppv4661145")
	if got == nil {
		t.Fatal("parseOneJavDetailHTML returned nil")
	}
	if got.Title != "FC2-PPV-4661145" || got.OriginalName != "FC2-PPV-4661145" || got.PosterURL != "https://onejav.com/fc2.jpg" {
		t.Fatalf("unexpected onejav match: %+v", got)
	}
}

func TestParseFD2PPVDetailHTML(t *testing.T) {
	body := `<html>
		<h1 class="work-title">3780016 <span>copy</span></h1>
		<div class="work-brief">【#102】色白コスメ店員の作品タイトル</div>
		<div class="work-original-photos">
			https://storage.example/cover.jpg
			https://preview.example/ps1.webp
			https://preview.example/ps2.webp
		</div>
		<div class="work-meta-label">カテゴリ</div><div class="work-meta-value"><span>流出</span></div>
		<div class="work-meta-label">モザイク</div><div class="work-meta-value"><span>無</span></div>
		<div class="work-meta-label">顔出し</div><div class="work-meta-value"><span>○</span></div>
		<div class="work-meta-label">配信日</div><div class="work-meta-value">2023-09-07</div>
		<div class="work-meta-label">収録時間</div><div class="work-meta-value" id="duration">00:48:57</div>
		<div class="work-meta-label">販売者</div><div class="work-meta-value"><a href="/channels/test">趣味はめ</a></div>
		<h3 class="artist-name"><a href="/actresses/1669" class="artistUrl" data-actress="1669">中田ゆめ</a></h3>
	</html>`

	got := parseFD2PPVDetailHTML(body, "FC2-PPV-3780016", "https://fd2ppv.cc/articles/3780016")
	if got == nil {
		t.Fatal("parseFD2PPVDetailHTML returned nil")
	}
	if got.Title != "【#102】色白コスメ店員の作品タイトル" || got.OriginalName != "FC2-PPV-3780016" {
		t.Fatalf("titles = %q / %q", got.Title, got.OriginalName)
	}
	if got.ReleaseDate != "2023-09-07" || got.Year != 2023 || got.DurationMinutes != 48 || got.Maker != "趣味はめ" {
		t.Fatalf("detail fields = %+v", got)
	}
	if got.PosterURL != "https://storage.example/cover.jpg" || got.BackdropURL != got.PosterURL {
		t.Fatalf("artwork = poster %q backdrop %q", got.PosterURL, got.BackdropURL)
	}
	if len(got.PreviewImages) != 2 || got.PreviewImages[0] != "https://preview.example/ps1.webp" {
		t.Fatalf("preview images = %#v", got.PreviewImages)
	}
	if len(got.Actors) != 1 || got.Actors[0] != "中田ゆめ" || len(got.People) != 1 {
		t.Fatalf("people = %#v actors = %#v", got.People, got.Actors)
	}
	if got.People[0].Source != "fd2ppv" || got.People[0].SourceID != "1669" || got.People[0].ProfileURL != "https://fd2ppv.cc/actresses/1669" {
		t.Fatalf("person source = %+v", got.People[0])
	}
	if len(got.Genres) != 5 || got.Genres[2] != "流出" || got.Genres[3] != "无码" || got.Genres[4] != "露脸" {
		t.Fatalf("genres = %#v", got.Genres)
	}
}

func TestParseFD2PPVDetailHTMLRejectsMismatchedWork(t *testing.T) {
	body := `<h1 class="work-title">3780017</h1><div class="work-brief">wrong work</div>`
	if got := parseFD2PPVDetailHTML(body, "FC2-PPV-3780016", "https://fd2ppv.cc/articles/3780016"); got != nil {
		t.Fatalf("mismatched work = %+v", got)
	}
}

func TestAdultProviderUsesFD2PPVForFC2BeforeLegacySources(t *testing.T) {
	withAdultDefaultBases(t, []string{"https://unused.invalid"})
	requestedURL := ""
	flare := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/v1" {
			t.Fatalf("FlareSolverr path = %q", r.URL.Path)
		}
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
		requestedURL = request.URL
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(map[string]any{
			"status": "ok",
			"solution": map[string]any{
				"status": 200,
				"response": `<h1 class="work-title">3780016</h1>
					<div class="work-brief">FD2 rich title</div>
					<div class="work-original-photos">https://img.example/fd2.jpg</div>
					<div class="work-meta-label">配信日</div><div class="work-meta-value">2023-09-07</div>`,
			},
		})
	}))
	defer flare.Close()

	provider := NewAdultProvider(zap.NewNop(), nil)
	provider.SetFlareSolverr(flare.URL, 5)
	match, err := provider.Search(context.Background(), "FC2-PPV-3780016")
	if err != nil {
		t.Fatal(err)
	}
	if requestedURL != "https://fd2ppv.cc/articles/3780016" {
		t.Fatalf("target URL = %q", requestedURL)
	}
	if match == nil || match.Title != "FD2 rich title" || match.PosterURL != "https://img.example/fd2.jpg" {
		t.Fatalf("fd2ppv match = %+v", match)
	}
	if len(match.Genres) < 2 || match.Genres[1] != "fd2ppv" {
		t.Fatalf("match genres = %#v", match.Genres)
	}
}

func TestAdultProviderDoesNotHideFD2PPVFailureBehindLegacyFC2Match(t *testing.T) {
	legacyCalls := 0
	legacy := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		legacyCalls++
		_, _ = w.Write([]byte(`<title>FC2PPV3780016 - OneJAV.com</title><img class="image" src="/cover.jpg">`))
	}))
	defer legacy.Close()
	withAdultDefaultBases(t, []string{legacy.URL})

	flare := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(map[string]any{
			"status":  "error",
			"message": "challenge failed",
		})
	}))
	defer flare.Close()

	provider := NewAdultProvider(zap.NewNop(), nil)
	provider.SetFlareSolverr(flare.URL, 5)
	match, err := provider.Search(context.Background(), "FC2-PPV-3780016")
	if err == nil || match != nil {
		t.Fatalf("match = %+v, err = %v", match, err)
	}
	if legacyCalls != 0 {
		t.Fatalf("legacy FC2 source calls = %d, want 0", legacyCalls)
	}
}
