package service

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/glebarez/sqlite"
	"go.uber.org/zap"
	"gorm.io/gorm"

	"github.com/ShukeBta/MediaStationGo/internal/config"
	"github.com/ShukeBta/MediaStationGo/internal/model"
	"github.com/ShukeBta/MediaStationGo/internal/repository"
)

func TestManualRequestMatchFallsBackToCandidatePayload(t *testing.T) {
	scraper := &ScraperService{}
	match, err := scraper.manualRequestMatch(t.Context(), ManualScrapeRequest{
		Source:   "douban",
		Title:    "手动选择的电影",
		DoubanID: "1234567",
		Year:     2026,
	})
	if err != nil {
		t.Fatal(err)
	}
	if match.Title != "手动选择的电影" || match.DoubanID != "1234567" || match.Year != 2026 {
		t.Fatalf("fallback match = %#v", match)
	}
}

func TestParsePositiveIDStringAcceptsProviderPrefixes(t *testing.T) {
	cases := map[string]string{
		"12345":          "12345",
		"tmdb:12345":     "12345",
		"[tmdbid-12345]": "12345",
		"douban:67890":   "67890",
		"{douban=67890}": "67890",
		"thetvdb:24680":  "24680",
	}
	for input, want := range cases {
		got, ok := parsePositiveIDString(input)
		if !ok || got != want {
			t.Fatalf("parsePositiveIDString(%q) = %q,%v; want %q,true", input, got, ok, want)
		}
	}
	if got, ok := parsePositiveIDString("tmdb:not-a-number"); ok || got != "" {
		t.Fatalf("parsePositiveIDString invalid = %q,%v; want empty,false", got, ok)
	}
	if got, ok := parseProviderIDString("[tmdbid-12345]", "douban"); ok || got != "" {
		t.Fatalf("parseProviderIDString provider mismatch = %q,%v; want empty,false", got, ok)
	}
}

func TestManualSearchReturnsTMDbCandidatePage(t *testing.T) {
	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		if r.URL.Path != "/search/movie" {
			http.NotFound(w, r)
			return
		}
		_ = json.NewEncoder(w).Encode(map[string]any{
			"results": []map[string]any{
				{
					"id":            101,
					"title":         "错误的同名电影",
					"poster_path":   "/wrong.jpg",
					"release_date":  "2021-01-01",
					"vote_average":  5.1,
					"genre_ids":     []int{18},
					"backdrop_path": "/wrong-backdrop.jpg",
				},
				{
					"id":            202,
					"title":         "正确的同名电影",
					"poster_path":   "/right.jpg",
					"release_date":  "2021-08-01",
					"vote_average":  8.2,
					"genre_ids":     []int{28},
					"backdrop_path": "/right-backdrop.jpg",
				},
			},
		})
	}))
	defer upstream.Close()

	db, err := gorm.Open(sqlite.Open("file::memory:?cache=shared"), &gorm.Config{})
	if err != nil {
		t.Fatal(err)
	}
	if err := db.AutoMigrate(&model.Library{}, &model.Series{}, &model.Media{}); err != nil {
		t.Fatal(err)
	}
	repos := repository.New(db)
	cfg := &config.Config{}
	cfg.Secrets.TMDbAPIKey = "test-key"
	cfg.Secrets.TMDbAPIProxy = upstream.URL
	log := zap.NewNop()
	scraper := NewScraperService(cfg, log, repos, NewTMDbProvider(cfg, log, nil), nil, nil, nil, NewHub(log))

	lib := model.Library{Name: "电影", Path: "/media/movie", Type: "movie", Enabled: true}
	if err := repos.DB.Create(&lib).Error; err != nil {
		t.Fatal(err)
	}
	media := model.Media{LibraryID: lib.ID, Title: "同名电影", Path: "/media/movie/同名电影.mkv"}
	if err := repos.DB.Create(&media).Error; err != nil {
		t.Fatal(err)
	}

	results, err := scraper.ManualSearch(t.Context(), &media, "同名电影", "tmdb", "movie")
	if err != nil {
		t.Fatal(err)
	}
	if len(results) != 2 || results[0].TMDbID != 101 || results[1].TMDbID != 202 {
		t.Fatalf("manual TMDb candidates = %#v", results)
	}
}

