package service

import (
	"testing"

	"go.uber.org/zap"

	"github.com/ShukeBta/MediaStationGo/internal/model"
	"github.com/ShukeBta/MediaStationGo/internal/repository"
)

func TestRepairRescrapeOptionsDefaultSkipsEpisodeArtwork(t *testing.T) {
	options := repairRescrapeOptions()
	if !options.RetryNoMatch {
		t.Fatal("repair rescrape should retry no_match rows")
	}
	if !options.IncludeMatched {
		t.Fatal("repair rescrape should refresh already matched rows")
	}
	if options.EpisodeArtwork == nil {
		t.Fatal("repair rescrape should set an explicit episode artwork option")
	}
	if *options.EpisodeArtwork {
		t.Fatal("repair rescrape should skip episode artwork by default")
	}
}

func TestRepairRescrapeOptionsCanEnableEpisodeArtwork(t *testing.T) {
	episodeArtwork := true
	options := repairRescrapeOptions(ScrapeOptions{EpisodeArtwork: &episodeArtwork})
	if !options.RetryNoMatch {
		t.Fatal("repair rescrape should force retry no_match rows")
	}
	if !options.IncludeMatched {
		t.Fatal("repair rescrape should force refreshing already matched rows")
	}
	if options.EpisodeArtwork == nil || !*options.EpisodeArtwork {
		t.Fatal("repair rescrape should keep explicit episode artwork=true")
	}
}

func TestRepairRescrapeOptionsKeepsExplicitEpisodeArtworkFalse(t *testing.T) {
	episodeArtwork := false
	options := repairRescrapeOptions(ScrapeOptions{EpisodeArtwork: &episodeArtwork})
	if !options.RetryNoMatch {
		t.Fatal("repair rescrape should force retry no_match rows")
	}
	if !options.IncludeMatched {
		t.Fatal("repair rescrape should force refreshing already matched rows")
	}
	if options.EpisodeArtwork == nil {
		t.Fatal("repair rescrape should keep explicit episode artwork option")
	}
	if *options.EpisodeArtwork {
		t.Fatal("repair rescrape should keep explicit episode artwork=false")
	}
}

// TestResetEpisodicMatchedForRescrape 验证「修复+重刮」会把脏的 matched 剧集行
// 重置为 pending(让 EnrichLibrary 能重新刮削),而电影行与其它库不受影响。
func TestResetEpisodicMatchedForRescrape(t *testing.T) {
	db := newServiceTestDB(t, &model.Library{}, &model.Media{})
	container := &Container{Repo: repository.New(db), Log: zap.NewNop()}

	rows := []model.Media{
		// 目标库的剧集行(matched, 有季集号)→ 应被重置。
		{Base: model.Base{ID: "ep1"}, LibraryID: "lib-a", SeasonNum: 1, EpisodeNum: 1, ScrapeStatus: "matched", Path: "/a/show/S01/ep1.mkv"},
		{Base: model.Base{ID: "ep2"}, LibraryID: "lib-a", SeasonNum: 1, EpisodeNum: 2, ScrapeStatus: "matched", Path: "/a/show/S01/ep2.mkv"},
		// 目标库的电影行(无季集号)→ 不应被重置。
		{Base: model.Base{ID: "movie1"}, LibraryID: "lib-a", ScrapeStatus: "matched", Path: "/a/movie.mkv"},
		// 目标库已是 pending 的剧集行 → 不计入重置数。
		{Base: model.Base{ID: "ep3"}, LibraryID: "lib-a", SeasonNum: 1, EpisodeNum: 3, ScrapeStatus: "pending", Path: "/a/show/S01/ep3.mkv"},
		// 其它库的剧集行(matched)→ 单库重置时不应受影响。
		{Base: model.Base{ID: "ep-other"}, LibraryID: "lib-b", SeasonNum: 1, EpisodeNum: 1, ScrapeStatus: "matched", Path: "/b/show/S01/ep1.mkv"},
	}
	for i := range rows {
		if err := db.Create(&rows[i]).Error; err != nil {
			t.Fatalf("create media %s: %v", rows[i].ID, err)
		}
	}

	reset, err := container.resetEpisodicMatchedForRescrape(t.Context(), "lib-a")
	if err != nil {
		t.Fatalf("reset: %v", err)
	}
	if reset != 2 {
		t.Fatalf("reset = %d, want 2 (only matched episodic rows in lib-a)", reset)
	}

	status := func(id string) string {
		var m model.Media
		if err := db.First(&m, "id = ?", id).Error; err != nil {
			t.Fatalf("load %s: %v", id, err)
		}
		return m.ScrapeStatus
	}
	if status("ep1") != "pending" || status("ep2") != "pending" {
		t.Fatalf("episodic matched rows should be pending, got ep1=%q ep2=%q", status("ep1"), status("ep2"))
	}
	if status("movie1") != "matched" {
		t.Fatalf("movie row should stay matched, got %q", status("movie1"))
	}
	if status("ep-other") != "matched" {
		t.Fatalf("other library row should stay matched, got %q", status("ep-other"))
	}
}

