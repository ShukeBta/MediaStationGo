package service

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/ShukeBta/MediaStationGo/internal/config"
	"github.com/ShukeBta/MediaStationGo/internal/model"
	"github.com/ShukeBta/MediaStationGo/internal/repository"
	"go.uber.org/zap"
)

func TestPipelineScrapePropagatesEpisodeMetadataToSiblingRows(t *testing.T) {
	seasonCalls := 0
	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/tv/272432/season/1" {
			http.NotFound(w, r)
			return
		}
		seasonCalls++
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(map[string]any{
			"episodes": []map[string]any{
				{"episode_number": 1, "name": "第一章", "overview": "第一集简介", "still_path": "/s01e01.jpg"},
				{"episode_number": 2, "name": "第二章", "overview": "第二集简介", "still_path": "/s01e02.jpg"},
				{"episode_number": 3, "name": "第三章", "overview": "第三集简介"},
			},
		})
	}))
	defer upstream.Close()

	db := newServiceTestDB(t, &model.Library{}, &model.LibraryRoot{}, &model.Media{})
	repos := repository.New(db)
	cfg := &config.Config{}
	cfg.Secrets.TMDbAPIKey = "test-key"
	cfg.Secrets.TMDbAPIProxy = upstream.URL
	cfg.Secrets.TMDbImageProxy = upstream.URL + "/images"
	log := zap.NewNop()
	scraper := NewScraperService(cfg, log, repos, NewTMDbProvider(cfg, log, nil), nil, nil, nil, NewHub(log))
	svc := NewPipelineScrapeService(repos, scraper)

	lib := model.Library{Name: "TV", Path: "cloud://openlist/115/剧集", Type: "tv", Enabled: true}
	if err := repos.DB.Create(&lib).Error; err != nil {
		t.Fatal(err)
	}
	root := model.LibraryRoot{LibraryID: lib.ID, Name: "TV", Path: "cloud://openlist/115/剧集", Enabled: true}
	if err := repos.DB.Create(&root).Error; err != nil {
		t.Fatal(err)
	}
	rows := []model.Media{
		{LibraryID: lib.ID, LibraryRootID: root.ID, Title: "Noisy", Path: "cloud://openlist/115/剧集/低智商犯罪/S01E01.mkv", SeasonNum: 1, EpisodeNum: 1, EpisodeTitle: "第1集", ScrapeStatus: "pending"},
		{LibraryID: lib.ID, LibraryRootID: root.ID, Title: "低智商犯罪", Path: "cloud://openlist/115/剧集/低智商犯罪/S01E02.mkv", SeasonNum: 1, EpisodeNum: 2, EpisodeTitle: "第2集", ScrapeStatus: "matched", TMDbID: 272432, PosterURL: "/cache/poster.jpg", BackdropURL: "/cache/backdrop.jpg"},
		{LibraryID: lib.ID, LibraryRootID: root.ID, Title: "Noisy", Path: "cloud://openlist/115/剧集/低智商犯罪/S01E03.mkv", SeasonNum: 1, EpisodeNum: 3, EpisodeTitle: "第3集", ScrapeStatus: "pending", BackdropURL: "/cache/backdrop.jpg"},
		{LibraryID: lib.ID, LibraryRootID: root.ID, Title: "Other", Path: "cloud://openlist/115/剧集/其他剧/S01E01.mkv", SeasonNum: 1, EpisodeNum: 1, ScrapeStatus: "pending"},
	}
	if err := repos.DB.Create(&rows).Error; err != nil {
		t.Fatal(err)
	}

	updated, err := svc.propagateEpisodeMatch(t.Context(), &rows[1], &rows[1], PipelineScrapeRequest{Category: "tv"})
	if err != nil {
		t.Fatal(err)
	}
	if updated != 2 {
		t.Fatalf("updated=%d, want 2 sibling rows", updated)
	}

	var got []model.Media
	if err := repos.DB.Where("library_id = ?", lib.ID).Order("path").Find(&got).Error; err != nil {
		t.Fatal(err)
	}
	for _, row := range got[:3] {
		if row.Title != "低智商犯罪" || row.ScrapeStatus != "matched" || row.TMDbID != 272432 || row.PosterURL != "/cache/poster.jpg" {
			t.Fatalf("episode metadata not propagated: %#v", row)
		}
		wantTitle := map[int]string{1: "第一章", 2: "第二章", 3: "第三章"}[row.EpisodeNum]
		wantBackdrop := map[int]string{
			1: upstream.URL + "/images/w500/s01e01.jpg",
			2: upstream.URL + "/images/w500/s01e02.jpg",
			3: "",
		}[row.EpisodeNum]
		if row.EpisodeTitle != wantTitle || row.BackdropURL != wantBackdrop {
			t.Fatalf("episode detail not applied: got title=%q backdrop=%q, want title=%q backdrop=%q", row.EpisodeTitle, row.BackdropURL, wantTitle, wantBackdrop)
		}
	}
	if seasonCalls != 1 {
		t.Fatalf("season detail requests=%d, want 1", seasonCalls)
	}
	if got[3].Title != "Other" || got[3].ScrapeStatus != "pending" {
		t.Fatalf("unrelated row changed: %#v", got[3])
	}
}

