package service

import (
	"net/http"
	"net/http/httptest"
	"sync/atomic"
	"testing"

	"go.uber.org/zap"

	"github.com/ShukeBta/MediaStationGo/internal/model"
	"github.com/ShukeBta/MediaStationGo/internal/repository"
)

func TestAddDownloadWithMetaDoesNotDedupRangeAgainstSingleEpisodeTask(t *testing.T) {
	var addCalls int32
	qb := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/api/v2/auth/login":
			_, _ = w.Write([]byte("Ok."))
		case "/api/v2/torrents/info":
			http.Error(w, "temporary list unavailable", http.StatusInternalServerError)
		case "/api/v2/torrents/add":
			atomic.AddInt32(&addCalls, 1)
			_, _ = w.Write([]byte("Ok."))
		default:
			http.NotFound(w, r)
		}
	}))
	defer qb.Close()

	db := newServiceTestDB(t, &model.DownloadTask{}, &model.DownloadClient{}, &model.Setting{})
	repos := repository.New(db)
	configureTestDefaultQB(t, repos, qb.URL)
	if err := repos.Download.Create(t.Context(), &model.DownloadTask{
		UserID:   "u1",
		Source:   "qbittorrent",
		URL:      "https://pt.example/download?id=old",
		Title:    "Archives The Nanyang Mystery 2026 S01E07 2160p WEB-DL",
		SavePath: "/downloads/tv",
		Status:   "completed",
		Progress: 1,
	}); err != nil {
		t.Fatal(err)
	}

	svc := NewDownloadService(zap.NewNop(), repos, NewHub(zap.NewNop()), nil)
	task, err := svc.AddDownloadWithMeta(t.Context(), "u1", "magnet:?xt=urn:btih:cdcdcdcdcdcdcdcdcdcdcdcdcdcdcdcdcdcdcdcd&dn=Archives+The+Nanyang+Mystery+2026+S01E07-S01E08", "/downloads", DownloadTaskMeta{
		Title: "Archives The Nanyang Mystery 2026 S01E07-S01E08 2160p WEB-DL",
	})
	if err != nil {
		t.Fatalf("AddDownloadWithMeta returned %v, want queued because existing task covers only E07", err)
	}
	if task == nil {
		t.Fatal("task = nil, want queued task")
	}
	if got := atomic.LoadInt32(&addCalls); got != 1 {
		t.Fatalf("qb add calls = %d, want 1", got)
	}
}

func TestAddDownloadWithMetaDoesNotDedupRangeAgainstSeasonOnlyTask(t *testing.T) {
	var addCalls int32
	qb := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/api/v2/auth/login":
			_, _ = w.Write([]byte("Ok."))
		case "/api/v2/torrents/info":
			http.Error(w, "temporary list unavailable", http.StatusInternalServerError)
		case "/api/v2/torrents/add":
			atomic.AddInt32(&addCalls, 1)
			_, _ = w.Write([]byte("Ok."))
		default:
			http.NotFound(w, r)
		}
	}))
	defer qb.Close()

	db := newServiceTestDB(t, &model.DownloadTask{}, &model.DownloadClient{}, &model.Setting{})
	repos := repository.New(db)
	configureTestDefaultQB(t, repos, qb.URL)
	if err := repos.Download.Create(t.Context(), &model.DownloadTask{
		UserID:         "u1",
		SubscriptionID: "sub-nanyang",
		Source:         "qbittorrent",
		URL:            "https://pt.example/download?id=old-season",
		Title:          "Archives The Nanyang Mystery 2026 S01 2160p WEB-DL",
		SavePath:       "/downloads/tv",
		Status:         "completed",
		Progress:       1,
	}); err != nil {
		t.Fatal(err)
	}

	svc := NewDownloadService(zap.NewNop(), repos, NewHub(zap.NewNop()), nil)
	task, err := svc.AddDownloadWithMeta(t.Context(), "u1", "magnet:?xt=urn:btih:abababababababababababababababababababab&dn=Archives+The+Nanyang+Mystery+2026+S01E09-E10", "/downloads/tv", DownloadTaskMeta{
		SubscriptionID: "sub-nanyang",
		Title:          "Archives The Nanyang Mystery 2026 S01E09-E10 2160p WEB-DL",
	})
	if err != nil {
		t.Fatalf("AddDownloadWithMeta returned %v, want queued because season-only task does not prove E09-E10 exists", err)
	}
	if task == nil {
		t.Fatal("task = nil, want queued task")
	}
	if got := atomic.LoadInt32(&addCalls); got != 1 {
		t.Fatalf("qb add calls = %d, want 1", got)
	}
}

