package service

import (
	"github.com/ShukeBta/MediaStationGo/internal/model"
	"os"
	"path/filepath"
	"testing"
)

func TestScannerAutomaticMediaPartsAndManualIsolation(t *testing.T) {
	s, repo := newScannerTestEnv(t)
	dir := t.TempDir()
	lib := model.Library{Name: "Parts", Path: dir, Type: "movie", Enabled: true}
	if err := repo.Library.Create(t.Context(), &lib); err != nil {
		t.Fatal(err)
	}
	for _, name := range []string{"Movie Part 1.mkv", "Movie Part 2.mkv", "Single Part 1.mkv", "Other Part 1.mkv", "Other Part 2.mkv"} {
		path := filepath.Join(dir, name)
		if err := os.WriteFile(path, []byte(name), 0600); err != nil {
			t.Fatal(err)
		}
		if _, err := s.IngestPath(t.Context(), lib.ID, path); err != nil {
			t.Fatal(err)
		}
	}
	var rows []model.Media
	if err := repo.DB.Where("library_id = ?", lib.ID).Find(&rows).Error; err != nil {
		t.Fatal(err)
	}
	byName := map[string]model.Media{}
	for _, row := range rows {
		byName[filepath.Base(row.Path)] = row
	}
	a, b := byName["Movie Part 1.mkv"], byName["Movie Part 2.mkv"]
	if a.PartGroupKey == "" || a.PartGroupKey != b.PartGroupKey || a.PartIndex != 1 || b.PartIndex != 2 {
		t.Fatalf("invalid grouping: %+v / %+v", a, b)
	}
	if byName["Single Part 1.mkv"].PartGroupKey != "" {
		t.Fatal("single Part title was grouped")
	}
	if err := repo.DB.Model(&model.Media{}).Where("id = ?", a.ID).Updates(map[string]any{"part_group_key": "manual", "part_index": 9, "part_group_title": "Manual"}).Error; err != nil {
		t.Fatal(err)
	}
	if _, err := s.reconcileMediaParts(t.Context(), lib.ID, dir); err != nil {
		t.Fatal(err)
	}
	if err := repo.DB.First(&a, "id = ?", a.ID).Error; err != nil {
		t.Fatal(err)
	}
	if a.PartGroupKey != "manual" || a.PartIndex != 9 {
		t.Fatal("manual group overwritten")
	}
	second := byName["Other Part 2.mkv"]
	if err := os.Remove(second.Path); err != nil {
		t.Fatal(err)
	}
	if _, err := s.RemovePath(t.Context(), second.Path); err != nil {
		t.Fatal(err)
	}
	first := byName["Other Part 1.mkv"]
	if err := repo.DB.First(&first, "id = ?", first.ID).Error; err != nil {
		t.Fatal(err)
	}
	if first.PartGroupKey != "" {
		t.Fatal("stale auto group after part removal")
	}
}

func TestParseMediaPartCandidate(t *testing.T) {
	for _, name := range []string{"Film Part 2.mkv", "Film.cd1.mkv", "Film (disc A).mkv", "Film [dvd-3].mp4"} {
		if _, ok := parseMediaPartCandidate(name); !ok {
			t.Errorf("not recognized %q", name)
		}
	}
	for _, name := range []string{"Film.mkv", "Part 1.mkv", "Film Part 0.mkv", "Film Part 1 Extended.mkv"} {
		if _, ok := parseMediaPartCandidate(name); ok {
			t.Errorf("false positive %q", name)
		}
	}
}
