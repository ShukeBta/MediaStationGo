package repository

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/glebarez/sqlite"
	"gorm.io/gorm"

	"github.com/ShukeBta/MediaStationGo/internal/database"
	"github.com/ShukeBta/MediaStationGo/internal/model"
)

func TestMediaUpsertSkipsUnchangedExistingRow(t *testing.T) {
	db, err := gorm.Open(sqlite.Open(":memory:"), &gorm.Config{})
	if err != nil {
		t.Fatal(err)
	}
	if err := database.AutoMigrate(db); err != nil {
		t.Fatalf("migrate: %v", err)
	}
	repos := New(db)
	lib := model.Library{Name: "电影", Path: "/media/movie", Type: "movie", Enabled: true}
	if err := repos.Library.Create(t.Context(), &lib); err != nil {
		t.Fatal(err)
	}
	media := model.Media{
		LibraryID:    lib.ID,
		Title:        "已有影片",
		Path:         "/media/movie/existing.mkv",
		SizeBytes:    1024,
		DurationSec:  60,
		Width:        1920,
		Height:       1080,
		VideoCodec:   "h264",
		AudioCodec:   "aac",
		Container:    "matroska,webm",
		ScrapeStatus: "pending",
	}
	if err := repos.Media.Upsert(t.Context(), &media); err != nil {
		t.Fatal(err)
	}
	var before model.Media
	if err := repos.DB.Where("path = ?", media.Path).First(&before).Error; err != nil {
		t.Fatal(err)
	}
	time.Sleep(10 * time.Millisecond)
	again := model.Media{
		LibraryID:    lib.ID,
		Title:        before.Title,
		Path:         before.Path,
		SizeBytes:    before.SizeBytes,
		DurationSec:  before.DurationSec,
		Width:        before.Width,
		Height:       before.Height,
		VideoCodec:   before.VideoCodec,
		AudioCodec:   before.AudioCodec,
		Container:    before.Container,
		ScrapeStatus: before.ScrapeStatus,
	}
	if err := repos.Media.Upsert(t.Context(), &again); err != nil {
		t.Fatal(err)
	}
	var after model.Media
	if err := repos.DB.Where("path = ?", media.Path).First(&after).Error; err != nil {
		t.Fatal(err)
	}
	if !after.UpdatedAt.Equal(before.UpdatedAt) {
		t.Fatalf("unchanged upsert touched updated_at: before=%s after=%s", before.UpdatedAt, after.UpdatedAt)
	}
}

func TestMediaUpsertRefreshesCloudExternalIDFromPathHint(t *testing.T) {
	db, err := gorm.Open(sqlite.Open(":memory:"), &gorm.Config{})
	if err != nil {
		t.Fatal(err)
	}
	if err := database.AutoMigrate(db); err != nil {
		t.Fatalf("migrate: %v", err)
	}
	repos := New(db)
	lib := model.Library{Name: "OpenList · 国产剧", Path: "cloud://openlist/国产剧", Type: "tv", Enabled: true}
	if err := repos.Library.Create(t.Context(), &lib); err != nil {
		t.Fatal(err)
	}
	path := "cloud://openlist/国产剧/折腰 (2025) {tmdb-296753}/Season 1/折腰.S01E01.mkv"
	existing := model.Media{
		LibraryID:    lib.ID,
		Title:        "折腰",
		Path:         path,
		SeasonNum:    1,
		EpisodeNum:   1,
		TMDbID:       220269,
		ScrapeStatus: "matched",
	}
	if err := repos.Media.Upsert(t.Context(), &existing); err != nil {
		t.Fatal(err)
	}
	next := model.Media{
		LibraryID:  lib.ID,
		Title:      "折腰",
		Path:       path,
		SeasonNum:  1,
		EpisodeNum: 1,
		TMDbID:     296753,
	}
	if err := repos.Media.Upsert(t.Context(), &next); err != nil {
		t.Fatal(err)
	}
	var got model.Media
	if err := repos.DB.Where("path = ?", path).First(&got).Error; err != nil {
		t.Fatal(err)
	}
	if got.TMDbID != 296753 || got.ScrapeStatus != "pending" {
		t.Fatalf("cloud path hint should refresh tmdb and retry scrape, got tmdb=%d status=%q", got.TMDbID, got.ScrapeStatus)
	}
}

