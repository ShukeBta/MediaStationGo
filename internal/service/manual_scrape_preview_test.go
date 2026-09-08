package service

import (
	"github.com/ShukeBta/MediaStationGo/internal/model"
	"strings"
	"testing"
)

func TestPreviewRangeApplyAndStaleRevision(t *testing.T) {
	s, repos, closeServer := newTestScraper(t)
	defer closeServer()
	lib := model.Library{Name: "TV", Type: "tv", Path: "cloud://openlist/tv"}
	if err := repos.DB.Create(&lib).Error; err != nil {
		t.Fatal(err)
	}
	m := model.Media{LibraryID: lib.ID, Title: "Before", Path: "cloud://openlist/tv/Show.S01E01E02.mkv", ScrapeStatus: "pending"}
	if err := repos.DB.Create(&m).Error; err != nil {
		t.Fatal(err)
	}
	season, episode, end := 1, 1, 2
	req := ManualScrapeRequest{Source: "tmdb", MediaType: "tv", TMDbID: 12345, SeasonNum: &season, EpisodeNum: &episode, EpisodeEndNum: &end}
	rows, err := s.PreviewManualMatch(t.Context(), []string{m.ID}, req, false)
	if err != nil || len(rows) != 1 || !rows[0].Valid || rows[0].End != 2 {
		t.Fatalf("preview %+v %v", rows, err)
	}
	unchanged, err := repos.Media.FindByID(t.Context(), m.ID)
	if err != nil || unchanged.Title != "Before" || unchanged.ScrapeStatus != "pending" {
		t.Fatal("preview wrote data")
	}
	req.ExpectedRevisions = map[string]string{m.ID: rows[0].Revision}
	got, err := s.ApplyManualMatch(t.Context(), m.ID, req)
	if err != nil || got.EpisodeEndNum != 2 || !strings.Contains(got.EpisodeTitle, "第二集") || got.DurationSec != 0 {
		t.Fatalf("range %+v %v", got, err)
	}
	if _, err = s.ApplyManualMatch(t.Context(), m.ID, req); err == nil {
		t.Fatal("stale preview accepted")
	}
	req.ExpectedRevisions = nil
	end = 3
	if _, err = s.ApplyManualMatch(t.Context(), m.ID, req); err == nil {
		t.Fatal("missing range member accepted")
	}
	stored, _ := repos.Media.FindByID(t.Context(), m.ID)
	if stored.EpisodeEndNum != 2 {
		t.Fatal("failed range changed data")
	}
}

func TestEpisodeCoverageKeepsVersionsAndSegmentsDistinct(t *testing.T) {
	base := model.Media{TMDbID: 1, SeasonNum: 1, EpisodeNum: 1}
	rangeFile := base
	rangeFile.EpisodeEndNum = 2
	part1 := base
	part1.EpisodePartNum = 1
	part2 := base
	part2.EpisodePartNum = 2
	keys := map[string]bool{}
	for _, m := range []model.Media{base, rangeFile, part1, part2} {
		key := mediaVersionGroupKey(m)
		if keys[key] {
			t.Fatal("coverage merged as versions")
		}
		keys[key] = true
	}
	version := rangeFile
	version.Height = 2160
	if mediaVersionGroupKey(version) != mediaVersionGroupKey(rangeFile) {
		t.Fatal("same coverage different resolution split")
	}
}

func TestMappingBounds(t *testing.T) {
	season, episode, end, part := 1, 2, 1, 100
	for _, req := range []ManualScrapeRequest{{EpisodeEndNum: &end}, {SeasonNum: &season, EpisodeNum: &episode, EpisodeEndNum: &end}, {SeasonNum: &season, EpisodeNum: &episode, EpisodePartNum: &part}} {
		if configureManualEpisodeMapping(req, &ScrapeOptions{}) == nil {
			t.Fatal("invalid mapping accepted")
		}
	}
}