func TestManualSearchFallsBackToMovieFolderForGenericQuery(t *testing.T) {
	var queries []string
	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		queries = append(queries, r.URL.Query().Get("query"))
		w.Header().Set("Content-Type", "application/json")
		if r.URL.Path != "/search/movie" {
			http.NotFound(w, r)
			return
		}
		if r.URL.Query().Get("query") != "inception" {
			_ = json.NewEncoder(w).Encode(map[string]any{"results": []any{}})
			return
		}
		_ = json.NewEncoder(w).Encode(map[string]any{
			"results": []map[string]any{{
				"id":             27205,
				"title":          "Inception",
				"overview":       "A thief enters dreams.",
				"poster_path":    "/inception.jpg",
				"release_date":   "2010-07-16",
				"vote_average":   8.4,
				"original_title": "Inception",
			}},
		})
	}))
	defer upstream.Close()

	db, err := gorm.Open(sqlite.Open("file::memory:?cache=shared"), &gorm.Config{})
	if err != nil {
		t.Fatal(err)
	}
	if err := db.AutoMigrate(&model.Library{}, &model.Series{}, &model.Media{}); err != nil {
		t.Fatal(err)
	}
	repos := repository.New(db)
	cfg := &config.Config{}
	cfg.Secrets.TMDbAPIKey = "test-key"
	cfg.Secrets.TMDbAPIProxy = upstream.URL
	log := zap.NewNop()
	scraper := NewScraperService(cfg, log, repos, NewTMDbProvider(cfg, log, nil), nil, nil, nil, NewHub(log))

	lib := model.Library{Name: "Movies", Path: `/media/movies`, Type: "movie", Enabled: true}
	if err := repos.DB.Create(&lib).Error; err != nil {
		t.Fatal(err)
	}
	media := model.Media{
		LibraryID: lib.ID,
		Title:     "00000",
		Path:      `/media/movies/Inception (2010)/BDMV/STREAM/00000.m2ts`,
	}

	results, err := scraper.ManualSearch(t.Context(), &media, "00000", "tmdb", "movie")
	if err != nil {
		t.Fatal(err)
	}
	if len(results) != 1 || results[0].TMDbID != 27205 {
		t.Fatalf("manual search results=%#v, want folder fallback candidate; queries=%v", results, queries)
	}
	if len(queries) < 2 || queries[0] != "00000" || queries[1] != "inception" {
		t.Fatalf("manual search queries=%v, want explicit query then folder fallback", queries)
	}
}

func TestManualSearchReturnsMovieFallbackForTVTypedTMDbSearch(t *testing.T) {
	var paths []string
	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		paths = append(paths, r.URL.Path)
		w.Header().Set("Content-Type", "application/json")
		switch r.URL.Path {
		case "/search/tv":
			_ = json.NewEncoder(w).Encode(map[string]any{"results": []any{}})
		case "/search/movie":
			_ = json.NewEncoder(w).Encode(map[string]any{
				"results": []map[string]any{{
					"id":            808,
					"title":         "正义女神",
					"poster_path":   "/movie.jpg",
					"release_date":  "2024-01-01",
					"vote_average":  7.1,
					"backdrop_path": "/movie-backdrop.jpg",
				}},
			})
		default:
			http.NotFound(w, r)
		}
	}))
	defer upstream.Close()

	db, err := gorm.Open(sqlite.Open("file::memory:?cache=shared"), &gorm.Config{})
	if err != nil {
		t.Fatal(err)
	}
	if err := db.AutoMigrate(&model.Library{}, &model.Series{}, &model.Media{}); err != nil {
		t.Fatal(err)
	}
	repos := repository.New(db)
	cfg := &config.Config{}
	cfg.Secrets.TMDbAPIKey = "test-key"
	cfg.Secrets.TMDbAPIProxy = upstream.URL
	log := zap.NewNop()
	scraper := NewScraperService(cfg, log, repos, NewTMDbProvider(cfg, log, nil), nil, nil, nil, NewHub(log))

	lib := model.Library{Name: "电视剧", Path: `/media/tv`, Type: "tv", Enabled: true}
	if err := repos.DB.Create(&lib).Error; err != nil {
		t.Fatal(err)
	}
	media := model.Media{LibraryID: lib.ID, Title: "正义女神", Path: `/media/tv/正义女神.mkv`}
	if err := repos.DB.Create(&media).Error; err != nil {
		t.Fatal(err)
	}

	results, err := scraper.ManualSearch(t.Context(), &media, "正义女神", "tmdb", "tv")
	if err != nil {
		t.Fatal(err)
	}
	if len(results) != 1 || results[0].TMDbID != 808 || results[0].MediaType != "movie" {
		t.Fatalf("manual TMDb fallback results=%#v, paths=%v", results, paths)
	}
	if len(paths) < 2 || paths[0] != "/search/tv" || paths[1] != "/search/movie" {
		t.Fatalf("tmdb search paths=%v, want tv first then movie fallback", paths)
	}
}

