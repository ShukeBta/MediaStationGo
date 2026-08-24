package service

import (
	"fmt"
	"testing"
	"time"

	"go.uber.org/zap"

	"github.com/ShukeBta/MediaStationGo/internal/config"
	"github.com/ShukeBta/MediaStationGo/internal/model"
	"github.com/ShukeBta/MediaStationGo/internal/repository"
)

func TestWeeklyFeaturedCardRotatesHighRatedVisibleNonAdultWorks(t *testing.T) {
	db := newServiceTestDB(t, &model.Library{}, &model.Media{}, &model.WeeklyFeaturedSelection{})
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
	first, firstWeek, err := svc.WeeklyFeaturedCard(t.Context(), "viewer", weekOne, visibility)
	if err != nil {
		t.Fatal(err)
	}
	again, againWeek, err := svc.WeeklyFeaturedCard(t.Context(), "viewer", weekOne.Add(3*24*time.Hour), visibility)
	if err != nil {
		t.Fatal(err)
	}
	next, nextWeek, err := svc.WeeklyFeaturedCard(t.Context(), "viewer", weekOne.AddDate(0, 0, 7), visibility)
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
	db := newServiceTestDB(t, &model.Library{}, &model.Media{}, &model.WeeklyFeaturedSelection{})
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
	item, week, err := svc.WeeklyFeaturedCard(t.Context(), "viewer", time.Date(2026, 8, 24, 0, 0, 0, 0, time.UTC), MediaVisibility{IncludeNSFW: true})
	if err != nil {
		t.Fatal(err)
	}
	if item != nil || week != "2026-W35" {
		t.Fatalf("featured=%#v week=%q, want nil item and ISO week", item, week)
	}
}

func TestWeeklyFeaturedSelectionPersistsCooldownAndRelaxesToOldestWork(t *testing.T) {
	db := newServiceTestDB(t, &model.WeeklyFeaturedSelection{})
	newService := func() *MediaService {
		return NewMediaService(&config.Config{}, zap.NewNop(), repository.New(db))
	}
	candidates := []weeklyFeaturedCandidate{
		{card: SeriesCard{Key: "series-a", Rep: model.Media{Base: model.Base{ID: "episode-a"}}}, order: 1},
		{card: SeriesCard{Key: "series-b", Rep: model.Media{Base: model.Base{ID: "episode-b"}}}, order: 2},
		{card: SeriesCard{Key: "movie-c", Rep: model.Media{Base: model.Base{ID: "movie-c"}}}, order: 3},
	}
	weekOne := time.Date(2026, 8, 24, 12, 0, 0, 0, time.UTC)
	selected := make([]SeriesCard, 0, 4)
	for week := 0; week < 3; week++ {
		now := weekOne.AddDate(0, 0, 7*week)
		card, err := newService().selectWeeklyFeaturedCandidate(
			t.Context(), "viewer-a", now, weeklyFeaturedWeekKey(now), candidates,
		)
		if err != nil {
			t.Fatal(err)
		}
		selected = append(selected, card)
	}
	if selected[0].Key == selected[1].Key || selected[0].Key == selected[2].Key || selected[1].Key == selected[2].Key {
		t.Fatalf("cooldown repeated a work while unused candidates remained: %#v", selected)
	}

	// Recreating the service proves the current-week choice and cooldown are
	// persisted in the database rather than kept in process memory.
	weekThree := weekOne.AddDate(0, 0, 14)
	again, err := newService().selectWeeklyFeaturedCandidate(
		t.Context(), "viewer-a", weekThree, weeklyFeaturedWeekKey(weekThree), candidates,
	)
	if err != nil {
		t.Fatal(err)
	}
	if again.Key != selected[2].Key {
		t.Fatalf("persisted same-week selection changed after service restart: got %q want %q", again.Key, selected[2].Key)
	}

	weekFour := weekOne.AddDate(0, 0, 21)
	relaxed, err := newService().selectWeeklyFeaturedCandidate(
		t.Context(), "viewer-a", weekFour, weeklyFeaturedWeekKey(weekFour), candidates,
	)
	if err != nil {
		t.Fatal(err)
	}
	if relaxed.Key != selected[0].Key {
		t.Fatalf("exhausted cooldown did not relax to least recently featured work: got %q want %q", relaxed.Key, selected[0].Key)
	}

	other, err := newService().selectWeeklyFeaturedCandidate(
		t.Context(), "viewer-b", weekFour, weeklyFeaturedWeekKey(weekFour), candidates,
	)
	if err != nil {
		t.Fatal(err)
	}
	if other.Key == "" {
		t.Fatal("second user did not receive an independent recommendation")
	}
	var counts []struct {
		UserID string
		Count  int64
	}
	if err := db.Model(&model.WeeklyFeaturedSelection{}).
		Select("user_id, count(*) AS count").
		Group("user_id").
		Order("user_id").
		Scan(&counts).Error; err != nil {
		t.Fatal(err)
	}
	if len(counts) != 2 || counts[0].UserID != "viewer-a" || counts[0].Count != 4 || counts[1].UserID != "viewer-b" || counts[1].Count != 1 {
		t.Fatalf("weekly histories were not isolated per user: %#v", counts)
	}
}

func TestWeeklyFeaturedSelectionAvoidsThePreviousEightRecommendationWeeks(t *testing.T) {
	db := newServiceTestDB(t, &model.WeeklyFeaturedSelection{})
	svc := NewMediaService(&config.Config{}, zap.NewNop(), repository.New(db))
	candidates := make([]weeklyFeaturedCandidate, 0, weeklyFeaturedCooldownWeeks+1)
	for index := 0; index <= weeklyFeaturedCooldownWeeks; index++ {
		key := fmt.Sprintf("work-%d", index)
		candidates = append(candidates, weeklyFeaturedCandidate{
			card:  SeriesCard{Key: key, Rep: model.Media{Base: model.Base{ID: key}}},
			order: uint64(index),
		})
	}

	weekOne := time.Date(2026, 8, 24, 12, 0, 0, 0, time.UTC)
	seen := make(map[string]bool, len(candidates))
	var first string
	for week := 0; week <= weeklyFeaturedCooldownWeeks; week++ {
		now := weekOne.AddDate(0, 0, 7*week)
		card, err := svc.selectWeeklyFeaturedCandidate(
			t.Context(), "viewer", now, weeklyFeaturedWeekKey(now), candidates,
		)
		if err != nil {
			t.Fatal(err)
		}
		if seen[card.Key] {
			t.Fatalf("work %q repeated within the previous %d recommendation weeks", card.Key, weeklyFeaturedCooldownWeeks)
		}
		if week == 0 {
			first = card.Key
		}
		seen[card.Key] = true
	}

	weekAfterCooldown := weekOne.AddDate(0, 0, 7*(weeklyFeaturedCooldownWeeks+1))
	card, err := svc.selectWeeklyFeaturedCandidate(
		t.Context(), "viewer", weekAfterCooldown, weeklyFeaturedWeekKey(weekAfterCooldown), candidates,
	)
	if err != nil {
		t.Fatal(err)
	}
	if card.Key != first {
		t.Fatalf("oldest eligible work after cooldown = %q, want %q", card.Key, first)
	}
}