func TestMediaUpsertMatchedIncomingRefreshesScrapedMetadata(t *testing.T) {
	db, err := gorm.Open(sqlite.Open(":memory:"), &gorm.Config{})
	if err != nil {
		t.Fatal(err)
	}
	if err := database.AutoMigrate(db); err != nil {
		t.Fatalf("migrate: %v", err)
	}
	repos := New(db)
	lib := model.Library{Name: "剧集", Path: "/media/tv", Type: "tv", Enabled: true}
	if err := repos.Library.Create(t.Context(), &lib); err != nil {
		t.Fatal(err)
	}
	path := "/media/tv/show/S01E01.mkv"
	existing := model.Media{
		LibraryID:    lib.ID,
		Title:        "扫描标题",
		Path:         path,
		ScrapeStatus: "no_match",
		PosterURL:    "/old-poster.jpg",
	}
	if err := repos.Media.Upsert(t.Context(), &existing); err != nil {
		t.Fatal(err)
	}
	incoming := model.Media{
		LibraryID:    lib.ID,
		Title:        "中文剧名",
		OriginalName: "Original Show",
		EpisodeTitle: "第一集",
		Path:         path,
		PosterURL:    "/poster.jpg",
		BackdropURL:  "/backdrop.jpg",
		Overview:     "剧情简介",
		Rating:       8.6,
		Year:         2026,
		SeasonNum:    1,
		EpisodeNum:   1,
		ScrapeStatus: "matched",
		TMDbID:       123,
		BangumiID:    456,
		DoubanID:     "db-1",
		TheTVDBID:    "tvdb-1",
		Languages:    "zh,en",
		Countries:    "CN",
		Genres:       "剧情,悬疑",
		NSFW:         true,
	}
	if err := repos.Media.Upsert(t.Context(), &incoming); err != nil {
		t.Fatal(err)
	}
	var got model.Media
	if err := repos.DB.Where("path = ?", path).First(&got).Error; err != nil {
		t.Fatal(err)
	}
	if got.Title != "中文剧名" || got.OriginalName != "Original Show" || got.EpisodeTitle != "第一集" {
		t.Fatalf("matched names not refreshed: %#v", got)
	}
	if got.PosterURL != "/poster.jpg" || got.BackdropURL != "/backdrop.jpg" || got.Overview != "剧情简介" {
		t.Fatalf("matched artwork/overview not refreshed: %#v", got)
	}
	if got.ScrapeStatus != "matched" || got.TMDbID != 123 || got.BangumiID != 456 || got.DoubanID != "db-1" || got.TheTVDBID != "tvdb-1" {
		t.Fatalf("matched provider metadata not refreshed: %#v", got)
	}
	if got.Year != 2026 || got.SeasonNum != 1 || got.EpisodeNum != 1 || got.Rating != 8.6 || got.Languages != "zh,en" || got.Countries != "CN" || got.Genres != "剧情,悬疑" || !got.NSFW {
		t.Fatalf("matched detail metadata not refreshed: %#v", got)
	}
}

func TestMediaUpsertPromotesNSFWWithoutMatchedMetadata(t *testing.T) {
	db, err := gorm.Open(sqlite.Open(":memory:"), &gorm.Config{})
	if err != nil {
		t.Fatal(err)
	}
	if err := database.AutoMigrate(db); err != nil {
		t.Fatalf("migrate: %v", err)
	}
	repos := New(db)
	path := "/media/adult/ABF-363.mp4"
	existing := model.Media{LibraryID: "adult", Title: "ABF-363", Path: path, ScrapeStatus: "pending"}
	if err := repos.Media.Upsert(t.Context(), &existing); err != nil {
		t.Fatal(err)
	}
	incoming := model.Media{LibraryID: "adult", Title: "ABF-363", Path: path, ScrapeStatus: "pending", NSFW: true}
	if err := repos.Media.Upsert(t.Context(), &incoming); err != nil {
		t.Fatal(err)
	}
	var got model.Media
	if err := repos.DB.Where("path = ?", path).First(&got).Error; err != nil {
		t.Fatal(err)
	}
	if !got.NSFW {
		t.Fatal("scanner upsert must be able to promote NSFW before metadata is matched")
	}
}

