package service

import (
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"strconv"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/ShukeBta/MediaStationGo/internal/config"
	"github.com/ShukeBta/MediaStationGo/internal/model"
	"github.com/ShukeBta/MediaStationGo/internal/repository"
	"go.uber.org/zap"
)

// Exercise the production tracker adapter, subscription planner and downloader
// HTTP client. No AI service, resource-import tables or pipeline are installed.
func TestPTSubscriptionTracksNewEpisodesWithoutAIOrPipeline(t *testing.T) {
	t.Setenv("MEDIASTATION_DOWNLOAD_CONTAINER_DIR", "")
	t.Setenv("MEDIASTATION_DOWNLOAD_DIR", "")
	customRoot, defaultRoot := t.TempDir(), t.TempDir()
	var released atomic.Int32
	released.Store(1)
	var searches, otherSearches, metadataCalls, addAttempts atomic.Int32
	var rejectDownload atomic.Bool
	var rejectToken atomic.Bool
	var liveMu sync.Mutex
	live := []map[string]any{}
	payloads := make(map[string][]byte)
	names := make(map[string]string)
	for episode := 1; episode <= 3; episode++ {
		name := fmt.Sprintf("Embedded Show S01E%02d 2160p WEB-DL HEVC 10bit DDP5.1", episode)
		payload := []byte(fmt.Sprintf("d4:infod4:name%d:%see", len(name), name))
		payloads[strconv.Itoa(episode)] = payload
		names[torrentInfoHash(payload)] = name
	}
	var tracker *httptest.Server
	tracker = httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Header.Get("x-api-key") != "test-tracker-key" {
			http.Error(w, "missing tracker credential", http.StatusUnauthorized)
			return
		}
		switch r.URL.Path {
		case "/api/torrent/search":
			searches.Add(1)
			var request map[string]any
			_ = json.NewDecoder(r.Body).Decode(&request)
			if request["keyword"] != "Embedded Show" {
				t.Errorf("tracker keyword = %#v", request["keyword"])
			}
			rows := []map[string]any{}
			for episode := 1; episode <= int(released.Load()); episode++ {
				rows = append(rows, map[string]any{
					"id": strconv.Itoa(episode), "name": names[torrentInfoHash(payloads[strconv.Itoa(episode)])],
					"size": "1000000000", "status": map[string]any{"seeders": "10"},
				})
			}
			_ = json.NewEncoder(w).Encode(map[string]any{"code": "0", "data": map[string]any{"total": len(rows), "data": rows}})
		case "/api/torrent/genDlToken":
			if rejectToken.Load() {
				http.Error(w, "tracker token temporarily unavailable", http.StatusServiceUnavailable)
				return
			}
			_ = json.NewEncoder(w).Encode(map[string]any{"code": "0", "data": tracker.URL + "/torrent?id=" + r.URL.Query().Get("id")})
		case "/torrent":
			w.Header().Set("Content-Type", "application/x-bittorrent")
			_, _ = w.Write(payloads[r.URL.Query().Get("id")])
		default:
			http.NotFound(w, r)
		}
	}))
	t.Cleanup(tracker.Close)
	other := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		otherSearches.Add(1)
		http.Error(w, "wrong tracker scope", http.StatusInternalServerError)
	}))
	t.Cleanup(other.Close)
	metadata := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		metadataCalls.Add(1)
		http.Error(w, "metadata is optional", http.StatusServiceUnavailable)
	}))
	t.Cleanup(metadata.Close)
	qb := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/api/v2/auth/login":
			_, _ = w.Write([]byte("Ok."))
		case "/api/v2/torrents/info":
			liveMu.Lock()
			defer liveMu.Unlock()
			_ = json.NewEncoder(w).Encode(live)
		case "/api/v2/torrents/add":
			addAttempts.Add(1)
			if rejectDownload.Load() {
				http.Error(w, "downloader temporarily unavailable", http.StatusServiceUnavailable)
				return
			}
			if err := r.ParseMultipartForm(1 << 20); err != nil {
				t.Errorf("parse torrent upload: %v", err)
				http.Error(w, "invalid upload", http.StatusBadRequest)
				return
			}
			defer r.MultipartForm.RemoveAll()
			if r.FormValue("autoTMM") != "false" {
				t.Error("explicit subscription destination must disable qB automatic path management")
			}
			file, _, err := r.FormFile("torrents")
			if err != nil {
				t.Errorf("expected authenticated torrent-file upload: %v", err)
				http.Error(w, "missing torrent", http.StatusBadRequest)
				return
			}
			defer file.Close()
			data, _ := io.ReadAll(file)
			hash := torrentInfoHash(data)
			name, ok := names[hash]
			if !ok {
				t.Errorf("unexpected torrent uploaded: %q", data)
			}
			liveMu.Lock()
			live = append(live, map[string]any{"hash": hash, "name": name, "state": "downloading", "progress": 0.1, "save_path": r.FormValue("savepath")})
			liveMu.Unlock()
			_, _ = w.Write([]byte("Ok."))
		default:
			http.NotFound(w, r)
		}
	}))
	t.Cleanup(qb.Close)

	db := newServiceTestDB(t, &model.Subscription{}, &model.Setting{}, &model.Site{}, &model.DownloadTask{}, &model.DownloadClient{}, &model.Media{})
	repos := repository.New(db)
	sites := NewSiteService(zap.NewNop(), repos, "")
	site := model.Site{Name: "selected tracker", Type: "mteam", URL: tracker.URL, APIKey: "test-tracker-key", AuthType: "api_key", Enabled: true, Timeout: 5}
	if err := sites.Create(t.Context(), &site); err != nil {
		t.Fatal(err)
	}
	if err := sites.Create(t.Context(), &model.Site{Name: "other tracker", Type: "mteam", URL: other.URL, APIKey: "other", AuthType: "api_key", Enabled: true, Timeout: 5}); err != nil {
		t.Fatal(err)
	}
	configureTestDefaultQB(t, repos, qb.URL)
	if err := repos.Setting.Set(t.Context(), "qbittorrent.savepath", defaultRoot); err != nil {
		t.Fatal(err)
	}
	downloads := NewDownloadService(zap.NewNop(), repos, NewHub(zap.NewNop()), nil, sites)
	cfg := &config.Config{}
	svc := NewSubscriptionService(cfg, zap.NewNop(), repos, downloads, sites, NewHub(zap.NewNop()))
	svc.SetScraper(&ScraperService{tmdb: NewTMDbProvider(&config.Config{Secrets: config.SecretsConfig{TMDbAPIKey: "optional", TMDbAPIProxy: metadata.URL}}, zap.NewNop(), nil)})
	sub := &model.Subscription{UserID: "u1", Name: "Embedded Show 自动订阅", Filter: "Embedded Show", FeedURL: SiteSearchURL("Embedded Show", site.ID, "", false), Enabled: true, PollIntervalMinutes: 5, SavePath: customRoot, MediaCategory: "国产剧"}
	if err := svc.Create(t.Context(), sub); err != nil {
		t.Fatal(err)
	}
	if sub.DeliveryMode != "download" || svc.resourceImport != nil || cfg.ResourceImport.Enabled {
		t.Fatal("PT subscription must use internal download delivery")
	}
	assertRun := func(wantQueued int) {
		t.Helper()
		queued, err := svc.RunNow(t.Context(), sub.ID)
		if err != nil || queued != wantQueued {
			t.Fatalf("RunNow = %d, %v; want %d", queued, err, wantQueued)
		}
	}
	assertRun(1)
	assertSavePath := func(episode int, want string) {
		t.Helper()
		hash := torrentInfoHash(payloads[strconv.Itoa(episode)])
		liveMu.Lock()
		var clientPath string
		for _, torrent := range live {
			if torrent["hash"] == hash {
				clientPath, _ = torrent["save_path"].(string)
			}
		}
		liveMu.Unlock()
		if clientPath != want {
			t.Fatalf("episode %d qB savepath = %q, want %q", episode, clientPath, want)
		}
		var task model.DownloadTask
		if err := db.Where("subscription_id = ? AND title = ?", sub.ID, names[hash]).First(&task).Error; err != nil {
			t.Fatal(err)
		}
		if task.SavePath != want {
			t.Fatalf("episode %d task save_path = %q, want %q", episode, task.SavePath, want)
		}
	}
	assertSavePath(1, filepath.Join(customRoot, "国产剧"))
	assertRun(0)
	if addAttempts.Load() != 1 {
		t.Fatalf("duplicate run posted %d downloads", addAttempts.Load())
	}
	var stored model.Subscription
	if err := db.First(&stored, "id = ?", sub.ID).Error; err != nil {
		t.Fatal(err)
	}
	if stored.TotalEpisodes != 0 || stored.ArchivedAt != nil || !stored.Enabled || stored.MediaType != "tv" {
		t.Fatalf("release frontier prematurely completed subscription: %#v", stored)
	}
	// Clearing the override must affect the next release, without moving an
	// already queued torrent or breaking duplicate detection for that episode.
	if err := svc.Update(t.Context(), sub.ID, map[string]any{"save_path": ""}); err != nil {
		t.Fatal(err)
	}
	released.Store(2)
	assertRun(1)
	assertSavePath(1, filepath.Join(customRoot, "国产剧"))
	assertSavePath(2, filepath.Join(defaultRoot, "国产剧"))

	// A failed enqueue must remain retryable; it must not enter the seen set.
	released.Store(3)
	rejectToken.Store(true)
	beforeAdd := addAttempts.Load()
	if queued, err := svc.RunNow(t.Context(), sub.ID); queued != 0 || err == nil {
		t.Fatalf("failed tracker token RunNow = %d, %v", queued, err)
	}
	if addAttempts.Load() != beforeAdd {
		t.Fatal("tracker token failure was sent to the downloader")
	}
	rejectToken.Store(false)
	rejectDownload.Store(true)
	if queued, err := svc.RunNow(t.Context(), sub.ID); queued != 0 || err == nil {
		t.Fatalf("failed downloader RunNow = %d, %v", queued, err)
	}
	if count := ptSubscriptionDownloadCount(t, repos, sub.ID); count != 2 {
		t.Fatalf("failed enqueue persisted %d tasks, want 2", count)
	}
	rejectDownload.Store(false)
	before := searches.Load()
	svc.runAll(t.Context())
	if searches.Load() != before {
		t.Fatal("scheduler ignored the saved five-minute interval")
	}
	past := time.Now().Add(-6 * time.Minute)
	if err := db.Model(sub).Update("last_run_at", &past).Error; err != nil {
		t.Fatal(err)
	}
	svc.runAll(t.Context())
	if count := ptSubscriptionDownloadCount(t, repos, sub.ID); count != 3 {
		t.Fatalf("scheduled retry persisted %d tasks, want 3", count)
	}
	assertRun(0)
	if metadataCalls.Load() != 0 || otherSearches.Load() != 0 {
		t.Fatalf("unexpected dependencies: metadata=%d other tracker=%d", metadataCalls.Load(), otherSearches.Load())
	}
}

