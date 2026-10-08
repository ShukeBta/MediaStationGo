package service

import (
	"fmt"
	"strings"
	"testing"
	"time"

	"github.com/ShukeBta/MediaStationGo/internal/config"
	"github.com/ShukeBta/MediaStationGo/internal/model"
	"github.com/ShukeBta/MediaStationGo/internal/repository"
	"go.uber.org/zap"
)

func TestWorkSearchRepairsEpisodeProjectionWithCurrentSeriesKeys(t *testing.T) {
	for _, stale := range []struct {
		name   string
		fields map[string]any
	}{
		{"missing-name", map[string]any{"emby_series_name": ""}},
		{"whitespace-name", map[string]any{"emby_series_name": "   "}},
		{"old-version", map[string]any{"emby_series_name": "Wrong old name", "emby_key_version": 0}},
	} {
		t.Run(stale.name, func(t *testing.T) {
			db := newServiceTestDB(t, &model.Library{}, &model.Media{})
			repos := repository.New(db)
			svc := NewMediaService(&config.Config{}, zap.NewNop(), repos)
			NewEmbyService(&config.Config{}, zap.NewNop(), repos)
			libs := []model.Library{
				{Base: model.Base{ID: "visible"}, Name: "剧集", Path: "/media/tv", Type: "tv", Enabled: true},
				{Base: model.Base{ID: "hidden"}, Name: "隐藏", Path: "/media/hidden", Type: "tv", Enabled: true},
			}
			if err := db.Create(&libs).Error; err != nil {
				t.Fatal(err)
			}
			now := time.Date(2026, 10, 1, 0, 0, 0, 0, time.UTC)
			rows := []model.Media{
				{Base: model.Base{ID: "target-1", CreatedAt: now}, LibraryID: "visible", Title: "目标剧", Path: "/media/tv/目标剧/Season 1/S01E01.mkv", SeasonNum: 1, EpisodeNum: 1, EpisodeTitle: "OnlyEpisodeSecret"},
				{Base: model.Base{ID: "target-2", CreatedAt: now}, LibraryID: "visible", Title: "目标剧", Path: "/media/tv/目标剧/Season 1/S01E02.mkv", SeasonNum: 1, EpisodeNum: 2},
				{Base: model.Base{ID: "other", CreatedAt: now.Add(time.Hour)}, LibraryID: "visible", Title: "其他剧", Path: "/media/tv/其他剧/Season 1/S01E01.mkv", SeasonNum: 1, EpisodeNum: 1},
				{Base: model.Base{ID: "aaa-hidden"}, LibraryID: "hidden", Title: "目标剧", Path: "/media/hidden/目标剧/S01E01.mkv", SeasonNum: 1, EpisodeNum: 1},
				{Base: model.Base{ID: "aaa-nsfw"}, LibraryID: "visible", Title: "目标剧", Path: "/media/tv/秘密剧/S01E01.mkv", SeasonNum: 1, EpisodeNum: 1, NSFW: true},
				{Base: model.Base{ID: "aaa-deleted"}, LibraryID: "visible", Title: "目标剧", Path: "/media/tv/删除剧/S01E01.mkv", SeasonNum: 1, EpisodeNum: 1},
			}
			for i := range rows {
				if err := repos.Media.Upsert(t.Context(), &rows[i]); err != nil {
					t.Fatal(err)
				}
			}
			if err := db.Delete(&model.Media{}, "id = ?", "aaa-deleted").Error; err != nil {
				t.Fatal(err)
			}
			if err := db.Unscoped().Model(&model.Media{}).Where("1 = 1").UpdateColumns(stale.fields).Error; err != nil {
				t.Fatal(err)
			}
			visibility := MediaVisibility{AllowedLibraryIDs: []string{"visible", "hidden"}, HiddenLibraryIDs: []string{"hidden"}}
			cards, total, err := svc.SearchMediaVisibleSeriesPage(t.Context(), "目标剧", 1, 1, visibility)
			if err != nil || total != 1 || len(cards) != 1 || cards[0].Count != 2 {
				t.Fatalf("cards=%#v total=%d err=%v", cards, total, err)
			}
			for _, page := range []int{1, 2, 3} {
				cards, total, err = svc.SearchMediaVisibleSeriesPage(t.Context(), "剧", page, 1, visibility)
				wantCount := 1
				if page == 3 {
					wantCount = 0
				}
				if err != nil || total != 2 || len(cards) != wantCount {
					t.Fatalf("page %d: cards=%#v total=%d err=%v", page, cards, total, err)
				}
			}
			cards, total, err = svc.SearchMediaVisibleSeriesPage(t.Context(), "OnlyEpisodeSecret", 1, 20, visibility)
			if err != nil || total != 0 || len(cards) != 0 {
				t.Fatalf("episode title leaked into work search: total=%d err=%v", total, err)
			}
			for _, original := range rows {
				var stored model.Media
				if err := db.Unscoped().First(&stored, "id = ?", original.ID).Error; err != nil {
					t.Fatal(err)
				}
				if stored.SeriesKey != original.SeriesKey || stored.SeriesKeyVersion != original.SeriesKeyVersion {
					t.Fatal("valid series identity changed")
				}
				if strings.HasPrefix(original.ID, "aaa-") {
					if stored.EmbySeriesName != fmt.Sprint(stale.fields["emby_series_name"]) {
						t.Fatalf("out-of-scope row %s was repaired", stored.ID)
					}
				} else if strings.TrimSpace(stored.EmbySeriesName) == "" || stored.EmbyKeyVersion != repository.EmbyKeyVersion {
					t.Fatalf("projection not repaired: %s", stored.ID)
				}
			}
		})
	}
}

