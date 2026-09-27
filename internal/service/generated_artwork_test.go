package service

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"sync/atomic"
	"testing"
	"time"

	"go.uber.org/zap"

	"github.com/ShukeBta/MediaStationGo/internal/config"
	"github.com/ShukeBta/MediaStationGo/internal/model"
	"github.com/ShukeBta/MediaStationGo/internal/repository"
)

func TestGeneratedArtworkProcessesOnlyEnabledNonEpisodicMissingMedia(t *testing.T) {
	db := newServiceTestDB(t, &model.Library{}, &model.Media{})
	if sqlDB, err := db.DB(); err == nil {
		sqlDB.SetMaxOpenConns(1)
	}
	repos := repository.New(db)
	lib := model.Library{Name: "其他媒体", Path: t.TempDir(), Type: "movie", Enabled: true, GenerateArtwork: true}
	other := model.Library{Name: "电影", Path: t.TempDir(), Type: "movie", Enabled: true}
	for _, item := range []*model.Library{&lib, &other} {
		if err := repos.DB.Create(item).Error; err != nil {
			t.Fatal(err)
		}
	}
	source := filepath.Join(t.TempDir(), "movie.mkv")
	if err := os.WriteFile(source, []byte("video"), 0o600); err != nil {
		t.Fatal(err)
	}
	rows := []model.Media{
		{LibraryID: lib.ID, Title: "target", Path: source, DurationSec: 900},
		{LibraryID: lib.ID, Title: "episode", Path: source + ".episode", SeasonNum: 1, EpisodeNum: 1},
		{LibraryID: other.ID, Title: "disabled", Path: source + ".other"},
	}
	if err := repos.DB.Create(&rows).Error; err != nil {
		t.Fatal(err)
	}
	if err := repos.DB.Exec("UPDATE media SET generated_artwork_attempts = NULL WHERE id = ?", rows[0].ID).Error; err != nil {
		t.Fatal(err)
	}

	cfg := &config.Config{App: config.AppConfig{DataDir: t.TempDir(), FFmpegPath: "ffmpeg"}}
	svc := NewGeneratedArtworkService(cfg, zap.NewNop(), repos, nil, NewRuntimeCacheService(cfg, zap.NewNop()), nil)
	var calls atomic.Int64
	svc.run = func(_ context.Context, _ string, _ map[string]string, seek float64, poster, backdrop string) error {
		calls.Add(1)
		if seek != 90 {
			t.Fatalf("seek = %v, want 90", seek)
		}
		data := make([]byte, 2048)
		if err := os.WriteFile(poster, data, 0o600); err != nil {
			return err
		}
		return os.WriteFile(backdrop, data, 0o600)
	}
	ctx, cancel := context.WithCancel(t.Context())
	defer cancel()
	svc.Start(ctx)
	queued, err := svc.QueueMissingForLibrary(ctx, lib.ID)
	if err != nil || queued != 1 {
		t.Fatalf("queued = %d, err = %v", queued, err)
	}

	waitForGeneratedArtworkStatus(t, repos, rows[0].ID, GeneratedArtworkStatusCompleted)
	var got model.Media
	if err := repos.DB.First(&got, "id = ?", rows[0].ID).Error; err != nil {
		t.Fatal(err)
	}
	if calls.Load() != 1 || got.GeneratedPosterURL == "" || got.GeneratedBackdropURL == "" || got.GeneratedArtworkHash == "" {
		t.Fatalf("generated media = %+v, calls = %d", got, calls.Load())
	}
	for _, excluded := range rows[1:] {
		var current model.Media
		if err := repos.DB.First(&current, "id = ?", excluded.ID).Error; err != nil {
			t.Fatal(err)
		}
		if current.GeneratedArtworkStatus != "" {
			t.Fatalf("excluded media %s status = %q", current.ID, current.GeneratedArtworkStatus)
		}
	}
}

func TestGeneratedArtworkCancelStopsRunningLibraryJob(t *testing.T) {
	db := newServiceTestDB(t, &model.Library{}, &model.Media{})
	if sqlDB, err := db.DB(); err == nil {
		sqlDB.SetMaxOpenConns(1)
	}
	repos := repository.New(db)
	lib := model.Library{Name: "其他媒体", Path: t.TempDir(), Type: "movie", Enabled: true, GenerateArtwork: true}
	if err := repos.DB.Create(&lib).Error; err != nil {
		t.Fatal(err)
	}
	source := filepath.Join(t.TempDir(), "movie.mkv")
	if err := os.WriteFile(source, []byte("video"), 0o600); err != nil {
		t.Fatal(err)
	}
	media := model.Media{LibraryID: lib.ID, Title: "target", Path: source}
	if err := repos.DB.Create(&media).Error; err != nil {
		t.Fatal(err)
	}
	cfg := &config.Config{App: config.AppConfig{DataDir: t.TempDir(), FFmpegPath: "ffmpeg"}}
	svc := NewGeneratedArtworkService(cfg, zap.NewNop(), repos, nil, nil, nil)
	started := make(chan struct{})
	svc.run = func(ctx context.Context, _ string, _ map[string]string, _ float64, _, _ string) error {
		close(started)
		<-ctx.Done()
		return ctx.Err()
	}
	ctx, cancel := context.WithCancel(t.Context())
	defer cancel()
	svc.Start(ctx)
	if queued, err := svc.QueueMissingForLibrary(ctx, lib.ID); err != nil || queued != 1 {
		t.Fatalf("queued = %d, err = %v", queued, err)
	}
	select {
	case <-started:
	case <-time.After(2 * time.Second):
		t.Fatal("artwork runner did not start")
	}
	if _, err := svc.CancelLibrary(ctx, lib.ID); err != nil {
		t.Fatal(err)
	}
	waitForGeneratedArtworkStatus(t, repos, media.ID, GeneratedArtworkStatusCanceled)
}

