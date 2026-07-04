package service

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"sync/atomic"
	"testing"

	"go.uber.org/zap"

	"github.com/ShukeBta/MediaStationGo/internal/model"
	"github.com/ShukeBta/MediaStationGo/internal/repository"
)

func TestReloadConfigDoesNotFallbackToLegacyAfterClientDeleted(t *testing.T) {
	var addCalls int32
	qb := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/api/v2/auth/login":
			_, _ = w.Write([]byte("Ok."))
		case "/api/v2/torrents/info":
			if atomic.LoadInt32(&addCalls) > 0 {
				_, _ = w.Write([]byte(`[{"hash":"abc123","name":"Movie 2026 1080p","state":"downloading","progress":0.1}]`))
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

	db := newServiceTestDB(t, &model.DownloadClient{}, &model.DownloadTask{}, &model.Setting{})
	repos := repository.New(db)
	if err := repos.Setting.Set(t.Context(), "qbittorrent.url", qb.URL); err != nil {
		t.Fatal(err)
	}
	if err := repos.Setting.Set(t.Context(), "qbittorrent.username", "admin"); err != nil {
		t.Fatal(err)
	}
	if err := repos.Setting.Set(t.Context(), "qbittorrent.password", "admin"); err != nil {
		t.Fatal(err)
	}
	client := &model.DownloadClient{Name: "qB", Type: "qbittorrent", Host: qb.URL, Username: "admin", Password: "admin", IsDefault: true, Enabled: true}
	if err := repos.DownloadClient.Create(t.Context(), client); err != nil {
		t.Fatal(err)
	}
	if err := repos.DownloadClient.Delete(t.Context(), client.ID); err != nil {
		t.Fatal(err)
	}

	svc := NewDownloadService(zap.NewNop(), repos, NewHub(zap.NewNop()), nil)
	if err := svc.ReloadConfig(t.Context()); err != nil {
		t.Fatal(err)
	}
	_, err := svc.AddDownloadWithMeta(t.Context(), "u1", "magnet:?xt=urn:btih:aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa&dn=Movie+2026+1080p", "/downloads", DownloadTaskMeta{
		Title: "Movie 2026 1080p",
	})
	if err == nil {
		t.Fatal("expected add to fail when the configured downloader was deleted")
	}
	if got := atomic.LoadInt32(&addCalls); got != 0 {
		t.Fatalf("qb add calls = %d, want 0", got)
	}
}

func TestReloadConfigDoesNotFallbackToLegacyAfterClientDisabled(t *testing.T) {
	var addCalls int32
	qb := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/api/v2/auth/login":
			_, _ = w.Write([]byte("Ok."))
		case "/api/v2/torrents/info":
			_, _ = w.Write([]byte(`[]`))
		case "/api/v2/torrents/add":
			atomic.AddInt32(&addCalls, 1)
			_, _ = w.Write([]byte("Ok."))
		default:
			http.NotFound(w, r)
		}
	}))
	defer qb.Close()

	db := newServiceTestDB(t, &model.DownloadClient{}, &model.DownloadTask{}, &model.Setting{})
	repos := repository.New(db)
	if err := repos.Setting.Set(t.Context(), "qbittorrent.url", qb.URL); err != nil {
		t.Fatal(err)
	}
	if err := repos.Setting.Set(t.Context(), "qbittorrent.username", "admin"); err != nil {
		t.Fatal(err)
	}
	if err := repos.Setting.Set(t.Context(), "qbittorrent.password", "admin"); err != nil {
		t.Fatal(err)
	}
	client := &model.DownloadClient{Name: "qB", Type: "qbittorrent", Host: qb.URL, Username: "admin", Password: "admin", IsDefault: true, Enabled: true}
	if err := repos.DownloadClient.Create(t.Context(), client); err != nil {
		t.Fatal(err)
	}
	client.Enabled = false
	if err := repos.DownloadClient.Update(t.Context(), client); err != nil {
		t.Fatal(err)
	}

	svc := NewDownloadService(zap.NewNop(), repos, NewHub(zap.NewNop()), nil)
	if err := svc.ReloadConfig(t.Context()); err != nil {
		t.Fatal(err)
	}
	_, err := svc.AddDownloadWithMeta(t.Context(), "u1", "magnet:?xt=urn:btih:bbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbb&dn=Movie+2026+1080p", "/downloads", DownloadTaskMeta{
		Title: "Movie 2026 1080p",
	})
	if err == nil {
		t.Fatal("expected add to fail when the configured downloader was disabled")
	}
	if got := atomic.LoadInt32(&addCalls); got != 0 {
		t.Fatalf("qb add calls = %d, want 0", got)
	}
}

