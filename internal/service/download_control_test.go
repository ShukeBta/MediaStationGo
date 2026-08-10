package service

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"reflect"
	"testing"

	"go.uber.org/zap"

	"github.com/ShukeBta/MediaStationGo/internal/model"
	"github.com/ShukeBta/MediaStationGo/internal/repository"
)

func TestPauseAndResumeRouteThroughTaskDownloadClient(t *testing.T) {
	var methods []string
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
		methods = append(methods, req.Method)
		_ = json.NewEncoder(w).Encode(transmissionRPCResponse{Result: "success", Arguments: map[string]interface{}{}})
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
		Title:            "Controlled Transmission Movie",
		Status:           "downloading",
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

	if err := svc.PauseDownloadTask(t.Context(), task.ID); err != nil {
		t.Fatal(err)
	}
	if err := svc.ResumeDownloadTask(t.Context(), task.ID); err != nil {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(methods, []string{"torrent-stop", "torrent-start"}) {
		t.Fatalf("methods = %#v", methods)
	}
	var updated model.DownloadTask
	if err := db.Where("id = ?", task.ID).First(&updated).Error; err != nil {
		t.Fatal(err)
	}
	if updated.Status != "queued" {
		t.Fatalf("status = %q", updated.Status)
	}
}
