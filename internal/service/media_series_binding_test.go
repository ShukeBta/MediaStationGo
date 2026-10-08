package service

import (
	"fmt"
	"testing"

	"github.com/ShukeBta/MediaStationGo/internal/config"
	"github.com/ShukeBta/MediaStationGo/internal/database"
	"github.com/ShukeBta/MediaStationGo/internal/model"
	"github.com/ShukeBta/MediaStationGo/internal/repository"
	"go.uber.org/zap"
)

func TestSeriesBindingRepairsLegacyMixedScrapeStates(t *testing.T) {
	db := newServiceTestDB(t, &model.Media{}, &model.Library{})
	repos := repository.New(db)
	svc := NewMediaService(&config.Config{}, zap.NewNop(), repos)
	lib := model.Library{Base: model.Base{ID: "tv"}, Name: "TV", Path: "/media/tv", Type: "tv", Enabled: true}
	if err := db.Create(&lib).Error; err != nil {
		t.Fatal(err)
	}
	for i := 0; i < 3; i++ {
		row := model.Media{Base: model.Base{ID: fmt.Sprintf("episode-%d", i)}, LibraryID: lib.ID, Title: fmt.Sprintf("单集标题 %d", i),
			Path: fmt.Sprintf("/media/tv/完整作品/Season 1/Show.S01E%02d.mkv", i+1), SeasonNum: 1, EpisodeNum: i + 1,
			SeriesKey: fmt.Sprintf("legacy-%d", i), SeriesKeyVersion: 2}
		if i == 0 {
			row.ScrapeStatus, row.TMDbID = "matched", 321
		}
		if err := db.Create(&row).Error; err != nil {
			t.Fatal(err)
		}
	}
	if n, err := repos.Media.BackfillSeriesKeys(t.Context(), 1); err != nil || n != 1 {
		t.Fatalf("legacy repair: n=%d err=%v", n, err)
	}
	cards, total, err := svc.ListLibrarySeriesCards(t.Context(), lib.ID, 1, 10, MediaVisibility{})
	if err != nil || total != 1 || len(cards) != 1 || cards[0].Count != 3 {
		t.Fatalf("legacy mixed work remains split: total=%d cards=%+v err=%v", total, cards, err)
	}
	var pending []model.Media
	if err := db.Where("scrape_status = 'pending'").Find(&pending).Error; err != nil {
		t.Fatal(err)
	}
	if len(pending) != 2 {
		t.Fatalf("pending episode states changed: %+v", pending)
	}
	for _, row := range pending {
		if row.TMDbID != 0 || row.SeriesBindingKey != "tmdb:321" || row.SeriesBindingScope == "" {
			t.Fatalf("invalid inherited binding: %+v", row)
		}
	}
}

func TestSeriesBindingRejectsUnsafeDirectoryBridges(t *testing.T) {
	for _, scenario := range []string{"generic-category", "custom-library-root", "second-library-root", "different-directory", "other-library", "conflicting-pending-id", "conflicting-pending-secondary-id", "different-confirmed-id", "conflicting-secondary-id"} {
		t.Run(scenario, func(t *testing.T) {
			db := newServiceTestDB(t, &model.Media{}, &model.Library{}, &model.LibraryRoot{})
			repos := repository.New(db)
			NewMediaService(&config.Config{}, zap.NewNop(), repos)
			lib := model.Library{Base: model.Base{ID: "tv"}, Name: "TV", Path: "/media/tv", Type: "tv", Enabled: true}
			workDir := "/media/tv/作品"
			if scenario == "generic-category" {
				workDir = "/media/tv/国产剧"
			}
			if scenario == "custom-library-root" {
				lib.Path, workDir = "/media/自定义收藏", "/media/自定义收藏"
			}
			if err := db.Create(&lib).Error; err != nil {
				t.Fatal(err)
			}
			rootID := ""
			if scenario == "second-library-root" {
				root := model.LibraryRoot{Base: model.Base{ID: "second-root"}, LibraryID: lib.ID, Path: "/media/第二个自定义收藏", Enabled: true}
				if err := db.Create(&root).Error; err != nil {
					t.Fatal(err)
				}
				rootID, workDir = root.ID, root.Path
			}
			authority := model.Media{LibraryID: lib.ID, Path: workDir + "/S01E01.mkv", Title: "已刮削单集", SeasonNum: 1, EpisodeNum: 1, ScrapeStatus: "matched", TMDbID: 321, BangumiID: 111}
			authority.LibraryRootID = rootID
			if err := repos.Media.Upsert(t.Context(), &authority); err != nil {
				t.Fatal(err)
			}
			pending := model.Media{LibraryID: lib.ID, Path: workDir + "/S01E02.mkv", Title: "未刮削单集", SeasonNum: 1, EpisodeNum: 2}
			pending.LibraryRootID = rootID
			switch scenario {
			case "different-directory":
				pending.Path = "/media/tv/另一目录/作品/S01E02.mkv"
			case "other-library":
				pending.LibraryID = "another-library"
			case "conflicting-pending-id":
				pending.TMDbID = 654
			case "conflicting-pending-secondary-id":
				pending.BangumiID = 999
			case "different-confirmed-id", "conflicting-secondary-id":
				conflict := authority
				conflict.ID, conflict.Path = "", workDir+"/S01E03.mkv"
				if scenario == "different-confirmed-id" {
					conflict.TMDbID = 654
				} else {
					conflict.BangumiID = 999
				}
				if err := repos.Media.Upsert(t.Context(), &conflict); err != nil {
					t.Fatal(err)
				}
			}
			if err := repos.Media.Upsert(t.Context(), &pending); err != nil {
				t.Fatal(err)
			}
			if pending.SeriesBindingKey != "" || boundSeriesIdentity(pending) != "" {
				t.Fatalf("unsafe bridge in %s: %+v", scenario, pending)
			}
		})
	}
}