func TestManualSearchAllProvidersTMDbNumericIDTriesMovieAndTVNamespaces(t *testing.T) {
	var paths []string
	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		paths = append(paths, r.URL.Path)
		w.Header().Set("Content-Type", "application/json")
		switch r.URL.Path {
		case "/movie/12345":
			http.NotFound(w, r)
		case "/tv/12345":
			_ = json.NewEncoder(w).Encode(map[string]any{
				"id":             12345,
				"name":           "数字 ID 剧集",
				"original_name":  "Numeric ID Show",
				"overview":       "Matched from TV namespace.",
				"poster_path":    "/tv.jpg",
				"first_air_date": "2025-01-01",
				"vote_average":   7.8,
			})
		default:
			http.NotFound(w, r)
		}
	}))
	defer upstream.Close()

	db, err := gorm.Open(sqlite.Open("file::memory:?cache=shared"), &gorm.Config{})
	if err != nil {
		t.Fatal(err)
	}
	if err := db.AutoMigrate(&model.Library{}, &model.Series{}, &model.Media{}); err != nil {
		t.Fatal(err)
	}
	repos := repository.New(db)
	cfg := &config.Config{}
	cfg.Secrets.TMDbAPIKey = "test-key"
	cfg.Secrets.TMDbAPIProxy = upstream.URL
	log := zap.NewNop()
	scraper := NewScraperService(cfg, log, repos, NewTMDbProvider(cfg, log, nil), nil, nil, nil, NewHub(log))

	lib := model.Library{Name: "电影", Path: `/media/movie`, Type: "movie", Enabled: true}
	if err := repos.DB.Create(&lib).Error; err != nil {
		t.Fatal(err)
	}
	media := model.Media{LibraryID: lib.ID, Title: "待匹配", Path: `/media/movie/raw-provider-id.mkv`}
	if err := repos.DB.Create(&media).Error; err != nil {
		t.Fatal(err)
	}

	results, err := scraper.ManualSearch(t.Context(), &media, "12345", "all", "movie")
	if err != nil {
		t.Fatal(err)
	}
	if len(results) != 1 || results[0].TMDbID != 12345 || results[0].MediaType != "tv" {
		t.Fatalf("manual TMDb numeric results=%#v, paths=%v", results, paths)
	}
	if len(paths) < 2 || paths[0] != "/movie/12345" || paths[1] != "/tv/12345" {
		t.Fatalf("tmdb numeric paths=%v, want movie then tv", paths)
	}
}

