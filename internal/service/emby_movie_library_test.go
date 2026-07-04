package service

import (
	"testing"
	"time"

	"github.com/ShukeBta/MediaStationGo/internal/model"
)

func TestEmbyMovieLibrarySeasonNumbersStayMovies(t *testing.T) {
	svc := newTestEmbyService(t)
	lib := model.Library{Name: "动画电影", Path: `/media/movies/animation`, Type: "Movie", Enabled: true}
	if err := svc.repo.Library.Create(t.Context(), &lib); err != nil {
		t.Fatalf("create library: %v", err)
	}
	media := model.Media{
		Base:       model.Base{ID: "movie-with-episode-numbers"},
		LibraryID:  lib.ID,
		Title:      "Movie Mistaken S01E01",
		Path:       `/media/movies/animation/Movie.Mistaken.S01E01.mkv`,
		PosterURL:  `/poster.jpg`,
		SeasonNum:  1,
		EpisodeNum: 1,
	}
	if err := svc.repo.DB.Create(&media).Error; err != nil {
		t.Fatalf("create media: %v", err)
	}

	out, err := svc.Items(t.Context(), ItemsParams{ParentID: lib.ID, IncludeItemTypes: []string{"Movie"}, Limit: 50})
	if err != nil {
		t.Fatalf("items: %v", err)
	}
	items := out["Items"].([]map[string]any)
	if len(items) != 1 {
		t.Fatalf("movie library item filtered out by season numbers: %#v", out)
	}
	if items[0]["Type"] != "Movie" || items[0]["ParentId"] != lib.ID {
		t.Fatalf("movie library item should stay Movie, got %#v", items[0])
	}
	tags := items[0]["ImageTags"].(map[string]string)
	if tags["Primary"] == "" {
		t.Fatalf("movie poster should expose Primary image tag: %#v", items[0])
	}

	episodes, err := svc.Items(t.Context(), ItemsParams{ParentID: lib.ID, IncludeItemTypes: []string{"Episode"}, Limit: 50})
	if err != nil {
		t.Fatalf("episode query: %v", err)
	}
	if len(episodes["Items"].([]map[string]any)) != 0 {
		t.Fatalf("movie library should not expose movies as episodes, got %#v", episodes)
	}

	item, err := svc.Item(t.Context(), media.ID, "user-1")
	if err != nil {
		t.Fatalf("item: %v", err)
	}
	if item["Type"] != "Movie" || item["ParentId"] != lib.ID {
		t.Fatalf("direct item should stay Movie, got %#v", item)
	}

	rootMovies, err := svc.Items(t.Context(), ItemsParams{IncludeItemTypes: []string{"Movie"}, Recursive: true, Limit: 50})
	if err != nil {
		t.Fatalf("root movie query: %v", err)
	}
	rootItems := rootMovies["Items"].([]map[string]any)
	if len(rootItems) != 1 || rootItems[0]["Id"] != media.ID || rootItems[0]["Type"] != "Movie" {
		t.Fatalf("root movie query should include movie-library item despite season numbers, got %#v", rootItems)
	}

	rootEpisodes, err := svc.Items(t.Context(), ItemsParams{IncludeItemTypes: []string{"Episode"}, Recursive: true, Limit: 50})
	if err != nil {
		t.Fatalf("root episode query: %v", err)
	}
	if len(rootEpisodes["Items"].([]map[string]any)) != 0 {
		t.Fatalf("root episode query should not expose movie-library item, got %#v", rootEpisodes)
	}
}

func TestEmbyMovieLibraryFiltersMisplacedSeriesPaths(t *testing.T) {
	svc := newTestEmbyService(t)
	lib := model.Library{Name: "电影", Path: `/media/movies`, Type: "movie", Enabled: true}
	if err := svc.repo.Library.Create(t.Context(), &lib); err != nil {
		t.Fatalf("create library: %v", err)
	}
	show := model.Media{
		Base:       model.Base{ID: "misplaced-show"},
		LibraryID:  lib.ID,
		Title:      "错放剧集",
		Path:       `/media/movies/国产剧/错放剧集/Season 01/错放剧集 - S01E01.mkv`,
		SeasonNum:  1,
		EpisodeNum: 1,
	}
	movie := model.Media{
		Base:      model.Base{ID: "movie"},
		LibraryID: lib.ID,
		Title:     "普通电影",
		Path:      `/media/movies/普通电影.2026.mkv`,
	}
	if err := svc.repo.DB.Create(&show).Error; err != nil {
		t.Fatalf("create show: %v", err)
	}
	if err := svc.repo.DB.Create(&movie).Error; err != nil {
		t.Fatalf("create movie: %v", err)
	}

	out, err := svc.Items(t.Context(), ItemsParams{ParentID: lib.ID, IncludeItemTypes: []string{"Movie"}, Limit: 50})
	if err != nil {
		t.Fatalf("items: %v", err)
	}
	items := out["Items"].([]map[string]any)
	if len(items) != 1 || items[0]["Id"] != movie.ID {
		t.Fatalf("movie library should filter misplaced series paths, got %#v", items)
	}

	item, err := svc.Item(t.Context(), show.ID, "user-1")
	if err != nil {
		t.Fatalf("direct item: %v", err)
	}
	if item["Type"] != "Episode" {
		t.Fatalf("misplaced series path should be typed as Episode directly, got %#v", item)
	}
}

