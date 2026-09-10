package repository

import (
	"testing"

	"github.com/glebarez/sqlite"
	"gorm.io/gorm"

	"github.com/ShukeBta/MediaStationGo/internal/model"
)

func TestMediaPlaybackPreferenceRepositoryIsUserScopedAndUpsertsDisabledSubtitles(t *testing.T) {
	db, err := gorm.Open(sqlite.Open(":memory:"), &gorm.Config{})
	if err != nil {
		t.Fatal(err)
	}
	if err := db.AutoMigrate(&model.UserMediaPlaybackPreference{}); err != nil {
		t.Fatal(err)
	}
	repo := &MediaPlaybackPreferenceRepository{db: db}
	preference := &model.UserMediaPlaybackPreference{
		UserID:           "user-1",
		MediaID:          "media-1",
		SubtitleEnabled:  true,
		SubtitleTrackKey: "stream:2",
		AudioTrackKey:    "stream:1",
	}
	if err := repo.Upsert(t.Context(), preference); err != nil {
		t.Fatal(err)
	}

	row, err := repo.FindByUserAndMedia(t.Context(), "user-1", "media-1")
	if err != nil || row == nil || !row.SubtitleEnabled || row.SubtitleTrackKey != "stream:2" || row.AudioTrackKey != "stream:1" {
		t.Fatalf("row = %#v err=%v", row, err)
	}
	if other, err := repo.FindByUserAndMedia(t.Context(), "user-2", "media-1"); err != nil || other != nil {
		t.Fatalf("other = %#v err=%v", other, err)
	}

	preference.SubtitleEnabled = false
	preference.AudioTrackKey = "stream:3"
	if err := repo.Upsert(t.Context(), preference); err != nil {
		t.Fatal(err)
	}
	row, err = repo.FindByUserAndMedia(t.Context(), "user-1", "media-1")
	if err != nil || row == nil || row.SubtitleEnabled || row.SubtitleTrackKey != "stream:2" || row.AudioTrackKey != "stream:3" {
		t.Fatalf("updated row = %#v err=%v", row, err)
	}
}

func TestMediaPlaybackPreferenceRepositorySetsMediaSourceWithoutOverwritingTracks(t *testing.T) {
	db, err := gorm.Open(sqlite.Open(":memory:"), &gorm.Config{})
	if err != nil {
		t.Fatal(err)
	}
	if err := db.AutoMigrate(&model.UserMediaPlaybackPreference{}); err != nil {
		t.Fatal(err)
	}
	repo := &MediaPlaybackPreferenceRepository{db: db}
	preference := &model.UserMediaPlaybackPreference{
		UserID:           "user-1",
		MediaID:          "version-scope",
		SubtitleEnabled:  false,
		SubtitleTrackKey: "stream:2",
		AudioTrackKey:    "stream:1",
		HiddenFromResume: true,
	}
	if err := repo.Upsert(t.Context(), preference); err != nil {
		t.Fatal(err)
	}
	if err := repo.SetPreferredMediaSource(t.Context(), "user-1", "version-scope", "source-2"); err != nil {
		t.Fatal(err)
	}

	row, err := repo.FindByUserAndMedia(t.Context(), "user-1", "version-scope")
	if err != nil {
		t.Fatal(err)
	}
	if row == nil || row.PreferredMediaSourceID != "source-2" || row.SubtitleEnabled ||
		row.SubtitleTrackKey != "stream:2" || row.AudioTrackKey != "stream:1" || !row.HiddenFromResume {
		t.Fatalf("updated row = %#v", row)
	}
	if other, err := repo.FindByUserAndMedia(t.Context(), "user-2", "version-scope"); err != nil || other != nil {
		t.Fatalf("other user row = %#v err=%v", other, err)
	}
}