func TestManualSearchTMDbProviderIDUsesIDLookupOnly(t *testing.T) {
	var paths []string
	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		paths = append(paths, r.URL.Path)
		w.Header().Set("Content-Type", "application/json")
		switch r.URL.Path {
		case "/movie/1208850":
			_ = json.NewEncoder(w).Encode(map[string]any{
				"id":             1208850,
				"title":          "多拉特行动",
				"original_title": "Malbatt: Misi Bakara",
				"overview":       "ID matched movie.",
				"poster_path":    "/malbatt.jpg",
				"release_date":   "2024-01-11",
				"vote_average":   6.9,
			})
		default:
			http.NotFound(w, r)
		}
	}))
	defer upstream.Close()

	db, err := gorm.Open(sqlite.Open("file::memory:?cache=shared"), &gorm.Config{})
	if err != nil {
		t.Fatal(err)
	}
	if err := db.AutoMigrate(&model.Library{}, &model.Series{}, &model.Media{}); err != nil {
		t.Fatal(err)
	}
	repos := repository.New(db)
	cfg := &config.Config{}
	cfg.Secrets.TMDbAPIKey = "test-key"
	cfg.Secrets.TMDbAPIProxy = upstream.URL
	log := zap.NewNop()
	scraper := NewScraperService(cfg, log, repos, NewTMDbProvider(cfg, log, nil), nil, nil, nil, NewHub(log))

	lib := model.Library{Name: "电影", Path: `/media/movie`, Type: "movie", Enabled: true}
	if err := repos.DB.Create(&lib).Error; err != nil {
		t.Fatal(err)
	}
	media := model.Media{LibraryID: lib.ID, Title: "待匹配", Path: `/media/movie/raw-provider-id-skip-adult.mkv`}
	if err := repos.DB.Create(&media).Error; err != nil {
		t.Fatal(err)
	}

	results, err := scraper.ManualSearch(t.Context(), &media, "[tmdbid-1208850]", "all", "movie")
	if err != nil {
		t.Fatal(err)
	}
	if len(results) != 1 || results[0].TMDbID != 1208850 || results[0].Title != "多拉特行动" {
		t.Fatalf("manual TMDb provider-id results=%#v, paths=%v", results, paths)
	}
	for _, path := range paths {
		if strings.HasPrefix(path, "/search/") {
			t.Fatalf("provider-id search should not use fuzzy search; paths=%v", paths)
		}
	}
	if len(paths) == 0 || paths[0] != "/movie/1208850" {
		t.Fatalf("tmdb provider-id paths=%v, want /movie/1208850 first", paths)
	}
}

func TestManualSearchAllProvidersProviderIDSkipsAdultSource(t *testing.T) {
	var tmdbPaths []string
	tmdbUpstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		tmdbPaths = append(tmdbPaths, r.URL.Path)
		w.Header().Set("Content-Type", "application/json")
		switch r.URL.Path {
		case "/movie/1208850":
			_ = json.NewEncoder(w).Encode(map[string]any{
				"id":             1208850,
				"title":          "多拉特行动",
				"original_title": "Malbatt: Misi Bakara",
				"release_date":   "2024-01-11",
			})
		default:
			http.NotFound(w, r)
		}
	}))
	defer tmdbUpstream.Close()

	var adultCalls atomic.Int32
	adultUpstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		adultCalls.Add(1)
		http.NotFound(w, r)
	}))
	defer adultUpstream.Close()

	db, err := gorm.Open(sqlite.Open("file::memory:?cache=shared"), &gorm.Config{})
	if err != nil {
		t.Fatal(err)
	}
	if err := db.AutoMigrate(&model.Library{}, &model.Series{}, &model.Media{}, &model.APIConfig{}); err != nil {
		t.Fatal(err)
	}
	repos := repository.New(db)
	cfg := &config.Config{}
	cfg.Secrets.TMDbAPIKey = "test-key"
	cfg.Secrets.TMDbAPIProxy = tmdbUpstream.URL
	log := zap.NewNop()
	apiConfig := NewAPIConfigService(log, repos, NewCryptoService("", log))
	adultBaseURL := adultUpstream.URL
	if _, err := apiConfig.Update(t.Context(), "adult", APIConfigPatch{BaseURL: &adultBaseURL}); err != nil {
		t.Fatal(err)
	}
	scraper := NewScraperService(cfg, log, repos, NewTMDbProvider(cfg, log, nil), nil, nil, nil, NewHub(log), NewAdultProvider(log, apiConfig))

	lib := model.Library{Name: "电影", Path: `/media/movie`, Type: "movie", Enabled: true}
	if err := repos.DB.Create(&lib).Error; err != nil {
		t.Fatal(err)
	}
	media := model.Media{LibraryID: lib.ID, Title: "待匹配", Path: `/media/movie/raw.mkv`}
	if err := repos.DB.Create(&media).Error; err != nil {
		t.Fatal(err)
	}

	results, err := scraper.ManualSearch(t.Context(), &media, "[tmdbid-1208850]", "all", "movie")
	if err != nil {
		t.Fatal(err)
	}
	if len(results) != 1 || results[0].TMDbID != 1208850 || results[0].Source != "tmdb" {
		t.Fatalf("manual provider-id results=%#v, tmdb paths=%v", results, tmdbPaths)
	}
	if calls := adultCalls.Load(); calls != 0 {
		t.Fatalf("adult provider was called %d times for explicit tmdb id", calls)
	}
}

