package service

import (
	"testing"

	"go.uber.org/zap"

	"github.com/ShukeBta/MediaStationGo/internal/model"
)

func TestMediaExternalIDMatchTrustsExactEpisodeID(t *testing.T) {
	scraper := &ScraperService{log: zap.NewNop()}
	media := &model.Media{
		Title:      "电锯人",
		Path:       "/media/anime/电锯人/Season 01/电锯人 - S01E01.mkv",
		TMDbID:     114410,
		SeasonNum:  1,
		EpisodeNum: 1,
	}
	lib := &model.Library{Type: "anime", Path: "/media/anime"}
	match := &Match{
		Title:        "チェンソーマン",
		OriginalName: "チェンソーマン",
		TMDbID:       114410,
	}

	if !scraper.mediaExternalIDMatchTrusted(media, lib, match, "tmdb") {
		t.Fatal("exact external id match should be trusted for episodic media")
	}
}

func TestPreferExistingLocalizedMediaTitleKeepsCleanChineseTitle(t *testing.T) {
	media := &model.Media{Title: "电锯人", SeasonNum: 1, EpisodeNum: 1}
	lib := &model.Library{Type: "anime"}
	match := &Match{Title: "チェンソーマン"}

	preferExistingLocalizedEpisodeTitle(media, lib, match)

	if match.Title != "电锯人" || match.OriginalName != "チェンソーマン" {
		t.Fatalf("match title=%q original=%q, want localized title with original preserved", match.Title, match.OriginalName)
	}
}

func TestPreferExistingLocalizedMediaTitleIgnoresReleaseTitle(t *testing.T) {
	media := &model.Media{Title: "电锯人.S01E01.1080p.WEB-DL", SeasonNum: 1, EpisodeNum: 1}
	lib := &model.Library{Type: "anime"}
	match := &Match{Title: "チェンソーマン"}

	preferExistingLocalizedEpisodeTitle(media, lib, match)

	if match.Title != "チェンソーマン" {
		t.Fatalf("release-like media title should not replace provider title, got %q", match.Title)
	}
}
