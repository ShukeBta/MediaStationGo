package handler

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/glebarez/sqlite"
	"go.uber.org/zap"
	"gorm.io/gorm"

	"github.com/ShukeBta/MediaStationGo/internal/config"
	"github.com/ShukeBta/MediaStationGo/internal/middleware"
	"github.com/ShukeBta/MediaStationGo/internal/model"
	"github.com/ShukeBta/MediaStationGo/internal/repository"
	"github.com/ShukeBta/MediaStationGo/internal/service"
)

func TestListLibrariesHidesAdultDirectoriesUnlessAdminRequestsAll(t *testing.T) {
	gin.SetMode(gin.TestMode)
	db, err := gorm.Open(sqlite.Open(":memory:"), &gorm.Config{})
	if err != nil {
		t.Fatal(err)
	}
	if err := db.AutoMigrate(&model.User{}, &model.Library{}, &model.Media{}, &model.Setting{}, &model.PlayProfile{}); err != nil {
		t.Fatal(err)
	}
	repos := repository.New(db)
	viewer := &model.User{Username: "viewer", PasswordHash: "hash", Role: "admin", HideAdult: true}
	if err := repos.User.Create(t.Context(), viewer); err != nil {
		t.Fatal(err)
	}
	safe := model.Library{Name: "电影", Path: "/media/movie", Type: "movie", Enabled: true}
	adult := model.Library{Name: "9KG", Path: "/media/9KG", Type: "movie", Enabled: true}
	if err := repos.Library.Create(t.Context(), &safe); err != nil {
		t.Fatal(err)
	}
	if err := repos.Library.Create(t.Context(), &adult); err != nil {
		t.Fatal(err)
	}
	if err := repos.Setting.Set(t.Context(), service.AdultLibraryIDsSettingKey, `["`+adult.ID+`"]`); err != nil {
		t.Fatal(err)
	}
	if err := repos.Media.Upsert(t.Context(), &model.Media{LibraryID: safe.ID, Title: "误入普通库的成人条目", Path: "/media/movie/nsfw.mkv", NSFW: true}); err != nil {
		t.Fatal(err)
	}
	svc := &service.Container{
		Repo:  repos,
		Media: service.NewMediaService(&config.Config{}, zap.NewNop(), repos),
	}

	visible := requestLibraries(t, svc, viewer.ID, "admin", "/api/libraries")
	if len(visible) != 1 || visible[0].ID != safe.ID {
		t.Fatalf("watching library list should hide adult directories, got %#v", visible)
	}

	all := requestLibraries(t, svc, viewer.ID, "admin", "/api/libraries?include_hidden=1")
	if len(all) != 2 {
		t.Fatalf("admin include_hidden list should keep management access, got %#v", all)
	}
}

func TestListLibrariesIncludeHiddenNormalizesCloudDisplayNames(t *testing.T) {
	gin.SetMode(gin.TestMode)
	db, err := gorm.Open(sqlite.Open(":memory:"), &gorm.Config{})
	if err != nil {
		t.Fatal(err)
	}
	if err := db.AutoMigrate(&model.Library{}, &model.Media{}); err != nil {
		t.Fatal(err)
	}
	repos := repository.New(db)
	cloud := model.Library{Name: "OpenList · 国产剧", Path: service.BuildCloudLibraryPath("openlist", "/国产剧", "/国产剧"), Type: "tv", Enabled: true}
	if err := repos.Library.Create(t.Context(), &cloud); err != nil {
		t.Fatal(err)
	}
	svc := &service.Container{
		Repo:  repos,
		Media: service.NewMediaService(&config.Config{}, zap.NewNop(), repos),
	}

	all := requestLibraries(t, svc, "admin", "admin", "/api/libraries?include_hidden=1")
	if len(all) != 1 {
		t.Fatalf("include_hidden list = %#v, want one library", all)
	}
	if all[0].Name != "国产剧" {
		t.Fatalf("cloud display name = %q, want stripped directory name", all[0].Name)
	}
}

