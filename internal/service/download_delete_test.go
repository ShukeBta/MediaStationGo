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

func TestDeleteMarksMatchingDownloadTaskDeleted(t *testing.T) {
	const hash = "abc123"
	const title = "Delete Marker Show S01E01 1080p"
	var deleteCalls int32
	qb := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/api/v2/auth/login":
			_, _ = w.Write([]byte("Ok."))
		case "/api/v2/torrents/info":
			_, _ = w.Write([]byte(`[{"hash":"abc123","name":"Delete Marker Show S01E01 1080p","state":"downloading","progress":0.5}]`))
		case "/api/v2/torrents/delete":
			atomic.AddInt32(&deleteCalls, 1)
			_, _ = w.Write([]byte("Ok."))
		default:
			http.NotFound(w, r)
		}
	}))
	defer qb.Close()

	db := newServiceTestDB(t, &model.DownloadTask{}, &model.DownloadClient{}, &model.Setting{})
	repos := repository.New(db)
	configureTestDefaultQB(t, repos, qb.URL)
	task := &model.DownloadTask{
		UserID:   "u1",
		Source:   "qbittorrent",
		URL:      "https://pt.example/download?id=1",
		Title:    title,
		SavePath: "/downloads/tv",
		Status:   "downloading",
		Progress: 0.5,
	}
	if err := repos.Download.Create(t.Context(), task); err != nil {
		t.Fatal(err)
	}

	svc := NewDownloadService(zap.NewNop(), repos, NewHub(zap.NewNop()), nil)
	if err := svc.ReloadConfig(t.Context()); err != nil {
		t.Fatal(err)
	}
	if err := svc.Delete(t.Context(), hash, false); err != nil {
		t.Fatal(err)
	}
	if got := atomic.LoadInt32(&deleteCalls); got != 1 {
		t.Fatalf("delete calls = %d, want 1", got)
	}

	var updated model.DownloadTask
	if err := db.Where("id = ?", task.ID).First(&updated).Error; err != nil {
		t.Fatal(err)
	}
	if updated.Status != "deleted" {
		t.Fatalf("status = %q, want deleted", updated.Status)
	}
}

func TestDeleteMarksMagnetTaskDeletedWhenLiveTorrentNameMissing(t *testing.T) {
	const hash = "0123456789abcdef0123456789abcdef0123c0de"
	var deleteCalls int32
	qb := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/api/v2/auth/login":
			_, _ = w.Write([]byte("Ok."))
		case "/api/v2/torrents/info":
			_, _ = w.Write([]byte(`[]`))
		case "/api/v2/torrents/delete":
			atomic.AddInt32(&deleteCalls, 1)
			_, _ = w.Write([]byte("Ok."))
		default:
			http.NotFound(w, r)
		}
	}))
	defer qb.Close()

	db := newServiceTestDB(t, &model.DownloadTask{}, &model.DownloadClient{}, &model.Setting{})
	repos := repository.New(db)
	configureTestDefaultQB(t, repos, qb.URL)
	task := &model.DownloadTask{
		UserID:   "u1",
		Source:   "qbittorrent",
		URL:      "magnet:?xt=urn:btih:" + hash + "&dn=Codex.Path.Verify.S01E01.2026",
		Title:    "Codex Path Verify S01E01 2026",
		SavePath: "/downloads/tv",
		Status:   "queued",
	}
	if err := repos.Download.Create(t.Context(), task); err != nil {
		t.Fatal(err)
	}

	svc := NewDownloadService(zap.NewNop(), repos, NewHub(zap.NewNop()), nil)
	if err := svc.ReloadConfig(t.Context()); err != nil {
		t.Fatal(err)
	}
	if err := svc.Delete(t.Context(), hash, false); err != nil {
		t.Fatal(err)
	}
	if got := atomic.LoadInt32(&deleteCalls); got != 1 {
		t.Fatalf("delete calls = %d, want 1", got)
	}

	var updated model.DownloadTask
	if err := db.Where("id = ?", task.ID).First(&updated).Error; err != nil {
		t.Fatal(err)
	}
	if updated.Status != "deleted" {
		t.Fatalf("status = %q, want deleted", updated.Status)
	}
}