func TestReloadConfigUsesSoleEnabledQBitWhenNoExplicitDefault(t *testing.T) {
	var addCalls int32
	qb := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/api/v2/auth/login":
			_, _ = w.Write([]byte("Ok."))
		case "/api/v2/torrents/info":
			if atomic.LoadInt32(&addCalls) > 0 {
				_, _ = w.Write([]byte(`[{"hash":"sole123","name":"Movie 2026 1080p","state":"downloading","progress":0.1}]`))
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

	db := newServiceTestDB(t, &model.DownloadClient{}, &model.DownloadTask{}, &model.Setting{})
	repos := repository.New(db)
	if err := repos.Setting.Set(t.Context(), settingDownloadClientsManaged, "true"); err != nil {
		t.Fatal(err)
	}
	client := &model.DownloadClient{Name: "qB", Type: "qbittorrent", Host: qb.URL, Username: "admin", Password: "admin", IsDefault: false, Enabled: true}
	if err := repos.DownloadClient.Create(t.Context(), client); err != nil {
		t.Fatal(err)
	}

	svc := NewDownloadService(zap.NewNop(), repos, NewHub(zap.NewNop()), nil)
	task, err := svc.AddDownloadWithMeta(t.Context(), "u1", "magnet:?xt=urn:btih:abababababababababababababababababababab&dn=Movie+2026+1080p", "/downloads", DownloadTaskMeta{
		Title: "Movie 2026 1080p",
	})
	if err != nil {
		t.Fatal(err)
	}
	if task == nil {
		t.Fatal("expected task")
	}
	if got := atomic.LoadInt32(&addCalls); got != 1 {
		t.Fatalf("qb add calls = %d, want 1", got)
	}
}

func TestAddDownloadWithMetaFailsClosedWhenNoDownloaderConfigured(t *testing.T) {
	db := newServiceTestDB(t, &model.DownloadClient{}, &model.DownloadTask{}, &model.Setting{})
	repos := repository.New(db)
	svc := NewDownloadService(zap.NewNop(), repos, NewHub(zap.NewNop()), nil)

	task, err := svc.AddDownloadWithMeta(t.Context(), "u1", "magnet:?xt=urn:btih:cccccccccccccccccccccccccccccccccccccccc&dn=Movie+2026+1080p", "/downloads", DownloadTaskMeta{
		Title: "Movie 2026 1080p",
	})
	if err == nil {
		t.Fatal("expected no downloader configured error")
	}
	if !strings.Contains(err.Error(), "当前没有已启用的下载器") {
		t.Fatalf("err = %v, want enabled downloader guidance", err)
	}
	if task != nil {
		t.Fatalf("task = %#v, want nil", task)
	}
	rows, err := repos.Download.List(t.Context())
	if err != nil {
		t.Fatal(err)
	}
	if len(rows) != 0 {
		t.Fatalf("download rows = %d, want 0", len(rows))
	}
}

func TestAddDownloadSelectsFirstEnabledQBitWhenDefaultMissing(t *testing.T) {
	var firstAddCalls int32
	first := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/api/v2/auth/login":
			_, _ = w.Write([]byte("Ok."))
		case "/api/v2/torrents/info":
			if atomic.LoadInt32(&firstAddCalls) > 0 {
				_, _ = w.Write([]byte(`[{"hash":"abc123","name":"Movie 2026 1080p","state":"downloading","progress":0.1}]`))
				return
			}
			_, _ = w.Write([]byte(`[]`))
		case "/api/v2/torrents/add":
			atomic.AddInt32(&firstAddCalls, 1)
			_, _ = w.Write([]byte("Ok."))
		default:
			http.NotFound(w, r)
		}
	}))
	defer first.Close()
	second := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/api/v2/auth/login":
			_, _ = w.Write([]byte("Ok."))
		case "/api/v2/torrents/info":
			_, _ = w.Write([]byte(`[]`))
		case "/api/v2/torrents/add":
			t.Fatal("second qB should not be selected before first enabled qB")
		default:
			http.NotFound(w, r)
		}
	}))
	defer second.Close()

	db := newServiceTestDB(t, &model.DownloadClient{}, &model.DownloadTask{}, &model.Setting{})
	repos := repository.New(db)
	if err := repos.Setting.Set(t.Context(), settingDownloadClientsManaged, "true"); err != nil {
		t.Fatal(err)
	}
	firstClient := &model.DownloadClient{Name: "qB first", Type: "qbittorrent", Host: first.URL, Username: "admin", Password: "admin", IsDefault: false, Enabled: true}
	secondClient := &model.DownloadClient{Name: "qB second", Type: "qbittorrent", Host: second.URL, Username: "admin", Password: "admin", IsDefault: false, Enabled: true}
	if err := repos.DownloadClient.Create(t.Context(), firstClient); err != nil {
		t.Fatal(err)
	}
	if err := repos.DownloadClient.Create(t.Context(), secondClient); err != nil {
		t.Fatal(err)
	}

	svc := NewDownloadService(zap.NewNop(), repos, NewHub(zap.NewNop()), nil)
	task, err := svc.AddDownloadWithMeta(t.Context(), "u1", "magnet:?xt=urn:btih:eeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeee&dn=Movie+2026+1080p", "/downloads", DownloadTaskMeta{
		Title: "Movie 2026 1080p",
	})
	if err != nil {
		t.Fatal(err)
	}
	if task == nil {
		t.Fatal("expected task")
	}
	if got := atomic.LoadInt32(&firstAddCalls); got != 1 {
		t.Fatalf("first qb add calls = %d, want 1", got)
	}
	refreshed, err := repos.DownloadClient.FindByID(t.Context(), firstClient.ID)
	if err != nil {
		t.Fatal(err)
	}
	if refreshed == nil || !refreshed.IsDefault {
		t.Fatalf("first enabled qB should be persisted as default, got %#v", refreshed)
	}
}

