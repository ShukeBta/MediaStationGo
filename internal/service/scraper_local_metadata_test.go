package service

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"testing"

	"github.com/glebarez/sqlite"
	"go.uber.org/zap"
	"gorm.io/gorm"

	"github.com/ShukeBta/MediaStationGo/internal/config"
	"github.com/ShukeBta/MediaStationGo/internal/model"
	"github.com/ShukeBta/MediaStationGo/internal/repository"
)

func TestEnrichOneRejectsWrongYearMatchFromSeriesFolder(t *testing.T) {
	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		switch r.URL.Path {
		case "/search/tv":
			_ = json.NewEncoder(w).Encode(map[string]any{
				"results": []map[string]any{{
					"id":             999,
					"name":           "Parade of Stars Auto Show",
					"first_air_date": "1952-01-01",
				}},
			})
		default:
			http.NotFound(w, r)
		}
	}))
	defer upstream.Close()

	db, err := gorm.Open(sqlite.Open("file::memory:?cache=shared"), &gorm.Config{})
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
	scraper := NewScraperService(cfg, log, repos, NewTMDbProvider(cfg, log, nil), nil, nil, nil, NewHub(log))

	root := t.TempDir()
	mediaPath := filepath.Join(root, "Auto Show (2026)", "Season 1", "Auto Show - S01E03 - 第 3 集.mkv")
	lib := model.Library{Name: "剧集", Path: root, Type: "tv", Enabled: true}
	if err := repos.DB.Create(&lib).Error; err != nil {
		t.Fatal(err)
	}
	media := model.Media{
		LibraryID:    lib.ID,
		Title:        "auto show",
		Path:         mediaPath,
		SeasonNum:    1,
		EpisodeNum:   3,
		ScrapeStatus: "pending",
	}
	if err := repos.DB.Create(&media).Error; err != nil {
		t.Fatal(err)
	}

	if err := scraper.EnrichOne(t.Context(), &media); err != nil {
		t.Fatal(err)
	}

	var got model.Media
	if err := repos.DB.First(&got, "id = ?", media.ID).Error; err != nil {
		t.Fatal(err)
	}
	if got.ScrapeStatus != "no_match" || got.Title != "auto show" || got.Year != 0 || got.TMDbID != 0 {
		t.Fatalf("wrong-year scrape should be rejected, got status=%q title=%q year=%d tmdb=%d", got.ScrapeStatus, got.Title, got.Year, got.TMDbID)
	}
}

func TestEnrichOnePrefersLocalMetadataWithoutProvider(t *testing.T) {
	db, err := gorm.Open(sqlite.Open("file::memory:?cache=shared"), &gorm.Config{})
	if err != nil {
		t.Fatal(err)
	}
	if err := db.AutoMigrate(&model.Library{}, &model.Series{}, &model.Media{}); err != nil {
		t.Fatal(err)
	}
	repos := repository.New(db)
	log := zap.NewNop()
	scraper := NewScraperService(&config.Config{}, log, repos, nil, nil, nil, nil, NewHub(log))

	root := t.TempDir()
	showDir := filepath.Join(root, "间谍过家家")
	seasonDir := filepath.Join(showDir, "Season 02")
	if err := os.MkdirAll(seasonDir, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(showDir, "tvshow.nfo"), []byte(`<tvshow>
<title>间谍过家家</title>
<year>2022</year>
<tmdbid>120089</tmdbid>
<genre>Animation</genre>
</tvshow>`), 0o644); err != nil {
		t.Fatal(err)
	}
	mediaPath := filepath.Join(seasonDir, "间谍过家家 - S02E12.mkv")
	if err := os.WriteFile(nfoPath(mediaPath), []byte(`<episodedetails>
<title>企鹅公园</title>
<showtitle>间谍过家家</showtitle>
<season>2</season>
<episode>12</episode>
<plot>本地剧情</plot>
</episodedetails>`), 0o644); err != nil {
		t.Fatal(err)
	}

	lib := model.Library{Name: "番剧", Path: root, Type: "tv", Enabled: true}
	if err := repos.DB.Create(&lib).Error; err != nil {
		t.Fatal(err)
	}
	media := model.Media{
		LibraryID:    lib.ID,
		Title:        "bad title",
		Path:         mediaPath,
		ScrapeStatus: "pending",
	}
	if err := repos.DB.Create(&media).Error; err != nil {
		t.Fatal(err)
	}

	if err := scraper.EnrichOne(t.Context(), &media); err != nil {
		t.Fatal(err)
	}

	var got model.Media
	if err := repos.DB.First(&got, "id = ?", media.ID).Error; err != nil {
		t.Fatal(err)
	}
	if got.ScrapeStatus != "matched" || got.Title != "间谍过家家" || got.TMDbID != 120089 {
		t.Fatalf("unexpected local scrape: status=%q title=%q tmdb=%d", got.ScrapeStatus, got.Title, got.TMDbID)
	}
	if got.SeasonNum != 2 || got.EpisodeNum != 12 || got.Overview != "本地剧情" {
		t.Fatalf("unexpected local episode data: s=%d e=%d overview=%q", got.SeasonNum, got.EpisodeNum, got.Overview)
	}
	if got.EpisodeTitle != "企鹅公园" {
		t.Fatalf("episode_title = %q, want local episode title", got.EpisodeTitle)
	}
}
