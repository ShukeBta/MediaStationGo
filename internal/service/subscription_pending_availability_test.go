package service

import (
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"testing"

	"go.uber.org/zap"

	"github.com/ShukeBta/MediaStationGo/internal/model"
	"github.com/ShukeBta/MediaStationGo/internal/repository"
)

func TestSubscriptionPendingDownloadAvailabilitySkipsUnorganizedEpisodes(t *testing.T) {
	root := t.TempDir()
	seasonDir := filepath.Join(root, "间谍过家家", "Season 01")
	if err := os.MkdirAll(seasonDir, 0o755); err != nil {
		t.Fatal(err)
	}
	for _, name := range []string{
		"间谍过家家 - S01E01.mkv",
		"间谍过家家 - S01E02.mkv.!qB",
	} {
		if err := os.WriteFile(filepath.Join(seasonDir, name), []byte("video"), 0o644); err != nil {
			t.Fatal(err)
		}
	}

	sub := &model.Subscription{
		Name:          "间谍过家家 自动订阅",
		Filter:        "间谍过家家",
		MediaType:     "tv",
		SavePath:      root,
		TotalEpisodes: 3,
	}
	svc := NewSubscriptionService(nil, nil, nil, nil, nil, nil)
	availability := svc.pendingDownloadAvailability(t.Context(), sub)
	if availability.DownloadedEpisodes != 2 {
		t.Fatalf("downloaded episodes = %d, want 2", availability.DownloadedEpisodes)
	}
	if _, ok := availability.ExistingEpisodeKeys[episodeKey(1, 1)]; !ok {
		t.Fatalf("missing pending E01 key: %#v", availability.ExistingEpisodeKeys)
	}
	if _, ok := availability.ExistingEpisodeKeys[episodeKey(1, 2)]; !ok {
		t.Fatalf("missing pending E02 key: %#v", availability.ExistingEpisodeKeys)
	}

	results := []SearchResult{
		{Title: "间谍过家家 S01 Complete 1080p", DownloadURL: "https://pt/download/pack", Seeders: 100},
		{Title: "间谍过家家 S01E01 1080p", DownloadURL: "https://pt/download/1", Seeders: 90},
		{Title: "间谍过家家 S01E02 1080p", DownloadURL: "https://pt/download/2", Seeders: 80},
		{Title: "间谍过家家 S01E03 1080p", DownloadURL: "https://pt/download/3", Seeders: 70},
	}
	got := selectSiteSearchCandidates(results, sub, map[string]struct{}{}, availability)
	if len(got) != 1 || got[0].Episode != 3 {
		t.Fatalf("selected %#v, want only not-yet-downloaded episode 3", got)
	}
	if !svc.downloadPathHasCandidate(t.Context(), sub, "间谍过家家 S01E02 1080p", root) {
		t.Fatal("expected existing pending E02 file to be detected")
	}
	if svc.downloadPathHasCandidate(t.Context(), sub, "间谍过家家 S01E03 1080p", root) {
		t.Fatal("did not expect missing E03 to be detected")
	}
}

