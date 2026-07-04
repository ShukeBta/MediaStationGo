package service

import (
	"os"
	"path/filepath"
	"testing"
	"time"

	"go.uber.org/zap"

	"github.com/ShukeBta/MediaStationGo/internal/config"
	"github.com/ShukeBta/MediaStationGo/internal/model"
	"github.com/ShukeBta/MediaStationGo/internal/repository"
)

func TestAutoGenerateSTRMAfterScanUsesAllScopeRoot(t *testing.T) {
	db := newServiceTestDB(t, &model.Library{}, &model.Media{}, &model.STRMRecord{}, &model.Setting{})
	repos := repository.New(db)
	outDir := t.TempDir()
	if err := repos.Setting.Set(t.Context(), "strm.auto_generate_enabled", "true"); err != nil {
		t.Fatal(err)
	}
	if err := repos.Setting.Set(t.Context(), "strm.output_dir", outDir); err != nil {
		t.Fatal(err)
	}
	if err := repos.Setting.Set(t.Context(), "strm.output_scope", "all"); err != nil {
		t.Fatal(err)
	}
	lib := model.Library{
		Base:    model.Base{ID: "tv-lib"},
		Name:    "OpenList 欧美剧",
		Path:    BuildCloudLibraryPath("openlist", "/电视剧/欧美剧", "/电视剧/欧美剧"),
		Type:    "tv",
		Enabled: true,
	}
	if err := repos.Library.Create(t.Context(), &lib); err != nil {
		t.Fatal(err)
	}
	media := model.Media{
		Base:       model.Base{ID: "show-1"},
		LibraryID:  lib.ID,
		Title:      "第一集",
		Path:       "cloud://openlist/电视剧/欧美剧/Show/S01E01.mkv",
		STRMURL:    "/api/cloud/play/openlist?ref=show",
		SeasonNum:  1,
		EpisodeNum: 1,
	}
	if err := repos.DB.Create(&media).Error; err != nil {
		t.Fatal(err)
	}

	scanner := NewScannerService(&config.Config{}, zap.NewNop(), repos, NewHub(zap.NewNop()), nil, nil)
	scanner.maybeGenerateSTRMAfterScan(lib.ID)

	want := filepath.Join(outDir, "电视剧", "欧美剧", "Show", "Season 01", "Show - S01E01.strm")
	waitForFile(t, want)
	assertFileContains(t, want, "/api/stream/show-1")
	if _, err := os.Stat(filepath.Join(outDir, "电视剧", "欧美剧", "电视剧", "欧美剧")); !os.IsNotExist(err) {
		t.Fatalf("auto STRM output was nested twice")
	}
}

func waitForFile(t *testing.T, path string) {
	t.Helper()
	deadline := time.Now().Add(3 * time.Second)
	for {
		if _, err := os.Stat(path); err == nil {
			return
		}
		if time.Now().After(deadline) {
			t.Fatalf("timed out waiting for %s", path)
		}
		time.Sleep(20 * time.Millisecond)
	}
}
