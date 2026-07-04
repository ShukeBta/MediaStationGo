package service

import (
	"context"
	"net/http"
	"net/http/httptest"
	"sync/atomic"
	"testing"
	"time"

	"go.uber.org/zap"

	"github.com/ShukeBta/MediaStationGo/internal/model"
	"github.com/ShukeBta/MediaStationGo/internal/repository"
)

func TestSubscriptionEnrichProgressIncludesPendingDownloads(t *testing.T) {
	db := newServiceTestDB(t, &model.DownloadTask{}, &model.Media{})
	repos := repository.New(db)
	if err := repos.Download.Create(t.Context(), &model.DownloadTask{
		Source:   "qbittorrent",
		URL:      "magnet:?xt=urn:btih:4444444444444444444444444444444444444444",
		Title:    "Inception 2010 1080p",
		SavePath: "/downloads/movies",
		Status:   "completed",
		Progress: 1,
	}); err != nil {
		t.Fatal(err)
	}
	svc := NewSubscriptionService(nil, nil, repos, nil, nil, nil)
	items := []model.Subscription{{
		Name:      "Inception 2010",
		Filter:    "Inception 2010",
		MediaType: "movie",
		SavePath:  "/downloads/movies",
	}}

	svc.EnrichProgress(t.Context(), items)
	if items[0].InLibrary {
		t.Fatal("pending download should not be reported as in-library media")
	}
	if items[0].DownloadedEpisodes != 1 || items[0].LocalMediaCount != 1 || items[0].TotalEpisodes != 1 {
		t.Fatalf("unexpected enriched progress: %+v", items[0])
	}
}

func TestSubscriptionEnrichManagementProgressSkipsLiveQB(t *testing.T) {
	var qbCalls int32
	qb := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		atomic.AddInt32(&qbCalls, 1)
		http.Error(w, "management enrichment should not call qb", http.StatusInternalServerError)
	}))
	defer qb.Close()

	db := newServiceTestDB(t, &model.DownloadTask{}, &model.Media{})
	repos := repository.New(db)
	sub := model.Subscription{
		Base:          model.Base{ID: "sub-spy-family"},
		Name:          "间谍过家家 自动订阅",
		Filter:        "间谍过家家",
		MediaType:     "tv",
		SavePath:      "/downloads/tv",
		TotalEpisodes: 2,
	}
	if err := repos.Download.Create(t.Context(), &model.DownloadTask{
		SubscriptionID: sub.ID,
		Source:         "qbittorrent",
		URL:            "magnet:?xt=urn:btih:5555555555555555555555555555555555555555",
		Title:          "间谍过家家 S01E01 1080p",
		SavePath:       "/downloads/tv",
		Status:         "downloading",
		Progress:       0.4,
	}); err != nil {
		t.Fatal(err)
	}
	downloads := NewDownloadService(zap.NewNop(), repos, NewHub(zap.NewNop()), nil)
	downloads.qb.Configure(QBitConfig{BaseURL: qb.URL, Username: "admin", Password: "admin"})
	svc := NewSubscriptionService(nil, nil, repos, downloads, nil, nil)
	items := []model.Subscription{sub}

	svc.EnrichManagementProgress(t.Context(), items)
	if got := atomic.LoadInt32(&qbCalls); got != 0 {
		t.Fatalf("management progress called qb %d times, want 0", got)
	}
	if items[0].DownloadedEpisodes != 1 || items[0].LocalMediaCount != 1 {
		t.Fatalf("unexpected management progress: %+v", items[0])
	}
}

func TestSubscriptionPollIntervalDefaultsAndClampsMinimum(t *testing.T) {
	if subscriptionStartupDelay != defaultSubscriptionPollInterval {
		t.Fatalf("startup delay = %v, want default poll interval %v", subscriptionStartupDelay, defaultSubscriptionPollInterval)
	}

	db := newServiceTestDB(t, &model.Setting{})
	repos := repository.New(db)
	svc := NewSubscriptionService(nil, nil, repos, nil, nil, nil)
	if got := svc.pollInterval(t.Context()); got != defaultSubscriptionPollInterval {
		t.Fatalf("default poll interval = %v, want %v", got, defaultSubscriptionPollInterval)
	}

	if err := repos.Setting.Set(t.Context(), "subscription.interval_seconds", "1800"); err != nil {
		t.Fatal(err)
	}
	if got := svc.pollInterval(t.Context()); got != minSubscriptionPollInterval {
		t.Fatalf("clamped poll interval = %v, want %v", got, minSubscriptionPollInterval)
	}

	if err := repos.Setting.Set(t.Context(), "subscription.interval_seconds", "14400"); err != nil {
		t.Fatal(err)
	}
	if got := svc.pollInterval(t.Context()); got != 4*time.Hour {
		t.Fatalf("configured poll interval = %v, want 4h", got)
	}
}

