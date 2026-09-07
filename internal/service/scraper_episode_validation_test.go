package service

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/ShukeBta/MediaStationGo/internal/config"
	"github.com/ShukeBta/MediaStationGo/internal/model"
	"go.uber.org/zap"
)

func TestEpisodePathTitleTrust(t *testing.T) {
	for _, tc := range []struct {
		path, title, original, keyword string
		want                           bool
	}{
		{"/tv/NCIS/Season 1/NCIS.S01E01.mkv", "NCIS: Origins", "NCIS: Origins", "NCIS", false},
		{"/tv/Fringe/Season 1/Fringe.S01E01.mkv", "The Outer Limits", "The Outer Limits", "Fringe", false},
		{"/tv/小鬼当家/Season 1/小鬼当家.S01E01.mkv", "滑头鬼之孙", "Nura", "小鬼当家", false},
		{"/tv/NCIS/Season 1/NCIS.S01E01.mkv", "海军罪案调查处", "NCIS", "", true},
		{"/tv/NCIS.S01E01.mkv", "NCIS", "", "", true},
	} {
		t.Run(tc.title+tc.path, func(t *testing.T) {
			if got := episodePathTitleTrusted(tc.path, &Match{Title: tc.title, OriginalName: tc.original, SearchKeyword: tc.keyword}); got != tc.want {
				t.Fatalf("trusted=%v, want %v", got, tc.want)
			}
		})
	}
}

func TestEpisodeValidationManualOverrideAndFailures(t *testing.T) {
	for _, tc := range []struct {
		name, path, body string
		auto             bool
		status           int
		wantError        bool
	}{
		{"automatic rejects wrong show", "/tv/NCIS/Season 2/NCIS.S02E03.mkv", `{"episodes":[{"episode_number":3,"name":"Third"}]}`, true, 200, true},
		{"manual accepts chosen show", "/tv/NCIS/Season 2/NCIS.S02E03.mkv", `{"episodes":[{"episode_number":3,"name":"Third"}]}`, false, 200, false},
		{"manual rejects path conflict", "/tv/NCIS/Season 1/NCIS.S02E03.mkv", `{"episodes":[{"episode_number":3,"name":"Third"}]}`, false, 200, true},
		{"missing episode", "/tv/NCIS/Season 2/NCIS.S02E03.mkv", `{"episodes":[]}`, false, 200, true},
		{"provider failure", "/tv/NCIS/Season 2/NCIS.S02E03.mkv", `{}`, false, 503, true},
	} {
		t.Run(tc.name, func(t *testing.T) {
			requests := 0
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				requests++
				if r.URL.Path != "/tv/1/season/2" {
					t.Errorf("unexpected path %s", r.URL.Path)
				}
				w.WriteHeader(tc.status)
				_, _ = w.Write([]byte(tc.body))
			}))
			defer server.Close()
			cfg := &config.Config{}
			cfg.Secrets.TMDbAPIKey = "test"
			cfg.Secrets.TMDbAPIProxy = server.URL
			s := &ScraperService{tmdb: NewTMDbProvider(cfg, zap.NewNop(), nil)}
			media := &model.Media{Path: tc.path, SeasonNum: 9, EpisodeNum: 9, ScrapeStatus: "pending"}
			match := &Match{TMDbID: 1, MediaType: "tv", Title: "NCIS: Origins"}
			options := ScrapeOptions{automaticSelection: tc.auto, episodeValidation: make(map[[2]int]map[int]*TMDbEpisodeDetails)}
			got, err := s.validateEpisodeMatch(t.Context(), media, nil, match, options)
			if (err != nil) != tc.wantError {
				t.Fatalf("got=%+v err=%v", got, err)
			}
			if media.SeasonNum != 9 || media.EpisodeNum != 9 || media.ScrapeStatus != "pending" {
				t.Fatal("validation mutated input")
			}
			if !tc.wantError {
				if got.SeasonNum != 2 || got.EpisodeNum != 3 {
					t.Fatalf("wrong mapping %+v", got)
				}
				_, err = s.validateEpisodeMatch(t.Context(), media, nil, match, options)
				if err != nil || requests != 1 {
					t.Fatalf("season cache requests=%d err=%v", requests, err)
				}
			} else {
				// A nil repository would panic if a rejected match reached a write.
				if err := s.applyProviderMatchWithOptions(t.Context(), media, nil, match, options); err == nil {
					t.Fatal("rejected match reached persistence")
				}
			}
		})
	}
}

func TestEpisodePathSeasonEvidence(t *testing.T) {
	for _, path := range []string{`C:\tv\NCIS\Season 2\NCIS.S02E03.mkv`, "/tv/NCIS/第二季/第3集.mkv", "/tv/NCIS/Specials/NCIS.S00E03.mkv"} {
		m := &model.Media{Path: path, SeasonNum: 9, EpisodeNum: 9}
		if err := episodeIdentityFromPath(m); err != nil {
			t.Fatal(err)
		}
		season := 2
		if strings.Contains(path, "Specials") {
			season = 0
		}
		if m.SeasonNum != season || m.EpisodeNum != 3 {
			t.Fatalf("bad mapping %+v", m)
		}
	}
}

func TestManualEpisodeOverrideStillRequiresTMDbEpisode(t *testing.T) {
	s, repos, closeServer := newTestScraper(t)
	defer closeServer()
	lib := model.Library{Name: "TV", Path: "cloud://openlist/tv", Type: "tv"}
	if err := repos.DB.Create(&lib).Error; err != nil {
		t.Fatal(err)
	}
	media := model.Media{LibraryID: lib.ID, Path: "cloud://openlist/tv/Other/Season 1/Other.S02E03.mkv", Title: "Other", SeasonNum: 9, EpisodeNum: 9}
	if err := repos.DB.Create(&media).Error; err != nil {
		t.Fatal(err)
	}
	season, episode := 2, 1
	req := ManualScrapeRequest{Source: "tmdb", MediaType: "tv", TMDbID: 12345, Title: "间谍过家家", SeasonNum: &season, EpisodeNum: &episode}
	got, err := s.ApplyManualMatch(t.Context(), media.ID, req)
	if err != nil {
		t.Fatal(err)
	}
	if got.SeasonNum != 2 || got.EpisodeNum != 1 || got.ScrapeStatus != "matched" {
		t.Fatalf("wrong manual mapping %+v", got)
	}
	episode = 99
	if _, err := s.ApplyManualMatch(t.Context(), media.ID, req); err == nil {
		t.Fatal("nonexistent manual episode accepted")
	}
	stored, err := repos.Media.FindByID(t.Context(), media.ID)
	if err != nil || stored.EpisodeNum != 1 {
		t.Fatalf("failed override changed data: %+v %v", stored, err)
	}
	if _, err := s.ApplyManualMatchBatchWithOptions(t.Context(), []string{media.ID}, req, ScrapeOptions{}); err == nil {
		t.Fatal("batch override accepted")
	}
}
