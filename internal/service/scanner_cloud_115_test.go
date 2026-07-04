package service

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"go.uber.org/zap"

	"github.com/ShukeBta/MediaStationGo/internal/config"
	"github.com/ShukeBta/MediaStationGo/internal/model"
	"github.com/ShukeBta/MediaStationGo/internal/repository"
)

func TestScan115CloudLibraryKeepsDisplayHierarchyAndSeasonCounts(t *testing.T) {
	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/files" {
			t.Fatalf("unexpected path %s", r.URL.Path)
		}
		w.Header().Set("Content-Type", "application/json")
		switch r.URL.Query().Get("cid") {
		case "100":
			_, _ = w.Write([]byte(`{"state":true,"data":[
				{"cid":"s1","n":"Season 1","s":0},
				{"cid":"s2","n":"Season 2","s":0}
			]}`))
		case "s1":
			_, _ = w.Write([]byte(`{"state":true,"data":[
				{"fid":"f101","n":"剑来 - S01E01.mkv","s":1001,"pc":"pick101"},
				{"fid":"f125","n":"剑来 - S01E25.mkv","s":1025,"pc":"pick125"}
			]}`))
		case "s2":
			_, _ = w.Write([]byte(`{"state":true,"data":[
				{"fid":"f201","n":"剑来 - S02E01.mkv","s":2001,"pc":"pick201"},
				{"fid":"f204","n":"剑来 - S02E04.mkv","s":2004,"pc":"pick204"}
			]}`))
		default:
			t.Fatalf("unexpected cid %q", r.URL.Query().Get("cid"))
		}
	}))
	defer upstream.Close()

	db := newServiceTestDB(t, &model.Library{}, &model.Media{}, &model.Setting{}, &model.StorageConfig{})
	repos := repository.New(db)
	log := zap.NewNop()
	storage := NewStorageConfigService(log, repos, NewCryptoService("", log))
	if _, err := storage.Save(t.Context(), StorageInput{
		Type: "cloud115",
		Config: map[string]any{
			"cookie": "UID=test",
			"base":   upstream.URL,
		},
	}); err != nil {
		t.Fatal(err)
	}
	lib := model.Library{Name: "115 · 国漫 · 剑来", Path: BuildCloudLibraryPath("cloud115", "100", "动漫/国漫/剑来"), Type: "anime", Enabled: true}
	if err := repos.Library.Create(t.Context(), &lib); err != nil {
		t.Fatal(err)
	}
	scanner := NewScannerService(&config.Config{}, log, repos, NewHub(log), nil, nil)
	scanner.SetStorageConfig(storage)

	res, err := scanner.ScanLibrary(t.Context(), lib.ID)
	if err != nil {
		t.Fatalf("scan cloud115: %v", err)
	}
	if res.Added != 4 {
		t.Fatalf("scan result = %#v, want added=4", res)
	}
	var rows []model.Media
	if err := repos.DB.Order("path").Find(&rows).Error; err != nil {
		t.Fatal(err)
	}
	if len(rows) != 4 {
		t.Fatalf("media rows = %d, want 4", len(rows))
	}
	want := map[string][2]int{
		"cloud://cloud115/动漫/国漫/剑来/Season 1/剑来 - S01E01.mkv": {1, 1},
		"cloud://cloud115/动漫/国漫/剑来/Season 1/剑来 - S01E25.mkv": {1, 25},
		"cloud://cloud115/动漫/国漫/剑来/Season 2/剑来 - S02E01.mkv": {2, 1},
		"cloud://cloud115/动漫/国漫/剑来/Season 2/剑来 - S02E04.mkv": {2, 4},
	}
	for _, row := range rows {
		seasonEpisode, ok := want[row.Path]
		if !ok {
			t.Fatalf("unexpected path %q", row.Path)
		}
		if row.Title != "剑来" {
			t.Fatalf("title = %q, want 剑来", row.Title)
		}
		if row.SeasonNum != seasonEpisode[0] || row.EpisodeNum != seasonEpisode[1] {
			t.Fatalf("%s season/episode = %d/%d, want %d/%d", row.Path, row.SeasonNum, row.EpisodeNum, seasonEpisode[0], seasonEpisode[1])
		}
		if !strings.Contains(row.STRMURL, "ref=pick") {
			t.Fatalf("115 playback should keep pickcode ref, got %q", row.STRMURL)
		}
	}
}