func TestReloadConfigManagedModeDoesNotFallbackToLegacyWithoutRows(t *testing.T) {
	var addCalls int32
	qb := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/api/v2/auth/login":
			_, _ = w.Write([]byte("Ok."))
		case "/api/v2/torrents/info":
			_, _ = w.Write([]byte(`[]`))
		case "/api/v2/torrents/add":
			atomic.AddInt32(&addCalls, 1)
			_, _ = w.Write([]byte("Ok."))
		default:
			http.NotFound(w, r)
		}
	}))
	defer qb.Close()

	db := newServiceTestDB(t, &model.DownloadClient{}, &model.DownloadTask{}, &model.Setting{})
	repos := repository.New(db)
	if err := repos.Setting.Set(t.Context(), "qbittorrent.url", qb.URL); err != nil {
		t.Fatal(err)
	}
	if err := repos.Setting.Set(t.Context(), "qbittorrent.username", "admin"); err != nil {
		t.Fatal(err)
	}
	if err := repos.Setting.Set(t.Context(), "qbittorrent.password", "admin"); err != nil {
		t.Fatal(err)
	}
	if err := repos.Setting.Set(t.Context(), settingDownloadClientsManaged, "true"); err != nil {
		t.Fatal(err)
	}

	svc := NewDownloadService(zap.NewNop(), repos, NewHub(zap.NewNop()), nil)
	_, err := svc.AddDownloadWithMeta(t.Context(), "u1", "magnet:?xt=urn:btih:dddddddddddddddddddddddddddddddddddddddd&dn=Movie+2026+1080p", "/downloads", DownloadTaskMeta{
		Title: "Movie 2026 1080p",
	})
	if err == nil {
		t.Fatal("expected managed mode to reject missing default downloader")
	}
	if got := atomic.LoadInt32(&addCalls); got != 0 {
		t.Fatalf("qb add calls = %d, want 0", got)
	}
}

func TestAddDownloadWithMetaExplainsUnsupportedEnabledDownloader(t *testing.T) {
	db := newServiceTestDB(t, &model.DownloadClient{}, &model.DownloadTask{}, &model.Setting{})
	repos := repository.New(db)
	if err := repos.Setting.Set(t.Context(), settingDownloadClientsManaged, "true"); err != nil {
		t.Fatal(err)
	}
	if err := repos.DownloadClient.Create(t.Context(), &model.DownloadClient{
		Name:    "aria2",
		Type:    "aria2",
		Host:    "http://127.0.0.1:6800",
		Enabled: true,
	}); err != nil {
		t.Fatal(err)
	}

	svc := NewDownloadService(zap.NewNop(), repos, NewHub(zap.NewNop()), nil)
	_, err := svc.AddDownloadWithMeta(t.Context(), "u1", "magnet:?xt=urn:btih:ffffffffffffffffffffffffffffffffffffffff&dn=Movie+2026+1080p", "/downloads", DownloadTaskMeta{
		Title: "Movie 2026 1080p",
	})
	if err == nil {
		t.Fatal("expected unsupported downloader error")
	}
	if !strings.Contains(err.Error(), "订阅投递目前需要 qBittorrent") {
		t.Fatalf("err = %v, want qBittorrent guidance", err)
	}
}