func ptSubscriptionDownloadCount(t *testing.T, repos *repository.Container, id string) int64 {
	t.Helper()
	var count int64
	if err := repos.DB.Model(&model.DownloadTask{}).Where("subscription_id = ?", id).Count(&count).Error; err != nil {
		t.Fatal(err)
	}
	return count
}

func TestPTSubscriptionCompatibilityOnlyUsesExplicitCodecFilters(t *testing.T) {
	sub := &model.Subscription{FeedURL: "site-search://resources?keyword=Show", MediaType: "tv"}
	title := "Show S01E01 2160p WEB-DL HEVC 10bit DoVi Dolby Atmos DDP5.1"
	if !matchesSubscriptionRules(sub, title) {
		t.Fatal("default PT rules must accept common codecs")
	}
	sub.ExcludeWords = "hevc"
	if matchesSubscriptionRules(sub, title) {
		t.Fatal("explicit codec exclusion must still apply")
	}
	sub.ExcludeWords = ""
	for _, tag := range []string{"SAMPLE", "CAM", "禁止下载"} {
		if matchesSubscriptionRules(sub, "Show S01E01 1080p "+tag) {
			t.Errorf("default unwanted-release rule lost: %s", tag)
		}
	}
	sub.Resolution = "1080p"
	if matchesSubscriptionRules(sub, title) {
		t.Fatal("explicit resolution must still apply")
	}
}

