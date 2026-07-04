package service

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"testing"

	"go.uber.org/zap"

	"github.com/ShukeBta/MediaStationGo/internal/config"
	"github.com/ShukeBta/MediaStationGo/internal/model"
)

func TestOrganizeDirectoryCleansReleaseNoiseBeforeMetadataClassify(t *testing.T) {
	var queries []string
	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		if r.URL.Path != "/search/tv" {
			http.NotFound(w, r)
			return
		}
		query := r.URL.Query().Get("query")
		queries = append(queries, query)
		if query != "motherhood of taihang" {
			_ = json.NewEncoder(w).Encode(map[string]any{"results": []any{}})
			return
		}
		_ = json.NewEncoder(w).Encode(map[string]any{
			"results": []map[string]any{{
				"id":                219630,
				"name":              "太行之脊",
				"original_name":     "Motherhood of Taihang",
				"original_language": "zh",
				"origin_country":    []string{"CN"},
				"genre_ids":         []int{18},
				"first_air_date":    "2020-08-26",
			}},
		})
	}))
	defer upstream.Close()

	repos := newOrganizerTestRepo(t)
	cfg := &config.Config{}
	cfg.Organizer.SmartClassify = true
	cfg.Secrets.TMDbAPIKey = "test-key"
	cfg.Secrets.TMDbAPIProxy = upstream.URL
	scraper := NewScraperService(cfg, zap.NewNop(), repos, NewTMDbProvider(cfg, zap.NewNop(), nil), nil, nil, nil, NewHub(zap.NewNop()))

	root := t.TempDir()
	srcRoot := filepath.Join(root, "downloads")
	dest := filepath.Join(root, "media")
	sourceFile := filepath.Join(srcRoot, "Motherhood Of Taihang Aac2 Mweb", "Motherhood Of Taihang Aac2 Mweb - S01E01-Aac2.Mweb - 第 1 集.mkv")
	writeOrgFile(t, sourceFile, "episode")

	euusLib := model.Library{Name: "欧美剧", Path: filepath.Join(dest, "电视剧", "欧美剧"), Type: "tv", Enabled: true}
	domesticLib := model.Library{Name: "国产剧", Path: filepath.Join(dest, "电视剧", "国产剧"), Type: "tv", Enabled: true}
	if err := repos.Library.Create(t.Context(), &euusLib); err != nil {
		t.Fatal(err)
	}
	if err := repos.Library.Create(t.Context(), &domesticLib); err != nil {
		t.Fatal(err)
	}

	organizer := NewOrganizerService(cfg, zap.NewNop(), repos)
	organizer.SetScraper(scraper)
	res, err := organizer.OrganizeDirectory(t.Context(), OrganizeOptions{
		SourcePath:   srcRoot,
		DestPath:     euusLib.Path,
		TransferMode: TransferCopy,
	})
	if err != nil {
		t.Fatalf("organize directory: %v", err)
	}
	want := filepath.Join(domesticLib.Path, "太行之脊", "Season 01", "太行之脊 - S01E01.mkv")
	if res.Organized != 1 || res.Reclassified != 0 {
		t.Fatalf("result = %+v, want organized=1 only; queries=%v", res, queries)
	}
	if _, err := os.Stat(want); err != nil {
		t.Fatalf("organized media missing at %q: %v; items=%#v queries=%v", want, err, res.Items, queries)
	}
	if len(queries) == 0 || queries[0] != "motherhood of taihang" {
		t.Fatalf("first metadata query = %q, want cleaned title; all queries=%v", firstQuery(queries), queries)
	}
}
