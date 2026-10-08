package service

import (
	"fmt"
	"testing"

	"github.com/ShukeBta/MediaStationGo/internal/config"
	"github.com/ShukeBta/MediaStationGo/internal/model"
	"github.com/ShukeBta/MediaStationGo/internal/repository"
	"go.uber.org/zap"
)

func TestPersistedSeriesGroupsVersionsAndNewEpisodesAcrossScans(t *testing.T) {
	db := newServiceTestDB(t, &model.Library{}, &model.Media{})
	repos := repository.New(db)
	svc := NewMediaService(&config.Config{}, zap.NewNop(), repos)
	NewEmbyService(&config.Config{}, zap.NewNop(), repos)
	lib := model.Library{Name: "剧集", Path: "/media/tv", Type: "tv", Enabled: true}
	if err := repos.Library.Create(t.Context(), &lib); err != nil {
		t.Fatal(err)
	}
	var groupKey string
	for index, item := range []struct {
		release string
		episode int
	}{{"1080p", 1}, {"2160p", 1}, {"1080p", 2}, {"2160p", 3}} {
		row := model.Media{LibraryID: lib.ID, Title: "同一部剧", ScrapeStatus: "matched", TMDbID: 123,
			Path: fmt.Sprintf("/media/tv/同一部剧-%s/S01E%02d.mkv", item.release, item.episode), SeasonNum: 1, EpisodeNum: item.episode}
		if err := repos.Media.Upsert(t.Context(), &row); err != nil {
			t.Fatal(err)
		}
		cards, total, err := svc.ListLibrarySeriesCards(t.Context(), lib.ID, 1, 20, MediaVisibility{})
		if err != nil || total != 1 || len(cards) != 1 || cards[0].Count != index+1 {
			t.Fatalf("scan %d: cards=%+v total=%d err=%v", index, cards, total, err)
		}
		if index == 0 {
			groupKey = cards[0].Key
		} else if cards[0].Key != groupKey {
			t.Fatalf("scan %d changed collection identity: %q -> %q", index, groupKey, cards[0].Key)
		}
		found, count, err := svc.SearchMediaVisibleSeriesPage(t.Context(), "同一部剧", 1, 20, MediaVisibility{})
		if err != nil || count != 1 || len(found) != 1 || found[0].Count != index+1 {
			t.Fatalf("search after scan %d: cards=%+v total=%d err=%v", index, found, count, err)
		}
	}
}