func TestListByLibraryOrdersByReleaseDate(t *testing.T) {
	db, err := gorm.Open(sqlite.Open(":memory:"), &gorm.Config{})
	if err != nil {
		t.Fatal(err)
	}
	if err := database.AutoMigrate(db); err != nil {
		t.Fatalf("migrate: %v", err)
	}
	repos := New(db)
	lib := model.Library{Name: "国产剧", Path: "/media/tv", Type: "tv", Enabled: true}
	if err := repos.Library.Create(t.Context(), &lib); err != nil {
		t.Fatal(err)
	}
	rows := []model.Media{
		{LibraryID: lib.ID, Title: "旧片但最近更新", Path: "/media/tv/old.mkv", Year: 2026, ReleaseDate: "2026-01-10", ScrapeStatus: "matched"},
		{LibraryID: lib.ID, Title: "最新首播", Path: "/media/tv/newest.mkv", Year: 2026, ReleaseDate: "2026-06-23", ScrapeStatus: "matched"},
		{LibraryID: lib.ID, Title: "无完整日期", Path: "/media/tv/year-only.mkv", Year: 2025, ScrapeStatus: "matched"},
	}
	for i := range rows {
		if err := repos.Media.Upsert(t.Context(), &rows[i]); err != nil {
			t.Fatal(err)
		}
	}

	items, total, err := repos.Media.ListByLibrary(t.Context(), lib.ID, 0, 10)
	if err != nil {
		t.Fatal(err)
	}
	if total != 3 || len(items) != 3 {
		t.Fatalf("list total=%d len=%d", total, len(items))
	}
	if got := []string{items[0].Title, items[1].Title, items[2].Title}; got[0] != "最新首播" || got[1] != "旧片但最近更新" || got[2] != "无完整日期" {
		t.Fatalf("release-date order = %#v", got)
	}
}

func TestMediaUpsertScanDoesNotClearMatchedMetadata(t *testing.T) {
	db, err := gorm.Open(sqlite.Open(":memory:"), &gorm.Config{})
	if err != nil {
		t.Fatal(err)
	}
	if err := database.AutoMigrate(db); err != nil {
		t.Fatalf("migrate: %v", err)
	}
	repos := New(db)
	lib := model.Library{Name: "剧集", Path: "/media/tv", Type: "tv", Enabled: true}
	if err := repos.Library.Create(t.Context(), &lib); err != nil {
		t.Fatal(err)
	}
	path := "/media/tv/间谍过家家/Season 01/间谍过家家 - S01E01.mkv"
	existing := model.Media{
		LibraryID:    lib.ID,
		Title:        "间谍过家家",
		OriginalName: "SPY×FAMILY",
		Path:         path,
		PosterURL:    "/poster.jpg",
		BackdropURL:  "/backdrop.jpg",
		Overview:     "剧情简介",
		Year:         2022,
		SeasonNum:    1,
		EpisodeNum:   1,
		ScrapeStatus: "matched",
		TMDbID:       12345,
		BangumiID:    67890,
		DoubanID:     "db-spy",
		TheTVDBID:    "tvdb-spy",
	}
	if err := repos.Media.Upsert(t.Context(), &existing); err != nil {
		t.Fatal(err)
	}
	scan := model.Media{
		LibraryID:   lib.ID,
		Title:       "Spy.x.Family.S01E01.2022.1080p.WEB-DL",
		Path:        path,
		SizeBytes:   2048,
		DurationSec: 1500,
		Width:       1920,
		Height:      1080,
		VideoCodec:  "h264",
		AudioCodec:  "aac",
		Container:   "mkv",
		SeasonNum:   1,
		EpisodeNum:  1,
	}
	if err := repos.Media.Upsert(t.Context(), &scan); err != nil {
		t.Fatal(err)
	}

	var got model.Media
	if err := repos.DB.Where("path = ?", path).First(&got).Error; err != nil {
		t.Fatal(err)
	}
	if got.Title != "间谍过家家" || got.OriginalName != "SPY×FAMILY" || got.ScrapeStatus != "matched" {
		t.Fatalf("matched names/status were overwritten by scan: %#v", got)
	}
	if got.TMDbID != 12345 || got.BangumiID != 67890 || got.DoubanID != "db-spy" || got.TheTVDBID != "tvdb-spy" {
		t.Fatalf("matched provider ids were cleared by scan: %#v", got)
	}
	if got.PosterURL != "/poster.jpg" || got.BackdropURL != "/backdrop.jpg" || got.Overview != "剧情简介" {
		t.Fatalf("matched artwork/overview were overwritten by scan: %#v", got)
	}
	if got.SizeBytes != 2048 || got.DurationSec != 1500 || got.Width != 1920 || got.Height != 1080 || got.Container != "mkv" {
		t.Fatalf("file scan fields were not refreshed: %#v", got)
	}
	if scan.ID != got.ID || scan.Title != got.Title || scan.TMDbID != got.TMDbID || scan.ScrapeStatus != "matched" {
		t.Fatalf("upsert caller did not receive fresh matched row: %#v want %#v", scan, got)
	}
}