func TestSeriesBindingInvalidatesMovedAndConflictingEpisodes(t *testing.T) {
	for _, scenario := range []string{"pending-path-move", "pending-library-move", "authority-path-move", "authority-conflict", "direct-identity-reset", "direct-donor-delete"} {
		t.Run(scenario, func(t *testing.T) {
			db := newServiceTestDB(t)
			if err := database.AutoMigrate(db); err != nil {
				t.Fatal(err)
			}
			repos := repository.New(db)
			NewMediaService(&config.Config{}, zap.NewNop(), repos)
			lib := model.Library{Base: model.Base{ID: "tv"}, Name: "TV", Path: "/media/tv", Type: "tv", Enabled: true}
			if err := db.Create(&lib).Error; err != nil {
				t.Fatal(err)
			}
			rows := []model.Media{
				{LibraryID: lib.ID, Path: "/media/tv/作品/S01E01.mkv", Title: "单集一", SeasonNum: 1, EpisodeNum: 1, ScrapeStatus: "matched", TMDbID: 321},
				{LibraryID: lib.ID, Path: "/media/tv/作品/S01E02.mkv", Title: "单集二", SeasonNum: 1, EpisodeNum: 2},
				{LibraryID: lib.ID, Path: "/media/tv/作品/S01E03.mkv", Title: "单集三", SeasonNum: 1, EpisodeNum: 3},
			}
			for i := range rows {
				if err := repos.Media.Upsert(t.Context(), &rows[i]); err != nil {
					t.Fatal(err)
				}
			}
			if rows[1].SeriesBindingKey != "tmdb:321" {
				t.Fatalf("new pending episode was not bound: %+v", rows[1])
			}
			updates := map[string]any{}
			target := rows[1].ID
			switch scenario {
			case "pending-path-move":
				updates["path"] = "/media/tv/另一个作品/S01E02.mkv"
			case "pending-library-move":
				updates["library_id"] = "other"
			case "authority-path-move":
				target, updates["path"] = rows[0].ID, "/media/tv/另一个作品/S01E01.mkv"
			case "authority-conflict":
				target, updates["tm_db_id"], updates["scrape_status"] = rows[2].ID, 654, "matched"
			case "direct-identity-reset":
				if err := db.Model(&model.Media{}).Where("id = ?", rows[0].ID).Updates(map[string]any{"scrape_status": "pending", "tm_db_id": 0}).Error; err != nil {
					t.Fatal(err)
				}
			case "direct-donor-delete":
				if err := db.Delete(&model.Media{}, "id = ?", rows[0].ID).Error; err != nil {
					t.Fatal(err)
				}
			}
			if len(updates) > 0 {
				if err := repos.Media.UpdateWithCurrentSeriesKey(t.Context(), nil, target, updates); err != nil {
					t.Fatal(err)
				}
			} else if _, err := repos.Media.BackfillSeriesKeys(t.Context(), 10); err != nil {
				t.Fatal(err)
			}
			var pending model.Media
			if err := db.First(&pending, "id = ?", rows[1].ID).Error; err != nil {
				t.Fatal(err)
			}
			if pending.SeriesBindingKey != "" || boundSeriesIdentity(pending) != "" || pending.TMDbID != 0 || pending.ScrapeStatus != "pending" {
				t.Fatalf("obsolete series identity survived %s: %+v", scenario, pending)
			}
			if scenario == "authority-conflict" {
				if err := repos.Media.UpdateWithCurrentSeriesKey(t.Context(), nil, rows[2].ID, map[string]any{"tm_db_id": 321}); err != nil {
					t.Fatal(err)
				}
				if err := db.First(&pending, "id = ?", rows[1].ID).Error; err != nil {
					t.Fatal(err)
				}
				if pending.SeriesBindingKey != "tmdb:321" {
					t.Fatalf("resolved identity conflict did not restore binding: %+v", pending)
				}
			}
		})
	}
}
