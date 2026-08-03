package service

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/glebarez/sqlite"
	"go.uber.org/zap"
	"gorm.io/gorm"

	"github.com/ShukeBta/MediaStationGo/internal/config"
	"github.com/ShukeBta/MediaStationGo/internal/model"
	"github.com/ShukeBta/MediaStationGo/internal/repository"
)

func TestOrganizeDirectoryUsesTMDbChineseAlternativeTitle(t *testing.T) {
	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		switch {
		case strings.HasPrefix(r.URL.Path, "/search/tv"):
			_ = json.NewEncoder(w).Encode(map[string]any{
				"results": []map[string]any{{
					"id":                7583,
					"name":              "The Rookie",
					"original_name":     "The Rookie",
					"original_language": "en",
					"first_air_date":    "2007-03-26",
				}},
			})
		case r.URL.Path == "/tv/7583":
			_ = json.NewEncoder(w).Encode(map[string]any{
				"id":                7583,
				"name":              "The Rookie",
				"original_name":     "The Rookie",
				"original_language": "en",
				"first_air_date":    "2007-03-26",
				"origin_country":    []string{"US"},
				"alternative_titles": map[string]any{
					"results": []map[string]any{{
						"iso_3166_1": "CN",
						"title":      "菜鸟老警",
					}},
				},
			})
		default:
			http.NotFound(w, r)
		}
	}))
	defer upstream.Close()

	db, err := gorm.Open(sqlite.Open(":memory:"), &gorm.Config{})
	if err != nil {
		t.Fatal(err)
	}
	if err := db.AutoMigrate(&model.Library{}, &model.Series{}, &model.Media{}); err != nil {
		t.Fatal(err)
	}
	repos := repository.New(db)
	cfg := &config.Config{}
	cfg.Secrets.TMDbAPIKey = "test-key"
	cfg.Secrets.TMDbAPIProxy = upstream.URL
	log := zap.NewNop()
	tmdb := NewTMDbProvider(cfg, log, nil)
	scraper := NewScraperService(cfg, log, repos, tmdb, nil, nil, nil, NewHub(log))

	root := t.TempDir()
	src := filepath.Join(root, "downloads")
	dest := filepath.Join(root, "media")
	sourceFile := filepath.Join(src, "The.Rookie.2007.S04E01.1080p.WEB-DL.mkv")
	writeOrgFile(t, sourceFile, "episode")

	organizer := NewOrganizerService(cfg, log, repos)
	organizer.SetScraper(scraper)
	result, err := organizer.OrganizeDirectory(t.Context(), OrganizeOptions{
		SourcePath:   src,
		DestPath:     dest,
		TransferMode: TransferCopy,
		MediaType:    "tv",
	})
	if err != nil {
		t.Fatalf("organize directory: %v", err)
	}

	want := filepath.Join(dest, "电视剧", "菜鸟老警", "Season 04", "菜鸟老警 - S04E01.mkv")
	if _, err := os.Stat(want); err != nil {
		t.Fatalf("organized file should use TMDb Chinese alternative title %q: %v; items=%#v", want, err, result.Items)
	}
	var stored model.Media
	if err := repos.DB.First(&stored, "path = ?", want).Error; err != nil {
		t.Fatal(err)
	}
	if stored.Title != "菜鸟老警" || stored.OriginalName != "The Rookie" || stored.TMDbID != 7583 {
		t.Fatalf("stored title=%q original=%q tmdb=%d, want localized Chinese metadata", stored.Title, stored.OriginalName, stored.TMDbID)
	}
}