func TestEmbyArtworkPrefersRealImagesThenGeneratedImages(t *testing.T) {
	svc := &EmbyService{}
	media := &model.Media{
		PosterURL:            "real-poster.jpg",
		BackdropURL:          "real-backdrop.jpg",
		GeneratedPosterURL:   "generated-poster.jpg",
		GeneratedBackdropURL: "generated-backdrop.jpg",
	}
	if got := svc.mediaPrimaryArtwork(t.Context(), media); got != media.PosterURL {
		t.Fatalf("primary = %q", got)
	}
	if got := svc.mediaBackdropArtwork(t.Context(), media); got != media.BackdropURL {
		t.Fatalf("backdrop = %q", got)
	}
	media.PosterURL = ""
	media.BackdropURL = ""
	if got := svc.mediaPrimaryArtwork(t.Context(), media); got != media.GeneratedPosterURL {
		t.Fatalf("generated primary = %q", got)
	}
	if got := svc.mediaBackdropArtwork(t.Context(), media); got != media.GeneratedBackdropURL {
		t.Fatalf("generated backdrop = %q", got)
	}
}

func TestGeneratedArtworkSeekUsesShortSafeOffsetWithoutDuration(t *testing.T) {
	if got := generatedArtworkSeek(0); got != 10 {
		t.Fatalf("seek without duration = %v, want 10", got)
	}
	if got := generatedArtworkSeek(900); got != 90 {
		t.Fatalf("seek with duration = %v, want 90", got)
	}
}

func TestGeneratedArtworkGenerateAtForMediaUsesExactTimestamp(t *testing.T) {
	db := newServiceTestDB(t, &model.Library{}, &model.Media{})
	repos := repository.New(db)
	lib := model.Library{Name: "其他媒体", Path: t.TempDir(), Type: "movie", Enabled: true, GenerateArtwork: false}
	if err := repos.DB.Create(&lib).Error; err != nil {
		t.Fatal(err)
	}
	source := filepath.Join(t.TempDir(), "movie.mkv")
	if err := os.WriteFile(source, []byte("video"), 0o600); err != nil {
		t.Fatal(err)
	}
	media := model.Media{LibraryID: lib.ID, Title: "target", Path: source, DurationSec: 600}
	if err := repos.DB.Create(&media).Error; err != nil {
		t.Fatal(err)
	}
	cfg := &config.Config{App: config.AppConfig{DataDir: t.TempDir(), FFmpegPath: "ffmpeg"}}
	svc := NewGeneratedArtworkService(cfg, zap.NewNop(), repos, nil, nil, nil)
	var gotSeek float64
	svc.run = func(_ context.Context, _ string, _ map[string]string, seek float64, poster, backdrop string) error {
		gotSeek = seek
		data := make([]byte, 2048)
		if err := os.WriteFile(poster, data, 0o600); err != nil {
			return err
		}
		return os.WriteFile(backdrop, data, 0o600)
	}
	result, err := svc.GenerateAtForMedia(t.Context(), media.ID, 123.456)
	if err != nil {
		t.Fatal(err)
	}
	if gotSeek != 123.456 || result.GeneratedArtworkSeekSec != 123.456 || result.GeneratedArtworkStatus != GeneratedArtworkStatusCompleted {
		t.Fatalf("result = %+v seek=%v", result, gotSeek)
	}
	if result.GeneratedPosterURL == "" || result.GeneratedBackdropURL == "" || result.GeneratedArtworkHash == "" {
		t.Fatalf("generated artwork is incomplete: %+v", result)
	}
	if _, err := svc.GenerateAtForMedia(t.Context(), media.ID, 600); err == nil {
		t.Fatal("out-of-range timestamp was accepted")
	}
}

func TestCloudMediaInternalHeadersUseSigningUserAgent(t *testing.T) {
	original := map[string]string{"Referer": "https://example.test/"}
	headers := cloudMediaInternalHeaders(original)
	if headers["User-Agent"] != cloudMediaInternalUserAgent || headers["Referer"] == "" {
		t.Fatalf("headers = %#v", headers)
	}
	if original["User-Agent"] != "" {
		t.Fatalf("input headers were mutated: %#v", original)
	}
}