func TestPTSubscriptionSearchHonorsCategoryAndAdultScope(t *testing.T) {
	var payload map[string]any
	tracker := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_ = json.NewDecoder(r.Body).Decode(&payload)
		_, _ = w.Write([]byte(`{"code":"0","data":{"data":[
			{"id":"1","name":"Show S01E01","category":"123"},
			{"id":"2","name":"Show S01E02","category":"999"},
			{"id":"3","name":"Show XXX S01E03","category":"123"}
		]}}`))
	}))
	t.Cleanup(tracker.Close)
	db := newServiceTestDB(t, &model.Site{}, &model.Setting{})
	repos := repository.New(db)
	sites := NewSiteService(zap.NewNop(), repos, "")
	site := &model.Site{Name: "scoped", Type: "mteam", URL: tracker.URL, APIKey: "test", AuthType: "api_key", Enabled: true}
	if err := sites.Create(t.Context(), site); err != nil {
		t.Fatal(err)
	}
	svc := NewSubscriptionService(nil, zap.NewNop(), repos, nil, sites, nil)
	for _, adult := range []bool{false, true} {
		sub := &model.Subscription{FeedURL: SiteSearchURL("Show", site.ID, "123", adult)}
		items, err := svc.searchSubscriptionSiteScope(t.Context(), sub, "Show")
		if err != nil {
			t.Fatal(err)
		}
		want := 1
		mode := "normal"
		if adult {
			want, mode = 2, "adult"
		}
		if len(items) != want {
			t.Fatalf("include_adult=%v: got %#v, want %d scoped results", adult, items, want)
		}
		if payload["mode"] != mode {
			t.Errorf("request mode = %#v, want %s", payload["mode"], mode)
		}
		categories, ok := payload["categories"].([]any)
		if !ok || len(categories) != 1 || categories[0] != float64(123) {
			t.Errorf("request categories = %#v, want [123]", payload["categories"])
		}
		for _, item := range items {
			if item.ID == "2" || (!adult && item.Adult) {
				t.Errorf("out-of-scope result: %#v", item)
			}
		}
	}
}