func TestManualTMDbCandidatesSkipOtherProviderIDs(t *testing.T) {
	var paths []string
	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		paths = append(paths, r.URL.Path)
		http.NotFound(w, r)
	}))
	defer upstream.Close()

	cfg := &config.Config{}
	cfg.Secrets.TMDbAPIKey = "test-key"
	cfg.Secrets.TMDbAPIProxy = upstream.URL
	log := zap.NewNop()
	scraper := NewScraperService(cfg, log, nil, NewTMDbProvider(cfg, log, nil), nil, nil, nil, NewHub(log))

	if got := scraper.manualTMDbCandidates(t.Context(), "{douban=36941123}", 0, "movie"); len(got) != 0 {
		t.Fatalf("tmdb candidates for douban id = %#v, want none", got)
	}
	if len(paths) != 0 {
		t.Fatalf("tmdb should not be called for douban id hint, paths=%v", paths)
	}
}

func TestManualSearchIncludesAdultProvider(t *testing.T) {
	withAdultDefaultBases(t, nil)

	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/search":
			w.Header().Set("Content-Type", "text/html; charset=utf-8")
			_, _ = w.Write([]byte(`<a class="box" href="/v/ssis001"><strong>SSIS-001 手动候选</strong></a>`))
		case "/v/ssis001":
			w.Header().Set("Content-Type", "text/html; charset=utf-8")
			_, _ = w.Write([]byte(`<h2 class="title"><strong>SSIS-001 手动成人标题</strong></h2><img class="video-cover" src="/cover.jpg">`))
		default:
			http.NotFound(w, r)
		}
	}))
	defer upstream.Close()

	db, err := gorm.Open(sqlite.Open("file::memory:?cache=shared"), &gorm.Config{})
	if err != nil {
		t.Fatal(err)
	}
	if err := db.AutoMigrate(&model.Library{}, &model.Series{}, &model.Media{}, &model.APIConfig{}); err != nil {
		t.Fatal(err)
	}
	repos := repository.New(db)
	apiConfig := NewAPIConfigService(zap.NewNop(), repos, NewCryptoService("", zap.NewNop()))
	baseURL := upstream.URL
	if _, err := apiConfig.Update(t.Context(), "adult", APIConfigPatch{BaseURL: &baseURL}); err != nil {
		t.Fatal(err)
	}
	log := zap.NewNop()
	scraper := NewScraperService(&config.Config{}, log, repos, nil, nil, nil, nil, NewHub(log), NewAdultProvider(log, apiConfig))

	lib := model.Library{Name: "成人", Path: "/media/adult", Type: "movie", Enabled: true}
	if err := repos.DB.Create(&lib).Error; err != nil {
		t.Fatal(err)
	}
	media := model.Media{LibraryID: lib.ID, Title: "SSIS-001", OriginalName: "SSIS-001", Path: "/media/adult/SSIS-001.mkv"}
	if err := repos.DB.Create(&media).Error; err != nil {
		t.Fatal(err)
	}

	results, err := scraper.ManualSearch(t.Context(), &media, "SSIS-001", "adult", "adult")
	if err != nil {
		t.Fatal(err)
	}
	if len(results) != 1 || results[0].Source != "adult" || results[0].MediaType != "adult" || !results[0].NSFW || results[0].OriginalName != "SSIS-001" {
		t.Fatalf("manual adult candidates = %#v", results)
	}
}