func TestSubscriptionPendingDownloadAvailabilityIncludesQueuedTasks(t *testing.T) {
	db := newServiceTestDB(t, &model.DownloadTask{})
	repos := repository.New(db)
	if err := repos.Download.Create(t.Context(), &model.DownloadTask{
		Source:   "qbittorrent",
		URL:      "magnet:?xt=urn:btih:2222222222222222222222222222222222222222",
		Title:    "间谍过家家 S01E02 1080p",
		SavePath: "/downloads/tv",
		Status:   "queued",
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
	if availability.DownloadedEpisodes != 1 {
		t.Fatalf("downloaded episodes = %d, want 1", availability.DownloadedEpisodes)
	}
	if availability.InLibrary {
		t.Fatal("queued download should not be reported as already in library")
	}
	if _, ok := availability.ExistingEpisodeKeys[episodeKey(1, 2)]; !ok {
		t.Fatalf("missing queued E02 key: %#v", availability.ExistingEpisodeKeys)
	}

	results := []SearchResult{
		{Title: "间谍过家家 S01E02 1080p WEB-DL", DownloadURL: "https://pt/download/2", Seeders: 80},
		{Title: "间谍过家家 S01E03 1080p WEB-DL", DownloadURL: "https://pt/download/3", Seeders: 70},
	}
	got := selectSiteSearchCandidates(results, sub, map[string]struct{}{}, availability)
	if len(got) != 1 || got[0].Episode != 3 {
		t.Fatalf("selected %#v, want only not-yet-downloaded episode 3", got)
	}
}

func TestSubscriptionPendingDownloadAvailabilityUsesOriginalNameAliasForTasks(t *testing.T) {
	db := newServiceTestDB(t, &model.DownloadTask{})
	repos := repository.New(db)
	if err := repos.Download.Create(t.Context(), &model.DownloadTask{
		Source:   "qbittorrent",
		URL:      "https://pt/download/7-8",
		Title:    "Archives The Nanyang Mystery 2026 S01E07-S01E08 2160p WEB-DL",
		SavePath: "/downloads/tv",
		Status:   "queued",
	}); err != nil {
		t.Fatal(err)
	}
	svc := NewSubscriptionService(nil, nil, repos, nil, nil, nil)
	sub := &model.Subscription{
		Name:          "南部档案 自动订阅",
		Filter:        "南部档案 2026",
		OriginalName:  "Archives The Nanyang Mystery",
		MediaType:     "tv",
		SavePath:      "/downloads/tv",
		TotalEpisodes: 33,
	}

	availability := svc.pendingDownloadAvailability(t.Context(), sub)
	if availability.DownloadedEpisodes != 2 {
		t.Fatalf("downloaded episodes = %d, want 2", availability.DownloadedEpisodes)
	}
	for _, episode := range []int{7, 8} {
		if _, ok := availability.ExistingEpisodeKeys[episodeKey(1, episode)]; !ok {
			t.Fatalf("missing pending E%02d key: %#v", episode, availability.ExistingEpisodeKeys)
		}
	}
	got := selectSiteSearchCandidates([]SearchResult{
		{Title: "Archives The Nanyang Mystery 2026 S01E07-S01E08 2160p WEB-DL", SearchKeyword: "南部档案 2026", DownloadURL: "https://pt/download/7-8", Seeders: 80},
	}, sub, map[string]struct{}{}, availability)
	if len(got) != 0 {
		t.Fatalf("selected %#v, want existing alias range to satisfy E07-E08", got)
	}
}

func TestSubscriptionPendingDownloadAvailabilityUsesOriginalNameAliasForLiveTorrents(t *testing.T) {
	qb := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/api/v2/auth/login":
			_, _ = w.Write([]byte("Ok."))
		case "/api/v2/torrents/info":
			_, _ = w.Write([]byte(`[{"hash":"abc123","name":"Archives The Nanyang Mystery 2026 S01E29-E33 2160p WEB-DL","save_path":"/downloads/tv","state":"downloading","progress":0.3}]`))
		default:
			http.NotFound(w, r)
		}
	}))
	defer qb.Close()

	db := newServiceTestDB(t, &model.DownloadTask{}, &model.DownloadClient{}, &model.Setting{})
	repos := repository.New(db)
	downloads := NewDownloadService(zap.NewNop(), repos, NewHub(zap.NewNop()), nil)
	downloads.qb.Configure(QBitConfig{BaseURL: qb.URL, Username: "admin", Password: "admin"})
	svc := NewSubscriptionService(nil, nil, repos, downloads, nil, nil)
	sub := &model.Subscription{
		Name:          "南部档案 自动订阅",
		Filter:        "南部档案 2026",
		OriginalName:  "Archives The Nanyang Mystery",
		MediaType:     "tv",
		SavePath:      "/downloads/tv",
		TotalEpisodes: 33,
	}

	availability := svc.pendingDownloadAvailability(t.Context(), sub)
	if availability.DownloadedEpisodes != 5 {
		t.Fatalf("downloaded episodes = %d, want 5", availability.DownloadedEpisodes)
	}
	for _, episode := range []int{29, 30, 31, 32, 33} {
		if _, ok := availability.ExistingEpisodeKeys[episodeKey(1, episode)]; !ok {
			t.Fatalf("missing live E%02d key: %#v", episode, availability.ExistingEpisodeKeys)
		}
	}
	got := selectSiteSearchCandidates([]SearchResult{
		{Title: "Archives The Nanyang Mystery 2026 S01E29-E33 2160p WEB-DL", SearchKeyword: "南部档案 2026", DownloadURL: "https://pt/download/29-33", Seeders: 80},
	}, sub, map[string]struct{}{}, availability)
	if len(got) != 0 {
		t.Fatalf("selected %#v, want existing live alias range to satisfy E29-E33", got)
	}
}

