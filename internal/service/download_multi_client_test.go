package service

import (
	"encoding/base64"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"sort"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"go.uber.org/zap"

	"github.com/ShukeBta/MediaStationGo/internal/model"
	"github.com/ShukeBta/MediaStationGo/internal/repository"
)

func TestAddDownloadUsesDefaultTransmissionClient(t *testing.T) {
	var mu sync.Mutex
	var added map[string]interface{}
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
		switch req.Method {
		case "torrent-get":
			_ = json.NewEncoder(w).Encode(transmissionRPCResponse{Result: "success", Arguments: map[string]interface{}{"torrents": []interface{}{}}})
		case "torrent-add":
			mu.Lock()
			added = req.Arguments
			mu.Unlock()
			_ = json.NewEncoder(w).Encode(transmissionRPCResponse{
				Result: "success",
				Arguments: map[string]interface{}{
					"torrent-added": map[string]interface{}{"hashString": "transmission-hash", "name": "Movie 2026"},
				},
			})
		default:
			t.Errorf("unexpected transmission method %q", req.Method)
		}
	}))
	defer server.Close()

	db := newServiceTestDB(t, &model.DownloadClient{}, &model.DownloadTask{}, &model.Setting{})
	repos := repository.New(db)
	client := &model.DownloadClient{Name: "Transmission", Type: "transmission", Host: server.URL, IsDefault: true, Enabled: true}
	if err := repos.DownloadClient.Create(t.Context(), client); err != nil {
		t.Fatal(err)
	}
	manager := NewDownloadManager(zap.NewNop(), repos, nil)
	svc := NewDownloadService(zap.NewNop(), repos, NewHub(zap.NewNop()), nil)
	svc.SetDownloadManager(manager)

	task, err := svc.AddDownloadWithMeta(t.Context(), "user-1", "magnet:?xt=urn:btih:aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa&dn=Movie+2026", "/downloads/movies", DownloadTaskMeta{Title: "Movie 2026"})
	if err != nil {
		t.Fatal(err)
	}
	if task.Source != "transmission" || task.DownloadClientID != client.ID || task.ExternalID != "transmission-hash" {
		t.Fatalf("task downloader identity = %#v", task)
	}
	mu.Lock()
	defer mu.Unlock()
	if added["filename"] != "magnet:?xt=urn:btih:aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa&dn=Movie+2026" {
		t.Fatalf("transmission filename = %#v", added["filename"])
	}
	if added["download-dir"] != "/downloads/movies" {
		t.Fatalf("transmission download-dir = %#v", added["download-dir"])
	}
}

func TestAddDownloadSendsFetchedTorrentBytesToTransmission(t *testing.T) {
	torrentData := []byte("d4:infod4:name7:fixtureee")
	torrentServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "application/x-bittorrent")
		w.Header().Set("Content-Disposition", `attachment; filename="fixture.torrent"`)
		_, _ = w.Write(torrentData)
	}))
	defer torrentServer.Close()

	var metainfo string
	transmission := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
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
			arguments["torrents"] = []interface{}{}
		case "torrent-add":
			metainfo, _ = req.Arguments["metainfo"].(string)
			arguments["torrent-added"] = map[string]interface{}{"hashString": "torrent-file-hash"}
		}
		_ = json.NewEncoder(w).Encode(transmissionRPCResponse{Result: "success", Arguments: arguments})
	}))
	defer transmission.Close()

	db := newServiceTestDB(t, &model.Site{}, &model.DownloadClient{}, &model.DownloadTask{}, &model.Setting{})
	repos := repository.New(db)
	if err := repos.Site.Create(t.Context(), &model.Site{Name: "Fixture", Type: "custom_rss", URL: torrentServer.URL, AuthType: "cookie", Enabled: true}); err != nil {
		t.Fatal(err)
	}
	client := &model.DownloadClient{Name: "Transmission", Type: "transmission", Host: transmission.URL, IsDefault: true, Enabled: true}
	if err := repos.DownloadClient.Create(t.Context(), client); err != nil {
		t.Fatal(err)
	}
	manager := NewDownloadManager(zap.NewNop(), repos, nil)
	site := NewSiteService(zap.NewNop(), repos, "")
	svc := NewDownloadService(zap.NewNop(), repos, NewHub(zap.NewNop()), nil, site)
	svc.SetDownloadManager(manager)
	task, err := svc.AddDownloadWithMeta(t.Context(), "user-1", torrentServer.URL+"/fixture.torrent", "/downloads", DownloadTaskMeta{})
	if err != nil {
		t.Fatal(err)
	}
	if metainfo != base64.StdEncoding.EncodeToString(torrentData) {
		t.Fatalf("metainfo = %q", metainfo)
	}
	if task.ExternalID != "torrent-file-hash" || task.Title != "fixture" {
		t.Fatalf("task = %#v", task)
	}
}