func TestAddDownloadWithMetaDoesNotDedupSubscriptionRangeAgainstCompletePackTask(t *testing.T) {
	var addCalls int32
	qb := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/api/v2/auth/login":
			_, _ = w.Write([]byte("Ok."))
		case "/api/v2/torrents/info":
			http.Error(w, "temporary list unavailable", http.StatusInternalServerError)
		case "/api/v2/torrents/add":
			atomic.AddInt32(&addCalls, 1)
			_, _ = w.Write([]byte("Ok."))
		default:
			http.NotFound(w, r)
		}
	}))
	defer qb.Close()

	db := newServiceTestDB(t, &model.DownloadTask{}, &model.DownloadClient{}, &model.Setting{})
	repos := repository.New(db)
	configureTestDefaultQB(t, repos, qb.URL)
	if err := repos.Download.Create(t.Context(), &model.DownloadTask{
		UserID:         "u1",
		SubscriptionID: "sub-nanyang",
		Source:         "qbittorrent",
		URL:            "https://pt.example/download?id=old-complete",
		Title:          "Archives The Nanyang Mystery 2026 S01 Complete 2160p WEB-DL",
		SavePath:       "/downloads/tv",
		Status:         "completed",
		Progress:       1,
	}); err != nil {
		t.Fatal(err)
	}

	svc := NewDownloadService(zap.NewNop(), repos, NewHub(zap.NewNop()), nil)
	task, err := svc.AddDownloadWithMeta(t.Context(), "u1", "magnet:?xt=urn:btih:fafafafafafafafafafafafafafafafafafafafa&dn=Archives+The+Nanyang+Mystery+2026+S01E29-E33", "/downloads/tv", DownloadTaskMeta{
		SubscriptionID: "sub-nanyang",
		Title:          "Archives The Nanyang Mystery 2026 S01E29-E33 2160p WEB-DL",
	})
	if err != nil {
		t.Fatalf("AddDownloadWithMeta returned %v, want queued because complete-pack history does not prove missing range exists", err)
	}
	if task == nil {
		t.Fatal("task = nil, want queued task")
	}
	if got := atomic.LoadInt32(&addCalls); got != 1 {
		t.Fatalf("qb add calls = %d, want 1", got)
	}
}

func TestAddDownloadWithMetaTracksExistingQBTorrentForSubscription(t *testing.T) {
	torrentData := []byte("d4:infod4:name7:fixtureee")
	hash := torrentInfoHash(torrentData)
	if hash == "" {
		t.Fatal("expected fixture info hash")
	}
	var addCalls int32
	qb := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/fixture.torrent":
			w.Header().Set("Content-Type", "application/x-bittorrent")
			_, _ = w.Write(torrentData)
		case "/api/v2/auth/login":
			_, _ = w.Write([]byte("Ok."))
		case "/api/v2/torrents/info":
			_, _ = w.Write([]byte(`[{"hash":"` + hash + `","name":"Archives The Nanyang Mystery 2026 S01E07-S01E08 2160p WEB-DL","save_path":"/downloads/tv"}]`))
		case "/api/v2/torrents/add":
			atomic.AddInt32(&addCalls, 1)
			_, _ = w.Write([]byte("Ok."))
		default:
			http.NotFound(w, r)
		}
	}))
	defer qb.Close()

	db := newServiceTestDB(t, &model.DownloadTask{}, &model.DownloadClient{}, &model.Setting{})
	repos := repository.New(db)
	configureTestDefaultQB(t, repos, qb.URL)

	svc := NewDownloadService(zap.NewNop(), repos, NewHub(zap.NewNop()), nil)
	task, err := svc.AddDownloadWithMeta(t.Context(), "u1", qb.URL+"/fixture.torrent", "/downloads/tv", DownloadTaskMeta{
		SubscriptionID: "sub-nanyang",
		Title:          "Archives The Nanyang Mystery 2026 S01E07-S01E08 2160p WEB-DL",
	})
	if err != nil {
		t.Fatalf("AddDownloadWithMeta returned %v, want tracking task for existing qB torrent", err)
	}
	if task == nil || task.SubscriptionID != "sub-nanyang" {
		t.Fatalf("task = %#v, want subscription tracking task", task)
	}
	if got := atomic.LoadInt32(&addCalls); got != 0 {
		t.Fatalf("qb add calls = %d, want 0 because infohash already exists", got)
	}
}

func TestDownloadTitleCoversRequestKeepsCompletePackDedup(t *testing.T) {
	if !downloadTitleCoversRequest("Archives The Nanyang Mystery 2026 S01 Complete 2160p WEB-DL", "Archives The Nanyang Mystery 2026 S01E09-E10 2160p WEB-DL") {
		t.Fatal("complete pack should cover requested episode range")
	}
	if downloadTitleCoversRequest("Archives The Nanyang Mystery 2026 S01 2160p WEB-DL", "Archives The Nanyang Mystery 2026 S01E09-E10 2160p WEB-DL") {
		t.Fatal("season-only title must not cover requested episode range")
	}
}

