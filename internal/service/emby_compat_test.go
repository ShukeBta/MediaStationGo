package service

import (
	"context"
	"testing"
	"time"

	"go.uber.org/zap"

	"github.com/ShukeBta/MediaStationGo/internal/config"
	"github.com/ShukeBta/MediaStationGo/internal/model"
	"github.com/ShukeBta/MediaStationGo/internal/repository"
	"github.com/ShukeBta/MediaStationGo/internal/service/cloud"
)

func newTestEmbyService(t *testing.T) *EmbyService {
	t.Helper()
	db := newServiceTestDB(t, &model.Library{}, &model.Series{}, &model.Media{}, &model.Favorite{}, &model.PlaybackHistory{}, &model.User{}, &model.Setting{})
	// 内存库 + 异步探测协程：限制为单连接，避免连接池新建连接时
	// 拿到一个空白的 :memory: 实例（no such table）。
	if sqlDB, err := db.DB(); err == nil {
		sqlDB.SetMaxOpenConns(1)
	}
	repos := repository.New(db)
	return NewEmbyService(&config.Config{}, zap.NewNop(), repos)
}

func TestEmbyLatestItemsOrderByReleaseDate(t *testing.T) {
	svc := newTestEmbyService(t)
	lib := model.Library{Name: "电影", Path: `/media/movies`, Type: "movie", Enabled: true}
	if err := svc.repo.Library.Create(t.Context(), &lib); err != nil {
		t.Fatalf("create library: %v", err)
	}
	base := time.Now()
	rows := []model.Media{
		{
			Base:        model.Base{ID: "older-release-newer-scan", CreatedAt: base.Add(2 * time.Hour)},
			LibraryID:   lib.ID,
			Title:       "旧上映新入库",
			Path:        `/media/movies/old.mkv`,
			Year:        2026,
			ReleaseDate: "2026-01-10",
		},
		{
			Base:        model.Base{ID: "newer-release-older-scan", CreatedAt: base},
			LibraryID:   lib.ID,
			Title:       "新上映",
			Path:        `/media/movies/new.mkv`,
			Year:        2026,
			ReleaseDate: "2026-06-23",
		},
	}
	for i := range rows {
		if err := svc.repo.DB.Create(&rows[i]).Error; err != nil {
			t.Fatalf("create media: %v", err)
		}
	}

	items, err := svc.LatestItems(t.Context(), "", lib.ID, 10)
	if err != nil {
		t.Fatalf("latest items: %v", err)
	}
	if len(items) != 2 || items[0]["Id"] != "newer-release-older-scan" {
		t.Fatalf("latest items should prefer release date over created_at, got %#v", items)
	}
	if _, ok := items[0]["PremiereDate"].(time.Time); !ok {
		t.Fatalf("latest item should expose PremiereDate for Emby clients: %#v", items[0])
	}
}

type fakeCloudPlaybackResolver struct {
	link *cloud.DirectLink
	typ  string
	ref  string
	ua   string
}

func (f *fakeCloudPlaybackResolver) CloudResolve(_ context.Context, typ, fileRef, clientUA string) (*cloud.DirectLink, error) {
	f.typ = typ
	f.ref = fileRef
	f.ua = clientUA
	return f.link, nil
}

type fakeCloudPlaybackProber struct {
	probe   *ProbeResult
	rawURL  string
	headers map[string]string
}

func (f *fakeCloudPlaybackProber) ProbeHTTP(_ context.Context, rawURL string, headers map[string]string) (*ProbeResult, error) {
	f.rawURL = rawURL
	f.headers = headers
	return f.probe, nil
}
