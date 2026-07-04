package service

import (
	"errors"
	"net/http"
	"net/http/httptest"
	"sync/atomic"
	"testing"

	"go.uber.org/zap"

	"github.com/ShukeBta/MediaStationGo/internal/model"
	"github.com/ShukeBta/MediaStationGo/internal/repository"
)

func TestPublicDownloadTitleUsesMagnetDisplayName(t *testing.T) {
	got := publicDownloadTitle("magnet:?xt=urn:btih:abc&dn=%E6%B5%8B%E8%AF%95%E5%BD%B1%E7%89%87")
	if got != "测试影片" {
		t.Fatalf("publicDownloadTitle = %q, want %q", got, "测试影片")
	}
}

func configureTestDefaultQB(t *testing.T, repos *repository.Container, baseURL string) {
	t.Helper()
	if err := repos.DownloadClient.Create(t.Context(), &model.DownloadClient{
		Name:      "qB test",
		Type:      "qbittorrent",
		Host:      baseURL,
		Username:  "admin",
		Password:  "admin",
		IsDefault: true,
		Enabled:   true,
	}); err != nil {
		t.Fatalf("create default qB client: %v", err)
	}
	if err := repos.Setting.Set(t.Context(), settingDownloadClientsManaged, "true"); err != nil {
		t.Fatalf("mark download clients managed: %v", err)
	}
}

func TestAddDownloadWithMetaSkipsExistingTaskBeforeQBAdd(t *testing.T) {
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

	db := newServiceTestDB(t, &model.DownloadTask{}, &model.Setting{})
	repos := repository.New(db)
	existing := &model.DownloadTask{
		UserID:   "u1",
		Source:   "qbittorrent",
		URL:      "https://pt.example/download?id=old&passkey=old",
		Title:    "Some Show S01E01 1080p",
		SavePath: "/downloads/tv",
		Status:   "completed",
		Progress: 1,
	}
	if err := repos.Download.Create(t.Context(), existing); err != nil {
		t.Fatal(err)
	}

	svc := NewDownloadService(zap.NewNop(), repos, NewHub(zap.NewNop()), nil)
	svc.qb.Configure(QBitConfig{BaseURL: qb.URL, Username: "admin", Password: "admin"})
	task, err := svc.AddDownloadWithMeta(t.Context(), "u1", "https://pt.example/download?id=new&passkey=new", "/downloads/tv", DownloadTaskMeta{
		Title: "Some Show S01E01 2160p WEB-DL",
	})
	if !errors.Is(err, ErrDownloadAlreadyExists) {
		t.Fatalf("err = %v, want ErrDownloadAlreadyExists", err)
	}
	if task == nil || task.ID != existing.ID {
		t.Fatalf("task = %#v, want existing task %#v", task, existing)
	}
	if got := atomic.LoadInt32(&addCalls); got != 0 {
		t.Fatalf("qb add calls = %d, want 0", got)
	}
}

func TestAddDownloadWithMetaSkipsUserDeletedTaskBeforeQBAdd(t *testing.T) {
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

	db := newServiceTestDB(t, &model.DownloadTask{}, &model.Media{}, &model.Setting{})
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
	existing := &model.DownloadTask{
		UserID:   "u1",
		Source:   "qbittorrent",
		URL:      "https://pt.example/download?id=old&passkey=old",
		Title:    "User Deleted Show S01E01 1080p",
		SavePath: "/downloads/tv",
		Status:   "deleted",
	}
	if err := repos.Download.Create(t.Context(), existing); err != nil {
		t.Fatal(err)
	}

	svc := NewDownloadService(zap.NewNop(), repos, NewHub(zap.NewNop()), nil)
	task, err := svc.AddDownloadWithMeta(t.Context(), "u1", "https://pt.example/download?id=new&passkey=new", "/downloads/tv", DownloadTaskMeta{
		Title: "User Deleted Show S01E01 1080p WEB-DL",
	})
	if !errors.Is(err, ErrDownloadAlreadyExists) {
		t.Fatalf("err = %v, want ErrDownloadAlreadyExists", err)
	}
	if task == nil || task.ID != existing.ID {
		t.Fatalf("task = %#v, want existing task %#v", task, existing)
	}
	if got := atomic.LoadInt32(&addCalls); got != 0 {
		t.Fatalf("qb add calls = %d, want 0", got)
	}
}