func TestApplyManualMatchSavesSelectedCloudMatchWhenDetailsSlow(t *testing.T) {
	oldTimeout := tmdbDetailsTimeout
	tmdbDetailsTimeout = 20 * time.Millisecond
	defer func() { tmdbDetailsTimeout = oldTimeout }()

	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/movie/77" {
			http.NotFound(w, r)
			return
		}
		select {
		case <-r.Context().Done():
			return
		case <-time.After(time.Second):
			_ = json.NewEncoder(w).Encode(map[string]any{
				"id":    77,
				"title": "Slow Details",
			})
		}
	}))
	defer upstream.Close()

	db, err := gorm.Open(sqlite.Open("file::memory:?cache=shared"), &gorm.Config{})
	if err != nil {
		t.Fatal(err)
	}
	if err := db.AutoMigrate(&model.Library{}, &model.Series{}, &model.Media{}); err != nil {
		t.Fatal(err)
	}
	repos := repository.New(db)
	cfg := &config.Config{}
	cfg.Secrets.TMDbAPIKey = "test-key"
	cfg.Secrets.TMDbAPIProxy = upstream.URL
	log := zap.NewNop()
	scraper := NewScraperService(cfg, log, repos, NewTMDbProvider(cfg, log, nil), nil, nil, nil, NewHub(log))

	lib := model.Library{Name: "OpenList · Movies", Path: "cloud://openlist/Movies", Type: "movie", Enabled: true}
	if err := repos.DB.Create(&lib).Error; err != nil {
		t.Fatal(err)
	}
	media := model.Media{
		LibraryID:    lib.ID,
		Title:        "bad cloud title",
		Path:         "cloud://openlist/Movies/Bad.Title.2026.mkv",
		ScrapeStatus: "pending",
	}
	if err := repos.DB.Create(&media).Error; err != nil {
		t.Fatal(err)
	}

	start := time.Now()
	if _, err := scraper.ApplyManualMatch(t.Context(), media.ID, ManualScrapeRequest{
		Source:    "manual",
		MediaType: "movie",
		Title:     "Correct Cloud Movie",
		TMDbID:    77,
		Year:      2026,
	}); err != nil {
		t.Fatal(err)
	}
	if elapsed := time.Since(start); elapsed > 500*time.Millisecond {
		t.Fatalf("manual apply waited for optional details: %s", elapsed)
	}

	var got model.Media
	if err := repos.DB.First(&got, "id = ?", media.ID).Error; err != nil {
		t.Fatal(err)
	}
	if got.Title != "Correct Cloud Movie" || got.ScrapeStatus != "matched" || got.TMDbID != 77 {
		t.Fatalf("manual cloud match was not saved: title=%q status=%q tmdb=%d", got.Title, got.ScrapeStatus, got.TMDbID)
	}
}

