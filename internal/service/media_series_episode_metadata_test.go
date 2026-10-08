package service

import (
	"fmt"
	"testing"

	"github.com/ShukeBta/MediaStationGo/internal/config"
	"github.com/ShukeBta/MediaStationGo/internal/model"
	"github.com/ShukeBta/MediaStationGo/internal/repository"
	"go.uber.org/zap"
)

func TestPersistedSeriesIgnoresEpisodePresentationMetadata(t *testing.T) {
	db := newServiceTestDB(t, &model.Library{}, &model.Media{})
	repos := repository.New(db)
	svc := NewMediaService(&config.Config{}, zap.NewNop(), repos)
	lib := model.Library{Name: "剧集", Path: "/media/tv", Type: "tv", Enabled: true}
	if err := repos.Library.Create(t.Context(), &lib); err != nil {
		t.Fatal(err)
	}
	rows := make([]model.Media, 3)
	for i := range rows {
		rows[i] = model.Media{LibraryID: lib.ID, Title: "共同剧名", Path: fmt.Sprintf("/media/tv/共同剧名/S01E%02d.mkv", i+1), SeasonNum: 1, EpisodeNum: i + 1}
		if err := repos.Media.Upsert(t.Context(), &rows[i]); err != nil {
			t.Fatal(err)
		}
	}
	assertOne := func() string {
		t.Helper()
		cards, total, err := svc.ListLibrarySeriesCards(t.Context(), lib.ID, 1, 20, MediaVisibility{})
		if err != nil || total != 1 || len(cards) != 1 || cards[0].Count != 3 {
			t.Fatalf("episode presentation split work: total=%d cards=%+v err=%v", total, cards, err)
		}
		episodes, err := svc.ListLibrarySeriesEpisodes(t.Context(), lib.ID, cards[0].Key, MediaVisibility{})
		if err != nil || len(episodes) != 3 {
			t.Fatalf("episode membership=%d err=%v", len(episodes), err)
		}
		return cards[0].Key
	}
	assertOne()
	for i := range rows {
		if err := repos.Media.UpdateWithCurrentSeriesKey(t.Context(), nil, rows[i].ID, map[string]any{
			"tm_db_id": 98765, "scrape_status": "matched", "title": fmt.Sprintf("第%d集独立标题", i+1),
			"episode_title": fmt.Sprintf("单集名%d", i+1), "overview": fmt.Sprintf("单集简介%d", i+1),
			"poster_url": fmt.Sprintf("https://example.test/poster%d.jpg", i), "backdrop_url": fmt.Sprintf("https://example.test/still%d.jpg", i),
		}); err != nil {
			t.Fatal(err)
		}
		assertOne()
		for j := i + 1; j < len(rows); j++ {
			var pending model.Media
			if err := db.First(&pending, "id = ?", rows[j].ID).Error; err != nil {
				t.Fatal(err)
			}
			if pending.TMDbID != 0 || pending.ScrapeStatus != "pending" || pending.PosterURL != "" || pending.Overview != "" {
				t.Fatalf("series binding fabricated episode metadata: %+v", pending)
			}
		}
	}
	key := assertOne()
	for i := range rows {
		incoming := model.Media{LibraryID: lib.ID, Path: rows[i].Path, Title: fmt.Sprintf("filename-%d", i), SeasonNum: 1, EpisodeNum: i + 1, SizeBytes: 100}
		if err := repos.Media.Upsert(t.Context(), &incoming); err != nil {
			t.Fatal(err)
		}
		if err := repos.Media.UpdateWithCurrentSeriesKey(t.Context(), nil, rows[i].ID, map[string]any{"title": fmt.Sprintf("再更新单集标题%d", i), "overview": "新简介", "poster_url": "https://example.test/new.jpg", "backdrop_url": "https://example.test/newstill.jpg"}); err != nil {
			t.Fatal(err)
		}
	}
	if got := assertOne(); got != key {
		t.Fatalf("presentation refresh changed work identity: %q -> %q", key, got)
	}
	var stored []model.Media
	if err := db.Find(&stored).Error; err != nil {
		t.Fatal(err)
	}
	if cards := groupMediaSeriesCards(stored); len(cards) != 1 {
		t.Fatalf("resolver split work: %+v", cards)
	}
	for i := range stored {
		if err := db.Model(&model.Media{}).Where("id = ?", stored[i].ID).UpdateColumns(map[string]any{"series_key": fmt.Sprintf("old-title-group-%d", i), "series_key_version": 2}).Error; err != nil {
			t.Fatal(err)
		}
	}
	if got := assertOne(); got != key {
		t.Fatalf("old title-based projection was not repaired: %q -> %q", key, got)
	}
	other := stored[0]
	other.ID = "different-work"
	other.TMDbID++
	if cards := groupMediaSeriesCards(append(stored, other)); len(cards) != 2 {
		t.Fatalf("same episode title merged distinct authoritative works: %+v", cards)
	}
}

func TestPersistedSeriesIDWithoutProviderIDIgnoresEpisodeTitles(t *testing.T) {
	db := newServiceTestDB(t, &model.Library{}, &model.Media{})
	repos := repository.New(db)
	svc := NewMediaService(&config.Config{}, zap.NewNop(), repos)
	lib := model.Library{Name: "剧集", Path: "/media/tv", Type: "tv", Enabled: true}
	if err := repos.Library.Create(t.Context(), &lib); err != nil {
		t.Fatal(err)
	}
	rows := make([]model.Media, 2)
	for i := range rows {
		rows[i] = model.Media{LibraryID: lib.ID, SeriesID: "local-series", Title: fmt.Sprintf("单集标题%d", i), ScrapeStatus: "matched", Path: fmt.Sprintf("/media/tv/source-%d/S01E%02d.mkv", i, i+1), SeasonNum: 1, EpisodeNum: i + 1}
		if err := repos.Media.Upsert(t.Context(), &rows[i]); err != nil {
			t.Fatal(err)
		}
		if err := repos.Media.UpdateWithCurrentSeriesKey(t.Context(), nil, rows[i].ID, map[string]any{"title": fmt.Sprintf("更改单集标题%d", i), "overview": fmt.Sprintf("简介%d", i), "poster_url": fmt.Sprintf("https://example.test/%d.jpg", i)}); err != nil {
			t.Fatal(err)
		}
	}
	cards, total, err := svc.ListLibrarySeriesCards(t.Context(), lib.ID, 1, 20, MediaVisibility{})
	if err != nil || total != 1 || len(cards) != 1 || cards[0].Count != 2 {
		t.Fatalf("explicit SeriesID split: cards=%+v total=%d err=%v", cards, total, err)
	}
	if cards := groupMediaSeriesCards(rows); len(cards) != 1 {
		t.Fatalf("resolver split explicit SeriesID: %+v", cards)
	}
}