// TestEmbyMovieLibraryGroupsEpisodicContentIntoSeries 验证方案 B: 电影类型库里
// 混入的「剧集结构」内容(如整合成剧集的剧场版/合集动画, 路径含 Season/SxxE)
// 在常规浏览(未指定 IncludeItemTypes)时应聚成 Series 卡片, 而不是以散装单集
// (Episode)漏出; 同库里真正的电影仍按 Movie 显示。两类并存于同一电影库视图。
func TestEmbyMovieLibraryGroupsEpisodicContentIntoSeries(t *testing.T) {
	svc := newTestEmbyService(t)
	lib := model.Library{Name: "动画电影", Path: `/media/动画电影`, Type: "movie", Enabled: true}
	if err := svc.repo.Library.Create(t.Context(), &lib); err != nil {
		t.Fatalf("create library: %v", err)
	}
	base := time.Now()
	// 剧集结构内容: 同一部「高达剧场版」的两集, 单集 tmdb 各不相同(模拟 NFO 单集 id 污染)。
	rows := []model.Media{
		{
			Base:       model.Base{ID: "gundam-e13", CreatedAt: base.Add(2 * time.Minute)},
			LibraryID:  lib.ID,
			Title:      "高达剧场版",
			Path:       `/media/动画电影/高达剧场版/Season 01/高达剧场版 - S01E13.mkv`,
			PosterURL:  `/poster.jpg`,
			TMDbID:     4375419,
			SeasonNum:  1,
			EpisodeNum: 13,
		},
		{
			Base:       model.Base{ID: "gundam-e14", CreatedAt: base.Add(3 * time.Minute)},
			LibraryID:  lib.ID,
			Title:      "高达剧场版",
			Path:       `/media/动画电影/高达剧场版/Season 01/高达剧场版 - S01E14.mkv`,
			PosterURL:  `/poster.jpg`,
			TMDbID:     4375461,
			SeasonNum:  1,
			EpisodeNum: 14,
		},
		// 真正的电影。
		{
			Base:      model.Base{ID: "real-movie", CreatedAt: base.Add(1 * time.Minute)},
			LibraryID: lib.ID,
			Title:     "普通动画电影",
			Path:      `/media/动画电影/普通动画电影.2024.mkv`,
			PosterURL: `/poster.jpg`,
		},
	}
	for i := range rows {
		if err := svc.repo.DB.Create(&rows[i]).Error; err != nil {
			t.Fatalf("create media: %v", err)
		}
	}

	out, err := svc.Items(t.Context(), ItemsParams{ParentID: lib.ID, Limit: 50})
	if err != nil {
		t.Fatalf("items: %v", err)
	}
	items := out["Items"].([]map[string]any)

	var seriesCount, movieCount, episodeCount int
	var seriesPayload map[string]any
	for _, item := range items {
		switch item["Type"] {
		case "Series":
			seriesCount++
			seriesPayload = item
		case "Movie":
			movieCount++
		case "Episode":
			episodeCount++
		}
	}
	if episodeCount != 0 {
		t.Fatalf("movie library must not leak flat episodes, got %d episode items: %#v", episodeCount, items)
	}
	if seriesCount != 1 {
		t.Fatalf("episodic content should collapse into exactly one Series card, got %d: %#v", seriesCount, items)
	}
	if movieCount != 1 {
		t.Fatalf("real movie should stay as one Movie item, got %d: %#v", movieCount, items)
	}
	// 两集 tmdb 不同, 但按路径剧名聚成同一 Series, 集数应为 2。
	if got := seriesPayload["RecursiveItemCount"]; got != 2 {
		t.Fatalf("series should contain both episodes despite differing tmdb ids, got RecursiveItemCount=%v", got)
	}

	// Series 卡片可下钻: 解析其 season → episodes。
	seriesID, _ := seriesPayload["Id"].(string)
	if seriesID == "" {
		t.Fatalf("series payload missing Id: %#v", seriesPayload)
	}
	drill, err := svc.Items(t.Context(), ItemsParams{ParentID: seriesID, Recursive: true, Limit: 50})
	if err != nil {
		t.Fatalf("series drill-down: %v", err)
	}
	drillItems := drill["Items"].([]map[string]any)
	if len(drillItems) != 2 {
		t.Fatalf("series drill-down should list both episodes, got %#v", drillItems)
	}
}
