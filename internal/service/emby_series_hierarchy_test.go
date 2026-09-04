package service

import (
	"context"
	"fmt"
	"testing"
	"time"

	"github.com/ShukeBta/MediaStationGo/internal/model"
	"gorm.io/gorm"
)

func TestEmbyEpisodePayloadsBatchSeriesTitleLookups(t *testing.T) {
	svc := newTestEmbyService(t)
	lib := model.Library{Name: "剧集", Path: `/media/tv`, Type: "tv", Enabled: true}
	if err := svc.repo.Library.Create(t.Context(), &lib); err != nil {
		t.Fatalf("create library: %v", err)
	}
	seriesRows := []model.Series{
		{Base: model.Base{ID: "series-a"}, LibraryID: lib.ID, Title: "甲剧"},
		{Base: model.Base{ID: "series-b"}, LibraryID: lib.ID, Title: "乙剧"},
	}
	if err := svc.repo.DB.Create(&seriesRows).Error; err != nil {
		t.Fatalf("create series: %v", err)
	}
	mediaRows := []model.Media{
		{Base: model.Base{ID: "episode-a"}, LibraryID: lib.ID, SeriesID: "series-a", Title: "文件名甲", Path: `/media/tv/a.mkv`, SeasonNum: 1, EpisodeNum: 1},
		{Base: model.Base{ID: "episode-b"}, LibraryID: lib.ID, SeriesID: "series-b", Title: "文件名乙", Path: `/media/tv/b.mkv`, SeasonNum: 1, EpisodeNum: 1},
	}

	seriesQueries := 0
	callbackName := "test:batch-series-title-query-count"
	if err := svc.repo.DB.Callback().Query().Before("gorm:query").Register(callbackName, func(tx *gorm.DB) {
		if tx.Statement.Table == "series" {
			seriesQueries++
		}
	}); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = svc.repo.DB.Callback().Query().Remove(callbackName) })

	items, err := svc.payloadsForMediaRows(t.Context(), mediaRows, "", false, false)
	if err != nil {
		t.Fatalf("build payloads: %v", err)
	}
	if seriesQueries != 1 {
		t.Fatalf("series title queries = %d, want one batched query", seriesQueries)
	}
	if len(items) != 2 || items[0]["SeriesName"] != "甲剧" || items[1]["SeriesName"] != "乙剧" {
		t.Fatalf("batched series titles = %#v, want scraped series titles", items)
	}
}