func TestMediaUpsertClearsGeneratedArtworkWhenSourceSizeChanges(t *testing.T) {
	db, err := gorm.Open(sqlite.Open(":memory:"), &gorm.Config{})
	if err != nil {
		t.Fatal(err)
	}
	if err := database.AutoMigrate(db); err != nil {
		t.Fatalf("migrate: %v", err)
	}
	repos := New(db)
	path := "cloud://openlist/115/其他/video.mkv"
	existing := model.Media{
		LibraryID:                "lib-other",
		Title:                    "video",
		Path:                     path,
		SizeBytes:                1024,
		GeneratedPosterURL:       "/data/generated-artwork/media/primary.jpg",
		GeneratedBackdropURL:     "/data/generated-artwork/media/backdrop.jpg",
		GeneratedArtworkHash:     "old-hash",
		GeneratedArtworkStatus:   "completed",
		GeneratedArtworkError:    "old error",
		GeneratedArtworkAttempts: 2,
	}
	if err := repos.Media.Upsert(t.Context(), &existing); err != nil {
		t.Fatal(err)
	}
	if err := repos.Media.Upsert(t.Context(), &model.Media{LibraryID: existing.LibraryID, Title: existing.Title, Path: path, SizeBytes: 2048}); err != nil {
		t.Fatal(err)
	}
	var got model.Media
	if err := repos.DB.Where("path = ?", path).First(&got).Error; err != nil {
		t.Fatal(err)
	}
	if got.GeneratedPosterURL != "" || got.GeneratedBackdropURL != "" || got.GeneratedArtworkHash != "" || got.GeneratedArtworkStatus != "" || got.GeneratedArtworkError != "" || got.GeneratedArtworkAttempts != 0 {
		t.Fatalf("generated artwork was not invalidated: %#v", got)
	}
}