func TestSubscriptionPendingDownloadAvailabilityIncludesLinkedAliasTask(t *testing.T) {
	db := newServiceTestDB(t, &model.DownloadTask{})
	repos := repository.New(db)
	sub := &model.Subscription{
		Base:          model.Base{ID: "sub-qiao-chu"},
		Name:          "翘楚 S01E06 自动订阅",
		Filter:        "翘楚 S01E06",
		MediaType:     "tv",
		SavePath:      "/downloads/tv",
		TotalEpisodes: 24,
	}
	if err := repos.Download.Create(t.Context(), &model.DownloadTask{
		SubscriptionID: sub.ID,
		Source:         "qbittorrent",
		URL:            "https://pt/download/21",
		Title:          "Ashes to Crown 2026 S01E21 2160p WEB-DL",
		SavePath:       "/downloads/tv",
		Status:         "queued",
	}); err != nil {
		t.Fatal(err)
	}
	svc := NewSubscriptionService(nil, nil, repos, nil, nil, nil)

	availability := svc.pendingDownloadAvailability(t.Context(), sub)
	if _, ok := availability.ExistingEpisodeKeys[episodeKey(1, 21)]; !ok {
		t.Fatalf("missing linked alias E21 key: %#v", availability.ExistingEpisodeKeys)
	}
	got := selectSiteSearchCandidates([]SearchResult{
		{Title: "Ashes to Crown 2026 S01E21 2160p WEB-DL", DownloadURL: "https://pt/download/21", Seeders: 80},
	}, sub, map[string]struct{}{}, availability)
	if len(got) != 0 {
		t.Fatalf("selected %#v, want linked alias task to satisfy E21", got)
	}
}

func TestSubscriptionPendingDownloadAvailabilitySkipsStaleTaskMissingFromQB(t *testing.T) {
	db := newServiceTestDB(t, &model.DownloadTask{})
	repos := repository.New(db)
	sub := &model.Subscription{
		Base:          model.Base{ID: "sub-nanyang"},
		Name:          "南部档案 自动订阅",
		Filter:        "南部档案",
		MediaType:     "tv",
		SavePath:      "/downloads/tv",
		TotalEpisodes: 33,
	}
	if err := repos.Download.Create(t.Context(), &model.DownloadTask{
		SubscriptionID: sub.ID,
		Source:         "qbittorrent",
		URL:            "https://pt/download/stale",
		Title:          "Archives The Nanyang Mystery 2026 S01E07-S01E08 2160p WEB-DL",
		SavePath:       "/downloads/tv",
		Status:         "queued",
		Progress:       0,
	}); err != nil {
		t.Fatal(err)
	}
	downloads := NewDownloadService(zap.NewNop(), repos, NewHub(zap.NewNop()), nil)
	downloads.recordLiveTorrentSnapshot(nil)
	svc := NewSubscriptionService(nil, nil, repos, downloads, nil, nil)

	availability := svc.pendingDownloadAvailability(t.Context(), sub)
	if availability.DownloadedEpisodes != 0 {
		t.Fatalf("downloaded episodes = %d, want stale task not counted", availability.DownloadedEpisodes)
	}
	if _, ok := availability.ExistingEpisodeKeys[episodeKey(1, 7)]; ok {
		t.Fatalf("stale E07 task should not count as available: %#v", availability.ExistingEpisodeKeys)
	}
	got := selectSiteSearchCandidates([]SearchResult{
		{Title: "Archives The Nanyang Mystery 2026 S01E07-S01E08 2160p WEB-DL", SearchKeyword: "南部档案 2026", DownloadURL: "https://pt/download/7-8", Seeders: 80},
	}, sub, map[string]struct{}{}, availability)
	if len(got) != 1 || got[0].Episode != 7 {
		t.Fatalf("selected %#v, want stale missing range to be eligible", got)
	}
}