func TestEmbyItemsExposeSeriesSeasonEpisodeHierarchy(t *testing.T) {
	svc := newTestEmbyService(t)
	lib := model.Library{Name: "番剧", Path: `F:\downloads\日番`, Type: "anime", Enabled: true}
	if err := svc.repo.Library.Create(t.Context(), &lib); err != nil {
		t.Fatalf("create library: %v", err)
	}
	for _, media := range []model.Media{
		{
			Base:         model.Base{ID: "ep-1"},
			LibraryID:    lib.ID,
			Title:        "间谍过家家",
			OriginalName: "SPY×FAMILY",
			EpisodeTitle: "第 1 集",
			Path:         `F:\downloads\日番\剧集\间谍过家家\Season 02\间谍过家家 - S02E01.mkv`,
			PosterURL:    `F:\poster.jpg`,
			SeasonNum:    2,
			EpisodeNum:   1,
		},
		{
			Base:         model.Base{ID: "ep-2"},
			LibraryID:    lib.ID,
			Title:        "间谍过家家",
			OriginalName: "SPY×FAMILY",
			EpisodeTitle: "第 2 集",
			Path:         `F:\downloads\日番\剧集\间谍过家家\Season 02\间谍过家家 - S02E02.mkv`,
			PosterURL:    `F:\poster.jpg`,
			SeasonNum:    2,
			EpisodeNum:   2,
		},
	} {
		if err := svc.repo.DB.Create(&media).Error; err != nil {
			t.Fatalf("create media: %v", err)
		}
	}

	root, err := svc.Items(t.Context(), ItemsParams{ParentID: lib.ID, Limit: 50})
	if err != nil {
		t.Fatalf("library items: %v", err)
	}
	rootItems := root["Items"].([]map[string]any)
	if len(rootItems) != 1 {
		t.Fatalf("expected one series card, got %#v", rootItems)
	}
	seriesID := rootItems[0]["Id"].(string)
	if rootItems[0]["Type"] != "Series" || rootItems[0]["IsFolder"] != true || rootItems[0]["Name"] != "间谍过家家" {
		t.Fatalf("unexpected series payload: %#v", rootItems[0])
	}

	seasons, err := svc.Items(t.Context(), ItemsParams{ParentID: seriesID, Limit: 50})
	if err != nil {
		t.Fatalf("series items: %v", err)
	}
	seasonItems := seasons["Items"].([]map[string]any)
	if len(seasonItems) != 1 || seasonItems[0]["Type"] != "Season" || seasonItems[0]["IndexNumber"] != 2 {
		t.Fatalf("unexpected seasons: %#v", seasonItems)
	}

	episodes, err := svc.Items(t.Context(), ItemsParams{ParentID: seasonItems[0]["Id"].(string), IncludeItemTypes: []string{"Episode"}, Recursive: true, Limit: 50})
	if err != nil {
		t.Fatalf("season episodes: %v", err)
	}
	episodeItems := episodes["Items"].([]map[string]any)
	if len(episodeItems) != 2 || episodeItems[0]["Type"] != "Episode" || episodeItems[0]["Name"] != "第 1 集" {
		t.Fatalf("unexpected episodes: %#v", episodeItems)
	}
	if episodeItems[0]["SeriesId"] != seriesID || episodeItems[0]["ParentId"] != seasonItems[0]["Id"] {
		t.Fatalf("episode hierarchy not linked: %#v", episodeItems[0])
	}

	latest, err := svc.LatestItems(t.Context(), "user-1", lib.ID, 10)
	if err != nil {
		t.Fatalf("latest items: %v", err)
	}
	if len(latest) != 1 || latest[0]["Type"] != "Series" {
		t.Fatalf("latest should be grouped by series: %#v", latest)
	}

	playback, err := svc.PlaybackInfo(t.Context(), seriesID, "user-1")
	if err != nil {
		t.Fatalf("series playback fallback: %v", err)
	}
	sources := playback["MediaSources"].([]map[string]any)
	if sources[0]["Id"] != "ep-1" {
		t.Fatalf("series playback should fall back to first episode: %#v", sources)
	}
	if sources[0]["DirectStreamUrl"] != "/Videos/ep-1/stream.mkv" {
		t.Fatalf("playback should use Emby-compatible stream URL: %#v", sources[0])
	}
}

func TestEmbySeriesGroupingPaginatesAfterFullLibraryGrouping(t *testing.T) {
	svc := newTestEmbyService(t)
	lib := model.Library{Name: "国漫", Path: `/media/anime`, Type: "anime", Enabled: true}
	if err := svc.repo.Library.Create(t.Context(), &lib); err != nil {
		t.Fatalf("create library: %v", err)
	}
	now := time.Date(2026, 7, 2, 12, 0, 0, 0, time.UTC)
	rows := make([]model.Media, 0, 25*40)
	for series := 1; series <= 25; series++ {
		for episode := 1; episode <= 40; episode++ {
			created := now.Add(time.Duration(series*1000+episode) * time.Second)
			rows = append(rows, model.Media{
				Base:       model.Base{ID: fmt.Sprintf("show-%02d-ep-%02d", series, episode), CreatedAt: created, UpdatedAt: created},
				LibraryID:  lib.ID,
				Title:      fmt.Sprintf("测试番 %02d", series),
				Path:       fmt.Sprintf(`/media/anime/测试番 %02d/Season 01/测试番 %02d.S01E%02d.mkv`, series, series, episode),
				SeasonNum:  1,
				EpisodeNum: episode,
			})
		}
	}
	if err := svc.repo.DB.CreateInBatches(rows, 200).Error; err != nil {
		t.Fatalf("create media: %v", err)
	}

	root, err := svc.Items(t.Context(), ItemsParams{ParentID: lib.ID, Limit: 20})
	if err != nil {
		t.Fatalf("library items: %v", err)
	}
	if root["TotalRecordCount"] != 25 {
		t.Fatalf("series total = %#v, want 25", root["TotalRecordCount"])
	}
	rootItems := root["Items"].([]map[string]any)
	if len(rootItems) != 20 {
		t.Fatalf("first page series len = %d, want 20", len(rootItems))
	}
	if rootItems[0]["RecursiveItemCount"] != 40 {
		t.Fatalf("first series episode count = %#v, want 40", rootItems[0]["RecursiveItemCount"])
	}

	latest, err := svc.LatestItems(t.Context(), "user-1", lib.ID, 25)
	if err != nil {
		t.Fatalf("latest items: %v", err)
	}
	if len(latest) != 25 {
		t.Fatalf("latest series len = %d, want 25", len(latest))
	}

	counts, err := svc.ItemCounts(t.Context(), "user-1")
	if err != nil {
		t.Fatalf("item counts: %v", err)
	}
	if counts["SeriesCount"] != 25 || counts["EpisodeCount"] != int64(1000) {
		t.Fatalf("counts = %#v, want 25 series and 1000 episodes", counts)
	}
}