func TestListMediaGroupsMultipleVersionsByDefault(t *testing.T) {
	gin.SetMode(gin.TestMode)
	db, err := gorm.Open(sqlite.Open(":memory:"), &gorm.Config{})
	if err != nil {
		t.Fatal(err)
	}
	if err := db.AutoMigrate(&model.User{}, &model.Library{}, &model.Media{}, &model.Setting{}, &model.PlayProfile{}); err != nil {
		t.Fatal(err)
	}
	repos := repository.New(db)
	lib := model.Library{Name: "Movies", Path: "/media/movies", Type: "movie", Enabled: true}
	if err := repos.Library.Create(t.Context(), &lib); err != nil {
		t.Fatal(err)
	}
	if err := repos.DB.Create(&[]model.Media{
		{
			Base:      model.Base{ID: "movie-1080", CreatedAt: time.Now().Add(-time.Minute)},
			LibraryID: lib.ID,
			Title:     "流浪地球",
			Path:      "/media/movies/The.Wandering.Earth.2019.1080p.mkv",
			Year:      2019,
			Width:     1920,
			Height:    1080,
			SizeBytes: 100,
		},
		{
			Base:      model.Base{ID: "movie-2160", CreatedAt: time.Now()},
			LibraryID: lib.ID,
			Title:     "流浪地球",
			Path:      "cloud://openlist/Movies/The.Wandering.Earth.2019.2160p.mkv",
			Year:      2019,
			Width:     3840,
			Height:    2160,
			SizeBytes: 200,
		},
		{
			Base:      model.Base{ID: "movie-moon", CreatedAt: time.Now().Add(-2 * time.Minute)},
			LibraryID: lib.ID,
			Title:     "独行月球",
			Path:      "/media/movies/Moon.Man.2022.mkv",
			Year:      2022,
			Width:     1920,
			Height:    1080,
			SizeBytes: 300,
		},
	}).Error; err != nil {
		t.Fatal(err)
	}
	svc := &service.Container{
		Repo:  repos,
		Media: service.NewMediaService(&config.Config{}, zap.NewNop(), repos),
	}

	grouped := requestMediaList(t, svc, "/api/libraries/"+lib.ID+"/media?group_versions=1", lib.ID)
	if grouped.Total != 2 || len(grouped.Items) != 2 {
		t.Fatalf("grouped response total=%d len=%d body=%#v", grouped.Total, len(grouped.Items), grouped)
	}
	if grouped.Items[0].ID != "movie-1080" {
		t.Fatalf("primary id = %q, want local version to remain primary", grouped.Items[0].ID)
	}
	if len(grouped.Items[0].Versions) != 2 {
		t.Fatalf("versions = %#v, want both versions", grouped.Items[0].Versions)
	}
	if grouped.Items[0].Versions[0].ID != "movie-1080" || grouped.Items[0].Versions[1].ID != "movie-2160" {
		t.Fatalf("versions should keep local before cloud: %#v", grouped.Items[0].Versions)
	}
	paged := requestMediaList(t, svc, "/api/libraries/"+lib.ID+"/media?page_size=1&group_versions=1", lib.ID)
	if paged.Total != 2 || len(paged.Items) != 1 {
		t.Fatalf("paged grouped response total=%d len=%d body=%#v", paged.Total, len(paged.Items), paged)
	}

	raw := requestMediaList(t, svc, "/api/libraries/"+lib.ID+"/media", lib.ID)
	if raw.Total != 3 || len(raw.Items) != 3 {
		t.Fatalf("raw response total=%d len=%d body=%#v", raw.Total, len(raw.Items), raw)
	}
}

func requestLibraries(t *testing.T, svc *service.Container, userID, role, path string) []model.Library {
	t.Helper()
	w := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(w)
	c.Set(middleware.CtxUserID, userID)
	c.Set(middleware.CtxUserRole, role)
	c.Request = httptest.NewRequest(http.MethodGet, path, nil)
	listLibrariesHandler(svc)(c)
	if w.Code != http.StatusOK {
		t.Fatalf("GET %s status = %d body=%s", path, w.Code, w.Body.String())
	}
	var libs []model.Library
	if err := json.Unmarshal(w.Body.Bytes(), &libs); err != nil {
		t.Fatalf("decode libraries: %v", err)
	}
	return libs
}

type mediaListResponse struct {
	Items []service.MediaItem `json:"items"`
	Total int64               `json:"total"`
}

func requestMediaList(t *testing.T, svc *service.Container, path, libraryID string) mediaListResponse {
	t.Helper()
	w := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(w)
	c.Set(middleware.CtxUserID, "user-1")
	c.Set(middleware.CtxUserRole, "user")
	c.Params = gin.Params{{Key: "id", Value: libraryID}}
	c.Request = httptest.NewRequest(http.MethodGet, path, nil)
	listMediaHandler(svc)(c)
	if w.Code != http.StatusOK {
		t.Fatalf("GET %s status = %d body=%s", path, w.Code, w.Body.String())
	}
	var payload mediaListResponse
	if err := json.Unmarshal(w.Body.Bytes(), &payload); err != nil {
		t.Fatalf("decode media list: %v", err)
	}
	return payload
}