func TestRepairAndRescrapeLibraryExpandsMergedCloudLibraries(t *testing.T) {
	db := newServiceTestDB(t, &model.Library{}, &model.Media{})
	container := &Container{Repo: repository.New(db), Log: zap.NewNop()}

	local := model.Library{Name: "国产剧", Path: "/media/电视剧/国产剧", Type: "tv", Enabled: true}
	cloud := model.Library{
		Name:    "OpenList · 国产剧",
		Path:    BuildCloudLibraryPath("openlist", "/国产剧", "/国产剧"),
		Type:    "tv",
		Enabled: true,
	}
	if err := container.Repo.Library.Create(t.Context(), &local); err != nil {
		t.Fatal(err)
	}
	if err := container.Repo.Library.Create(t.Context(), &cloud); err != nil {
		t.Fatal(err)
	}
	repairMedia := model.Media{
		LibraryID:    cloud.ID,
		Title:        "主角",
		Path:         "cloud://openlist/国产剧/主角 (2026) {tmdb-284110}/Season 1/主角.S01E01.mkv",
		SeasonNum:    1,
		EpisodeNum:   1,
		ScrapeStatus: "matched",
	}
	resetMedia := model.Media{
		LibraryID:    cloud.ID,
		Title:        "无占位符剧集",
		Path:         "cloud://openlist/国产剧/无占位符剧集/Season 1/无占位符剧集.S01E01.mkv",
		SeasonNum:    1,
		EpisodeNum:   1,
		ScrapeStatus: "matched",
	}
	if err := db.Create(&repairMedia).Error; err != nil {
		t.Fatal(err)
	}
	if err := db.Create(&resetMedia).Error; err != nil {
		t.Fatal(err)
	}

	result, err := container.RepairAndRescrapeLibrary(t.Context(), local.ID)
	if err != nil {
		t.Fatal(err)
	}
	if result.Repaired != 1 || result.Reset != 1 {
		t.Fatalf("result = %+v, want repaired/reset for merged cloud row", result)
	}
	var repaired model.Media
	if err := db.First(&repaired, "id = ?", repairMedia.ID).Error; err != nil {
		t.Fatal(err)
	}
	if repaired.TMDbID != 284110 || repaired.ScrapeStatus != "pending" {
		t.Fatalf("merged cloud row not repaired/reset: tmdb=%d status=%q", repaired.TMDbID, repaired.ScrapeStatus)
	}
	var reset model.Media
	if err := db.First(&reset, "id = ?", resetMedia.ID).Error; err != nil {
		t.Fatal(err)
	}
	if reset.ScrapeStatus != "pending" {
		t.Fatalf("merged cloud row not reset: status=%q", reset.ScrapeStatus)
	}
}