// TestMediaUpsertMigratesCloudLibraryIDOnRescan 复现"一键挂载子目录后媒体消失"的
// 回归：同一 cloud:// 文件先被父目录库扫描入库，之后用户按二级分类重新挂载到更
// 精确的分类库并扫描，library_id 必须迁移到新分类库，否则媒体被钉死在旧库、新库
// 视图里看不到。本地媒体物理位置固定，不参与迁移。
func TestMediaUpsertMigratesCloudLibraryIDOnRescan(t *testing.T) {
	db, err := gorm.Open(sqlite.Open(":memory:"), &gorm.Config{})
	if err != nil {
		t.Fatal(err)
	}
	if err := database.AutoMigrate(db); err != nil {
		t.Fatalf("migrate: %v", err)
	}
	repos := New(db)
	parent := model.Library{Name: "OpenList", Path: "cloud://openlist", Type: "movie", Enabled: true}
	if err := repos.Library.Create(t.Context(), &parent); err != nil {
		t.Fatal(err)
	}
	category := model.Library{Name: "国漫", Path: "cloud://openlist/国漫", Type: "anime", Enabled: true}
	if err := repos.Library.Create(t.Context(), &category); err != nil {
		t.Fatal(err)
	}
	path := "cloud://openlist/国漫/成何体统 (2024) {tmdb-256783}/Season 1/成何体统.S01E01.mkv"
	// 第一次：被父目录库扫描入库。
	first := model.Media{LibraryID: parent.ID, Title: "成何体统", Path: path, SeasonNum: 1, EpisodeNum: 1}
	if err := repos.Media.Upsert(t.Context(), &first); err != nil {
		t.Fatal(err)
	}
	// 第二次：按二级分类重新挂载并扫描，归入更精确的「国漫」库。
	second := model.Media{LibraryID: category.ID, Title: "成何体统", Path: path, SeasonNum: 1, EpisodeNum: 1}
	if err := repos.Media.Upsert(t.Context(), &second); err != nil {
		t.Fatal(err)
	}
	var got model.Media
	if err := repos.DB.Where("path = ?", path).First(&got).Error; err != nil {
		t.Fatal(err)
	}
	if got.LibraryID != category.ID {
		t.Fatalf("cloud media library_id should migrate to category library %q, got %q", category.ID, got.LibraryID)
	}

	// 本地媒体不迁移：同一物理路径不应改库归属。
	localA := model.Library{Name: "Movies A", Path: "/media/a", Type: "movie", Enabled: true}
	localB := model.Library{Name: "Movies B", Path: "/media/b", Type: "movie", Enabled: true}
	if err := repos.Library.Create(t.Context(), &localA); err != nil {
		t.Fatal(err)
	}
	if err := repos.Library.Create(t.Context(), &localB); err != nil {
		t.Fatal(err)
	}
	localPath := "/media/a/Inception (2010)/Inception.mkv"
	if err := repos.Media.Upsert(t.Context(), &model.Media{LibraryID: localA.ID, Title: "Inception", Path: localPath}); err != nil {
		t.Fatal(err)
	}
	if err := repos.Media.Upsert(t.Context(), &model.Media{LibraryID: localB.ID, Title: "Inception", Path: localPath}); err != nil {
		t.Fatal(err)
	}
	var localGot model.Media
	if err := repos.DB.Where("path = ?", localPath).First(&localGot).Error; err != nil {
		t.Fatal(err)
	}
	if localGot.LibraryID != localA.ID {
		t.Fatalf("local media library_id must not migrate, want %q got %q", localA.ID, localGot.LibraryID)
	}

	// 重新创建同路径本地库：旧库已删除但媒体行残留时，扫描应把媒体接回新库。
	deletedLocal := model.Library{Name: "Old Local", Path: "/media", Type: "movie", Enabled: true}
	if err := repos.Library.Create(t.Context(), &deletedLocal); err != nil {
		t.Fatal(err)
	}
	recreatedLocal := model.Library{Name: "New Local", Path: "/media", Type: "movie", Enabled: true}
	if err := repos.Library.Create(t.Context(), &recreatedLocal); err != nil {
		t.Fatal(err)
	}
	reattachPath := "/media/Videos/EMBZ-220.mp4"
	if err := repos.Media.Upsert(t.Context(), &model.Media{LibraryID: deletedLocal.ID, Title: "EMBZ-220", Path: reattachPath}); err != nil {
		t.Fatal(err)
	}
	if err := repos.Library.Delete(t.Context(), deletedLocal.ID); err != nil {
		t.Fatal(err)
	}
	if err := repos.Media.Upsert(t.Context(), &model.Media{LibraryID: recreatedLocal.ID, Title: "EMBZ-220", Path: reattachPath}); err != nil {
		t.Fatal(err)
	}
	var reattached model.Media
	if err := repos.DB.Where("path = ?", reattachPath).First(&reattached).Error; err != nil {
		t.Fatal(err)
	}
	if reattached.LibraryID != recreatedLocal.ID {
		t.Fatalf("local media should migrate from deleted library to recreated library %q, got %q", recreatedLocal.ID, reattached.LibraryID)
	}
}

