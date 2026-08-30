package service

import (
	"errors"
	"testing"
	"time"

	"go.uber.org/zap"

	"github.com/ShukeBta/MediaStationGo/internal/config"
	"github.com/ShukeBta/MediaStationGo/internal/model"
	"github.com/ShukeBta/MediaStationGo/internal/repository"
)

func TestPlaybackProgressRequiresSuccessfulResolveForCloudMedia(t *testing.T) {
	db := newServiceTestDB(t, &model.User{}, &model.Library{}, &model.Media{}, &model.PlaybackHistory{}, &model.UserMediaPlaybackPreference{})
	repos := repository.New(db)
	if err := db.Create(&model.User{Base: model.Base{ID: "user-1"}, Username: "user-1", PasswordHash: "x", IsActive: true}).Error; err != nil {
		t.Fatal(err)
	}
	lib := model.Library{Base: model.Base{ID: "library-1"}, Name: "Cloud", Path: "cloud://openlist/Movies", Type: "movie", Enabled: true}
	if err := db.Create(&lib).Error; err != nil {
		t.Fatal(err)
	}
	media := model.Media{
		Base:      model.Base{ID: "media-1"},
		LibraryID: lib.ID,
		Title:     "Cloud Movie",
		Path:      "cloud://openlist/Movies/Movie.mkv",
		STRMURL:   "/api/cloud/play/openlist?ref=%2FMovies%2FMovie.mkv",
	}
	if err := db.Create(&media).Error; err != nil {
		t.Fatal(err)
	}

	playback := NewPlaybackService(zap.NewNop(), repos)
	emby := NewEmbyService(&config.Config{}, zap.NewNop(), repos)
	emby.SetPlaybackService(playback)
	if err := emby.RecordProgress(t.Context(), "user-1", media.ID, 30_000*10_000, 0); !errors.Is(err, ErrCloudPlaybackNotResolved) {
		t.Fatalf("record error = %v, want unresolved cloud playback", err)
	}
	var count int64
	if err := db.Model(&model.PlaybackHistory{}).Count(&count).Error; err != nil {
		t.Fatal(err)
	}
	if count != 0 {
		t.Fatalf("history count = %d, failed playback polluted history", count)
	}

	playback.AuthorizeResolvedCloudPlayback("user-1", media.ID)
	if err := emby.RecordProgress(t.Context(), "user-1", media.ID, 30_000*10_000, 0); err != nil {
		t.Fatal(err)
	}
	if err := db.Model(&model.PlaybackHistory{}).Count(&count).Error; err != nil {
		t.Fatal(err)
	}
	if count != 1 {
		t.Fatalf("history count = %d, want successful progress", count)
	}
}

func TestPlaybackProgressGuardAllowsLocalMediaAndScopesCloudGrantByUser(t *testing.T) {
	db := newServiceTestDB(t, &model.User{}, &model.Library{}, &model.Media{}, &model.PlaybackHistory{}, &model.UserMediaPlaybackPreference{})
	repos := repository.New(db)
	users := []model.User{
		{Base: model.Base{ID: "user-1"}, Username: "user-1", PasswordHash: "x", IsActive: true},
		{Base: model.Base{ID: "user-2"}, Username: "user-2", PasswordHash: "x", IsActive: true},
	}
	if err := db.Create(&users).Error; err != nil {
		t.Fatal(err)
	}
	localLib := model.Library{Base: model.Base{ID: "local-library"}, Name: "Local", Path: t.TempDir(), Type: "movie", Enabled: true}
	cloudLib := model.Library{Base: model.Base{ID: "cloud-library"}, Name: "Cloud", Path: "cloud://openlist/Movies", Type: "movie", Enabled: true}
	if err := db.Create(&[]model.Library{localLib, cloudLib}).Error; err != nil {
		t.Fatal(err)
	}
	rows := []model.Media{
		{Base: model.Base{ID: "local-media"}, LibraryID: localLib.ID, Title: "Local", Path: localLib.Path + "/local.mkv"},
		{Base: model.Base{ID: "cloud-media"}, LibraryID: cloudLib.ID, Title: "Cloud", Path: "cloud://openlist/Movies/cloud.mkv"},
	}
	if err := db.Create(&rows).Error; err != nil {
		t.Fatal(err)
	}

	playback := NewPlaybackService(zap.NewNop(), repos)
	if err := playback.ValidateProgressWrite(t.Context(), "user-1", "local-media"); err != nil {
		t.Fatalf("local media was rejected: %v", err)
	}
	playback.AuthorizeResolvedCloudPlayback("user-1", "cloud-media")
	if err := playback.ValidateProgressWrite(t.Context(), "user-1", "cloud-media"); err != nil {
		t.Fatalf("authorized user was rejected: %v", err)
	}
	if err := playback.ValidateProgressWrite(t.Context(), "user-2", "cloud-media"); !errors.Is(err, ErrCloudPlaybackNotResolved) {
		t.Fatalf("other user error = %v, want user-scoped rejection", err)
	}

	key := resolvedCloudPlaybackKey("user-1", "cloud-media")
	playback.resolvedMu.Lock()
	playback.resolvedPlay[key] = time.Now().Add(-time.Second)
	playback.resolvedMu.Unlock()
	if err := playback.ValidateProgressWrite(t.Context(), "user-1", "cloud-media"); !errors.Is(err, ErrCloudPlaybackNotResolved) {
		t.Fatalf("expired grant error = %v, want unresolved cloud playback", err)
	}
}
