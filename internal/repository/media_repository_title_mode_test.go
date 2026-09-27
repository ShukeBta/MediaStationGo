package repository

import (
	"testing"

	"github.com/ShukeBta/MediaStationGo/internal/model"
)

func TestMediaUpsertUpdatesPreservedFilenameAndClearsEpisodeIdentity(t *testing.T) {
	existing := model.Media{
		Title:        "cleaned title",
		ScrapeStatus: "no_match",
		SeasonNum:    1,
		EpisodeNum:   2,
		SeriesID:     "series:test",
		EpisodeTitle: "第二集",
	}
	incoming := model.Media{
		Title:               "Original.Release.Name [1080P]",
		ScrapeStatus:        "pending",
		PreserveSourceTitle: true,
	}

	updates := mediaUpsertUpdates(existing, incoming)
	if updates["title"] != incoming.Title {
		t.Fatalf("title update = %#v", updates["title"])
	}
	for key, want := range map[string]any{
		"season_num":    0,
		"episode_num":   0,
		"series_id":     "",
		"episode_title": "",
	} {
		if got, ok := updates[key]; !ok || got != want {
			t.Fatalf("%s update = %#v, want %#v", key, got, want)
		}
	}
	if _, ok := updates["scrape_status"]; ok {
		t.Fatalf("preserving a filename must not restart scraping: %#v", updates)
	}
}

func TestMediaUpsertKeepsLLMCleanedTitleInFilenameMode(t *testing.T) {
	existing := model.Media{Title: "清洗后的标题", ScrapeStatus: "title_cleaned"}
	incoming := model.Media{
		Title:               "raw.filename",
		ScrapeStatus:        "pending",
		PreserveSourceTitle: true,
	}
	updates := mediaUpsertUpdates(existing, incoming)
	if _, ok := updates["title"]; ok {
		t.Fatalf("LLM cleaned title was overwritten: %#v", updates)
	}
}
