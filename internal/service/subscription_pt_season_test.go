package service

import (
	"fmt"
	"testing"

	"github.com/ShukeBta/MediaStationGo/internal/model"
	"github.com/ShukeBta/MediaStationGo/internal/repository"
)

func TestPTSubscriptionBareEpisodesUseSelectedSeason(t *testing.T) {
	sub := &model.Subscription{FeedURL: SiteSearchURL("Show", "", "", false), Filter: "Show", MediaType: "tv", SeasonNumber: 2}
	for _, title := range []string{"Show 第1集 1080p", "Show 01 1080p", "Show01", "Show EP01 1080p", "Show 第1-2集 1080p"} {
		t.Run(title, func(t *testing.T) {
			selected := selectSiteSearchCandidates([]SearchResult{{Title: title, DownloadURL: "https://tracker/download"}}, sub, nil)
			if len(selected) != 1 || selected[0].Season != 2 || selected[0].Episode != 1 {
				t.Fatalf("selected = %#v; want season 2 episode 1", selected)
			}
			var availability LocalAvailability
			addSiteSearchCandidateAvailability(selected[0], &availability)
			if _, ok := availability.ExistingEpisodeKeys[episodeKey(2, 1)]; !ok {
				t.Fatalf("selected season was lost when marking availability: %#v", availability)
			}
			if _, ok := availability.ExistingEpisodeKeys[episodeKey(1, 1)]; ok {
				t.Fatalf("inferred season 1 leaked into availability: %#v", availability)
			}
		})
	}
	for _, title := range []string{"Show S01E01 1080p", "Show 第一季第1集", "Show 1x01", "Show S01 Complete"} {
		if selected := selectSiteSearchCandidates([]SearchResult{{Title: title, DownloadURL: "https://tracker/download"}}, sub, nil); len(selected) != 0 {
			t.Fatalf("explicit other-season result selected: %s: %#v", title, selected)
		}
	}
}

func TestPTSubscriptionSeasonPackAvailability(t *testing.T) {
	for _, source := range []string{"library", "pending"} {
		for _, tc := range []struct {
			title string
			want  bool
		}{
			{"Show S01 Complete", false},
			{"Show S02 Complete", true},
			{"Show Complete", false},
		} {
			t.Run(source+"/"+tc.title, func(t *testing.T) {
				db := newServiceTestDB(t, &model.Media{}, &model.DownloadTask{}, &model.Setting{})
				repos := repository.New(db)
				sub := &model.Subscription{Base: model.Base{ID: "sub-season2"}, Name: "Show", Filter: "Show", FeedURL: SiteSearchURL("Show", "", "", false), MediaType: "tv", SeasonNumber: 2}
				if source == "library" {
					if err := db.Create(&model.Media{Title: tc.title, Path: "/library/" + tc.title + ".mkv"}).Error; err != nil {
						t.Fatal(err)
					}
				} else if err := repos.Download.Create(t.Context(), &model.DownloadTask{SubscriptionID: sub.ID, Title: tc.title, URL: "https://tracker/download", Status: "downloading"}); err != nil {
					t.Fatal(err)
				}
				svc := NewSubscriptionService(nil, nil, repos, nil, nil, nil)
				availability := mergeLocalAvailability(SubscriptionLocalAvailability(t.Context(), repos, sub), svc.pendingDownloadAvailability(t.Context(), sub))
				availability = svc.finalizePendingAvailability(sub, availability)
				if availability.HasSeriesPack != tc.want || subscriptionShouldArchive(sub, availability) != tc.want {
					t.Fatalf("pack completion = %#v, want %v", availability, tc.want)
				}
				if !tc.want && (availability.LocalMediaCount != 0 || availability.InLibrary) {
					t.Fatalf("other-season pack polluted progress: %#v", availability)
				}
			})
		}
	}
}

func TestPTSubscriptionEnrichedProgressKeepsSelectedSeason(t *testing.T) {
	db := newServiceTestDB(t, &model.Media{}, &model.DownloadTask{}, &model.Setting{})
	repos := repository.New(db)
	for season := 1; season <= 2; season++ {
		if err := db.Create(&model.Media{Title: "Show", Path: fmt.Sprintf("/library/Show.S%02dE01.mkv", season), SeasonNum: season, EpisodeNum: 1}).Error; err != nil {
			t.Fatal(err)
		}
	}
	sub := model.Subscription{Base: model.Base{ID: "sub-season2"}, Name: "Show", Filter: "Show", FeedURL: SiteSearchURL("Show", "", "", false), MediaType: "tv", SeasonNumber: 2}
	for _, title := range []string{"Show S01E02", "Show 02"} {
		if err := repos.Download.Create(t.Context(), &model.DownloadTask{SubscriptionID: sub.ID, Title: title, URL: "https://tracker/" + title, Status: "downloading"}); err != nil {
			t.Fatal(err)
		}
	}
	svc := NewSubscriptionService(nil, nil, repos, nil, nil, nil)
	for _, management := range []bool{false, true} {
		for _, total := range []int{0, 3} {
			items := []model.Subscription{sub}
			items[0].TotalEpisodes = total
			var err error
			if management {
				err = svc.EnrichManagementProgress(t.Context(), items)
			} else {
				err = svc.EnrichProgress(t.Context(), items)
			}
			if err != nil {
				t.Fatal(err)
			}
			got := items[0]
			if got.DownloadedEpisodes != 2 || got.LocalMediaCount != 2 || got.TotalEpisodes != total || !got.InLibrary {
				t.Fatalf("management=%v total=%d: progress = %#v", management, total, got)
			}
			if total == 0 && len(got.MissingEpisodes) != 0 || total == 3 && (len(got.MissingEpisodes) != 1 || got.MissingEpisodes[0] != 3) {
				t.Fatalf("management=%v total=%d: missing = %v", management, total, got.MissingEpisodes)
			}
		}
	}
}