func TestAddDownloadSendsPublicTorrentURLBytesToAria2(t *testing.T) {
	torrentData := []byte("d4:infod4:name7:fixtureee")
	torrentServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "application/x-bittorrent")
		_, _ = w.Write(torrentData)
	}))
	defer torrentServer.Close()

	var addMethod string
	aria2 := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		var req aria2Request
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			t.Errorf("decode aria2 request: %v", err)
			return
		}
		result := interface{}(map[string]interface{}{"version": "1.37"})
		switch req.Method {
		case "aria2.tellActive", "aria2.tellWaiting", "aria2.tellStopped":
			result = []interface{}{}
		case "aria2.addTorrent", "aria2.addUri":
			addMethod = req.Method
			result = "aria2-torrent-gid"
		}
		_ = json.NewEncoder(w).Encode(map[string]interface{}{"jsonrpc": "2.0", "id": req.ID, "result": result})
	}))
	defer aria2.Close()

	db := newServiceTestDB(t, &model.Site{}, &model.DownloadClient{}, &model.DownloadTask{}, &model.Setting{})
	repos := repository.New(db)
	client := &model.DownloadClient{Name: "aria2", Type: "aria2", Host: aria2.URL, IsDefault: true, Enabled: true}
	if err := repos.DownloadClient.Create(t.Context(), client); err != nil {
		t.Fatal(err)
	}
	site := NewSiteService(zap.NewNop(), repos, "")
	svc := NewDownloadService(zap.NewNop(), repos, NewHub(zap.NewNop()), nil, site)
	svc.SetDownloadManager(NewDownloadManager(zap.NewNop(), repos, nil))
	task, err := svc.AddDownloadWithMeta(t.Context(), "user-1", torrentServer.URL+"/public.torrent", "/downloads", DownloadTaskMeta{Title: "Public Torrent"})
	if err != nil {
		t.Fatal(err)
	}
	if addMethod != "aria2.addTorrent" || task.ExternalID != "aria2-torrent-gid" {
		t.Fatalf("add method = %q task = %#v", addMethod, task)
	}
}

