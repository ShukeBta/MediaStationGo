package service

import (
	"fmt"
	"testing"

	"github.com/ShukeBta/MediaStationGo/internal/model"
)

func TestEmbySeriesBindingKeepsMixedVersionsTogether(t *testing.T) {
	for _, provider := range []string{"tmdb", "bangumi", "douban", "thetvdb", "series"} {
		t.Run(provider, func(t *testing.T) {
			e := newTestEmbyService(t)
			lib := model.Library{Name: "Shows", Path: "/media/shows", Type: "tv", Enabled: true}
			if err := e.repo.Library.Create(t.Context(), &lib); err != nil {
				t.Fatal(err)
			}
			rows := []model.Media{
				{Base: model.Base{ID: "matched"}, LibraryID: lib.ID, Title: "Scraped Episode One", Path: "/media/shows/Bound Show/Season 1/S01E01.1080.mkv", SeasonNum: 1, EpisodeNum: 1, ScrapeStatus: "matched", Width: 1920, PosterURL: "https://example.com/first.jpg", Overview: "First episode"},
				{Base: model.Base{ID: "pending-version"}, LibraryID: lib.ID, Title: "Different Release Title", Path: "/media/shows/Bound Show/Season 1/S01E01.2160.mkv", SeasonNum: 1, EpisodeNum: 1, ScrapeStatus: "pending", Width: 3840, BackdropURL: "https://example.com/second.jpg", Overview: "Different episode metadata"},
				{Base: model.Base{ID: "pending-next"}, LibraryID: lib.ID, Title: "Next Episode", Path: "/media/shows/Bound Show/Season 1/S01E02.mkv", SeasonNum: 1, EpisodeNum: 2, ScrapeStatus: "pending"},
				{Base: model.Base{ID: "pending-range"}, LibraryID: lib.ID, Title: "Combined Episodes", Path: "/media/shows/Bound Show/Season 1/S01E01-E02.mkv", SeasonNum: 1, EpisodeNum: 1, EpisodeEndNum: 2, ScrapeStatus: "pending"},
			}
			switch provider {
			case "tmdb":
				rows[0].TMDbID = 920
			case "bangumi":
				rows[0].BangumiID = 920
			case "douban":
				rows[0].DoubanID = "920"
			case "thetvdb":
				rows[0].TheTVDBID = "920"
			case "series":
				rows[0].SeriesID = "canonical-series"
				series := model.Series{Base: model.Base{ID: rows[0].SeriesID}, LibraryID: lib.ID, Title: "Bound Show"}
				if err := e.repo.DB.Create(&series).Error; err != nil {
					t.Fatal(err)
				}
			}
			if err := e.repo.DB.Create(&rows).Error; err != nil {
				t.Fatal(err)
			}
			if err := e.repo.Media.RefreshSeriesBindings(t.Context(), e.repo.DB, []string{rows[0].ID}); err != nil {
				t.Fatal(err)
			}
			page, err := e.Items(t.Context(), ItemsParams{ParentID: lib.ID, IncludeItemTypes: []string{"Series"}, Limit: 50})
			if err != nil {
				t.Fatal(err)
			}
			cards := page["Items"].([]map[string]any)
			if len(cards) != 1 || cards[0]["RecursiveItemCount"] != 3 || cards[0]["ChildCount"] != 1 {
				t.Fatalf("mixed metadata/versions split the series or inflated counts: %#v", page)
			}
			showID := cards[0]["Id"].(string)
			episodes, err := e.Items(t.Context(), ItemsParams{ShowID: showID, ParentID: seasonID(showID, 1), IncludeItemTypes: []string{"Episode"}, Limit: 50})
			if err != nil {
				t.Fatal(err)
			}
			items := episodes["Items"].([]map[string]any)
			if len(items) != 3 || episodes["TotalRecordCount"] != 3 {
				t.Fatalf("mixed versions did not collapse into logical episodes: %#v", episodes)
			}
			for _, item := range items {
				wantSources := 1
				if item["Id"] == "matched" || item["Id"] == "pending-version" {
					wantSources = 2
				}
				if item["SeriesId"] != showID || len(item["MediaSources"].([]map[string]any)) != wantSources {
					t.Fatalf("batched episode/version payload disagrees with series identity: %#v", item)
				}
			}
			for _, id := range []string{"matched", "pending-version"} {
				item, err := e.Item(t.Context(), id, "")
				if err != nil || item["SeriesId"] != showID || len(item["MediaSources"].([]map[string]any)) != 2 {
					t.Fatalf("direct version lookup disagrees with the list: %#v, %v", item, err)
				}
			}
			counts, err := e.mediaCountsForParent(t.Context(), "", lib.ID)
			if err != nil || counts.SeriesCount != 1 {
				t.Fatalf("library counts duplicated inherited/explicit series: %#v, %v", counts, err)
			}
			var pending model.Media
			if err := e.repo.DB.First(&pending, "id = ?", "pending-version").Error; err != nil {
				t.Fatal(err)
			}
			if pending.ScrapeStatus != "pending" || pending.TMDbID != 0 || pending.BangumiID != 0 || pending.DoubanID != "" || pending.TheTVDBID != "" || pending.SeriesID != "" {
				t.Fatalf("binding fabricated scraper metadata: %#v", pending)
			}
			pending.Path = "/media/shows/Another Show/S01E01.mkv"
			if err := e.repo.DB.Model(&pending).Update("path", pending.Path).Error; err != nil {
				t.Fatal(err)
			}
			var matched model.Media
			if err := e.repo.DB.First(&matched, "id = ?", "matched").Error; err != nil {
				t.Fatal(err)
			}
			for _, row := range []model.Media{matched, pending} {
				if sources := e.mediaVersionSiblings(t.Context(), &row); len(sources) != 1 || sources[0].ID != row.ID {
					t.Fatalf("stale binding after move leaked a version for %s: %v", row.ID, fmt.Sprint(sources))
				}
			}
		})
	}
}
