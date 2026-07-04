package service

import (
	"testing"

	"go.uber.org/zap"
	"gorm.io/gorm"

	"github.com/ShukeBta/MediaStationGo/internal/model"
	"github.com/ShukeBta/MediaStationGo/internal/repository"
)

func newCleanupTestContainer(t *testing.T) (*Container, *gorm.DB) {
	t.Helper()
	db := newServiceTestDB(t, &model.Media{}, &model.Setting{})
	if sqlDB, err := db.DB(); err == nil {
		sqlDB.SetMaxOpenConns(1)
	}
	repos := repository.New(db)
	return &Container{Repo: repos, Log: zap.NewNop()}, db
}

func TestShowDirFromEpisodePath(t *testing.T) {
	cases := []struct{ in, want string }{
		{`/tv/国漫/遮天 (2023)/Season 01/遮天 - S01E01.mkv`, `/tv/国漫/遮天 (2023)`},
		{`cloud://drive/国漫/武神主宰 (2020)/S01/武神主宰 - S01E05.mkv`, `cloud://drive/国漫/武神主宰 (2020)`},
		{`/tv/某剧/第 2 季/某剧 第05集.mkv`, `/tv/某剧`},
		// 无季文件夹: 取父目录。
		{`/tv/某剧/某剧 - S01E01.mkv`, `/tv/某剧`},
	}
	for _, tc := range cases {
		if got := showDirFromEpisodePath(tc.in); got != tc.want {
			t.Errorf("showDirFromEpisodePath(%q) = %q, want %q", tc.in, got, tc.want)
		}
	}
}

// TestNormalizePollutedEpisodeMetadata 验证: 被单集 id/名污染的剧组被清洗,
// 干净的剧组不动, 且迁移幂等(写入标记后再跑不重复处理)。
func TestNormalizePollutedEpisodeMetadata(t *testing.T) {
	container, db := newCleanupTestContainer(t)

	rows := []model.Media{
		// 污染组: 同一部剧每集 tmdb 不同、original_name 是单集名 → 应清洗。
		{Base: model.Base{ID: "z1"}, LibraryID: "lib", SeasonNum: 1, EpisodeNum: 1, TMDbID: 4375419, OriginalName: "九龙拉棺", ScrapeStatus: "matched", Path: `/tv/国漫/遮天 (2023)/Season 01/遮天 - S01E01.mkv`},
		{Base: model.Base{ID: "z2"}, LibraryID: "lib", SeasonNum: 1, EpisodeNum: 2, TMDbID: 4375461, OriginalName: "星空古路", ScrapeStatus: "matched", Path: `/tv/国漫/遮天 (2023)/Season 01/遮天 - S01E02.mkv`},
		// 干净组: 全组共享同一整剧 tmdb、original_name 一致 → 不动。
		{Base: model.Base{ID: "c1"}, LibraryID: "lib", SeasonNum: 1, EpisodeNum: 1, TMDbID: 1234, OriginalName: "Clean Show", ScrapeStatus: "matched", Path: `/tv/欧美剧/Clean Show (2020)/Season 01/Clean Show - S01E01.mkv`},
		{Base: model.Base{ID: "c2"}, LibraryID: "lib", SeasonNum: 1, EpisodeNum: 2, TMDbID: 1234, OriginalName: "Clean Show", ScrapeStatus: "matched", Path: `/tv/欧美剧/Clean Show (2020)/Season 01/Clean Show - S01E02.mkv`},
	}
	for i := range rows {
		if err := db.Create(&rows[i]).Error; err != nil {
			t.Fatalf("create %s: %v", rows[i].ID, err)
		}
	}

	cleaned, err := container.NormalizePollutedEpisodeMetadata(t.Context())
	if err != nil {
		t.Fatalf("normalize: %v", err)
	}
	if cleaned != 2 {
		t.Fatalf("cleaned = %d, want 2 (only the polluted show)", cleaned)
	}

	load := func(id string) model.Media {
		var m model.Media
		if err := db.First(&m, "id = ?", id).Error; err != nil {
			t.Fatalf("load %s: %v", id, err)
		}
		return m
	}
	// 污染组被清空字段并重置 pending。
	for _, id := range []string{"z1", "z2"} {
		m := load(id)
		if m.TMDbID != 0 || m.OriginalName != "" || m.ScrapeStatus != "pending" {
			t.Fatalf("polluted row %s not cleaned: %+v", id, m)
		}
	}
	// 干净组保持不变。
	for _, id := range []string{"c1", "c2"} {
		m := load(id)
		if m.TMDbID != 1234 || m.OriginalName != "Clean Show" || m.ScrapeStatus != "matched" {
			t.Fatalf("clean row %s should be untouched: %+v", id, m)
		}
	}

	// 幂等: 第二次运行不再处理(已写标记)。
	cleaned2, err := container.NormalizePollutedEpisodeMetadata(t.Context())
	if err != nil {
		t.Fatalf("normalize again: %v", err)
	}
	if cleaned2 != 0 {
		t.Fatalf("second run should be a no-op, cleaned=%d", cleaned2)
	}
}
