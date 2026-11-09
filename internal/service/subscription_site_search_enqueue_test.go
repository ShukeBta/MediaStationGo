package service

import (
	"fmt"
	"net/http"
	"net/http/httptest"
	"sync/atomic"
	"testing"

	"go.uber.org/zap"

	"github.com/ShukeBta/MediaStationGo/internal/model"
	"github.com/ShukeBta/MediaStationGo/internal/repository"
)

func TestEnqueueSiteSearchCandidatesHonorsMaxImportsPerRun(t *testing.T) {
	for _, test := range []struct {
		name      string
		limit     int
		failFirst bool
		want      int
	}{
		{name: "one_pack", limit: 1, want: 1},
		{name: "explicit_limit", limit: 3, want: 3},
		{name: "zero_defaults_to_two", want: 2},
		{name: "negative_defaults_to_two", limit: -1, want: 2},
		{name: "fewer_candidates", limit: 10, want: 4},
		{name: "failed_candidate_does_not_consume_limit", limit: 1, failFirst: true, want: 1},
	} {
		t.Run(test.name, func(t *testing.T) {
			var addCalls atomic.Int32
			qb := httptest.NewServer(http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
				switch request.URL.Path {
				case "/api/v2/auth/login":
					_, _ = writer.Write([]byte("Ok."))
				case "/api/v2/torrents/info":
					_, _ = writer.Write([]byte(`[]`))
				case "/api/v2/torrents/add":
					addCalls.Add(1)
					_, _ = writer.Write([]byte("Ok."))
				default:
					http.NotFound(writer, request)
				}
			}))
			defer qb.Close()
			repos := repository.New(newServiceTestDB(t, &model.DownloadTask{}, &model.DownloadClient{}, &model.Setting{}, &model.Media{}, &model.Site{}, &model.SubscriptionRejectedRelease{}))
			configureTestDefaultQB(t, repos, qb.URL)
			downloads := NewDownloadService(zap.NewNop(), repos, NewHub(zap.NewNop()), nil)
			svc := NewSubscriptionService(nil, zap.NewNop(), repos, downloads, NewSiteService(zap.NewNop(), repos, ""), nil)
			sub := &model.Subscription{
				Base: model.Base{ID: "sub-show"}, UserID: "u1", Name: "Some Show", Filter: "Some Show",
				MediaType: "tv", SavePath: t.TempDir(), MaxImportsPerRun: test.limit,
			}
			state := &siteSearchRunState{
				SeenSet: map[string]struct{}{},
				Availability: LocalAvailability{
					ExistingEpisodeKeys: map[string]struct{}{}, MissingEpisodeKeys: map[string]struct{}{},
				},
			}
			candidates := make([]siteSearchCandidate, 0, 4)
			for _, episode := range []int{1, 3, 4, 5} {
				title := fmt.Sprintf("Some Show S01E%02d 1080p", episode)
				if episode == 1 {
					title = "Some Show S01E01-E02 1080p"
				}
				candidates = append(candidates, siteSearchCandidate{
					Item: SearchResult{Title: title}, Download: fmt.Sprintf("magnet:?xt=urn:btih:%040d", episode),
					GUID: fmt.Sprintf("candidate-%d", episode), Season: 1, Episode: episode, Pack: episode == 1,
				})
			}
			if test.failFirst {
				candidates[0].Download = ""
			}
			result := svc.enqueueSiteSearchCandidates(t.Context(), sub, candidates, state)
			if result.Queued != test.want || len(result.Resources) != test.want || int(addCalls.Load()) != test.want {
				t.Fatalf("result = %+v, add calls = %d, want %d queued", result, addCalls.Load(), test.want)
			}
			if (result.LastEnqueueErr != nil) != test.failFirst {
				t.Fatalf("last error = %v, want failure = %v", result.LastEnqueueErr, test.failFirst)
			}
			if len(state.SeenSet) != test.want {
				t.Fatalf("seen = %+v, want %d queued candidates only", state.SeenSet, test.want)
			}
			firstUnseen := test.want
			if test.failFirst {
				if _, seen := state.SeenSet[candidates[0].GUID]; seen {
					t.Fatalf("failed candidate was marked seen: %s", candidates[0].GUID)
				}
				if _, seen := state.SeenSet[candidates[1].GUID]; !seen {
					t.Fatalf("candidate after failed attempt was not queued: %s", candidates[1].GUID)
				}
				firstUnseen = 2
			}
			for index := firstUnseen; index < len(candidates); index++ {
				if _, seen := state.SeenSet[candidates[index].GUID]; seen {
					t.Fatalf("candidate beyond limit was marked seen: %s", candidates[index].GUID)
				}
			}
		})
	}
}