func TestPipelineScrapeRejectsIncompleteSeasonEpisodeDetails(t *testing.T) {
	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/tv/119769/season/1" {
			http.NotFound(w, r)
			return
		}
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(map[string]any{
			"episodes": []map[string]any{
				{"episode_number": 1, "name": "正常标题", "still_path": "/s01e01.jpg"},
			},
		})
	}))
	defer upstream.Close()

	db := newServiceTestDB(t, &model.Library{}, &model.LibraryRoot{}, &model.Media{})
	repos := repository.New(db)
	cfg := &config.Config{}
	cfg.Secrets.TMDbAPIKey = "test-key"
	cfg.Secrets.TMDbAPIProxy = upstream.URL
	cfg.Secrets.TMDbImageProxy = upstream.URL + "/images"
	log := zap.NewNop()
	scraper := NewScraperService(cfg, log, repos, NewTMDbProvider(cfg, log, nil), nil, nil, nil, NewHub(log))
	svc := NewPipelineScrapeService(repos, scraper)

	lib := model.Library{Name: "TV", Path: "cloud://openlist/115/剧集", Type: "tv", Enabled: true}
	if err := repos.DB.Create(&lib).Error; err != nil {
		t.Fatal(err)
	}
	root := model.LibraryRoot{LibraryID: lib.ID, Name: "TV", Path: lib.Path, Enabled: true}
	if err := repos.DB.Create(&root).Error; err != nil {
		t.Fatal(err)
	}
	rows := []model.Media{
		{LibraryID: lib.ID, LibraryRootID: root.ID, Title: "模范出租车", Path: "cloud://openlist/115/剧集/模范出租车1/Taxi.Driver.E01.mkv", SeasonNum: 1, EpisodeNum: 1, TMDbID: 119769, ScrapeStatus: "matched"},
		{LibraryID: lib.ID, LibraryRootID: root.ID, Title: "模范出租车", Path: "cloud://openlist/115/剧集/模范出租车1/Taxi.Driver.E02.mkv", SeasonNum: 1, EpisodeNum: 2, ScrapeStatus: "pending"},
	}
	if err := repos.DB.Create(&rows).Error; err != nil {
		t.Fatal(err)
	}

	if _, err := svc.propagateEpisodeMatch(t.Context(), &rows[0], &rows[0], PipelineScrapeRequest{Category: "tv"}); err == nil {
		t.Fatal("incomplete TMDb season details unexpectedly reported success")
	} else if !strings.Contains(err.Error(), "S01E02") {
		t.Fatalf("error=%q, want missing episode identity", err)
	}

	var got []model.Media
	if err := repos.DB.Order("episode_num ASC").Find(&got).Error; err != nil {
		t.Fatal(err)
	}
	for _, row := range got {
		if row.ScrapeStatus != "pending" {
			t.Fatalf("episode %d status=%q, want pending after incomplete batch", row.EpisodeNum, row.ScrapeStatus)
		}
		if row.EpisodeTitle != "" || row.BackdropURL != "" {
			t.Fatalf("episode %d received partial details: %#v", row.EpisodeNum, row)
		}
	}
}

