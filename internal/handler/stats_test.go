package handler

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/gin-gonic/gin"
	"github.com/glebarez/sqlite"
	"gorm.io/gorm"

	"github.com/ShukeBta/MediaStationGo/internal/middleware"
	"github.com/ShukeBta/MediaStationGo/internal/model"
	"github.com/ShukeBta/MediaStationGo/internal/repository"
	"github.com/ShukeBta/MediaStationGo/internal/service"
)

func TestStatsSnapshotHidesAdultRecentlyAddedForUser(t *testing.T) {
	gin.SetMode(gin.TestMode)
	db, err := gorm.Open(sqlite.Open(":memory:"), &gorm.Config{})
	if err != nil {
		t.Fatal(err)
	}
	if err := db.AutoMigrate(&model.User{}, &model.Library{}, &model.Media{}, &model.Setting{}, &model.PlayProfile{}); err != nil {
		t.Fatal(err)
	}
	repos := repository.New(db)
	safe := model.Library{Name: "电影", Path: "/media/movie", Type: "movie", Enabled: true}
	adult := model.Library{Name: "9KG", Path: "/media/9KG", Type: "adult", Enabled: true}
	if err := repos.Library.Create(t.Context(), &safe); err != nil {
		t.Fatal(err)
	}
	if err := repos.Library.Create(t.Context(), &adult); err != nil {
		t.Fatal(err)
	}
	viewer := &model.User{Username: "viewer", PasswordHash: "hash", Role: "user", HideAdult: true, AllowedLibraryIDs: []string{safe.ID, adult.ID}}
	if err := repos.User.Create(t.Context(), viewer); err != nil {
		t.Fatal(err)
	}
	if err := db.Create(&model.Media{LibraryID: safe.ID, Title: "普通电影", Path: "/media/movie/a.mkv", SizeBytes: 100, DurationSec: 10}).Error; err != nil {
		t.Fatal(err)
	}
	if err := db.Create(&model.Media{LibraryID: adult.ID, Title: "成人影片", Path: "/media/9KG/a.mkv", SizeBytes: 200, DurationSec: 20, NSFW: true}).Error; err != nil {
		t.Fatal(err)
	}
	svc := &service.Container{Repo: repos}
	w := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(w)
	c.Set(middleware.CtxUserID, viewer.ID)
	c.Request = httptest.NewRequest("GET", "/api/stats", nil)

	snap := &service.Snapshot{}
	if err := applyStatsVisibility(c, svc, snap); err != nil {
		t.Fatalf("applyStatsVisibility: %v", err)
	}
	if snap.MediaCount != 1 || snap.TotalSizeBytes != 100 || snap.TotalSeconds != 10 {
		t.Fatalf("stats should only include visible media, got count=%d size=%d seconds=%d", snap.MediaCount, snap.TotalSizeBytes, snap.TotalSeconds)
	}
	if len(snap.RecentlyAdded) != 1 || snap.RecentlyAdded[0].LibraryID != safe.ID {
		t.Fatalf("recently added should hide adult library, got %#v", snap.RecentlyAdded)
	}
}

func TestStatsLibrariesCountsMergedCloudLibraryItems(t *testing.T) {
	gin.SetMode(gin.TestMode)
	db, err := gorm.Open(sqlite.Open(":memory:"), &gorm.Config{})
	if err != nil {
		t.Fatal(err)
	}
	if err := db.AutoMigrate(&model.User{}, &model.Library{}, &model.Media{}, &model.Setting{}, &model.PlayProfile{}); err != nil {
		t.Fatal(err)
	}
	repos := repository.New(db)
	local := model.Library{Name: "国产电影", Path: "/media/国产电影", Type: "movie", Enabled: true}
	cloud := model.Library{Name: "OpenList · 国产电影", Path: service.BuildCloudLibraryPath("openlist", "/国产电影", "/国产电影"), Type: "movie", Enabled: true}
	for _, lib := range []*model.Library{&local, &cloud} {
		if err := repos.Library.Create(t.Context(), lib); err != nil {
			t.Fatal(err)
		}
	}
	if err := db.Create(&[]model.Media{
		{LibraryID: local.ID, Title: "本地版本", Path: "/media/国产电影/local.mkv", SizeBytes: 100},
		{LibraryID: cloud.ID, Title: "云盘版本", Path: "cloud://openlist/国产电影/cloud.mkv", SizeBytes: 200},
	}).Error; err != nil {
		t.Fatal(err)
	}
	svc := &service.Container{Repo: repos}
	w := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(w)
	c.Request = httptest.NewRequest("GET", "/api/stats/libraries", nil)

	statsLibrariesHandler(svc)(c)
	if w.Code != http.StatusOK {
		t.Fatalf("status = %d body=%s", w.Code, w.Body.String())
	}
	var payload struct {
		Libraries []struct {
			ItemCount int64 `json:"item_count"`
			TotalSize int64 `json:"total_size"`
		} `json:"libraries"`
	}
	if err := json.Unmarshal(w.Body.Bytes(), &payload); err != nil {
		t.Fatal(err)
	}
	if len(payload.Libraries) != 1 {
		t.Fatalf("libraries = %#v, want one merged display library", payload.Libraries)
	}
	if payload.Libraries[0].ItemCount != 2 || payload.Libraries[0].TotalSize != 300 {
		t.Fatalf("merged stats = %#v, want count=2 size=300", payload.Libraries[0])
	}
}