func TestGeneratedArtworkQueuesDurationRefreshWithoutDroppingCurrentPreview(t *testing.T) {
	db := newServiceTestDB(t, &model.Library{}, &model.Media{})
	repos := repository.New(db)
	lib := model.Library{Name: "其他媒体", Path: t.TempDir(), Type: "movie", Enabled: true, GenerateArtwork: true}
	if err := repos.DB.Create(&lib).Error; err != nil {
		t.Fatal(err)
	}
	media := model.Media{
		LibraryID:                lib.ID,
		Title:                    "target",
		Path:                     "cloud://openlist/other/target.mkv",
		DurationSec:              0,
		GeneratedPosterURL:       "/data/generated-artwork/old-primary.jpg",
		GeneratedBackdropURL:     "/data/generated-artwork/old-backdrop.jpg",
		GeneratedArtworkStatus:   GeneratedArtworkStatusCompleted,
		GeneratedArtworkAttempts: 1,
	}
	media.GeneratedArtworkHash = generatedArtworkFingerprint(&media)
	if err := repos.DB.Create(&media).Error; err != nil {
		t.Fatal(err)
	}
	if err := repos.DB.Model(&model.Media{}).Where("id = ?", media.ID).Update("duration_sec", 900).Error; err != nil {
		t.Fatal(err)
	}

	svc := NewGeneratedArtworkService(&config.Config{}, zap.NewNop(), repos, nil, nil, nil)
	queued, err := svc.QueueRefreshForMedia(t.Context(), media.ID)
	if err != nil || !queued {
		t.Fatalf("queued = %v, err = %v", queued, err)
	}
	var got model.Media
	if err := repos.DB.First(&got, "id = ?", media.ID).Error; err != nil {
		t.Fatal(err)
	}
	if got.GeneratedArtworkStatus != GeneratedArtworkStatusPending || got.GeneratedArtworkAttempts != 0 {
		t.Fatalf("refresh state = status %q attempts %d", got.GeneratedArtworkStatus, got.GeneratedArtworkAttempts)
	}
	if got.GeneratedPosterURL != media.GeneratedPosterURL || got.GeneratedBackdropURL != media.GeneratedBackdropURL {
		t.Fatalf("current preview was dropped before replacement: %+v", got)
	}
}

func TestGeneratedArtworkRejectsPreviewWhenDurationChangesDuringExtraction(t *testing.T) {
	db := newServiceTestDB(t, &model.Library{}, &model.Media{})
	repos := repository.New(db)
	lib := model.Library{Name: "其他媒体", Path: t.TempDir(), Type: "movie", Enabled: true, GenerateArtwork: true}
	if err := repos.DB.Create(&lib).Error; err != nil {
		t.Fatal(err)
	}
	source := filepath.Join(t.TempDir(), "movie.mkv")
	if err := os.WriteFile(source, []byte("video"), 0o600); err != nil {
		t.Fatal(err)
	}
	media := model.Media{LibraryID: lib.ID, Title: "target", Path: source}
	if err := repos.DB.Create(&media).Error; err != nil {
		t.Fatal(err)
	}
	cfg := &config.Config{App: config.AppConfig{DataDir: t.TempDir(), FFmpegPath: "ffmpeg"}}
	svc := NewGeneratedArtworkService(cfg, zap.NewNop(), repos, nil, nil, nil)
	svc.run = func(_ context.Context, _ string, _ map[string]string, _ float64, poster, backdrop string) error {
		if err := repos.DB.Model(&model.Media{}).Where("id = ?", media.ID).Update("duration_sec", 900).Error; err != nil {
			return err
		}
		data := make([]byte, 2048)
		if err := os.WriteFile(poster, data, 0o600); err != nil {
			return err
		}
		return os.WriteFile(backdrop, data, 0o600)
	}
	_, _, _, err := svc.generateOne(t.Context(), &media)
	if !errors.Is(err, errGeneratedArtworkMetadataChanged) {
		t.Fatalf("generate error = %v, want metadata changed", err)
	}
}

func waitForGeneratedArtworkStatus(t *testing.T, repos *repository.Container, mediaID, want string) {
	t.Helper()
	deadline := time.Now().Add(3 * time.Second)
	for time.Now().Before(deadline) {
		var media model.Media
		if err := repos.DB.First(&media, "id = ?", mediaID).Error; err == nil && media.GeneratedArtworkStatus == want {
			return
		}
		time.Sleep(20 * time.Millisecond)
	}
	var media model.Media
	_ = repos.DB.First(&media, "id = ?", mediaID).Error
	t.Fatalf("media status = %q, want %q; error=%q", media.GeneratedArtworkStatus, want, media.GeneratedArtworkError)
}