func TestWorkSearchProjectionRepairRemainsBounded(t *testing.T) {
	db := newServiceTestDB(t, &model.Library{}, &model.Media{})
	repos := repository.New(db)
	svc := NewMediaService(&config.Config{}, zap.NewNop(), repos)
	NewEmbyService(&config.Config{}, zap.NewNop(), repos)
	lib := model.Library{Base: model.Base{ID: "visible"}, Name: "剧集", Path: "/media/tv", Type: "tv", Enabled: true}
	if err := db.Create(&lib).Error; err != nil {
		t.Fatal(err)
	}
	rows := make([]model.Media, 501)
	for i := range rows {
		rows[i] = model.Media{Base: model.Base{ID: fmt.Sprintf("episode-%04d", i)}, LibraryID: lib.ID, Title: "目标剧", Path: fmt.Sprintf("/media/tv/目标剧/Season 1/S01E%03d.mkv", i+1), SeasonNum: 1, EpisodeNum: i + 1}
		rows[i].SeriesKey = MediaSeriesKey(rows[i])
		rows[i].SeriesKeyVersion = 1
		repos.Media.PrepareEmbyKeys(&rows[i])
		rows[i].EmbySeriesName = ""
	}
	if err := db.CreateInBatches(&rows, 10).Error; err != nil {
		t.Fatal(err)
	}
	// Establish current grouping first; grouping migration now also refreshes
	// Emby projections, so simulate an independently missing work name afterward.
	if _, err := repos.Media.BackfillSeriesKeysFiltered(t.Context(), nil, repository.MediaQueryFilter{IncludeNSFW: true}, 1000); err != nil {
		t.Fatal(err)
	}
	if err := db.Model(&model.Media{}).Where("library_id = ?", lib.ID).UpdateColumn("emby_series_name", "").Error; err != nil {
		t.Fatal(err)
	}
	_, _, err := svc.SearchMediaVisibleSeriesPage(t.Context(), "目标剧", 1, 1, MediaVisibility{})
	if err == nil || !strings.Contains(err.Error(), "request limit 500") {
		t.Fatalf("expected explicit bounded repair error, got %v", err)
	}
	var missing int64
	if err := db.Model(&model.Media{}).Where("emby_series_name = ''").Count(&missing).Error; err != nil {
		t.Fatal(err)
	}
	if missing != 1 {
		t.Fatalf("missing=%d, want 1 after bounded repair", missing)
	}
	cards, total, err := svc.SearchMediaVisibleSeriesPage(t.Context(), "目标剧", 1, 1, MediaVisibility{})
	if err != nil || total != 1 || len(cards) != 1 || cards[0].Count != 501 {
		t.Fatalf("second request: total=%d cards=%#v err=%v", total, cards, err)
	}
}

func TestEmbyBackfillRepairsMissingWorkNameWithCurrentGroupingKeys(t *testing.T) {
	db := newServiceTestDB(t, &model.Library{}, &model.Media{})
	repos := repository.New(db)
	NewEmbyService(&config.Config{}, zap.NewNop(), repos)
	row := model.Media{LibraryID: "tv", Title: "目标剧", Path: "/media/tv/目标剧/S01E01.mkv", SeasonNum: 1, EpisodeNum: 1}
	if err := repos.Media.Upsert(t.Context(), &row); err != nil {
		t.Fatal(err)
	}
	if err := db.Model(&model.Media{}).Where("id = ?", row.ID).UpdateColumn("emby_series_name", "").Error; err != nil {
		t.Fatal(err)
	}
	count, err := repos.Media.BackfillEmbyKeys(t.Context(), 1)
	if err != nil || count != 1 {
		t.Fatalf("count=%d err=%v", count, err)
	}
	var stored model.Media
	if err := db.First(&stored, "id = ?", row.ID).Error; err != nil {
		t.Fatal(err)
	}
	if stored.EmbySeriesName != row.EmbySeriesName {
		t.Fatalf("name=%q want %q", stored.EmbySeriesName, row.EmbySeriesName)
	}
}

func TestUnchangedScanRepairsMissingWorkSearchName(t *testing.T) {
	db := newServiceTestDB(t, &model.Library{}, &model.Media{})
	repos := repository.New(db)
	NewMediaService(&config.Config{}, zap.NewNop(), repos)
	NewEmbyService(&config.Config{}, zap.NewNop(), repos)
	row := model.Media{LibraryID: "tv", Title: "目标剧", Path: "/media/tv/目标剧/S01E01.mkv", SeasonNum: 1, EpisodeNum: 1}
	if err := repos.Media.Upsert(t.Context(), &row); err != nil {
		t.Fatal(err)
	}
	wantName := row.EmbySeriesName
	if err := db.Model(&model.Media{}).Where("id = ?", row.ID).UpdateColumn("emby_series_name", "   ").Error; err != nil {
		t.Fatal(err)
	}
	if err := repos.Media.Upsert(t.Context(), &row); err != nil {
		t.Fatal(err)
	}
	if row.EmbySeriesName != wantName || strings.TrimSpace(row.EmbySeriesName) == "" {
		t.Fatalf("unchanged scan left stale work name %q, want %q", row.EmbySeriesName, wantName)
	}
}