func TestApplyManualMatchBatchFetchesSeriesOnceAndEpisodesBySeason(t *testing.T) {
	var mu sync.Mutex
	paths := make([]string, 0)
	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		mu.Lock()
		paths = append(paths, r.URL.Path)
		mu.Unlock()
		w.Header().Set("Content-Type", "application/json")
		switch r.URL.Path {
		case "/tv/1434":
			_ = json.NewEncoder(w).Encode(map[string]any{
				"id":               1434,
				"name":             "恶搞之家",
				"original_name":    "Family Guy",
				"overview":         "整剧简介",
				"poster_path":      "/poster.jpg",
				"backdrop_path":    "/backdrop.jpg",
				"first_air_date":   "1999-01-31",
				"vote_average":     7.4,
				"episode_run_time": []int{22},
				"origin_country":   []string{"US"},
				"spoken_languages": []map[string]any{{"iso_639_1": "en"}},
				"genres":           []map[string]any{{"name": "动画"}, {"name": "喜剧"}},
				"credits": map[string]any{
					"cast": []map[string]any{{"id": 1, "name": "Seth MacFarlane"}},
				},
			})
		case "/tv/1434/season/1":
			_ = json.NewEncoder(w).Encode(map[string]any{
				"episodes": []map[string]any{
					{"episode_number": 1, "name": "Death Has a Shadow", "overview": "S01E01", "still_path": "/s01e01.jpg", "air_date": "1999-01-31", "vote_average": 7.8, "runtime": 22},
					{"episode_number": 2, "name": "I Never Met the Dead Man", "overview": "S01E02", "still_path": "/s01e02.jpg", "air_date": "1999-04-11", "vote_average": 7.6, "runtime": 22},
				},
			})
		case "/tv/1434/season/2":
			_ = json.NewEncoder(w).Encode(map[string]any{
				"episodes": []map[string]any{
					{"episode_number": 1, "name": "Peter, Peter, Caviar Eater", "overview": "S02E01", "still_path": "/s02e01.jpg", "air_date": "1999-09-23", "vote_average": 7.7, "runtime": 22},
				},
			})
		default:
			http.NotFound(w, r)
		}
	}))
	defer upstream.Close()

	db := newServiceTestDB(t, &model.Library{}, &model.Series{}, &model.Media{}, &model.Person{})
	repos := repository.New(db)
	cfg := &config.Config{}
	cfg.Secrets.TMDbAPIKey = "test-key"
	cfg.Secrets.TMDbAPIProxy = upstream.URL
	cfg.Secrets.TMDbImageProxy = upstream.URL + "/images"
	log := zap.NewNop()
	scraper := NewScraperService(cfg, log, repos, NewTMDbProvider(cfg, log, nil), nil, nil, nil, NewHub(log))

	lib := model.Library{Name: "欧美剧", Path: "cloud://openlist/115/欧美剧", Type: "tv", Enabled: true}
	if err := repos.DB.Create(&lib).Error; err != nil {
		t.Fatal(err)
	}
	rows := []model.Media{
		{Base: model.Base{ID: "fg-s01e01"}, LibraryID: lib.ID, Title: "Family Guy", Path: "cloud://openlist/115/Family Guy/S01/Family Guy S01E01.mkv", SeasonNum: 1, EpisodeNum: 1, ScrapeStatus: "pending"},
		{Base: model.Base{ID: "fg-s01e02"}, LibraryID: lib.ID, Title: "Family Guy", Path: "cloud://openlist/115/Family Guy/S01/Family Guy S01E02.mkv", SeasonNum: 1, EpisodeNum: 2, ScrapeStatus: "pending"},
		{Base: model.Base{ID: "fg-s02e01"}, LibraryID: lib.ID, Title: "Family Guy", Path: "cloud://openlist/115/Family Guy/S02/Family Guy S02E01.mkv", SeasonNum: 2, EpisodeNum: 1, ScrapeStatus: "pending"},
	}
	if err := repos.DB.Create(&rows).Error; err != nil {
		t.Fatal(err)
	}
	ids := []string{rows[0].ID, rows[1].ID, rows[2].ID}
	result, err := scraper.ApplyManualMatchBatchWithOptions(t.Context(), ids, ManualScrapeRequest{
		Source:    "tmdb",
		MediaType: "tv",
		Title:     "恶搞之家",
		TMDbID:    1434,
		Genres:    []string{"16", "35", "999999"},
	}, ScrapeOptions{})
	if err != nil {
		t.Fatal(err)
	}
	if len(result.AppliedIDs) != 3 || len(result.Errors) != 0 {
		t.Fatalf("batch result = %+v, want three applied rows", result)
	}

	mu.Lock()
	gotPaths := append([]string(nil), paths...)
	mu.Unlock()
	counts := map[string]int{}
	for _, path := range gotPaths {
		counts[path]++
		if strings.Contains(path, "/episode/") {
			t.Fatalf("batch apply fetched per-episode endpoint: paths=%v", gotPaths)
		}
	}
	if counts["/tv/1434"] != 1 || counts["/tv/1434/season/1"] != 1 || counts["/tv/1434/season/2"] != 1 || len(gotPaths) != 3 {
		t.Fatalf("unexpected TMDb request paths: %v", gotPaths)
	}

	var stored []model.Media
	if err := repos.DB.Where("library_id = ?", lib.ID).Order("season_num ASC, episode_num ASC").Find(&stored).Error; err != nil {
		t.Fatal(err)
	}
	if len(stored) != 3 || stored[0].EpisodeTitle != "Death Has a Shadow" || stored[1].EpisodeTitle != "I Never Met the Dead Man" || stored[2].EpisodeTitle != "Peter, Peter, Caviar Eater" {
		t.Fatalf("season episode metadata not applied: %+v", stored)
	}
	for _, media := range stored {
		if media.Title != "恶搞之家" || media.OriginalName != "Family Guy" || media.TMDbID != 1434 || media.ScrapeStatus != "matched" || media.Actors != "Seth MacFarlane" || media.Genres != "动画,喜剧" {
			t.Fatalf("shared series metadata not applied: %+v", media)
		}
	}
}