func TestPipelineScrapeDefersAnchorEpisodeDetailsToSeasonBatch(t *testing.T) {
	if !pipelineScrapeOptions().DeferEpisodeDetails {
		t.Fatal("pipeline scrape must defer the anchor episode request to the season batch")
	}
}

func TestPipelineScrapeAppliesUniqueManualMatch(t *testing.T) {
	withAdultDefaultBases(t, nil)
	searchCalls := 0
	detailCalls := 0
	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/search":
			searchCalls++
			w.Header().Set("Content-Type", "text/html; charset=utf-8")
			_, _ = w.Write([]byte(`<a class="box" href="/v/mide949"><strong>MIDE-949 manual candidate</strong></a>`))
		case "/v/mide949":
			detailCalls++
			w.Header().Set("Content-Type", "text/html; charset=utf-8")
			_, _ = w.Write([]byte(`<h2 class="title"><strong>MIDE-949 correct title</strong></h2><img class="video-cover" src="/cover.jpg">`))
		default:
			http.NotFound(w, r)
		}
	}))
	defer upstream.Close()

	db := newServiceTestDB(t, &model.Library{}, &model.Series{}, &model.Media{}, &model.APIConfig{})
	repos := repository.New(db)
	apiConfig := NewAPIConfigService(zap.NewNop(), repos, NewCryptoService("", zap.NewNop()))
	baseURL := upstream.URL
	if _, err := apiConfig.Update(t.Context(), "adult", APIConfigPatch{BaseURL: &baseURL}); err != nil {
		t.Fatal(err)
	}
	log := zap.NewNop()
	scraper := NewScraperService(&config.Config{}, log, repos, nil, nil, nil, nil, NewHub(log), NewAdultProvider(log, apiConfig))
	svc := NewPipelineScrapeService(repos, scraper)

	lib := model.Library{Name: "Adult", Path: "cloud://openlist/115/adult", Type: "adult", Enabled: true}
	if err := repos.DB.Create(&lib).Error; err != nil {
		t.Fatal(err)
	}
	media := model.Media{LibraryID: lib.ID, Title: "mide 949", OriginalName: "mide 949", Path: "cloud://openlist/115/adult/MIDE-949/MIDE-949.mp4"}
	if err := repos.DB.Create(&media).Error; err != nil {
		t.Fatal(err)
	}

	result, err := svc.Scrape(t.Context(), media.ID, PipelineScrapeRequest{
		Category:  "adult",
		Title:     "mide 949",
		Queries:   []string{"MIDE-949"},
		Provider:  "adult",
		MediaType: "adult",
	})
	if err != nil {
		t.Fatal(err)
	}
	if result.Mode != PipelineScrapeModeApply || result.Query != "MIDE-949" || result.MatchCount != 1 {
		t.Fatalf("unexpected result: %#v", result)
	}

	var got model.Media
	if err := repos.DB.First(&got, "id = ?", media.ID).Error; err != nil {
		t.Fatal(err)
	}
	if got.Title != "correct title" || got.ScrapeStatus != "matched" {
		t.Fatalf("manual match was not applied: title=%q status=%q", got.Title, got.ScrapeStatus)
	}
	if searchCalls != 1 || detailCalls != 1 {
		t.Fatalf("pipeline repeated resolved adult lookup: search calls=%d detail calls=%d, want 1 each", searchCalls, detailCalls)
	}
}

