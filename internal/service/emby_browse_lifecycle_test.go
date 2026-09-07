package service

import (
	"encoding/json"
	"reflect"
	"strings"
	"testing"

	"github.com/ShukeBta/MediaStationGo/internal/model"
	"github.com/ShukeBta/MediaStationGo/internal/repository"
	"gorm.io/gorm"
)

func TestEmbySQLPageRejectsConcurrentMembershipChange(t *testing.T) {
	for _, operation := range []string{"delete", "move"} {
		t.Run(operation, func(t *testing.T) {
			e, lib := embyProjectionFixture(t, 1, 3)
			if _, err := e.InitializeBrowseKeys(t.Context()); err != nil {
				t.Fatal(err)
			}
			changed := false
			if err := e.repo.DB.Callback().Query().Before("gorm:query").Register("concurrent_membership", func(db *gorm.DB) {
				if changed || db.Statement.Table != "media" || len(db.Statement.Selects) != 0 {
					return
				}
				changed = true
				mutation := e.repo.DB.Model(&model.Media{}).Where("id = ?", "work-0000-episode-000")
				var err error
				if operation == "move" {
					err = mutation.Update("library_id", "moved-library").Error
				} else {
					err = mutation.Delete(&model.Media{}).Error
				}
				if err != nil {
					db.AddError(err)
				}
			}); err != nil {
				t.Fatal(err)
			}
			if _, err := e.seriesItemsForLibrary(t.Context(), lib.ID, ItemsParams{Limit: 12}); err == nil {
				t.Fatal("concurrent membership change silently returned a partial group")
			}
			if !changed {
				t.Fatal("mutation hook did not run")
			}
		})
	}
}

func TestLocalScanBatchPersistsEmbyBrowseFields(t *testing.T) {
	e, lib := embyProjectionFixture(t, 0, 0)
	scanner := NewScannerService(e.cfg, e.log, e.repo, nil, nil, nil)
	result := &ScanResult{}
	batch := newLocalMediaWriteBatch(scanner, t.Context(), result, 100)
	row := model.Media{LibraryID: lib.ID, Title: "New Show S01E01", Path: "/test/new/show.mkv", SeasonNum: 1, EpisodeNum: 1}
	batch.Add(row.Path, &row)
	batch.Flush()
	if result.Added != 1 {
		t.Fatalf("added=%d", result.Added)
	}
	var stored model.Media
	if err := e.repo.DB.Where("path = ?", row.Path).First(&stored).Error; err != nil {
		t.Fatal(err)
	}
	if stored.EmbyKeyVersion != repository.EmbyKeyVersion || stored.EmbySeriesKey != e.seriesIDForMedia(&stored) || stored.SeriesKeyVersion != 1 {
		t.Fatal("batch inserted unprepared grouping fields")
	}
	if n, err := e.InitializeBrowseKeys(t.Context()); err != nil || n != 0 {
		t.Fatalf("fresh ingest requires repair: n=%d err=%v", n, err)
	}
}

func TestEmbyGenreVariantsPreserveClassifierAndReactToLibraryType(t *testing.T) {
	e, lib := embyProjectionFixture(t, 1, 1)
	if _, err := e.InitializeBrowseKeys(t.Context()); err != nil {
		t.Fatal(err)
	}
	var row model.Media
	if err := e.repo.DB.Where("library_id = ?", lib.ID).First(&row).Error; err != nil {
		t.Fatal(err)
	}
	var variants map[string][]string
	if err := json.Unmarshal([]byte(row.EmbyGenreVariants), &variants); err != nil {
		t.Fatal(err)
	}
	for _, mediaType := range []string{"tv", "drama", "国漫", "adult", "nsfw", "music", "unknown", "韩漫", "成人影片"} {
		want := e.embyGenresForMedia(&row, mediaType)
		if got := variants[embyGenreVariant(mediaType)]; !reflect.DeepEqual(got, want) {
			t.Fatalf("type=%s got=%v want=%v", mediaType, got, want)
		}
		if err := e.repo.DB.Model(&model.Library{}).Where("id = ?", lib.ID).Update("type", mediaType).Error; err != nil {
			t.Fatal(err)
		}
		page, err := e.Genres(t.Context(), ItemsParams{Limit: 500})
		if err != nil {
			t.Fatal(err)
		}
		got := map[string]int{}
		for _, item := range page["Items"].([]map[string]any) {
			got[item["Name"].(string)] = item["RecursiveItemCount"].(int)
		}
		if len(got) != len(want) {
			t.Fatalf("type=%s got=%v want=%v", mediaType, got, want)
		}
		for _, name := range want {
			if got[name] != 1 {
				t.Fatalf("missing %s for %s", name, mediaType)
			}
		}
	}
	oldKey := row.EmbyConfigKey
	e.cfg.Organizer.Categories = map[string]string{"euus_tv": "Western Shows"}
	if _, err := e.InitializeBrowseKeys(t.Context()); err != nil {
		t.Fatal(err)
	}
	if err := e.repo.DB.First(&row, "id = ?", row.ID).Error; err != nil {
		t.Fatal(err)
	}
	if row.EmbyConfigKey == oldKey || !strings.Contains(row.EmbyGenreVariants, "Western Shows") {
		t.Fatal("category configuration change did not invalidate projections")
	}
}

func TestEmbySQLFolderArtworkMatchesGreedyReference(t *testing.T) {
	e, lib := embyProjectionFixture(t, 6, 3)
	if err := e.repo.DB.Model(&model.Media{}).Where("tm_db_id IN ?", []int{1000, 1001}).Updates(map[string]any{"poster_url": "https://img.example/shared.jpg", "backdrop_url": "https://img.example/shared-bd.jpg"}).Error; err != nil {
		t.Fatal(err)
	}
	if err := e.repo.DB.Model(&model.Media{}).Where("tm_db_id = ?", 1002).Update("poster_url", "").Error; err != nil {
		t.Fatal(err)
	}
	canonical := model.Series{LibraryID: lib.ID, Title: "Canonical", TMDbID: 1003, BackdropURL: ""}
	if err := e.repo.DB.Create(&canonical).Error; err != nil {
		t.Fatal(err)
	}
	var rows []model.Media
	if err := e.repo.DB.Where("library_id = ? AND (poster_url <> '' OR backdrop_url <> '')", lib.ID).Order("updated_at DESC, created_at DESC, id DESC").Find(&rows).Error; err != nil {
		t.Fatal(err)
	}
	groups, err := e.seriesGroupsFromMedia(t.Context(), rows)
	if err != nil {
		t.Fatal(err)
	}
	for _, kind := range []string{"Primary", "Backdrop", "Thumb"} {
		seenID, seenURL := map[string]bool{}, map[string]bool{}
		want := []EmbyFolderCoverArtwork{}
		for _, preferred := range folderCoverImageTypePreference(kind) {
			for i := range groups {
				art, ok := folderCoverArtworkForSeries(&groups[i], preferred)
				if !ok || seenID[art.MediaID] || seenURL[art.URL] {
					continue
				}
				want = append(want, art)
				seenID[art.MediaID] = true
				seenURL[art.URL] = true
				if len(want) == 4 {
					break
				}
			}
			if len(want) == 4 {
				break
			}
		}
		got, err := e.FolderCoverArtwork(t.Context(), lib.ID, kind, 4)
		if err != nil {
			t.Fatal(err)
		}
		if !reflect.DeepEqual(got, want) {
			t.Fatalf("kind=%s got=%v want=%v", kind, got, want)
		}
	}
}
