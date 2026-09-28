package handler

import (
	"encoding/json"
	"strings"
	"testing"

	"github.com/gin-gonic/gin"

	"github.com/ShukeBta/MediaStationGo/internal/middleware"
	"github.com/ShukeBta/MediaStationGo/internal/model"
	"github.com/ShukeBta/MediaStationGo/internal/service"
)

func TestMediaForResponseHidesStorageFieldsFromNonAdmin(t *testing.T) {
	gin.SetMode(gin.TestMode)
	media := model.Media{
		Base:               model.Base{ID: "media-1"},
		LibraryID:          "library-1",
		LibraryRootID:      "root-1",
		Title:              "测试影片",
		Path:               "cloud://openlist/115/电影/test.mkv",
		RelativePath:       "电影/test.mkv",
		STRMURL:            "https://storage.example/test.mkv",
		LibraryPath:        "cloud://openlist/115/电影",
		DisplayLibraryPath: "cloud://openlist/115",
		FileHash:           "hash-size",
		FileID:             "device:inode",
		Width:              3840,
		Height:             2160,
	}

	userContext, _ := gin.CreateTestContext(nil)
	userContext.Set(middleware.CtxUserRole, "user")
	got := mediaForResponse(userContext, media)
	if got.Path != "" || got.RelativePath != "" || got.STRMURL != "" || got.LibraryPath != "" ||
		got.DisplayLibraryPath != "" || got.FileHash != "" || got.FileID != "" {
		t.Fatalf("non-admin response kept storage fields: %+v", got)
	}
	if got.ID != media.ID || got.LibraryID != media.LibraryID || got.LibraryRootID != media.LibraryRootID ||
		got.Title != media.Title || got.Width != media.Width || got.Height != media.Height {
		t.Fatalf("non-admin response removed public playback metadata: %+v", got)
	}
	payload, err := json.Marshal(got)
	if err != nil {
		t.Fatal(err)
	}
	for _, field := range []string{"path", "relative_path", "strm_url", "library_path", "display_library_path", "file_hash", "file_id"} {
		if strings.Contains(string(payload), `"`+field+`"`) {
			t.Fatalf("non-admin JSON still contains %q: %s", field, payload)
		}
	}

	adminContext, _ := gin.CreateTestContext(nil)
	adminContext.Set(middleware.CtxUserRole, "admin")
	admin := mediaForResponse(adminContext, media)
	if admin.Path != media.Path || admin.STRMURL != media.STRMURL || admin.FileID != media.FileID {
		t.Fatalf("admin response lost storage fields: %+v", admin)
	}
}

func TestNestedMediaResponsesUseTheSameSanitizer(t *testing.T) {
	gin.SetMode(gin.TestMode)
	c, _ := gin.CreateTestContext(nil)
	c.Set(middleware.CtxUserRole, "user")
	media := model.Media{Base: model.Base{ID: "media-1"}, Title: "测试影片", Path: "/data/test.mkv", FileID: "device:inode"}

	grouped := mediaItemsForResponse(c, []service.MediaItem{{Media: media, Versions: []model.Media{media}, Parts: []model.Media{media}}})
	if grouped[0].Path != "" || grouped[0].Versions[0].Path != "" || grouped[0].Parts[0].Path != "" {
		t.Fatalf("grouped response kept nested paths: %+v", grouped[0])
	}
	versions := mediaVersionListForResponse(c, service.MediaVersionList{Items: []service.MediaVersionItem{{Media: media}}})
	if versions.Items[0].Path != "" || versions.Items[0].FileID != "" {
		t.Fatalf("version response kept storage fields: %+v", versions.Items[0])
	}
	parts := mediaPartListForResponse(c, service.MediaPartList{Items: []service.MediaPartItem{{Media: media}}})
	if parts.Items[0].Path != "" || parts.Items[0].FileID != "" {
		t.Fatalf("part response kept storage fields: %+v", parts.Items[0])
	}
}
