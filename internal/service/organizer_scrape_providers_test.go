package service

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"go.uber.org/zap"

	"github.com/ShukeBta/MediaStationGo/internal/config"
	"github.com/ShukeBta/MediaStationGo/internal/model"
)

func TestOrganizeDirectoryUsesAdultMetadataBeforeRename(t *testing.T) {
	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/search":
			w.Header().Set("Content-Type", "text/html; charset=utf-8")
			_, _ = w.Write([]byte(`<a class="box" href="/v/ssis001"><strong>SSIS-001 整理候选</strong></a>`))
		case "/v/ssis001":
			w.Header().Set("Content-Type", "text/html; charset=utf-8")
			_, _ = w.Write([]byte(`<h2 class="title"><strong>SSIS-001 整理成人标题</strong></h2><div>日期 2024-01-02</div>`))
		default:
			http.NotFound(w, r)
		}
	}))
	defer upstream.Close()

	repos := newOrganizerTestRepo(t)
	if err := repos.DB.AutoMigrate(&model.APIConfig{}); err != nil {
		t.Fatal(err)
	}
	apiConfig := NewAPIConfigService(zap.NewNop(), repos, NewCryptoService("", zap.NewNop()))
	baseURL := upstream.URL
	if _, err := apiConfig.Update(t.Context(), "adult", APIConfigPatch{BaseURL: &baseURL}); err != nil {
		t.Fatal(err)
	}
	log := zap.NewNop()
	scraper := NewScraperService(&config.Config{}, log, repos, nil, nil, nil, nil, NewHub(log), NewAdultProvider(log, apiConfig))

	root := t.TempDir()
	src := filepath.Join(root, "downloads")
	dest := filepath.Join(root, "media")
	sourceFile := filepath.Join(src, "SSIS-001.1080p.mkv")
	writeOrgFile(t, sourceFile, "adult")

	organizer := NewOrganizerService(&config.Config{}, log, repos)
	organizer.SetScraper(scraper)
	res, err := organizer.OrganizeDirectory(t.Context(), OrganizeOptions{
		SourcePath:   src,
		DestPath:     dest,
		TransferMode: TransferCopy,
		MediaType:    "adult",
		DryRun:       true,
	})
	if err != nil {
		t.Fatalf("organize adult directory: %v", err)
	}
	if res.Organized != 1 || len(res.Items) != 1 {
		t.Fatalf("result = %+v, want one organized preview", res)
	}
	if res.Items[0].Title != "整理成人标题" || res.Items[0].MediaType != "adult" {
		t.Fatalf("adult organize item = %+v", res.Items[0])
	}
	wantSuffix := filepath.Join("成人", "整理成人标题 (2024)", "整理成人标题 (2024).mkv")
	if !strings.Contains(res.Items[0].Target, wantSuffix) {
		t.Fatalf("adult target = %q, want suffix %q", res.Items[0].Target, wantSuffix)
	}
}

func TestOrganizeDirectoryUsesBangumiForAnimeRename(t *testing.T) {
	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/search/subject/frieren" {
			http.NotFound(w, r)
			return
		}
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(map[string]any{
			"results": 1,
			"list": []map[string]any{{
				"id":       889,
				"name":     "Frieren",
				"name_cn":  "葬送的芙莉莲",
				"air_date": "2023-09-29",
			}},
		})
	}))
	defer upstream.Close()

	repos := newOrganizerTestRepo(t)
	cfg := &config.Config{}
	bangumi := NewBangumiProvider(cfg, zap.NewNop())
	bangumi.base = upstream.URL
	scraper := NewScraperService(cfg, zap.NewNop(), repos, nil, bangumi, nil, nil, NewHub(zap.NewNop()))

	root := t.TempDir()
	src := filepath.Join(root, "downloads")
	dest := filepath.Join(root, "media")
	sourceFile := filepath.Join(src, "Frieren.S01E01.1080p.mkv")
	writeOrgFile(t, sourceFile, "episode")

	organizer := NewOrganizerService(cfg, zap.NewNop(), repos)
	organizer.SetScraper(scraper)
	res, err := organizer.OrganizeDirectory(t.Context(), OrganizeOptions{
		SourcePath:   src,
		DestPath:     dest,
		TransferMode: TransferCopy,
		MediaType:    "anime",
	})
	if err != nil {
		t.Fatalf("organize directory: %v", err)
	}
	want := filepath.Join(dest, "动漫", "葬送的芙莉莲", "Season 01", "葬送的芙莉莲 - S01E01.mkv")
	if res.Organized != 1 {
		t.Fatalf("organized = %d, want 1; items=%#v errors=%#v", res.Organized, res.Items, res.Errors)
	}
	if _, err := os.Stat(want); err != nil {
		t.Fatalf("organized file should use Bangumi metadata path %q: %v", want, err)
	}
}
