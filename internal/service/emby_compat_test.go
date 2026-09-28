package service

import (
	"os"
	"os/exec"
	"path/filepath"
	"testing"
	"time"

	"go.uber.org/zap"

	"github.com/ShukeBta/MediaStationGo/internal/config"
	"github.com/ShukeBta/MediaStationGo/internal/model"
	"github.com/ShukeBta/MediaStationGo/internal/repository"
)

func newTestEmbyService(t *testing.T) *EmbyService {
	t.Helper()
	db := newServiceTestDB(t, &model.Library{}, &model.Series{}, &model.Media{}, &model.Person{}, &model.Favorite{}, &model.PlaybackHistory{}, &model.UserMediaPlaybackPreference{}, &model.User{}, &model.Setting{})
	// 内存库 + 异步探测协程：限制为单连接，避免连接池新建连接时
	// 拿到一个空白的 :memory: 实例（no such table）。
	if sqlDB, err := db.DB(); err == nil {
		sqlDB.SetMaxOpenConns(1)
	}
	repos := repository.New(db)
	return NewEmbyService(&config.Config{}, zap.NewNop(), repos)
}

func TestEmbyImageURLFallsBackToLocalSidecarPoster(t *testing.T) {
	svc := newTestEmbyService(t)
	root := t.TempDir()
	mediaDir := filepath.Join(root, "本地电影")
	if err := os.MkdirAll(mediaDir, 0o755); err != nil {
		t.Fatal(err)
	}
	mediaPath := filepath.Join(mediaDir, "本地电影.mkv")
	if err := os.WriteFile(mediaPath, []byte("video"), 0o644); err != nil {
		t.Fatal(err)
	}
	posterPath := filepath.Join(mediaDir, "poster.jpg")
	if err := os.WriteFile(posterPath, []byte("poster"), 0o644); err != nil {
		t.Fatal(err)
	}
	lib := model.Library{Name: "电影", Path: root, Type: "movie", Enabled: true}
	if err := svc.repo.Library.Create(t.Context(), &lib); err != nil {
		t.Fatalf("create library: %v", err)
	}
	if err := svc.repo.DB.Create(&model.Media{
		Base:      model.Base{ID: "local-movie-no-poster"},
		LibraryID: lib.ID,
		Title:     "本地电影",
		Path:      mediaPath,
	}).Error; err != nil {
		t.Fatalf("create media: %v", err)
	}

	got, err := svc.ImageURL(t.Context(), "local-movie-no-poster", "Primary")
	if err != nil {
		t.Fatalf("image url: %v", err)
	}
	if got != posterPath {
		t.Fatalf("ImageURL Primary = %q, want local sidecar poster %q", got, posterPath)
	}
}

func TestEmbyImageURLGeneratesLocalVideoThumbnail(t *testing.T) {
	ffmpeg, err := resolveLocalExecutable("", "ffmpeg")
	if err != nil {
		t.Skipf("ffmpeg unavailable: %v", err)
	}
	svc := newTestEmbyService(t)
	svc.cfg.Cache.CacheDir = t.TempDir()
	root := t.TempDir()
	mediaPath := filepath.Join(root, "No Poster.mp4")
	cmd := exec.Command(ffmpeg,
		"-hide_banner",
		"-loglevel", "error",
		"-f", "lavfi",
		"-i", "color=c=blue:s=32x32:d=1",
		"-frames:v", "1",
		"-y", mediaPath,
	)
	if output, err := cmd.CombinedOutput(); err != nil {
		t.Fatalf("create video: %v output=%s", err, output)
	}
	lib := model.Library{Name: "电影", Path: root, Type: "movie", Enabled: true}
	if err := svc.repo.Library.Create(t.Context(), &lib); err != nil {
		t.Fatalf("create library: %v", err)
	}
	if err := svc.repo.DB.Create(&model.Media{
		Base:      model.Base{ID: "local-video-no-poster"},
		LibraryID: lib.ID,
		Title:     "No Poster",
		Path:      mediaPath,
	}).Error; err != nil {
		t.Fatalf("create media: %v", err)
	}

	got, err := svc.ImageURL(t.Context(), "local-video-no-poster", "Primary")
	if err != nil {
		t.Fatalf("image url: %v", err)
	}
	if got == "" {
		t.Fatal("ImageURL Primary should generate a local video thumbnail")
	}
	if stat, err := os.Stat(got); err != nil || stat.Size() == 0 {
		t.Fatalf("generated thumbnail %q stat=%v err=%v", got, stat, err)
	}
}

func TestEmbyLatestItemsOrderByCreatedAt(t *testing.T) {
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
	// 「最近添加」按入库时间倒序：更晚入库的排前面，上映日期不参与排序。
	if len(items) != 2 || items[0]["Id"] != "older-release-newer-scan" {
		t.Fatalf("latest items should order by created_at desc, got %#v", items)
	}
	if _, ok := items[0]["PremiereDate"].(time.Time); !ok {
		t.Fatalf("latest item should expose PremiereDate for Emby clients: %#v", items[0])
	}
}