func TestAddDownloadWithMetaScopesSubscriptionDedupBySubscriptionOrSavePath(t *testing.T) {
	var addCalls int32
	qb := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/api/v2/auth/login":
			_, _ = w.Write([]byte("Ok."))
		case "/api/v2/torrents/info":
			http.Error(w, "temporary list unavailable", http.StatusInternalServerError)
		case "/api/v2/torrents/add":
			atomic.AddInt32(&addCalls, 1)
			_, _ = w.Write([]byte("Ok."))
		default:
			http.NotFound(w, r)
		}
	}))
	defer qb.Close()

	db := newServiceTestDB(t, &model.DownloadTask{}, &model.DownloadClient{}, &model.Setting{})
	repos := repository.New(db)
	configureTestDefaultQB(t, repos, qb.URL)
	for _, existing := range []model.DownloadTask{
		{
			UserID:         "u1",
			SubscriptionID: "other-subscription",
			Source:         "qbittorrent",
			URL:            "https://pt.example/download?id=old-sub",
			Title:          "Archives The Nanyang Mystery 2026 S01E07-S01E08 2160p WEB-DL",
			SavePath:       "/downloads/other",
			Status:         "completed",
			Progress:       1,
		},
		{
			UserID:   "u1",
			Source:   "qbittorrent",
			URL:      "https://pt.example/download?id=old-manual",
			Title:    "Archives The Nanyang Mystery 2026 S01E09-S01E10 2160p WEB-DL",
			SavePath: "/downloads/archive",
			Status:   "completed",
			Progress: 1,
		},
	} {
		row := existing
		if err := repos.Download.Create(t.Context(), &row); err != nil {
			t.Fatal(err)
		}
	}

	svc := NewDownloadService(zap.NewNop(), repos, NewHub(zap.NewNop()), nil)
	task, err := svc.AddDownloadWithMeta(t.Context(), "u1", "magnet:?xt=urn:btih:efefefefefefefefefefefefefefefefefefefef&dn=Archives+The+Nanyang+Mystery+2026+S01E07-S01E08", "/downloads/tv", DownloadTaskMeta{
		SubscriptionID: "current-subscription",
		Title:          "Archives The Nanyang Mystery 2026 S01E07-S01E08 2160p WEB-DL",
	})
	if err != nil {
		t.Fatalf("AddDownloadWithMeta returned %v, want queued because old task is outside current subscription scope", err)
	}
	if task == nil {
		t.Fatal("task = nil, want queued task")
	}
	if got := atomic.LoadInt32(&addCalls); got != 1 {
		t.Fatalf("qb add calls = %d, want 1", got)
	}
}

func TestAddDownloadWithMetaRequeuesStaleSubscriptionTaskMissingFromQB(t *testing.T) {
	var addCalls int32
	qb := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/api/v2/auth/login":
			_, _ = w.Write([]byte("Ok."))
		case "/api/v2/torrents/info":
			if atomic.LoadInt32(&addCalls) > 0 {
				_, _ = w.Write([]byte(`[{"hash":"newhash","name":"Archives The Nanyang Mystery 2026 S01E07-S01E08 2160p WEB-DL","state":"downloading","progress":0.1}]`))
				return
			}
			_, _ = w.Write([]byte(`[]`))
		case "/api/v2/torrents/add":
			atomic.AddInt32(&addCalls, 1)
			_, _ = w.Write([]byte("Ok."))
		default:
			http.NotFound(w, r)
		}
	}))
	defer qb.Close()

	db := newServiceTestDB(t, &model.DownloadTask{}, &model.DownloadClient{}, &model.Setting{})
	repos := repository.New(db)
	configureTestDefaultQB(t, repos, qb.URL)
	if err := repos.Download.Create(t.Context(), &model.DownloadTask{
		UserID:         "u1",
		SubscriptionID: "sub-nanyang",
		Source:         "qbittorrent",
		URL:            "https://pt.example/download?id=stale",
		Title:          "Archives The Nanyang Mystery 2026 S01E07-S01E08 2160p WEB-DL",
		SavePath:       "/downloads/tv",
		Status:         "queued",
		Progress:       0,
	}); err != nil {
		t.Fatal(err)
	}

	svc := NewDownloadService(zap.NewNop(), repos, NewHub(zap.NewNop()), nil)
	svc.qb.Configure(QBitConfig{BaseURL: qb.URL, Username: "admin", Password: "admin"})
	task, err := svc.AddDownloadWithMeta(t.Context(), "u1", "magnet:?xt=urn:btih:bcbcbcbcbcbcbcbcbcbcbcbcbcbcbcbcbcbcbcbc&dn=Archives+The+Nanyang+Mystery+2026+S01E07-S01E08", "/downloads/tv", DownloadTaskMeta{
		SubscriptionID: "sub-nanyang",
		Title:          "Archives The Nanyang Mystery 2026 S01E07-S01E08 2160p WEB-DL",
	})
	if err != nil {
		t.Fatalf("AddDownloadWithMeta returned %v, want stale task ignored and candidate queued", err)
	}
	if task == nil {
		t.Fatal("task = nil, want requeued task")
	}
	if got := atomic.LoadInt32(&addCalls); got != 1 {
		t.Fatalf("qb add calls = %d, want 1", got)
	}
}