func TestSubscriptionServiceStartIsSingleLoopAndRestartable(t *testing.T) {
	ctx, cancel := context.WithCancel(t.Context())
	defer cancel()
	svc := NewSubscriptionService(nil, zap.NewNop(), nil, nil, nil, nil)

	svc.Start(ctx)
	firstStop := subscriptionStopChannel(svc)
	if firstStop == nil {
		t.Fatal("first Start did not create a stop channel")
	}
	svc.Start(ctx)
	if got := subscriptionStopChannel(svc); got != firstStop {
		t.Fatal("second Start should reuse the running loop instead of starting another")
	}

	svc.Stop()
	svc.Stop()
	svc.Start(ctx)
	secondStop := subscriptionStopChannel(svc)
	if secondStop == nil {
		t.Fatal("restart did not create a stop channel")
	}
	if secondStop == firstStop {
		t.Fatal("restart should create a fresh loop after Stop")
	}
	svc.Stop()
}

func subscriptionStopChannel(svc *SubscriptionService) chan struct{} {
	svc.mu.Lock()
	defer svc.mu.Unlock()
	return svc.stop
}

func TestMergeLocalAvailabilityKeepsLargerSeriesTotal(t *testing.T) {
	existing := map[string]struct{}{}
	for episode := 1; episode <= 6; episode++ {
		existing[episodeKey(1, episode)] = struct{}{}
	}

	got := mergeLocalAvailability(
		LocalAvailability{TotalEpisodes: 1, LocalMediaCount: 1},
		LocalAvailability{TotalEpisodes: 33, LocalMediaCount: 6, ExistingEpisodeKeys: existing},
	)
	if got.TotalEpisodes != 33 {
		t.Fatalf("TotalEpisodes = %d, want 33", got.TotalEpisodes)
	}
	if got.DownloadedEpisodes != 6 {
		t.Fatalf("DownloadedEpisodes = %d, want 6", got.DownloadedEpisodes)
	}
	if len(got.MissingEpisodes) != 27 {
		t.Fatalf("missing episodes = %d, want 27", len(got.MissingEpisodes))
	}
}

func TestSubscriptionLocalAvailabilityMatchesMediaPath(t *testing.T) {
	db := newServiceTestDB(t, &model.Media{})
	repos := repository.New(db)
	if err := db.Create(&model.Media{
		Title:      "Scraped English Title",
		Path:       "/media/电视剧/国产剧/凡人修仙传/Season 01/凡人修仙传 - S01E146.mkv",
		SeasonNum:  1,
		EpisodeNum: 146,
	}).Error; err != nil {
		t.Fatal(err)
	}
	sub := &model.Subscription{
		Name:          "凡人修仙传 年番",
		Filter:        "凡人修仙传",
		MediaType:     "tv",
		TotalEpisodes: 146,
	}

	availability := SubscriptionLocalAvailability(t.Context(), repos, sub)
	if _, ok := availability.ExistingEpisodeKeys[episodeKey(1, 146)]; !ok {
		t.Fatalf("missing path-matched E146 key: %#v", availability.ExistingEpisodeKeys)
	}
	results := []SearchResult{
		{Title: "凡人修仙传 年番 - 146 1080p", DownloadURL: "https://pt/download/146", Seeders: 80},
	}
	got := selectSiteSearchCandidates(results, sub, map[string]struct{}{}, availability)
	if len(got) != 0 {
		t.Fatalf("selected %#v, want none because path-matched local episode exists", got)
	}
}

func TestSubscriptionPendingDownloadAvailabilityIgnoresDeletedTasks(t *testing.T) {
	db := newServiceTestDB(t, &model.DownloadTask{})
	repos := repository.New(db)
	if err := repos.Download.Create(t.Context(), &model.DownloadTask{
		Source:   "qbittorrent",
		URL:      "magnet:?xt=urn:btih:3333333333333333333333333333333333333333",
		Title:    "间谍过家家 S01E02 1080p",
		SavePath: "/downloads/tv",
		Status:   "deleted",
	}); err != nil {
		t.Fatal(err)
	}
	svc := NewSubscriptionService(nil, nil, repos, nil, nil, nil)
	sub := &model.Subscription{
		Name:          "间谍过家家 自动订阅",
		Filter:        "间谍过家家",
		MediaType:     "tv",
		SavePath:      "/downloads/tv",
		TotalEpisodes: 3,
	}

	availability := svc.pendingDownloadAvailability(t.Context(), sub)
	if _, ok := availability.ExistingEpisodeKeys[episodeKey(1, 2)]; ok {
		t.Fatalf("deleted E02 task should not count as available: %#v", availability.ExistingEpisodeKeys)
	}
	results := []SearchResult{
		{Title: "间谍过家家 S01E02 1080p WEB-DL", DownloadURL: "https://pt/download/2", Seeders: 80},
		{Title: "间谍过家家 S01E03 1080p WEB-DL", DownloadURL: "https://pt/download/3", Seeders: 70},
	}
	got := selectSiteSearchCandidates(results, sub, map[string]struct{}{}, availability)
	if len(got) != 2 || got[0].Episode != 2 || got[1].Episode != 3 {
		t.Fatalf("selected %#v, want deleted episode 2 and new episode 3", got)
	}
}