func TestPipelineScrapeAdultKeepsOnlyExplicitNumberQuery(t *testing.T) {
	withAdultDefaultBases(t, nil)
	searchQueries := make([]string, 0, 2)
	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/search":
			searchQueries = append(searchQueries, r.URL.Query().Get("q"))
			w.Header().Set("Content-Type", "text/html; charset=utf-8")
			if r.URL.Query().Get("q") == "SIS-001" {
				_, _ = w.Write([]byte(`<a class="box" href="/v/sis001"><strong>SIS-001 wrong candidate</strong></a>`))
			}
		case "/v/sis001":
			w.Header().Set("Content-Type", "text/html; charset=utf-8")
			_, _ = w.Write([]byte(`<h2 class="title"><strong>SIS-001 wrong title</strong></h2>`))
		default:
			http.NotFound(w, r)
		}
	}))
	defer upstream.Close()

	db := newServiceTestDB(t, &model.Library{}, &model.Series{}, &model.Media{}, &model.APIConfig{})
	repos := repository.New(db)
	apiConfig := NewAPIConfigService(zap.NewNop(), repos, NewCryptoService("", zap.NewNop()))
	baseURL := upstream.URL
	if _, err := apiConfig.Update(t.Context(), "adult", APIConfigPatch{BaseURL: &baseURL}); err != nil {
		t.Fatal(err)
	}
	log := zap.NewNop()
	scraper := NewScraperService(&config.Config{}, log, repos, nil, nil, nil, nil, NewHub(log), NewAdultProvider(log, apiConfig))
	svc := NewPipelineScrapeService(repos, scraper)

	lib := model.Library{Name: "Adult", Path: "cloud://openlist/115/adult", Type: "adult", Enabled: true}
	if err := repos.DB.Create(&lib).Error; err != nil {
		t.Fatal(err)
	}
	media := model.Media{
		LibraryID:    lib.ID,
		Title:        "ABF-362",
		OriginalName: "ABF-362",
		Path:         "cloud://openlist/115/adult/第一會所@SIS001@ABF-362-U/ABF-362-U.mp4",
	}
	if err := repos.DB.Create(&media).Error; err != nil {
		t.Fatal(err)
	}

	result, err := svc.Scrape(t.Context(), media.ID, PipelineScrapeRequest{
		Category:  "adult",
		Title:     "ABF-362",
		Queries:   []string{"ABF-362", "无码破解"},
		Provider:  "adult",
		MediaType: "adult",
	})
	if err != nil {
		t.Fatal(err)
	}
	if result.Mode != PipelineScrapeModeSmart || result.ScrapeStatus == "matched" {
		t.Fatalf("unexpected result: %#v", result)
	}
	if len(searchQueries) == 0 {
		t.Fatal("expected adult lookup")
	}
	for _, query := range searchQueries {
		if query != "ABF-362" {
			t.Fatalf("adult pipeline searched %q, want only the explicit ABF-362 number; all queries=%v", query, searchQueries)
		}
	}

	var got model.Media
	if err := repos.DB.First(&got, "id = ?", media.ID).Error; err != nil {
		t.Fatal(err)
	}
	if got.Title != "ABF-362" || got.OriginalName != "ABF-362" || got.ScrapeStatus == "matched" {
		t.Fatalf("wrong adult candidate was applied: %+v", got)
	}
}