func TestMediaUpsertKeepsSoftDeletedCloudMediaHidden(t *testing.T) {
	db, err := gorm.Open(sqlite.Open(":memory:"), &gorm.Config{})
	if err != nil {
		t.Fatal(err)
	}
	if err := database.AutoMigrate(db); err != nil {
		t.Fatalf("migrate: %v", err)
	}
	repos := New(db)
	path := "cloud://openlist/Movies/Hidden.mkv"
	media := model.Media{
		Base:         model.Base{ID: "hidden-cloud"},
		LibraryID:    "lib-old",
		Title:        "Hidden",
		Path:         path,
		ScrapeStatus: "matched",
	}
	if err := repos.DB.Create(&media).Error; err != nil {
		t.Fatal(err)
	}
	if err := repos.DB.Delete(&model.Media{}, "id = ?", media.ID).Error; err != nil {
		t.Fatal(err)
	}

	incoming := model.Media{
		LibraryID:    "lib-new",
		Title:        "Hidden Rescan",
		Path:         path,
		SizeBytes:    2048,
		ScrapeStatus: "pending",
	}
	err = repos.Media.Upsert(t.Context(), &incoming)
	if !errors.Is(err, ErrMediaHiddenByUser) {
		t.Fatalf("upsert error = %v, want ErrMediaHiddenByUser", err)
	}
	var got model.Media
	if err := repos.DB.Unscoped().Where("path = ?", path).First(&got).Error; err != nil {
		t.Fatal(err)
	}
	if !got.DeletedAt.Valid {
		t.Fatalf("soft-deleted cloud row was restored: %#v", got)
	}
	if got.LibraryID != "lib-old" || got.Title != "Hidden" || got.SizeBytes != 0 {
		t.Fatalf("hidden cloud row was mutated by rescan: %#v", got)
	}
}

type fakeMediaSearchBackend struct {
	ids []string
	err error
}

func (f fakeMediaSearchBackend) SearchMediaIDs(context.Context, string, int, int, MediaQueryFilter) ([]string, int64, error) {
	if f.err != nil {
		return nil, 0, f.err
	}
	return append([]string(nil), f.ids...), int64(len(f.ids)), nil
}

func TestMediaSearchUsesExternalBackendAndFallsBack(t *testing.T) {
	db, err := gorm.Open(sqlite.Open(":memory:"), &gorm.Config{})
	if err != nil {
		t.Fatal(err)
	}
	if err := database.AutoMigrate(db); err != nil {
		t.Fatalf("migrate: %v", err)
	}
	repos := New(db)
	lib := model.Library{Name: "Movies", Path: "/media/movie", Type: "movie", Enabled: true}
	if err := repos.Library.Create(t.Context(), &lib); err != nil {
		t.Fatal(err)
	}
	for _, row := range []model.Media{
		{Base: model.Base{ID: "m-1"}, LibraryID: lib.ID, Title: "Alpha", Path: "/media/a.mkv"},
		{Base: model.Base{ID: "m-2"}, LibraryID: lib.ID, Title: "Beta", Path: "/media/b.mkv"},
	} {
		if err := repos.DB.Create(&row).Error; err != nil {
			t.Fatal(err)
		}
	}
	repos.Media.SetSearchBackend(fakeMediaSearchBackend{ids: []string{"m-2", "m-1"}})
	items, total, err := repos.Media.SearchFilteredPage(t.Context(), "anything", 0, 10, MediaQueryFilter{IncludeNSFW: true})
	if err != nil {
		t.Fatal(err)
	}
	if total != 2 || len(items) != 2 || items[0].ID != "m-2" || items[1].ID != "m-1" {
		t.Fatalf("external search result total=%d items=%#v", total, items)
	}

	repos.Media.SetSearchBackend(fakeMediaSearchBackend{err: errors.New("opensearch down")})
	items, total, err = repos.Media.SearchFilteredPage(t.Context(), "Alpha", 0, 10, MediaQueryFilter{IncludeNSFW: true})
	if err != nil {
		t.Fatal(err)
	}
	if total != 1 || len(items) != 1 || items[0].ID != "m-1" {
		t.Fatalf("fallback result total=%d items=%#v", total, items)
	}
}

