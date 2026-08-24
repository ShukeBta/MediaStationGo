package service

import (
	"testing"
	"time"

	"go.uber.org/zap"

	"github.com/ShukeBta/MediaStationGo/internal/config"
	"github.com/ShukeBta/MediaStationGo/internal/model"
	"github.com/ShukeBta/MediaStationGo/internal/repository"
)

func TestWeeklyFeaturedCardRotatesHighRatedVisibleNonAdultWorks(t *testing.T) {
	db := newServiceTestDB(t, &model.Library{}, &model.Media{})
	repos := repository.New(db)
	allowed := model.Library{Base: model.Base{ID: "allowed"}, Name: "普通媒体", Path: "/media/safe", Type: "movie", Enabled: true}
	denied := model.Library{Base: model.Base{ID: "denied"}, Name: "无权限媒体", Path: "/media/denied", Type: "movie", Enabled: true}
	adult := model.Library{Base: model.Base{ID: "adult"}, Name: "成人媒体", Path: "/media/adult", Type: "adult", Enabled: true}
	if err := db.Create(&[]model.Library{allowed, denied, adult}).Error; err != nil {
		t.Fatal(err)
	}
	rows := []model.Media{
		{Base: model.Base{ID: "allowed-a1"}, LibraryID: allowed.ID, SeriesID: "series-a", Title: "高分甲", Path: "/media/safe/a/S01E01.mkv", SeasonNum: 1, EpisodeNum: 1, Rating: 9.2},
		{Base: model.Base{ID: "allowed-a2"}, LibraryID: allowed.ID, SeriesID: "series-a", Title: "高分甲", Path: "/media/safe/a/S01E02.mkv", SeasonNum: 1, EpisodeNum: 2, Rating: 8.8},
		{Base: model.Base{ID: "allowed-b"}, LibraryID: allowed.ID, Title: "高分乙", Path: "/media/safe/高分乙/b.mkv", Rating: 8.8},
		{Base: model.Base{ID: "allowed-low"}, LibraryID: allowed.ID, Title: "低分作品", Path: "/media/safe/低分作品/low.mkv", Rating: 6.0},
		{Base: model.Base{ID: "allowed-nsfw"}, LibraryID: allowed.ID, Title: "混合库成人作品", Path: "/media/safe/混合库成人作品/nsfw.mkv", Rating: 10, NSFW: true},
		{Base: model.Base{ID: "denied-top"}, LibraryID: denied.ID, Title: "无权限高分作品", Path: "/media/denied/无权限高分作品/top.mkv", Rating: 10},
		// Explicit adult libraries must be excluded even if a stale row has not
		// yet been repaired to NSFW=true.
		{Base: model.Base{ID: "adult-unflagged"}, LibraryID: adult.ID, Title: "成人库漏标作品", Path: "/media/adult/成人库漏标作品/unflagged.mkv", Rating: 10},
	}
	if err := db.Create(&rows).Error; err != nil {
		t.Fatal(err)
	}
	svc := NewMediaService(&config.Config{}, zap.NewNop(), repos)
	visibility := MediaVisibility{
		IncludeNSFW:       true,
		AllowedLibraryIDs: []string{allowed.ID, adult.ID},
	}

	weekOne := time.Date(2026, 8, 24, 12, 0, 0, 0, time.FixedZone("CST", 8*60*60))
	first, firstWeek, err := svc.WeeklyFeaturedCard(t.Context(), weekOne, visibility)
	if err != nil {
		t.Fatal(err)
	}
	again, againWeek, err := svc.WeeklyFeaturedCard(t.Context(), weekOne.Add(3*24*time.Hour), visibility)
	if err != nil {
		t.Fatal(err)
	}
	next, nextWeek, err := svc.WeeklyFeaturedCard(t.Context(), weekOne.AddDate(0, 0, 7), visibility)
	if err != nil {
		t.Fatal(err)
	}
	if first == nil || again == nil || next == nil {
		t.Fatalf("weekly selections = first:%#v again:%#v next:%#v", first, again, next)
	}
	allowedTitles := map[string]bool{"高分甲": true, "高分乙": true}
	if !allowedTitles[first.Rep.Title] || !allowedTitles[again.Rep.Title] || !allowedTitles[next.Rep.Title] {
		t.Fatalf("recommendation escaped the visible high-rated safe pool: %q, %q, %q", first.Rep.Title, again.Rep.Title, next.Rep.Title)
	}
	if first.Key != again.Key || firstWeek != againWeek {
		t.Fatalf("same week changed selection: first=%q/%q again=%q/%q", first.Key, firstWeek, again.Key, againWeek)
	}
	if first.Key == next.Key || firstWeek == nextWeek {
		t.Fatalf("next week did not rotate: first=%q/%q next=%q/%q", first.Key, firstWeek, next.Key, nextWeek)
	}
	for _, card := range []*SeriesCard{first, next} {
		if card.Rep.Title == "高分甲" && card.Count != 2 {
			t.Fatalf("series recommendation was not aggregated as one work: %#v", card)
		}
	}
}

func TestWeeklyFeaturedCardReturnsNoItemWithoutHighRatedCandidate(t *testing.T) {
	db := newServiceTestDB(t, &model.Library{}, &model.Media{})
	repos := repository.New(db)
	lib := model.Library{Base: model.Base{ID: "safe"}, Name: "普通媒体", Path: "/media/safe", Type: "movie", Enabled: true}
	if err := db.Create(&lib).Error; err != nil {
		t.Fatal(err)
	}
	if err := db.Create(&model.Media{
		Base: model.Base{ID: "low"}, LibraryID: lib.ID, Title: "普通评分", Path: "/media/safe/low.mkv", Rating: 7.4,
	}).Error; err != nil {
		t.Fatal(err)
	}
	svc := NewMediaService(&config.Config{}, zap.NewNop(), repos)
	item, week, err := svc.WeeklyFeaturedCard(t.Context(), time.Date(2026, 8, 24, 0, 0, 0, 0, time.UTC), MediaVisibility{IncludeNSFW: true})
	if err != nil {
		t.Fatal(err)
	}
	if item != nil || week != "2026-W35" {
		t.Fatalf("featured=%#v week=%q, want nil item and ISO week", item, week)
	}
}