func TestSubscriptionPendingDownloadAvailabilityIncludesLiveQBTorrents(t *testing.T) {
	qb := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/api/v2/auth/login":
			_, _ = w.Write([]byte("Ok."))
		case "/api/v2/torrents/info":
			_, _ = w.Write([]byte(`[{"hash":"abc123","name":"间谍过家家 S01E01 1080p","state":"downloading","progress":0.2}]`))
		default:
			http.NotFound(w, r)
		}
	}))
	defer qb.Close()

	db := newServiceTestDB(t, &model.DownloadTask{})
	repos := repository.New(db)
	downloads := NewDownloadService(zap.NewNop(), repos, NewHub(zap.NewNop()), nil)
	downloads.qb.Configure(QBitConfig{BaseURL: qb.URL, Username: "admin", Password: "admin"})
	svc := NewSubscriptionService(nil, nil, repos, downloads, nil, nil)
	sub := &model.Subscription{
		Name:          "间谍过家家 自动订阅",
		Filter:        "间谍过家家",
		MediaType:     "tv",
		SavePath:      "/downloads/tv",
		TotalEpisodes: 2,
	}

	availability := svc.pendingDownloadAvailability(t.Context(), sub)
	if availability.DownloadedEpisodes != 1 {
		t.Fatalf("downloaded episodes = %d, want 1", availability.DownloadedEpisodes)
	}
	if _, ok := availability.ExistingEpisodeKeys[episodeKey(1, 1)]; !ok {
		t.Fatalf("missing live qB E01 key: %#v", availability.ExistingEpisodeKeys)
	}
}

func TestSiteSearchDownloadDedupMarksCandidateAvailable(t *testing.T) {
	db := newServiceTestDB(t, &model.DownloadTask{}, &model.Setting{})
	repos := repository.New(db)
	sub := &model.Subscription{
		Base:          model.Base{ID: "sub-nanyang"},
		UserID:        "u1",
		Name:          "南部档案 自动订阅",
		Filter:        "南部档案",
		MediaType:     "tv",
		SavePath:      "/downloads/tv",
		TotalEpisodes: 33,
	}
	if err := repos.Download.Create(t.Context(), &model.DownloadTask{
		SubscriptionID: sub.ID,
		Source:         "qbittorrent",
		URL:            "https://pt/download/existing",
		Title:          "Archives The Nanyang Mystery 2026 S01E07-S01E08 2160p WEB-DL",
		SavePath:       "/downloads/tv",
		Status:         "queued",
		Progress:       0,
	}); err != nil {
		t.Fatal(err)
	}
	downloads := NewDownloadService(zap.NewNop(), repos, NewHub(zap.NewNop()), nil)
	siteSvc := NewSiteService(zap.NewNop(), repos, "")
	svc := NewSubscriptionService(nil, zap.NewNop(), repos, downloads, siteSvc, NewHub(zap.NewNop()))
	state := &siteSearchRunState{
		Keyword: "南部档案",
		SeenSet: map[string]struct{}{},
		Availability: LocalAvailability{
			TotalEpisodes:       33,
			ExistingEpisodeKeys: map[string]struct{}{},
			MissingEpisodeKeys:  map[string]struct{}{},
		},
	}

	title, err := svc.enqueueSiteSearchCandidate(t.Context(), sub, siteSearchCandidate{
		Item: SearchResult{
			Title:       "Archives The Nanyang Mystery 2026 S01E07-S01E08 2160p WEB-DL",
			DownloadURL: "https://pt/download/existing",
		},
		Download: "https://pt/download/existing",
		GUID:     "site|mteam|nanyang-7-8",
		Season:   1,
		Episode:  7,
		Episodes: []int{7, 8},
		Pack:     true,
		Score:    80,
	}, state)
	if err != nil {
		t.Fatalf("enqueueSiteSearchCandidate returned %v, want dedup skipped without error", err)
	}
	if title != "" {
		t.Fatalf("queued title = %q, want empty on dedup", title)
	}
	for _, episode := range []int{7, 8} {
		if _, ok := state.Availability.ExistingEpisodeKeys[episodeKey(1, episode)]; !ok {
			t.Fatalf("deduped candidate should mark E%d available: %#v", episode, state.Availability.ExistingEpisodeKeys)
		}
	}
	if len(state.Seen) != 0 {
		t.Fatalf("deduped candidate should stay retryable instead of being marked seen: %#v", state.Seen)
	}
}