func TestReloadConfigHotSwapsUpdatedTransmissionClient(t *testing.T) {
	newServer := func(addCalls *int32, hash string) *httptest.Server {
		return httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
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
				arguments["torrents"] = []interface{}{}
			case "torrent-add":
				atomic.AddInt32(addCalls, 1)
				arguments["torrent-added"] = map[string]interface{}{"hashString": hash}
			}
			_ = json.NewEncoder(w).Encode(transmissionRPCResponse{Result: "success", Arguments: arguments})
		}))
	}
	var firstCalls, secondCalls int32
	first := newServer(&firstCalls, "first-hash")
	defer first.Close()
	second := newServer(&secondCalls, "second-hash")
	defer second.Close()

	db := newServiceTestDB(t, &model.DownloadClient{}, &model.DownloadTask{}, &model.Setting{})
	repos := repository.New(db)
	client := &model.DownloadClient{Name: "Transmission", Type: "transmission", Host: first.URL, IsDefault: true, Enabled: true}
	if err := repos.DownloadClient.Create(t.Context(), client); err != nil {
		t.Fatal(err)
	}
	manager := NewDownloadManager(zap.NewNop(), repos, nil)
	svc := NewDownloadService(zap.NewNop(), repos, NewHub(zap.NewNop()), nil)
	svc.SetDownloadManager(manager)
	if _, err := svc.AddDownloadWithMeta(t.Context(), "user-1", "magnet:?xt=urn:btih:aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa&dn=First+Movie", "/downloads", DownloadTaskMeta{Title: "First Movie"}); err != nil {
		t.Fatal(err)
	}

	client.Host = second.URL
	if err := repos.DownloadClient.Update(t.Context(), client); err != nil {
		t.Fatal(err)
	}
	task, err := svc.AddDownloadWithMeta(t.Context(), "user-1", "magnet:?xt=urn:btih:bbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbb&dn=Second+Movie", "/downloads", DownloadTaskMeta{Title: "Second Movie"})
	if err != nil {
		t.Fatal(err)
	}
	if atomic.LoadInt32(&firstCalls) != 1 || atomic.LoadInt32(&secondCalls) != 1 || task.ExternalID != "second-hash" {
		t.Fatalf("hot reload calls = %d/%d task = %#v", firstCalls, secondCalls, task)
	}
}

func TestDownloadManagerPersistsOldestEnabledClientAsDefault(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method == http.MethodGet {
			w.Header().Set("X-Transmission-Session-Id", "session-test")
			w.WriteHeader(http.StatusConflict)
			return
		}
		http.NotFound(w, r)
	}))
	defer server.Close()

	db := newServiceTestDB(t, &model.DownloadClient{})
	repos := repository.New(db)
	first := &model.DownloadClient{
		Base:      model.Base{CreatedAt: time.Now().Add(-time.Hour)},
		Name:      "First Transmission",
		Type:      "transmission",
		Host:      server.URL,
		Enabled:   true,
		IsDefault: false,
	}
	second := &model.DownloadClient{
		Base:      model.Base{CreatedAt: time.Now()},
		Name:      "Second Transmission",
		Type:      "transmission",
		Host:      server.URL,
		Enabled:   true,
		IsDefault: false,
	}
	if err := repos.DownloadClient.Create(t.Context(), first); err != nil {
		t.Fatal(err)
	}
	if err := repos.DownloadClient.Create(t.Context(), second); err != nil {
		t.Fatal(err)
	}
	manager := NewDownloadManager(zap.NewNop(), repos, nil)
	if err := manager.LoadAll(t.Context()); err != nil {
		t.Fatal(err)
	}
	selected, _, err := manager.GetDefault(t.Context())
	if err != nil {
		t.Fatal(err)
	}
	if selected.ID != first.ID {
		t.Fatalf("default client = %#v", selected)
	}
	refreshed, err := repos.DownloadClient.FindByID(t.Context(), first.ID)
	if err != nil {
		t.Fatal(err)
	}
	if refreshed == nil || !refreshed.IsDefault {
		t.Fatalf("persisted default = %#v", refreshed)
	}
}

func TestRelocateRejectsNonQBitClient(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method == http.MethodGet {
			w.Header().Set("X-Transmission-Session-Id", "session-test")
			w.WriteHeader(http.StatusConflict)
			return
		}
		t.Errorf("unexpected Transmission request during unsupported relocation")
	}))
	defer server.Close()

	db := newServiceTestDB(t, &model.DownloadClient{}, &model.DownloadTask{}, &model.Setting{})
	repos := repository.New(db)
	client := &model.DownloadClient{Name: "Transmission", Type: "transmission", Host: server.URL, IsDefault: true, Enabled: true}
	if err := repos.DownloadClient.Create(t.Context(), client); err != nil {
		t.Fatal(err)
	}
	manager := NewDownloadManager(zap.NewNop(), repos, nil)
	svc := NewDownloadService(zap.NewNop(), repos, NewHub(zap.NewNop()), nil)
	svc.SetDownloadManager(manager)
	if err := svc.ReloadConfig(t.Context()); err != nil {
		t.Fatal(err)
	}

	err := svc.RelocateTorrent(t.Context(), "transmission-hash", "/new/location", client.ID)
	if !errors.Is(err, ErrDownloadOperationUnsupported) {
		t.Fatalf("err = %v", err)
	}
}