func TestEmbyItemsKeepSpecialsInSeasonZero(t *testing.T) {
	svc := newTestEmbyService(t)
	lib := model.Library{Name: "番剧", Path: `F:\downloads\日番`, Type: "anime", Enabled: true}
	if err := svc.repo.Library.Create(t.Context(), &lib); err != nil {
		t.Fatalf("create library: %v", err)
	}
	media := model.Media{
		Base:       model.Base{ID: "sp-1"},
		LibraryID:  lib.ID,
		Title:      "间谍过家家",
		Path:       `F:\downloads\日番\间谍过家家\Specials\间谍过家家 - S00E01.mkv`,
		PosterURL:  `F:\episode-still.jpg`,
		SeasonNum:  0,
		EpisodeNum: 1,
	}
	if err := svc.repo.DB.Create(&media).Error; err != nil {
		t.Fatalf("create media: %v", err)
	}

	root, err := svc.Items(t.Context(), ItemsParams{ParentID: lib.ID, Limit: 50})
	if err != nil {
		t.Fatalf("library items: %v", err)
	}
	rootItems := root["Items"].([]map[string]any)
	if len(rootItems) != 1 || rootItems[0]["Type"] != "Series" {
		t.Fatalf("expected one series card, got %#v", rootItems)
	}

	seasons, err := svc.Items(t.Context(), ItemsParams{ParentID: rootItems[0]["Id"].(string), Limit: 50})
	if err != nil {
		t.Fatalf("series seasons: %v", err)
	}
	seasonItems := seasons["Items"].([]map[string]any)
	if len(seasonItems) != 1 || seasonItems[0]["Type"] != "Season" || seasonItems[0]["IndexNumber"] != 0 || seasonItems[0]["Name"] != "特别篇" {
		t.Fatalf("specials should be exposed as season zero: %#v", seasonItems)
	}

	episodes, err := svc.Items(t.Context(), ItemsParams{ParentID: seasonItems[0]["Id"].(string), IncludeItemTypes: []string{"Episode"}, Recursive: true, Limit: 50})
	if err != nil {
		t.Fatalf("special episodes: %v", err)
	}
	episodeItems := episodes["Items"].([]map[string]any)
	if len(episodeItems) != 1 {
		t.Fatalf("expected one special episode, got %#v", episodeItems)
	}
	if episodeItems[0]["ParentIndexNumber"] != 0 || episodeItems[0]["SeasonId"] != seasonItems[0]["Id"] || episodeItems[0]["ParentId"] != seasonItems[0]["Id"] {
		t.Fatalf("special episode linked to wrong season: %#v season=%#v", episodeItems[0], seasonItems[0])
	}
	wantPrimary := embyImageTag(media.ID, "primary", media.PosterURL, media.UpdatedAt)
	if tags, ok := episodeItems[0]["ImageTags"].(map[string]string); !ok || tags["Primary"] != wantPrimary {
		t.Fatalf("episode still should be exposed as Primary image: %#v", episodeItems[0]["ImageTags"])
	}
}

func TestEmbyEpisodeStillIsPrimaryImageNotArt(t *testing.T) {
	svc := newTestEmbyService(t)
	lib := model.Library{Name: "剧集", Path: `/media/tv`, Type: "tv", Enabled: true}
	if err := svc.repo.Library.Create(t.Context(), &lib); err != nil {
		t.Fatalf("create library: %v", err)
	}
	media := model.Media{
		Base:        model.Base{ID: "ep-still"},
		LibraryID:   lib.ID,
		Title:       "间谍过家家",
		Path:        `/media/tv/间谍过家家/Season 02/间谍过家家 - S02E01.mkv`,
		PosterURL:   `https://image.example/show-poster.jpg`,
		BackdropURL: `https://image.example/episode-still.jpg`,
		SeasonNum:   2,
		EpisodeNum:  1,
	}
	if err := svc.repo.DB.Create(&media).Error; err != nil {
		t.Fatalf("create media: %v", err)
	}

	item := svc.itemPayload(t.Context(), &media, false, 0)
	wantPrimary := embyImageTag(media.ID, "primary", media.BackdropURL, media.UpdatedAt)
	if tags, ok := item["ImageTags"].(map[string]string); !ok || tags["Primary"] != wantPrimary {
		t.Fatalf("episode should expose a primary image tag: %#v", item["ImageTags"])
	}
	wantBackdrop := embyImageTag(media.ID, "backdrop", media.BackdropURL, media.UpdatedAt)
	if tags, ok := item["BackdropImageTags"].([]string); !ok || len(tags) != 1 || tags[0] != wantBackdrop {
		t.Fatalf("episode backdrop tags = %#v, want own still %q", item["BackdropImageTags"], wantBackdrop)
	}
	primary, err := svc.ImageURL(t.Context(), "ep-still", "Primary")
	if err != nil {
		t.Fatalf("primary image url: %v", err)
	}
	if primary != media.BackdropURL {
		t.Fatalf("episode Primary image = %q, want still %q", primary, media.BackdropURL)
	}
	art, err := svc.ImageURL(t.Context(), "ep-still", "Art")
	if err != nil {
		t.Fatalf("art image url: %v", err)
	}
	if art != media.BackdropURL {
		t.Fatalf("episode Art image = %q, want still %q", art, media.BackdropURL)
	}
}

