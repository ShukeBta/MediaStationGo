package service

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/ShukeBta/MediaStationGo/internal/model"
)

func TestEmbyLocalVideoThumbnailFallbackUsesCachedPrimaryOnly(t *testing.T) {
	svc := newTestEmbyService(t)
	svc.cfg.Cache.CacheDir = t.TempDir()

	mediaDir := t.TempDir()
	videoPath := filepath.Join(mediaDir, "movie.mkv")
	if err := os.WriteFile(videoPath, []byte("fake video"), 0o600); err != nil {
		t.Fatalf("write video: %v", err)
	}
	absVideoPath, err := filepath.Abs(videoPath)
	if err != nil {
		t.Fatalf("abs video path: %v", err)
	}
	cachePath, _ := svc.localVideoThumbnailPaths(absVideoPath)
	if err := os.MkdirAll(filepath.Dir(cachePath), 0o750); err != nil {
		t.Fatalf("make thumbnail cache dir: %v", err)
	}
	if err := os.WriteFile(cachePath, []byte{0xff, 0xd8, 0xff, 0xd9}, 0o600); err != nil {
		t.Fatalf("write cached thumbnail: %v", err)
	}

	lib := model.Library{Name: "电影", Path: mediaDir, Type: "movie", Enabled: true}
	if err := svc.repo.Library.Create(t.Context(), &lib); err != nil {
		t.Fatalf("create library: %v", err)
	}
	media := model.Media{
		Base:      model.Base{ID: "local-no-poster"},
		LibraryID: lib.ID,
		Title:     "本地无封面电影",
		Path:      videoPath,
	}
	if err := svc.repo.DB.Create(&media).Error; err != nil {
		t.Fatalf("create media: %v", err)
	}

	primary, err := svc.ImageURL(t.Context(), media.ID, "Primary")
	if err != nil {
		t.Fatalf("primary image url: %v", err)
	}
	if primary != cachePath {
		t.Fatalf("primary image = %q, want cached thumbnail %q", primary, cachePath)
	}
	backdrop, err := svc.ImageURL(t.Context(), media.ID, "Backdrop")
	if err != nil {
		t.Fatalf("backdrop image url: %v", err)
	}
	if backdrop != "" {
		t.Fatalf("backdrop image = %q, want empty", backdrop)
	}

	for _, tc := range []model.Media{
		{Base: model.Base{ID: "has-poster"}, LibraryID: lib.ID, Title: "有封面", Path: videoPath, PosterURL: "/poster.jpg"},
		{Base: model.Base{ID: "has-strm"}, LibraryID: lib.ID, Title: "STRM", Path: videoPath, STRMURL: "/api/cloud/play/openlist?ref=%2Fmovie.mkv"},
		{Base: model.Base{ID: "cloud-path"}, LibraryID: lib.ID, Title: "云盘", Path: "cloud://openlist/movie.mkv"},
		{Base: model.Base{ID: "http-path"}, LibraryID: lib.ID, Title: "HTTP", Path: "https://example.invalid/movie.mkv"},
	} {
		if got, err := svc.localVideoThumbnail(t.Context(), &tc); err != nil || got != "" {
			t.Fatalf("local thumbnail for %#v = %q, %v; want empty nil", tc.ID, got, err)
		}
	}
}