func TestListAggregatesEnabledClientsWithNormalizedProgress(t *testing.T) {
	transmission := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
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
		if req.Method == "torrent-get" {
			arguments["torrents"] = []map[string]interface{}{{
				"hashString":   "transmission-hash",
				"name":         "Transmission Movie",
				"totalSize":    1000,
				"percentDone":  0.5,
				"rateDownload": 100,
				"rateUpload":   10,
				"status":       4,
				"downloadDir":  "/downloads/transmission",
				"addedDate":    100,
				"doneDate":     0,
			}}
		}
		_ = json.NewEncoder(w).Encode(transmissionRPCResponse{Result: "success", Arguments: arguments})
	}))
	defer transmission.Close()

	aria2 := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		var req aria2Request
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			t.Errorf("decode aria2 request: %v", err)
			return
		}
		result := interface{}(map[string]interface{}{"version": "1.37"})
		switch req.Method {
		case "aria2.tellActive":
			result = []map[string]interface{}{{
				"gid":             "aria2-gid",
				"bittorrent":      map[string]interface{}{"info": map[string]interface{}{"name": "Aria Movie"}, "infoHash": "aria-info-hash"},
				"totalLength":     "2000",
				"completedLength": "500",
				"downloadSpeed":   "200",
				"uploadSpeed":     "20",
				"status":          "active",
				"dir":             "/downloads/aria2",
			}}
		case "aria2.tellWaiting", "aria2.tellStopped":
			result = []interface{}{}
		}
		_ = json.NewEncoder(w).Encode(map[string]interface{}{
			"jsonrpc": "2.0",
			"id":      req.ID,
			"result":  result,
		})
	}))
	defer aria2.Close()

	db := newServiceTestDB(t, &model.DownloadClient{}, &model.DownloadTask{}, &model.Setting{})
	repos := repository.New(db)
	transmissionClient := &model.DownloadClient{Name: "Transmission", Type: "transmission", Host: transmission.URL, IsDefault: true, Enabled: true}
	aria2Client := &model.DownloadClient{Name: "aria2", Type: "aria2", Host: aria2.URL, Enabled: true}
	if err := repos.DownloadClient.Create(t.Context(), transmissionClient); err != nil {
		t.Fatal(err)
	}
	if err := repos.DownloadClient.Create(t.Context(), aria2Client); err != nil {
		t.Fatal(err)
	}
	manager := NewDownloadManager(zap.NewNop(), repos, nil)
	svc := NewDownloadService(zap.NewNop(), repos, NewHub(zap.NewNop()), nil)
	svc.SetDownloadManager(manager)
	if err := svc.ReloadConfig(t.Context()); err != nil {
		t.Fatal(err)
	}

	_, live, err := svc.List(t.Context())
	if err != nil {
		t.Fatal(err)
	}
	if len(live) != 2 {
		t.Fatalf("live torrents = %#v", live)
	}
	sort.Slice(live, func(i, j int) bool { return live[i].Source < live[j].Source })
	if live[0].Source != "aria2" || live[0].ClientID != aria2Client.ID || live[0].Progress != 0.25 || live[0].ContentPath != "/downloads/aria2/Aria Movie" {
		t.Fatalf("aria2 live torrent = %#v", live[0])
	}
	if live[1].Source != "transmission" || live[1].ClientID != transmissionClient.ID || live[1].Progress != 0.5 || live[1].ContentPath != "/downloads/transmission/Transmission Movie" {
		t.Fatalf("transmission live torrent = %#v", live[1])
	}
}