func TestEmbyVirtualSeriesArtworkUsesListCache(t *testing.T) {
	svc := newTestEmbyService(t)
	lib := model.Library{Name: "番剧", Path: `/media/anime`, Type: "anime", Enabled: true}
	if err := svc.repo.Library.Create(t.Context(), &lib); err != nil {
		t.Fatalf("create library: %v", err)
	}
	media := model.Media{
		Base:        model.Base{ID: "ep-1"},
		LibraryID:   lib.ID,
		Title:       "剑来",
		Path:        `/media/anime/剑来/Season 01/剑来 - S01E01.mkv`,
		PosterURL:   `/poster.jpg`,
		BackdropURL: `/backdrop.jpg`,
		SeasonNum:   1,
		EpisodeNum:  1,
	}
	if err := svc.repo.DB.Create(&media).Error; err != nil {
		t.Fatalf("create media: %v", err)
	}
	root, err := svc.Items(t.Context(), ItemsParams{ParentID: lib.ID, Limit: 50})
	if err != nil {
		t.Fatalf("library items: %v", err)
	}
	items := root["Items"].([]map[string]any)
	seriesID := items[0]["Id"].(string)

	cancelled, cancel := context.WithCancel(t.Context())
	cancel()
	poster, err := svc.ImageURL(cancelled, seriesID, "Primary")
	if err != nil {
		t.Fatalf("image url from cache: %v", err)
	}
	if poster != "/poster.jpg" {
		t.Fatalf("poster = %q, want cached poster", poster)
	}
	backdrop, err := svc.ImageURL(cancelled, seriesID, "Backdrop")
	if err != nil {
		t.Fatalf("backdrop url from cache: %v", err)
	}
	if backdrop != "/backdrop.jpg" {
		t.Fatalf("backdrop = %q, want cached backdrop", backdrop)
	}
}

func TestEmbySeriesNamePrefersScrapedTitleOverNoisyPath(t *testing.T) {
	svc := newTestEmbyService(t)
	lib := model.Library{Name: "TV", Path: `cloud://openlist/tv`, Type: "tv", Enabled: true}
	if err := svc.repo.Library.Create(t.Context(), &lib); err != nil {
		t.Fatalf("create library: %v", err)
	}
	media := model.Media{
		Base:       model.Base{ID: "ep-scraped-title"},
		LibraryID:  lib.ID,
		Title:      "罚罪2",
		Path:       `cloud://openlist/tv/FX S02/FX.S02E01.mkv`,
		SeasonNum:  2,
		EpisodeNum: 1,
	}
	if err := svc.repo.DB.Create(&media).Error; err != nil {
		t.Fatalf("create media: %v", err)
	}

	root, err := svc.Items(t.Context(), ItemsParams{ParentID: lib.ID, Limit: 50})
	if err != nil {
		t.Fatalf("library items: %v", err)
	}
	items := root["Items"].([]map[string]any)
	if len(items) != 1 || items[0]["Name"] != "罚罪2" {
		t.Fatalf("series card should prefer scraped title, got %#v", items)
	}
}