func TestMediaSearchFilteredSupportsChineseFuzzyTerms(t *testing.T) {
	db, err := gorm.Open(sqlite.Open(":memory:"), &gorm.Config{})
	if err != nil {
		t.Fatal(err)
	}
	if err := database.AutoMigrate(db); err != nil {
		t.Fatalf("migrate: %v", err)
	}
	repos := New(db)
	lib := model.Library{Name: "国产剧", Path: "/media/国产剧", Type: "tv", Enabled: true}
	if err := repos.Library.Create(t.Context(), &lib); err != nil {
		t.Fatalf("create library: %v", err)
	}
	rows := []model.Media{
		{
			Base:         model.Base{ID: "m-ferry"},
			LibraryID:    lib.ID,
			Title:        "灵魂摆渡·十年",
			OriginalName: "The Ferry Man 10th Anniversary",
			Path:         "/media/国产剧/灵魂摆渡·十年/S01E01.mkv",
			Genres:       "悬疑,奇幻",
		},
		{
			Base:         model.Base{ID: "m-ashes"},
			LibraryID:    lib.ID,
			Title:        "翘楚",
			OriginalName: "Ashes to Crown",
			Path:         "/media/国产剧/翘楚/S01E01.mkv",
			Genres:       "剧情",
		},
	}
	for i := range rows {
		if err := repos.Media.Upsert(t.Context(), &rows[i]); err != nil {
			t.Fatalf("upsert media: %v", err)
		}
	}

	items, err := repos.Media.SearchFiltered(t.Context(), "灵魂 十年", 10, MediaQueryFilter{IncludeNSFW: true})
	if err != nil {
		t.Fatalf("search chinese terms: %v", err)
	}
	if len(items) == 0 || items[0].ID != "m-ferry" {
		t.Fatalf("chinese fuzzy search missed target: %#v", items)
	}

	items, err = repos.Media.SearchFiltered(t.Context(), "Ferry", 10, MediaQueryFilter{IncludeNSFW: true})
	if err != nil {
		t.Fatalf("search original name: %v", err)
	}
	if len(items) == 0 || items[0].ID != "m-ferry" {
		t.Fatalf("original-name search missed target: %#v", items)
	}

	items, err = repos.Media.SearchFiltered(t.Context(), "悬疑", 10, MediaQueryFilter{IncludeNSFW: true})
	if err != nil {
		t.Fatalf("search genre: %v", err)
	}
	if len(items) == 0 || items[0].ID != "m-ferry" {
		t.Fatalf("genre search missed target: %#v", items)
	}
}