func TestPipelineScrapeReusesResolvedFD2Match(t *testing.T) {
	withAdultDefaultBases(t, []string{"https://unused.invalid"})
	flareCalls := 0
	flare := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		flareCalls++
		if r.URL.Path != "/v1" {
			t.Fatalf("FlareSolverr path = %q", r.URL.Path)
		}
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(map[string]any{
			"status": "ok",
			"solution": map[string]any{
				"status": 200,
				"response": `<h1 class="work-title">4516148</h1>
					<div class="work-brief">FD2 resolved title</div>
					<div class="work-original-photos">https://img.example/fd2.jpg</div>
					<div class="work-meta-label">配信日</div><div class="work-meta-value">2024-08-11</div>`,
			},
		})
	}))
	defer flare.Close()

	db := newServiceTestDB(t, &model.Library{}, &model.Series{}, &model.Media{})
	repos := repository.New(db)
	log := zap.NewNop()
	adult := NewAdultProvider(log, nil)
	adult.SetFlareSolverr(flare.URL, 5)
	scraper := NewScraperService(&config.Config{}, log, repos, nil, nil, nil, nil, NewHub(log), adult)
	svc := NewPipelineScrapeService(repos, scraper)

	lib := model.Library{Name: "Adult", Path: "cloud://openlist/115/adult", Type: "adult", Enabled: true}
	if err := repos.DB.Create(&lib).Error; err != nil {
		t.Fatal(err)
	}
	media := model.Media{
		LibraryID:    lib.ID,
		Title:        "FC2-PPV-4516148",
		OriginalName: "FC2-PPV-4516148",
		Path:         "cloud://openlist/115/adult/FC2-PPV-4516148/FC2-PPV-4516148.mp4",
	}
	if err := repos.DB.Create(&media).Error; err != nil {
		t.Fatal(err)
	}
	result, err := svc.Scrape(t.Context(), media.ID, PipelineScrapeRequest{
		Category:  "adult",
		Title:     "FC2-PPV-4516148",
		Queries:   []string{"FC2-PPV-4516148"},
		Provider:  "adult",
		MediaType: "adult",
	})
	if err != nil {
		t.Fatal(err)
	}
	if result.Mode != PipelineScrapeModeApply || result.AppliedCount != 1 {
		t.Fatalf("unexpected result: %#v, source requests = %d", result, flareCalls)
	}
	if flareCalls != 1 {
		t.Fatalf("FD2 requests = %d, want 1", flareCalls)
	}

	var got model.Media
	if err := repos.DB.First(&got, "id = ?", media.ID).Error; err != nil {
		t.Fatal(err)
	}
	if got.Title != "FD2 resolved title" || got.ReleaseDate != "2024-08-11" || got.ScrapeStatus != "matched" {
		t.Fatalf("FD2 match was not applied: %+v", got)
	}
}

func TestPipelineScrapeMatchesForMediaType(t *testing.T) {
	matches := []ExternalMediaResult{
		{MediaType: "movie", Title: "黑衣人2", TMDbID: 608},
		{MediaType: "tv", Title: "Bo' Selecta!", TMDbID: 608},
	}

	filtered := pipelineScrapeMatchesForMediaType(matches, "movie")
	if len(filtered) != 1 {
		t.Fatalf("filtered matches = %d, want 1", len(filtered))
	}
	if filtered[0].MediaType != "movie" || filtered[0].Title != "黑衣人2" || filtered[0].TMDbID != 608 {
		t.Fatalf("filtered match = %+v", filtered[0])
	}
}

func TestPipelineScrapeNoMatchIsNotReportedAsApplied(t *testing.T) {
	db := newServiceTestDB(t, &model.Library{}, &model.Media{})
	repos := repository.New(db)
	log := zap.NewNop()
	scraper := NewScraperService(&config.Config{}, log, repos, nil, nil, nil, nil, NewHub(log))
	svc := NewPipelineScrapeService(repos, scraper)

	lib := model.Library{Name: "TV", Path: "cloud://openlist/115/剧集", Type: "tv", Enabled: true}
	if err := repos.DB.Create(&lib).Error; err != nil {
		t.Fatal(err)
	}
	media := model.Media{
		LibraryID:  lib.ID,
		Title:      "unknown hybrid multi",
		Path:       "cloud://openlist/115/剧集/Unknown.Hybrid.MULTI/S01/Unknown.S01E01.mkv",
		SeasonNum:  1,
		EpisodeNum: 1,
	}
	if err := repos.DB.Create(&media).Error; err != nil {
		t.Fatal(err)
	}

	result, err := svc.Scrape(t.Context(), media.ID, PipelineScrapeRequest{Category: "tv"})
	if err != nil {
		t.Fatal(err)
	}
	if result.Mode != PipelineScrapeModeSmart || result.ScrapeStatus != "no_match" || result.AppliedCount != 0 {
		t.Fatalf("unexpected no-match result: %#v", result)
	}

	var got model.Media
	if err := repos.DB.First(&got, "id = ?", media.ID).Error; err != nil {
		t.Fatal(err)
	}
	if got.ScrapeStatus != "no_match" {
		t.Fatalf("media scrape status = %q, want no_match", got.ScrapeStatus)
	}
}
