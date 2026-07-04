package service

import (
	"testing"

	"github.com/ShukeBta/MediaStationGo/internal/model"
)

func TestStorageConfigLogoutClearsCredentialsAndCloudLibraries(t *testing.T) {
	repos, storage := newStorageUploadTestService(t)
	enabled := true
	if _, err := storage.Save(t.Context(), StorageInput{
		Type: "openlist",
		Config: map[string]any{
			"server":          "http://openlist.test",
			"url":             "http://openlist.test/dav/",
			"username":        "user",
			"password":        "pass",
			"token":           "token",
			"timeout_seconds": "120",
			"force_302":       "true",
		},
		Enabled: &enabled,
	}); err != nil {
		t.Fatalf("save storage: %v", err)
	}
	cloudLib := model.Library{Name: "OpenList", Path: BuildCloudLibraryPath("openlist", "/TV", "/TV"), Type: "tv", Enabled: true}
	if err := repos.Library.Create(t.Context(), &cloudLib); err != nil {
		t.Fatalf("create cloud library: %v", err)
	}
	localLib := model.Library{Name: "Local", Path: t.TempDir(), Type: "movie", Enabled: true}
	if err := repos.Library.Create(t.Context(), &localLib); err != nil {
		t.Fatalf("create local library: %v", err)
	}
	if err := repos.Media.Upsert(t.Context(), &model.Media{LibraryID: cloudLib.ID, Title: "Cloud", Path: "cloud://openlist/TV/Movie.mkv"}); err != nil {
		t.Fatalf("create cloud media: %v", err)
	}
	if err := repos.Media.Upsert(t.Context(), &model.Media{LibraryID: localLib.ID, Title: "Local", Path: localLib.Path + "/Movie.mkv"}); err != nil {
		t.Fatalf("create local media: %v", err)
	}

	view, err := storage.Logout(t.Context(), "openlist")
	if err != nil {
		t.Fatalf("logout: %v", err)
	}
	if view.Enabled {
		t.Fatal("storage should be disabled after logout")
	}
	for _, key := range []string{"username", "password", "token", "force_302", "force_proxy"} {
		if _, ok := view.Config[key]; ok {
			t.Fatalf("logout should clear %s, config = %#v", key, view.Config)
		}
	}
	if view.Config["server"] != "http://openlist.test" || view.Config["url"] != "http://openlist.test/dav/" || view.Config["timeout_seconds"] != "120" {
		t.Fatalf("logout should keep non-secret connection hints, config = %#v", view.Config)
	}
	if got, err := repos.Library.FindByID(t.Context(), cloudLib.ID); err != nil {
		t.Fatalf("find cloud library: %v", err)
	} else if got != nil {
		t.Fatalf("cloud library should be removed after logout: %#v", got)
	}
	if got, err := repos.Library.FindByID(t.Context(), localLib.ID); err != nil {
		t.Fatalf("find local library: %v", err)
	} else if got == nil {
		t.Fatal("local library should remain after cloud logout")
	}
	var cloudMediaCount int64
	if err := repos.DB.Unscoped().Model(&model.Media{}).Where("path = ?", "cloud://openlist/TV/Movie.mkv").Count(&cloudMediaCount).Error; err != nil {
		t.Fatalf("count cloud media: %v", err)
	}
	if cloudMediaCount != 0 {
		t.Fatalf("cloud media should be purged after logout, count=%d", cloudMediaCount)
	}
}

func TestStorageConfigListHidesDeprecatedQuarkRows(t *testing.T) {
	repos, storage := newStorageUploadTestService(t)
	if err := repos.StorageConfig.Upsert(t.Context(), &model.StorageConfig{
		Type:    LegacyQuarkProvider,
		Config:  storage.crypto.Encrypt(`{"cookie":"legacy"}`),
		Enabled: true,
	}); err != nil {
		t.Fatalf("insert legacy quark row: %v", err)
	}
	if _, err := storage.Save(t.Context(), StorageInput{
		Type: "openlist",
		Config: map[string]any{
			"server": "http://openlist.test",
			"token":  "token",
		},
	}); err != nil {
		t.Fatalf("save openlist row: %v", err)
	}

	rows, err := storage.List(t.Context())
	if err != nil {
		t.Fatal(err)
	}
	if len(rows) != 1 || rows[0].Type != "openlist" {
		t.Fatalf("storage list = %#v, want only supported OpenList row", rows)
	}
}
