package service

import (
	"encoding/json"
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

func TestDeleteRoutesToRequestedTransmissionClient(t *testing.T) {
	var removed map[string]interface{}
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method == http.MethodGet {
			w.Header().Set("X-Transmission-Session-Id", "session-test")
			w.WriteHeader(http.StatusConflict)
			return
		}
		var req transmissionRPCRequest
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			t.Errorf("decode transmission request: %v", err)
			return
		}
		arguments := map[string]interface{}{}
		switch req.Method {
		case "torrent-get":
			arguments["torrents"] = []map[string]interface{}{{
				"hashString":  "transmission-hash",
				"name":        "Delete Transmission Movie",
				"percentDone": 0.5,
				"status":      4,
			}}
		case "torrent-remove":
			removed = req.Arguments
		default:
			t.Errorf("unexpected transmission method %q", req.Method)
		}
		_ = json.NewEncoder(w).Encode(transmissionRPCResponse{Result: "success", Arguments: arguments})
	}))
	defer server.Close()

	db := newServiceTestDB(t, &model.DownloadTask{}, &model.DownloadClient{}, &model.Setting{})
	repos := repository.New(db)
	client := &model.DownloadClient{Name: "Transmission", Type: "transmission", Host: server.URL, IsDefault: true, Enabled: true}
	if err := repos.DownloadClient.Create(t.Context(), client); err != nil {
		t.Fatal(err)
	}
	task := &model.DownloadTask{
		UserID:           "u1",
		Source:           "transmission",
		DownloadClientID: client.ID,
		ExternalID:       "transmission-hash",
		URL:              "magnet:?xt=urn:btih:aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa",
		Title:            "Delete Transmission Movie",
		Status:           "downloading",
		Progress:         0.5,
	}
	if err := repos.Download.Create(t.Context(), task); err != nil {
		t.Fatal(err)
	}
	manager := NewDownloadManager(zap.NewNop(), repos, nil)
	svc := NewDownloadService(zap.NewNop(), repos, NewHub(zap.NewNop()), nil)
	svc.SetDownloadManager(manager)
	if err := svc.ReloadConfig(t.Context()); err != nil {
		t.Fatal(err)
	}

	if err := svc.Delete(t.Context(), "transmission-hash", true, client.ID); err != nil {
		t.Fatal(err)
	}
	ids, ok := removed["ids"].([]interface{})
	if !ok || len(ids) != 1 || ids[0] != "transmission-hash" || removed["delete-local-data"] != true {
		t.Fatalf("remove arguments = %#v", removed)
	}
	var updated model.DownloadTask
	if err := db.Where("id = ?", task.ID).First(&updated).Error; err != nil {
		t.Fatal(err)
	}
	if updated.Status != "deleted" {
		t.Fatalf("status = %q", updated.Status)
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