func TestEmbySeriesNameNormalizesReleaseNoiseAcrossSeasons(t *testing.T) {
	svc := newTestEmbyService(t)
	lib := model.Library{Name: "TV", Path: `cloud://openlist/tv`, Type: "tv", Enabled: true}
	if err := svc.repo.Library.Create(t.Context(), &lib); err != nil {
		t.Fatalf("create library: %v", err)
	}
	rows := []model.Media{
		{
			Base:       model.Base{ID: "orville-s1e1"},
			LibraryID:  lib.ID,
			Title:      "【高清剧集网发布 www bphdtv com】奥维尔号 the orville",
			Path:       `cloud://openlist/tv/orville-s1/Orville.S01E01.mkv`,
			SeasonNum:  1,
			EpisodeNum: 1,
		},
		{
			Base:       model.Base{ID: "orville-s2e1"},
			LibraryID:  lib.ID,
			Title:      "奥维尔号",
			Path:       `cloud://openlist/tv/orville-s2/Orville.S02E01.mkv`,
			SeasonNum:  2,
			EpisodeNum: 1,
		},
	}
	for _, media := range rows {
		if err := svc.repo.DB.Create(&media).Error; err != nil {
			t.Fatalf("create media: %v", err)
		}
	}

	root, err := svc.Items(t.Context(), ItemsParams{ParentID: lib.ID, Limit: 50})
	if err != nil {
		t.Fatalf("library items: %v", err)
	}
	items := root["Items"].([]map[string]any)
	if len(items) != 1 || items[0]["Name"] != "奥维尔号" || items[0]["RecursiveItemCount"] != 2 {
		t.Fatalf("orville seasons should be grouped as one normalized series, got %#v", items)
	}
}

func TestEmbySeriesNameNormalizesTotalEpisodeSuffix(t *testing.T) {
	svc := newTestEmbyService(t)
	lib := model.Library{Name: "TV", Path: `cloud://openlist/tv`, Type: "tv", Enabled: true}
	if err := svc.repo.Library.Create(t.Context(), &lib); err != nil {
		t.Fatalf("create library: %v", err)
	}
	rows := []model.Media{
		{
			Base:       model.Base{ID: "fz2-e01"},
			LibraryID:  lib.ID,
			Title:      "\u7f5a\u7f6a2 \u516840\u96c6",
			Path:       `cloud://openlist/tv/fazui2/fazui2.E01.mkv`,
			SeasonNum:  1,
			EpisodeNum: 1,
		},
		{
			Base:       model.Base{ID: "fz2-e40"},
			LibraryID:  lib.ID,
			Title:      "\u7f5a\u7f6a2",
			Path:       `cloud://openlist/tv/fazui2/fazui2.E40.mkv`,
			SeasonNum:  1,
			EpisodeNum: 40,
		},
	}
	for _, media := range rows {
		if err := svc.repo.DB.Create(&media).Error; err != nil {
			t.Fatalf("create media: %v", err)
		}
	}

	root, err := svc.Items(t.Context(), ItemsParams{ParentID: lib.ID, Limit: 50})
	if err != nil {
		t.Fatalf("library items: %v", err)
	}
	items := root["Items"].([]map[string]any)
	if len(items) != 1 || items[0]["Name"] != "\u7f5a\u7f6a2" || items[0]["RecursiveItemCount"] != 2 {
		t.Fatalf("total episode suffix should not split one series, got %#v", items)
	}
}

func TestEmbyCloudAnimeUsesSeriesNameFromChineseSeasonFolder(t *testing.T) {
	svc := newTestEmbyService(t)
	lib := model.Library{Name: "OpenList · 国漫", Path: `cloud://openlist/国漫`, Type: "anime", Enabled: true}
	if err := svc.repo.Library.Create(t.Context(), &lib); err != nil {
		t.Fatalf("create library: %v", err)
	}
	for _, media := range []model.Media{
		{
			Base:       model.Base{ID: "cloud-ep-1"},
			LibraryID:  lib.ID,
			Title:      "04",
			Path:       `cloud://openlist/国漫/剑来/第二季/04.mkv`,
			SeasonNum:  2,
			EpisodeNum: 4,
		},
		{
			Base:       model.Base{ID: "cloud-ep-2"},
			LibraryID:  lib.ID,
			Title:      "05",
			Path:       `cloud://openlist/国漫/剑来/第二季/05.mkv`,
			SeasonNum:  2,
			EpisodeNum: 5,
		},
	} {
		if err := svc.repo.DB.Create(&media).Error; err != nil {
			t.Fatalf("create media: %v", err)
		}
	}

	root, err := svc.Items(t.Context(), ItemsParams{ParentID: lib.ID, Limit: 50})
	if err != nil {
		t.Fatalf("library items: %v", err)
	}
	items := root["Items"].([]map[string]any)
	if len(items) != 1 || items[0]["Type"] != "Series" || items[0]["Name"] != "剑来" {
		t.Fatalf("cloud anime should be grouped as one series named 剑来, got %#v", items)
	}
}