func TestPTSubscriptionCandidatesRespectSelectedSeason(t *testing.T) {
	sub := &model.Subscription{FeedURL: SiteSearchURL("Show", "", "", false), Filter: "Show", MediaType: "tv", SeasonNumber: 2}
	items := []SearchResult{
		{Title: "Show S01E01 1080p", DownloadURL: "https://tracker/season1"},
		{Title: "Show S02E01 1080p", DownloadURL: "https://tracker/season2"},
	}
	selected := selectSiteSearchCandidates(items, sub, nil)
	if len(selected) != 1 || selected[0].Season != 2 {
		t.Fatalf("selected candidates = %#v, want only season 2", selected)
	}
}

func TestPTSubscriptionSelectedSeasonDoesNotUseOtherSeasonAvailability(t *testing.T) {
	sub := &model.Subscription{FeedURL: SiteSearchURL("Show", "", "", false), Filter: "Show", MediaType: "tv", SeasonNumber: 2, TotalEpisodes: 2}
	svc := &SubscriptionService{}
	availability := svc.finalizePendingAvailability(sub, LocalAvailability{
		LocalMediaCount: 3, InLibrary: true, TotalEpisodes: 2,
		ExistingEpisodeKeys: map[string]struct{}{episodeKey(1, 1): {}, episodeKey(1, 2): {}, episodeKey(2, 1): {}},
	})
	if availability.DownloadedEpisodes != 1 || len(availability.MissingEpisodes) != 1 || availability.MissingEpisodes[0] != 2 {
		t.Fatalf("season 2 availability = %#v", availability)
	}
	if subscriptionShouldArchive(sub, availability) {
		t.Fatal("other season episodes must not complete this subscription")
	}
	availability.ExistingEpisodeKeys[episodeKey(2, 2)] = struct{}{}
	availability = svc.finalizePendingAvailability(sub, availability)
	if !subscriptionShouldArchive(sub, availability) {
		t.Fatal("the user's declared season total should complete after all selected episodes exist")
	}
}

func TestPTSubscriptionRejectsNegativeSeasonAndTotal(t *testing.T) {
	svc := &SubscriptionService{}
	for _, sub := range []*model.Subscription{
		{DeliveryMode: "download", FeedURL: "site-search://resources?keyword=Show", SeasonNumber: -1},
		{DeliveryMode: "download", FeedURL: "site-search://resources?keyword=Show", TotalEpisodes: -1},
	} {
		if err := svc.ValidateForSave(t.Context(), sub); err == nil {
			t.Fatalf("invalid PT subscription accepted: %#v", sub)
		}
	}
	for _, feedURL := range []string{"https://tracker.example/rss", "site-search://resources?keyword=Show"} {
		if err := svc.ValidateForSave(t.Context(), &model.Subscription{DeliveryMode: "download", FeedURL: feedURL}); err != nil {
			t.Fatalf("legacy zero season/unknown total rejected: %v", err)
		}
	}
}