func TestApplyManualAdultMatchUsesSelectedCandidateWithoutRefetch(t *testing.T) {
	upstreamCalls := 0
	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		upstreamCalls++
		http.Error(w, "adult source must not be requested during apply", http.StatusInternalServerError)
	}))
	defer upstream.Close()
	withAdultDefaultBases(t, []string{upstream.URL})

	db := newServiceTestDB(t, &model.Library{}, &model.Media{})
	repos := repository.New(db)
	log := zap.NewNop()
	scraper := NewScraperService(&config.Config{}, log, repos, nil, nil, nil, nil, NewHub(log), NewAdultProvider(log, nil))

	lib := model.Library{Name: "Adult", Path: "cloud://openlist/115/adult", Type: "adult", Enabled: true}
	if err := repos.DB.Create(&lib).Error; err != nil {
		t.Fatal(err)
	}
	media := model.Media{LibraryID: lib.ID, Title: "MIDE-949", Path: "cloud://openlist/115/adult/MIDE-949/MIDE-949.mp4"}
	if err := repos.DB.Create(&media).Error; err != nil {
		t.Fatal(err)
	}

	got, err := scraper.ApplyManualMatch(t.Context(), media.ID, ManualScrapeRequest{
		Source:       "adult",
		MediaType:    "adult",
		Title:        "Selected adult title",
		OriginalName: "MIDE-949",
		PosterURL:    "https://img.example/mide949.jpg",
		Genres:       []string{"Adult", "javdb", "69"},
		NSFW:         true,
	})
	if err != nil {
		t.Fatal(err)
	}
	if upstreamCalls != 0 {
		t.Fatalf("adult apply source calls = %d, want 0", upstreamCalls)
	}
	if got == nil || got.Title != "Selected adult title" || got.PosterURL != "https://img.example/mide949.jpg" || got.ScrapeStatus != "matched" || got.Genres != "Adult,javdb,69" {
		t.Fatalf("selected adult match was not applied: %+v", got)
	}
}

func TestApplyManualMovieMatchClearsEpisodeMarkers(t *testing.T) {
	db, err := gorm.Open(sqlite.Open("file::memory:?cache=shared"), &gorm.Config{})
	if err != nil {
		t.Fatal(err)
	}
	if err := db.AutoMigrate(&model.Library{}, &model.Series{}, &model.Media{}); err != nil {
		t.Fatal(err)
	}
	repos := repository.New(db)
	log := zap.NewNop()
	scraper := NewScraperService(&config.Config{}, log, repos, nil, nil, nil, nil, NewHub(log))

	lib := model.Library{Name: "欧美剧", Path: "/media/tv/euus", Type: "tv", Enabled: true}
	if err := repos.DB.Create(&lib).Error; err != nil {
		t.Fatal(err)
	}
	media := model.Media{
		LibraryID:    lib.ID,
		Title:        "错误剧集标题",
		Path:         "/media/tv/euus/Dune/Season 01/Dune - S01E202.mkv",
		SeasonNum:    1,
		EpisodeNum:   202,
		EpisodeTitle: "第 202 集",
		SeriesID:     "series:wrong",
		TMDbID:       999999,
		TheTVDBID:    "888",
		ScrapeStatus: "matched",
	}
	if err := repos.DB.Create(&media).Error; err != nil {
		t.Fatal(err)
	}

	if _, err := scraper.ApplyManualMatch(t.Context(), media.ID, ManualScrapeRequest{
		Source:    "manual",
		MediaType: "movie",
		Title:     "Dune",
		Year:      2021,
	}); err != nil {
		t.Fatal(err)
	}

	var got model.Media
	if err := repos.DB.First(&got, "id = ?", media.ID).Error; err != nil {
		t.Fatal(err)
	}
	if got.SeasonNum != 0 || got.EpisodeNum != 0 || got.EpisodeTitle != "" || got.SeriesID != "" {
		t.Fatalf("episode markers were not cleared: %#v", got)
	}
	if got.TMDbID != 0 || got.TheTVDBID != "" {
		t.Fatalf("stale external IDs were not cleared for manual movie fallback: tmdb=%d thetvdb=%q", got.TMDbID, got.TheTVDBID)
	}
}