func TestEnqueueSiteSearchDedupMarksEnglishRangeAvailableForChineseSubscription(t *testing.T) {
	db := newServiceTestDB(t, &model.DownloadTask{}, &model.Setting{})
	repos := repository.New(db)
	if err := repos.Download.Create(t.Context(), &model.DownloadTask{
		UserID:   "u1",
		Source:   "qbittorrent",
		URL:      "https://pt.example/download?id=old",
		Title:    "Archives The Nanyang Mystery 2026 S01E07-S01E08 2160p WEB-DL",
		SavePath: "/downloads/国产剧",
		Status:   "queued",
		Progress: 0.1,
	}); err != nil {
		t.Fatal(err)
	}

	site := NewSiteService(zap.NewNop(), repos, "")
	downloads := NewDownloadService(zap.NewNop(), repos, NewHub(zap.NewNop()), nil)
	svc := NewSubscriptionService(nil, zap.NewNop(), repos, downloads, site, NewHub(zap.NewNop()))
	sub := &model.Subscription{
		Base:          model.Base{ID: "sub-nanyang"},
		UserID:        "u1",
		Name:          "南部档案 自动订阅",
		Filter:        "南部档案 2026",
		MediaType:     "tv",
		MediaCategory: "国产剧",
		SavePath:      "/downloads",
		TotalEpisodes: 33,
	}
	state := &siteSearchRunState{
		Keyword: "南部档案 2026",
		SeenSet: map[string]struct{}{},
		Availability: LocalAvailability{
			TotalEpisodes:       33,
			ExistingEpisodeKeys: map[string]struct{}{},
			MissingEpisodeKeys:  map[string]struct{}{},
		},
	}
	candidate := siteSearchCandidate{
		Item: SearchResult{
			Title:       "Archives The Nanyang Mystery 2026 S01E07-S01E08 2160p WEB-DL",
			DownloadURL: "https://pt.example/download?id=new",
		},
		Download: "https://pt.example/download?id=new",
		GUID:     "site|m-team|nanyang-7-8",
		Season:   1,
		Episode:  7,
		Episodes: []int{7, 8},
		Pack:     true,
	}

	title, err := svc.enqueueSiteSearchCandidate(t.Context(), sub, candidate, state)
	if err != nil {
		t.Fatalf("enqueueSiteSearchCandidate returned %v, want dedup skip without error", err)
	}
	if title != "" {
		t.Fatalf("title = %q, want empty because candidate was deduped", title)
	}
	for _, episode := range []int{7, 8} {
		if _, ok := state.Availability.ExistingEpisodeKeys[episodeKey(1, episode)]; !ok {
			t.Fatalf("availability missing E%d after dedup range: %#v", episode, state.Availability.ExistingEpisodeKeys)
		}
	}
	if _, ok := state.SeenSet[candidate.GUID]; ok {
		t.Fatalf("deduped candidate should not be marked seen before the next availability scan")
	}
}

func TestEnqueueSiteSearchDedupLinksEnglishHistoryTaskToChineseSubscription(t *testing.T) {
	db := newServiceTestDB(t, &model.DownloadTask{}, &model.Setting{})
	repos := repository.New(db)
	if err := repos.Download.Create(t.Context(), &model.DownloadTask{
		UserID:   "u1",
		Source:   "qbittorrent",
		URL:      "https://pt.example/download?id=history",
		Title:    "The Heir 2026 S01E33-E34 2160p WEB-DL",
		SavePath: "/downloads/国产剧",
		Status:   "queued",
		Progress: 0.2,
	}); err != nil {
		t.Fatal(err)
	}

	site := NewSiteService(zap.NewNop(), repos, "")
	downloads := NewDownloadService(zap.NewNop(), repos, NewHub(zap.NewNop()), nil)
	svc := NewSubscriptionService(nil, zap.NewNop(), repos, downloads, site, NewHub(zap.NewNop()))
	sub := &model.Subscription{
		Base:          model.Base{ID: "sub-heir"},
		UserID:        "u1",
		Name:          "家业 自动订阅",
		Filter:        "家业 2026",
		MediaType:     "tv",
		MediaCategory: "国产剧",
		SavePath:      "/downloads",
		TotalEpisodes: 42,
	}
	state := &siteSearchRunState{
		Keyword: "家业 2026",
		SeenSet: map[string]struct{}{},
		Availability: LocalAvailability{
			TotalEpisodes:       42,
			ExistingEpisodeKeys: map[string]struct{}{},
			MissingEpisodeKeys:  map[string]struct{}{},
		},
	}
	candidate := siteSearchCandidate{
		Item: SearchResult{
			Title:       "The Heir 2026 S01E33-E34 2160p WEB-DL",
			DownloadURL: "https://pt.example/download?id=new",
		},
		Download: "https://pt.example/download?id=new",
		GUID:     "site|m-team|heir-33-34",
		Season:   1,
		Episode:  33,
		Episodes: []int{33, 34},
		Pack:     true,
	}

	title, err := svc.enqueueSiteSearchCandidate(t.Context(), sub, candidate, state)
	if err != nil {
		t.Fatalf("enqueueSiteSearchCandidate returned %v, want dedup skip without error", err)
	}
	if title != "" {
		t.Fatalf("title = %q, want empty because candidate was deduped", title)
	}
	var rows []model.DownloadTask
	if err := db.Find(&rows).Error; err != nil {
		t.Fatal(err)
	}
	if len(rows) != 1 {
		t.Fatalf("download rows = %d, want 1", len(rows))
	}
	if rows[0].SubscriptionID != sub.ID {
		t.Fatalf("history task subscription_id = %q, want %q", rows[0].SubscriptionID, sub.ID)
	}
	if rows[0].MediaType != "tv" || rows[0].MediaCategory != "国产剧" {
		t.Fatalf("history task metadata = (%q, %q), want (tv, 国产剧)", rows[0].MediaType, rows[0].MediaCategory)
	}

	availability := svc.pendingDownloadAvailability(t.Context(), sub)
	for _, episode := range []int{33, 34} {
		if _, ok := availability.ExistingEpisodeKeys[episodeKey(1, episode)]; !ok {
			t.Fatalf("linked history task missing E%d availability: %#v", episode, availability.ExistingEpisodeKeys)
		}
	}
}