func TestManualSeriesGroupAdoptsNewEpisodeAndVersion(t *testing.T) {
	db := newServiceTestDB(t, &model.Library{}, &model.Media{})
	repos := repository.New(db)
	svc := NewMediaService(&config.Config{}, zap.NewNop(), repos)
	NewEmbyService(&config.Config{}, zap.NewNop(), repos)
	lib := model.Library{Name: "剧集", Path: "/media/tv", Type: "tv", Enabled: true}
	if err := repos.Library.Create(t.Context(), &lib); err != nil {
		t.Fatal(err)
	}
	rows := []model.Media{
		{LibraryID: lib.ID, Title: "来源一", Path: "/media/tv/release-a/S01E01.mkv", SeasonNum: 1, EpisodeNum: 1, TMDbID: 123},
		{LibraryID: lib.ID, Title: "来源二", Path: "/media/tv/release-b/S01E02.mkv", SeasonNum: 1, EpisodeNum: 2, TMDbID: 123},
	}
	for i := range rows {
		if err := repos.Media.Upsert(t.Context(), &rows[i]); err != nil {
			t.Fatal(err)
		}
	}
	group, err := svc.UpdateMediaAggregation(t.Context(), lib.ID, MediaAggregationRequest{Action: "group", Title: "我的剧集", MediaIDs: []string{rows[0].ID, rows[1].ID}})
	if err != nil {
		t.Fatal(err)
	}
	for i, item := range []struct {
		path            string
		episode         int
		delayedMetadata bool
	}{
		{"/media/tv/release-c/S01E03.mkv", 3, false},
		{"/media/tv/release-2160p/S01E01.mkv", 1, false},
		{"/media/tv/release-d/S01E04.mkv", 4, true},
	} {
		row := model.Media{LibraryID: lib.ID, Title: "新文件", Path: item.path, SeasonNum: 1, EpisodeNum: item.episode, TMDbID: 123}
		if item.delayedMetadata {
			row.TMDbID = 0
		}
		if err := repos.Media.Upsert(t.Context(), &row); err != nil {
			t.Fatal(err)
		}
		if item.delayedMetadata {
			if err := repos.Media.UpdateWithCurrentSeriesKey(t.Context(), nil, row.ID, map[string]any{"tm_db_id": 123, "scrape_status": "matched"}); err != nil {
				t.Fatal(err)
			}
			if err := db.First(&row, "id = ?", row.ID).Error; err != nil {
				t.Fatal(err)
			}
		}
		if row.PartGroupKey != group.GroupKey || row.PartGroupTitle != "我的剧集" {
			t.Fatalf("new file escaped manual group: %+v", row)
		}
		cards, total, err := svc.ListLibrarySeriesCards(t.Context(), lib.ID, 1, 20, MediaVisibility{})
		if err != nil || total != 1 || len(cards) != 1 || cards[0].Count != i+3 {
			t.Fatalf("manual group after new file: cards=%+v total=%d err=%v", cards, total, err)
		}
	}
	if _, err := svc.UpdateMediaAggregation(t.Context(), lib.ID, MediaAggregationRequest{Action: "detach", MediaIDs: []string{rows[0].ID}}); err != nil {
		t.Fatal(err)
	}
	if err := repos.Media.Upsert(t.Context(), &rows[0]); err != nil {
		t.Fatal(err)
	}
	if err := repos.Media.UpdateWithCurrentSeriesKey(t.Context(), nil, rows[0].ID, map[string]any{"title": "刷新后的标题", "tm_db_id": 123, "scrape_status": "matched"}); err != nil {
		t.Fatal(err)
	}
	if err := db.First(&rows[0], "id = ?", rows[0].ID).Error; err != nil {
		t.Fatal(err)
	}
	if rows[0].PartGroupKey != "" {
		t.Fatal("explicitly detached member was automatically reattached")
	}
}

func TestManualSeriesInheritanceRejectsAmbiguousOrConflictingIdentity(t *testing.T) {
	for _, scenario := range []string{"other-library", "conflicting-id", "ambiguous-groups", "unknown-year", "existing-choice"} {
		t.Run(scenario, func(t *testing.T) {
			db := newServiceTestDB(t, &model.Library{}, &model.Media{})
			repos := repository.New(db)
			NewMediaService(&config.Config{}, zap.NewNop(), repos)
			member := model.Media{LibraryID: "tv", Title: "同名剧", Path: "/media/tv/original/S01E01.mkv", SeasonNum: 1, EpisodeNum: 1, TMDbID: 123, BangumiID: 456, PartGroupKey: "manual-a", PartGroupTitle: "我的剧集", PartIndex: 1}
			if err := repos.Media.Upsert(t.Context(), &member); err != nil {
				t.Fatal(err)
			}
			incoming := model.Media{LibraryID: "tv", Title: "同名剧", Path: "/media/tv/new/S01E02.mkv", SeasonNum: 1, EpisodeNum: 2, TMDbID: 123}
			switch scenario {
			case "other-library":
				incoming.LibraryID = "other"
			case "conflicting-id":
				incoming.BangumiID = 999
			case "ambiguous-groups":
				other := member
				other.ID = ""
				other.Path = "/media/tv/other/S01E01.mkv"
				other.PartGroupKey = "manual-b"
				if err := repos.Media.Upsert(t.Context(), &other); err != nil {
					t.Fatal(err)
				}
			case "unknown-year":
				incoming.TMDbID = 0
				incoming.ScrapeStatus = "matched"
			case "existing-choice":
				incoming.PartGroupKey = "manual-choice"
			}
			want := incoming.PartGroupKey
			if err := repos.Media.Upsert(t.Context(), &incoming); err != nil {
				t.Fatal(err)
			}
			if incoming.PartGroupKey != want {
				t.Fatalf("unsafe inheritance: got %q want %q", incoming.PartGroupKey, want)
			}
		})
	}
}