func TestMediaSearchFilteredMatchesCaseInsensitiveTermsAcrossFields(t *testing.T) {
	db, err := gorm.Open(sqlite.Open(":memory:"), &gorm.Config{})
	if err != nil {
		t.Fatal(err)
	}
	if err := database.AutoMigrate(db); err != nil {
		t.Fatalf("migrate: %v", err)
	}
	repos := New(db)
	lib := model.Library{Name: "TV", Path: "/media/tv", Type: "tv", Enabled: true}
	if err := repos.Library.Create(t.Context(), &lib); err != nil {
		t.Fatalf("create library: %v", err)
	}
	rows := []model.Media{
		{
			Base:         model.Base{ID: "taxi"},
			LibraryID:    lib.ID,
			Title:        "Taxi Driver",
			OriginalName: "Model Taxi",
			Path:         "/media/tv/taxi/Season2/S02E01.mkv",
			RelativePath: "taxi/Season2/S02E01.mkv",
			Overview:     "A detective pilot case",
			Genres:       "Crime,Action",
		},
		{
			Base:         model.Base{ID: "dune"},
			LibraryID:    lib.ID,
			Title:        "Dune",
			OriginalName: "DUNE PART TWO",
			Path:         "/media/movie/dune.mkv",
		},
	}
	for i := range rows {
		if err := repos.Media.Upsert(t.Context(), &rows[i]); err != nil {
			t.Fatalf("upsert media: %v", err)
		}
	}

	items, err := repos.Media.SearchFiltered(t.Context(), "TAXI season2", 10, MediaQueryFilter{IncludeNSFW: true})
	if err != nil {
		t.Fatalf("search mixed-case split terms: %v", err)
	}
	if len(items) != 1 || items[0].ID != "taxi" {
		t.Fatalf("mixed-case split search should match taxi only: %#v", items)
	}

	items, err = repos.Media.SearchFiltered(t.Context(), "pilot crime", 10, MediaQueryFilter{IncludeNSFW: true})
	if err != nil {
		t.Fatalf("search overview and genre terms: %v", err)
	}
	if len(items) != 1 || items[0].ID != "taxi" {
		t.Fatalf("overview/genre split search should match taxi only: %#v", items)
	}

	items, err = repos.Media.SearchFiltered(t.Context(), "dune part", 10, MediaQueryFilter{IncludeNSFW: true})
	if err != nil {
		t.Fatalf("search original name terms: %v", err)
	}
	if len(items) != 1 || items[0].ID != "dune" {
		t.Fatalf("original-name split search should match dune only: %#v", items)
	}

	items, err = repos.Media.SearchFiltered(t.Context(), "taxi dune", 10, MediaQueryFilter{IncludeNSFW: true})
	if err != nil {
		t.Fatalf("search unrelated combined terms: %v", err)
	}
	if len(items) != 0 {
		t.Fatalf("split search should require every term, got %#v", items)
	}
}

func TestMediaSearchIndexBackfillRunsInBatches(t *testing.T) {
	db, err := gorm.Open(sqlite.Open(":memory:"), &gorm.Config{})
	if err != nil {
		t.Fatal(err)
	}
	if err := database.AutoMigrate(db); err != nil {
		t.Fatalf("migrate: %v", err)
	}
	repos := New(db)
	lib := model.Library{Name: "电影", Path: "/media/movie", Type: "movie", Enabled: true}
	if err := repos.Library.Create(t.Context(), &lib); err != nil {
		t.Fatal(err)
	}
	if err := repos.DB.Create(&model.Media{
		Base:      model.Base{ID: "m-backfill"},
		LibraryID: lib.ID,
		Title:     "后台索引",
		Path:      "/media/movie/后台索引.mkv",
	}).Error; err != nil {
		t.Fatal(err)
	}
	// 插入触发器应当同步维护 FTS 行。
	var indexed int64
	if err := repos.DB.Raw(`SELECT COUNT(*) FROM media_search_fts`).Scan(&indexed).Error; err != nil {
		t.Fatal(err)
	}
	if indexed != 1 {
		t.Fatalf("insert trigger should index new media, got %d rows", indexed)
	}
	// 清空 FTS 模拟旧库升级后索引缺失，回填应按批补齐且 rowid 对齐。
	if err := repos.DB.Exec(`DELETE FROM media_search_fts`).Error; err != nil {
		t.Fatal(err)
	}
	n, err := repos.Media.BackfillSearchIndex(t.Context(), 1)
	if err != nil {
		t.Fatal(err)
	}
	if n != 1 {
		t.Fatalf("backfilled rows = %d, want 1", n)
	}
	var aligned int64
	if err := repos.DB.Raw(`SELECT COUNT(*) FROM media_search_fts f JOIN media m ON f.rowid = m.rowid AND f.media_id = m.id`).Scan(&aligned).Error; err != nil {
		t.Fatal(err)
	}
	if aligned != 1 {
		t.Fatalf("fts rows aligned with media rowid = %d, want 1", aligned)
	}
	// 软删除后触发器应清理对应 FTS 行，避免搜索命中已删媒体。
	if err := repos.DB.Delete(&model.Media{}, "id = ?", "m-backfill").Error; err != nil {
		t.Fatal(err)
	}
	var after int64
	if err := repos.DB.Raw(`SELECT COUNT(*) FROM media_search_fts`).Scan(&after).Error; err != nil {
		t.Fatal(err)
	}
	if after != 0 {
		t.Fatalf("soft delete should drop fts row, got %d", after)
	}
}
