package service

import (
	"strings"
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
	for _, key := range []string{"SeasonId", "SeasonName", "SeriesId", "SeriesName", "ParentIndexNumber", "IndexNumber"} {
		if _, ok := item[key]; ok {
			t.Fatalf("movie item should not expose episodic field %s: %#v", key, item)
		}
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

func TestEmbyMovieLibraryExposesMultipartVideoAsVirtualSeries(t *testing.T) {
	svc := newTestEmbyService(t)
	lib := model.Library{Name: "其他媒体", Path: "/media/other", Type: "movie", Enabled: true}
	if err := svc.repo.Library.Create(t.Context(), &lib); err != nil {
		t.Fatal(err)
	}
	rows := []model.Media{
		{Base: model.Base{ID: "part-3"}, LibraryID: lib.ID, Title: "作品 A 第 3 段", Path: "/media/other/作品 A/003.mp4", PartGroupKey: "work-a", PartGroupTitle: "作品 A", PartIndex: 3, Container: "mp4"},
		{Base: model.Base{ID: "part-1"}, LibraryID: lib.ID, Title: "作品 A 第 1 段", Path: "/media/other/作品 A/001.mp4", PartGroupKey: "work-a", PartGroupTitle: "作品 A", PartIndex: 1, Container: "mp4", GeneratedPosterURL: "https://image.example/work-a-generated.jpg", GeneratedBackdropURL: "https://image.example/work-a-generated-backdrop.jpg"},
		{Base: model.Base{ID: "part-2"}, LibraryID: lib.ID, Title: "作品 A 第 2 段", Path: "/media/other/作品 A/002.mp4", PartGroupKey: "work-a", PartGroupTitle: "作品 A", PartIndex: 2, Container: "mp4"},
	}
	if err := svc.repo.DB.Create(&rows).Error; err != nil {
		t.Fatal(err)
	}

	root, err := svc.Items(t.Context(), ItemsParams{ParentID: lib.ID, Limit: 50})
	if err != nil {
		t.Fatal(err)
	}
	items := root["Items"].([]map[string]any)
	if len(items) != 1 || items[0]["Type"] != "Series" || items[0]["Name"] != "作品 A" {
		t.Fatalf("multipart work was not exposed as one virtual series: %#v", items)
	}
	seriesID, _ := items[0]["Id"].(string)
	if seriesID == "" || !strings.HasPrefix(seriesID, embyVirtualSeriesPrefix) {
		t.Fatalf("multipart series has invalid id: %#v", items[0])
	}
	if tags, ok := items[0]["ImageTags"].(map[string]string); !ok || tags["Primary"] == "" {
		t.Fatalf("multipart series did not expose generated primary artwork: %#v", items[0])
	}
	if tags, ok := items[0]["BackdropImageTags"].([]string); !ok || len(tags) != 1 {
		t.Fatalf("multipart series did not expose generated backdrop artwork: %#v", items[0])
	}
	recursive, err := svc.Items(t.Context(), ItemsParams{
		ParentID: lib.ID, Recursive: true, IncludeItemTypes: []string{"Movie"}, Limit: 1,
	})
	if err != nil {
		t.Fatal(err)
	}
	if recursive["TotalRecordCount"] != 1 || len(recursive["Items"].([]map[string]any)) != 1 || recursive["Items"].([]map[string]any)[0]["Type"] != "Series" {
		t.Fatalf("recursive movie query did not preserve the multipart group: %#v", recursive)
	}

	episodes, err := svc.Items(t.Context(), ItemsParams{
		ParentID: seriesID, Recursive: true, IncludeItemTypes: []string{"Episode"}, Limit: 50,
	})
	if err != nil {
		t.Fatal(err)
	}
	episodeItems := episodes["Items"].([]map[string]any)
	if len(episodeItems) != 3 {
		t.Fatalf("multipart series did not expose every ordered part: %#v", episodes)
	}
	for index, item := range episodeItems {
		wantID := []string{"part-1", "part-2", "part-3"}[index]
		if item["Id"] != wantID || item["Type"] != "Episode" || item["IndexNumber"] != index+1 || item["SeriesId"] != seriesID {
			t.Fatalf("multipart episode %d is invalid: %#v", index, item)
		}
		partSources := item["MediaSources"].([]map[string]any)
		if len(partSources) != 1 || partSources[0]["Id"] != wantID {
			t.Fatalf("multipart episode %d leaked another source: %#v", index, partSources)
		}
	}
	svc.invalidateVirtualSeriesCache()
	coldEpisodes, err := svc.Items(t.Context(), ItemsParams{
		ParentID: seriesID, Recursive: true, IncludeItemTypes: []string{"Episode"}, Limit: 50,
	})
	if err != nil || len(coldEpisodes["Items"].([]map[string]any)) != 3 {
		t.Fatalf("cold-cache multipart series lookup failed: %#v err=%v", coldEpisodes, err)
	}
	artwork, err := svc.ImageURL(t.Context(), seriesID, "Primary")
	if err != nil || artwork != "https://image.example/work-a-generated.jpg" {
		t.Fatalf("cold-cache multipart series artwork failed: %q err=%v", artwork, err)
	}
	backdrop, err := svc.ImageURL(t.Context(), seriesID, "Backdrop")
	if err != nil || backdrop != "https://image.example/work-a-generated-backdrop.jpg" {
		t.Fatalf("cold-cache multipart series backdrop failed: %q err=%v", backdrop, err)
	}
	directPart, err := svc.Item(t.Context(), "part-2", "")
	if err != nil {
		t.Fatal(err)
	}
	if directPart["Type"] != "Episode" || directPart["IndexNumber"] != 2 || directPart["SeriesId"] != seriesID {
		t.Fatalf("direct multipart item lost its virtual episode identity: %#v", directPart)
	}
	counts, err := svc.ItemCounts(t.Context(), "")
	if err != nil {
		t.Fatal(err)
	}
	if counts["MovieCount"] != int64(0) || counts["SeriesCount"] != 1 || counts["EpisodeCount"] != int64(3) {
		t.Fatalf("multipart item counts do not match the virtual hierarchy: %#v", counts)
	}

	additional, err := svc.AdditionalParts(t.Context(), "part-1", "")
	if err != nil {
		t.Fatal(err)
	}
	partItems := additional["Items"].([]map[string]any)
	if len(partItems) != 2 || partItems[0]["Id"] != "part-2" || partItems[1]["Id"] != "part-3" {
		t.Fatalf("additional parts are not ordered by part_index: %#v", partItems)
	}
	for index, item := range partItems {
		partSources := item["MediaSources"].([]map[string]any)
		wantID := []string{"part-2", "part-3"}[index]
		if len(partSources) != 1 || partSources[0]["Id"] != wantID {
			t.Fatalf("additional part %d leaked another source: %#v", index, partSources)
		}
	}

	latest, err := svc.LatestItems(t.Context(), "", lib.ID, 10)
	if err != nil || len(latest) != 1 || latest[0]["Id"] != "part-1" {
		t.Fatalf("latest items leaked physical parts: %#v err=%v", latest, err)
	}
	playback, err := svc.PlaybackInfo(t.Context(), "part-1", "")
	if err != nil {
		t.Fatal(err)
	}
	playSources := playback["MediaSources"].([]map[string]any)
	if len(playSources) != 1 || playSources[0]["Id"] != "part-1" {
		t.Fatalf("playback info treated parts as versions: %#v", playSources)
	}
}

func TestEmbyMovieLibraryMultipartBrowseHonorsDateCreatedSort(t *testing.T) {
	svc := newTestEmbyService(t)
	lib := model.Library{Name: "其他媒体", Path: "/media/other", Type: "movie", Enabled: true}
	if err := svc.repo.Library.Create(t.Context(), &lib); err != nil {
		t.Fatal(err)
	}
	base := time.Now().Add(-24 * time.Hour)
	rows := []model.Media{
		{
			Base:           model.Base{ID: "older-part-1", CreatedAt: base},
			LibraryID:      lib.ID,
			Title:          "较早添加的多片段作品",
			Path:           "/media/other/较早添加的多片段作品/part-1.mp4",
			PartGroupKey:   "older-work",
			PartGroupTitle: "较早添加的多片段作品",
			PartIndex:      1,
			ReleaseDate:    "2026-07-20",
		},
		{
			Base:           model.Base{ID: "older-part-2", CreatedAt: base.Add(time.Minute)},
			LibraryID:      lib.ID,
			Title:          "较早添加的多片段作品",
			Path:           "/media/other/较早添加的多片段作品/part-2.mp4",
			PartGroupKey:   "older-work",
			PartGroupTitle: "较早添加的多片段作品",
			PartIndex:      2,
			ReleaseDate:    "2026-07-20",
		},
		{
			Base:        model.Base{ID: "newer-movie", CreatedAt: base.Add(2 * time.Hour)},
			LibraryID:   lib.ID,
			Title:       "最近添加的电影",
			Path:        "/media/other/最近添加的电影.mp4",
			ReleaseDate: "2020-01-01",
		},
	}
	if err := svc.repo.DB.Create(&rows).Error; err != nil {
		t.Fatal(err)
	}

	out, err := svc.Items(t.Context(), ItemsParams{
		ParentID:         lib.ID,
		Recursive:        true,
		IncludeItemTypes: []string{"Movie"},
		SortBy:           "DateCreated,SortName",
		SortOrder:        "Descending",
		Limit:            12,
	})
	if err != nil {
		t.Fatal(err)
	}
	items := out["Items"].([]map[string]any)
	if len(items) != 2 || items[0]["Id"] != "newer-movie" || items[1]["Type"] != "Series" {
		t.Fatalf("DateCreated descending sort was ignored: %#v", items)
	}
}

func TestEmbyMovieLibraryPagesLogicalEntriesBeforeBuildingMoviePayloads(t *testing.T) {
	svc := newTestEmbyService(t)
	lib := model.Library{Name: "Movies", Path: "/media/movies", Type: "movie", Enabled: true}
	if err := svc.repo.Library.Create(t.Context(), &lib); err != nil {
		t.Fatal(err)
	}
	base := time.Now().Add(-24 * time.Hour)
	rows := []model.Media{
		{Base: model.Base{ID: "older", CreatedAt: base.Add(time.Minute)}, LibraryID: lib.ID, Title: "Older", Path: "/media/movies/older.mkv"},
		{Base: model.Base{ID: "version-low", CreatedAt: base.Add(2 * time.Minute)}, LibraryID: lib.ID, Title: "Versioned", Path: "/media/movies/version-low.mkv", TMDbID: 1001, Width: 1920},
		{Base: model.Base{ID: "version-high", CreatedAt: base.Add(3 * time.Minute)}, LibraryID: lib.ID, Title: "Versioned", Path: "/media/movies/version-high.mkv", TMDbID: 1001, Width: 3840},
		{Base: model.Base{ID: "part-1", CreatedAt: base.Add(4 * time.Minute)}, LibraryID: lib.ID, Title: "Multipart Work Part 1", Path: "/media/movies/multipart/part-1.mkv", PartGroupKey: "multipart-work", PartGroupTitle: "Multipart Work", PartIndex: 1},
		{Base: model.Base{ID: "part-2", CreatedAt: base.Add(5 * time.Minute)}, LibraryID: lib.ID, Title: "Multipart Work Part 2", Path: "/media/movies/multipart/part-2.mkv", PartGroupKey: "multipart-work", PartGroupTitle: "Multipart Work", PartIndex: 2},
		{Base: model.Base{ID: "newer", CreatedAt: base.Add(6 * time.Minute)}, LibraryID: lib.ID, Title: "Newer", Path: "/media/movies/newer.mkv"},
	}
	if err := svc.repo.DB.Create(&rows).Error; err != nil {
		t.Fatal(err)
	}

	params := ItemsParams{
		ParentID:         lib.ID,
		Recursive:        true,
		IncludeItemTypes: []string{"Movie"},
		SortBy:           "DateCreated,SortName",
		SortOrder:        "Descending",
		Limit:            2,
		OmitMediaSources: true,
	}
	first, err := svc.Items(t.Context(), params)
	if err != nil {
		t.Fatal(err)
	}
	if first["TotalRecordCount"] != 4 {
		t.Fatalf("logical total = %#v, want 4", first["TotalRecordCount"])
	}
	firstItems := first["Items"].([]map[string]any)
	if len(firstItems) != 2 || firstItems[0]["Id"] != "newer" || firstItems[1]["Type"] != "Series" {
		t.Fatalf("first logical page = %#v", firstItems)
	}
	for _, item := range firstItems {
		if _, ok := item["MediaSources"]; ok {
			t.Fatalf("lightweight list item unexpectedly contains MediaSources: %#v", item)
		}
	}

	params.StartIndex = 2
	second, err := svc.Items(t.Context(), params)
	if err != nil {
		t.Fatal(err)
	}
	secondItems := second["Items"].([]map[string]any)
	if len(secondItems) != 2 || secondItems[0]["Id"] != "version-high" || secondItems[1]["Id"] != "older" {
		t.Fatalf("second logical page = %#v", secondItems)
	}

	params.OmitMediaSources = false
	full, err := svc.Items(t.Context(), params)
	if err != nil {
		t.Fatal(err)
	}
	fullItems := full["Items"].([]map[string]any)
	sources, ok := fullItems[0]["MediaSources"].([]map[string]any)
	if !ok || len(sources) != 2 {
		t.Fatalf("full versioned list item should retain both MediaSources: %#v", fullItems[0])
	}
}
