package service

import (
	"path/filepath"
	"testing"

	"github.com/ShukeBta/MediaStationGo/internal/model"
	"github.com/ShukeBta/MediaStationGo/internal/repository"
)

func TestSTRMOutputPresetsIncludesDefaultsAndLocalLibraries(t *testing.T) {
	db := newServiceTestDB(t, &model.Library{}, &model.LibraryRoot{})
	repos := repository.New(db)
	base := t.TempDir()
	movieRoot := filepath.Join(base, "strm", "电影")
	tvRoot := filepath.Join(base, "strm", "电视剧")
	cloudRoot := BuildCloudLibraryPath("openlist", "/电影", "/电影")
	disabledRoot := filepath.Join(base, "strm", "动漫")

	libraries := []*model.Library{
		{Name: "电影 STRM", Path: movieRoot, Type: "movie", Enabled: true},
		{Name: "电视剧 STRM", Path: tvRoot, Type: "tv", Enabled: true},
		{Name: "云盘", Path: cloudRoot, Type: "movie", Enabled: true},
		{Name: "停用", Path: disabledRoot, Type: "tv", Enabled: false},
	}
	for _, lib := range libraries {
		if err := repos.Library.Create(t.Context(), lib); err != nil {
			t.Fatal(err)
		}
	}
	if err := repos.DB.Model(&model.Library{}).Where("id = ?", libraries[3].ID).Update("enabled", false).Error; err != nil {
		t.Fatal(err)
	}

	presets, err := STRMOutputPresets(t.Context(), repos)
	if err != nil {
		t.Fatal(err)
	}
	got := map[string]STRMOutputPreset{}
	for _, preset := range presets {
		got[preset.Path] = preset
	}

	if got[filepath.Clean("data/strm")].Kind != "default" || got[filepath.Clean("data/strm/tree")].Kind != "default" {
		t.Fatalf("defaults missing from presets: %#v", presets)
	}
	if got[movieRoot].Label != "电影 STRM" || got[movieRoot].Kind != "library" {
		t.Fatalf("movie preset = %#v, want local library preset", got[movieRoot])
	}
	if got[tvRoot].Label != "电视剧 STRM" || got[tvRoot].Kind != "library" {
		t.Fatalf("tv preset = %#v, want local library preset", got[tvRoot])
	}
	if _, ok := got[cloudRoot]; ok {
		t.Fatalf("cloud library should not be an output preset: %#v", presets)
	}
	if _, ok := got[disabledRoot]; ok {
		t.Fatalf("disabled library should not be an output preset: %#v", presets)
	}
}

func TestSTRMOutputPresetsDeduplicatesLibraryRoots(t *testing.T) {
	db := newServiceTestDB(t, &model.Library{}, &model.LibraryRoot{})
	repos := repository.New(db)
	root := filepath.Join(t.TempDir(), "strm")
	lib := model.Library{Name: "STRM", Path: root, Type: "movie", Enabled: true}
	if err := repos.Library.CreateWithRoots(t.Context(), &lib, []model.LibraryRoot{
		{Path: root, Enabled: true},
		{Path: root, Enabled: true},
	}); err != nil {
		t.Fatal(err)
	}

	presets, err := STRMOutputPresets(t.Context(), repos)
	if err != nil {
		t.Fatal(err)
	}
	count := 0
	for _, preset := range presets {
		if preset.Path == root {
			count++
		}
	}
	if count != 1 {
		t.Fatalf("root preset count = %d, presets=%#v", count, presets)
	}
}
