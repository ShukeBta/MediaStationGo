package service

import (
	"strings"
	"testing"

	"github.com/ShukeBta/MediaStationGo/internal/model"
)

func TestScrapeQueryCandidatesPreferSeriesFolderAndCJKTitle(t *testing.T) {
	lib := &model.Library{
		Path: `F:\downloads\国产剧`,
		Type: "movie",
	}
	media := &model.Media{
		Title:      "亏成首富从游戏开始 the ri est in game",
		Path:       `F:\downloads\国产剧\亏成首富从游戏开始 The Richest in Game\Season 01\亏成首富从游戏开始 The Richest in Game - S01E11 - 4K.mp4`,
		SeasonNum:  1,
		EpisodeNum: 11,
	}

	got := scrapeQueryCandidates(media, lib)
	if len(got) == 0 {
		t.Fatal("scrapeQueryCandidates returned no candidates")
	}
	if got[0] != "亏成首富从游戏开始" {
		t.Fatalf("first query candidate = %q, want Chinese series title", got[0])
	}
	for _, candidate := range got {
		if strings.Contains(candidate, "ri est") {
			t.Fatalf("query candidate kept substring-stripped title: %#v", got)
		}
	}
}

func TestScrapeQueryCandidatesUseCloudSeriesFolder(t *testing.T) {
	lib := &model.Library{
		Path: "cloud://openlist/国产剧",
		Type: "movie",
	}
	media := &model.Media{
		Title:      "折腰 S01E01",
		Path:       "cloud://openlist/国产剧/折腰 (2025)/Season 1/折腰.S01E01.mkv",
		SeasonNum:  1,
		EpisodeNum: 1,
	}

	got := scrapeQueryCandidates(media, lib)
	if len(got) == 0 {
		t.Fatal("scrapeQueryCandidates returned no candidates")
	}
	if got[0] != "折腰" {
		t.Fatalf("first query candidate = %q, want cloud series folder title; all candidates=%#v", got[0], got)
	}
}

func TestScrapeQueryCandidatesUseSeriesLibraryRootWhenMountedAtShowFolder(t *testing.T) {
	lib := &model.Library{
		Path: `/downloads/国产剧/折腰 (2025)`,
		Type: "tv",
	}
	media := &model.Media{
		Title:      "第 1 集",
		Path:       `/downloads/国产剧/折腰 (2025)/Season 01/第01集.mkv`,
		SeasonNum:  1,
		EpisodeNum: 1,
	}

	got := scrapeQueryCandidates(media, lib)
	if len(got) == 0 {
		t.Fatal("scrapeQueryCandidates returned no candidates")
	}
	if got[0] != "折腰" {
		t.Fatalf("first query candidate = %q, want library root show title; all candidates=%#v", got[0], got)
	}
}

func TestScrapeQueryCandidatesSkipEpisodeOnlyTitles(t *testing.T) {
	lib := &model.Library{
		Path: `F:\media\电视剧\欧美剧`,
		Type: "tv",
	}
	media := &model.Media{
		Title:      "第1期上：最狠开局！五哈团命悬一线好刺激",
		Path:       `F:\media\电视剧\欧美剧\第1期上：最狠开局！五哈团命悬一线好刺激 (2026)\Season 6\第1期上：最狠开局！五哈团命悬一线好刺激 - S06E01 - 第 1 集.mkv`,
		SeasonNum:  6,
		EpisodeNum: 1,
	}

	got := scrapeQueryCandidates(media, lib)
	for _, candidate := range got {
		if unsafeAutomaticEpisodeQuery(candidate) {
			t.Fatalf("query candidates kept unsafe episode title %q: %#v", candidate, got)
		}
	}
}

func TestSeriesTitleFromMediaPathIgnoresEpisodeOnlyFolder(t *testing.T) {
	got := seriesTitleFromMediaPath(`F:\media\电视剧\欧美剧\第 11 集 (2026)\Season 6\第 11 集 - S06E11.mkv`)
	if got != "" {
		t.Fatalf("series title from episode-only folder = %q, want empty", got)
	}
}

func TestMediaIsEpisodicUsesEpisodePatternInPath(t *testing.T) {
	lib := &model.Library{
		Path: `/media/movies`,
		Type: "movie",
	}
	media := &model.Media{
		Title: "折腰 S01E01",
		Path:  `/media/movies/折腰/Season 01/折腰.S01E01.mkv`,
	}

	if !mediaIsEpisodic(media, lib) {
		t.Fatal("media with an SxxEyy path should be treated as episodic even in a movie library")
	}
}

func TestScrapeQueryCandidatesSkipCategoryFolderAsSeriesTitle(t *testing.T) {
	lib := &model.Library{
		Path: `/downloads`,
		Type: "tv",
	}
	media := &model.Media{
		Title:      "Ashes To Crown",
		Path:       `/downloads/国产剧/Ashes.to.Crown.S01E06.1080p.WEB-DL.mkv`,
		SeasonNum:  1,
		EpisodeNum: 6,
	}

	got := scrapeQueryCandidates(media, lib)
	if len(got) == 0 {
		t.Fatal("scrapeQueryCandidates returned no candidates")
	}
	if got[0] == "国产剧" {
		t.Fatalf("first query candidate = %q, category folders must not be used as title candidates: %#v", got[0], got)
	}
	if !strings.EqualFold(got[0], "Ashes To Crown") {
		t.Fatalf("first query candidate = %q, want release title; all candidates=%#v", got[0], got)
	}
}
