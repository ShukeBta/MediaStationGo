package handler

import (
	"bytes"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
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
	adult := model.Library{Name: "9KG", Path: "/media/9KG", Type: "adult", Enabled: true}
	if err := repos.Library.Create(t.Context(), &safe); err != nil {
		t.Fatal(err)
	}
	if err := repos.Library.Create(t.Context(), &adult); err != nil {
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

func TestUpdateLibraryEnabledDoesNotChangeRootStatus(t *testing.T) {
	gin.SetMode(gin.TestMode)
	db, err := gorm.Open(sqlite.Open(":memory:"), &gorm.Config{})
	if err != nil {
		t.Fatal(err)
	}
	if err := db.AutoMigrate(&model.Library{}, &model.LibraryRoot{}); err != nil {
		t.Fatal(err)
	}
	repos := repository.New(db)
	lib := model.Library{Name: "电影", Path: "/media/movies", Type: "movie", Enabled: true}
	if err := repos.Library.Create(t.Context(), &lib); err != nil {
		t.Fatal(err)
	}
	root := model.LibraryRoot{LibraryID: lib.ID, Name: "电影路径", Path: "/media/movies", Enabled: true}
	if err := repos.Library.CreateRoot(t.Context(), &root); err != nil {
		t.Fatal(err)
	}
	svc := &service.Container{
		Repo:  repos,
		Media: service.NewMediaService(&config.Config{}, zap.NewNop(), repos),
	}

	body := bytes.NewBufferString(`{"enabled":false}`)
	req := httptest.NewRequest(http.MethodPatch, "/api/libraries/"+lib.ID, body)
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(w)
	c.Request = req
	c.Params = gin.Params{{Key: "id", Value: lib.ID}}
	updateLibraryHandler(svc)(c)
	if w.Code != http.StatusOK {
		t.Fatalf("status = %d body=%s", w.Code, w.Body.String())
	}
	updated, err := repos.Library.FindByID(t.Context(), lib.ID)
	if err != nil {
		t.Fatal(err)
	}
	if updated == nil || updated.Enabled {
		t.Fatalf("library enabled = %#v, want false", updated)
	}
	updatedRoot, err := repos.Library.FindRootByID(t.Context(), lib.ID, root.ID)
	if err != nil {
		t.Fatal(err)
	}
	if updatedRoot == nil || !updatedRoot.Enabled {
		t.Fatalf("root enabled = %#v, want true", updatedRoot)
	}
}

func TestUpdateLibraryTitleMode(t *testing.T) {
	gin.SetMode(gin.TestMode)
	db, err := gorm.Open(sqlite.Open(":memory:"), &gorm.Config{})
	if err != nil {
		t.Fatal(err)
	}
	if err := db.AutoMigrate(&model.Library{}); err != nil {
		t.Fatal(err)
	}
	repos := repository.New(db)
	lib := model.Library{Name: "其他媒体", Path: "cloud://openlist/115/其他", Type: "movie", Enabled: true}
	if err := repos.Library.Create(t.Context(), &lib); err != nil {
		t.Fatal(err)
	}
	svc := &service.Container{Repo: repos, Media: service.NewMediaService(&config.Config{}, zap.NewNop(), repos)}

	body := bytes.NewBufferString(`{"title_mode":"filename"}`)
	req := httptest.NewRequest(http.MethodPatch, "/api/libraries/"+lib.ID, body)
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(w)
	c.Request = req
	c.Params = gin.Params{{Key: "id", Value: lib.ID}}
	updateLibraryHandler(svc)(c)
	if w.Code != http.StatusOK {
		t.Fatalf("status = %d body=%s", w.Code, w.Body.String())
	}
	updated, err := repos.Library.FindByID(t.Context(), lib.ID)
	if err != nil {
		t.Fatal(err)
	}
	if updated == nil || updated.TitleMode != service.LibraryTitleModeFilename || !updated.Enabled {
		t.Fatalf("updated library = %#v", updated)
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

func TestListLibrariesShowsAutoCategoryLibraries(t *testing.T) {
	gin.SetMode(gin.TestMode)
	db, err := gorm.Open(sqlite.Open(":memory:"), &gorm.Config{})
	if err != nil {
		t.Fatal(err)
	}
	if err := db.AutoMigrate(&model.Library{}, &model.Media{}); err != nil {
		t.Fatal(err)
	}
	repos := repository.New(db)
	root := model.Library{Name: "OpenList", Path: "cloud://openlist", Type: "movie", Enabled: true}
	auto := model.Library{Name: "欧美剧", Path: service.BuildCloudAutoCategoryLibraryPath("openlist", "电视剧/欧美剧"), Type: "tv", Enabled: true}
	for _, lib := range []*model.Library{&root, &auto} {
		if err := repos.Library.Create(t.Context(), lib); err != nil {
			t.Fatal(err)
		}
	}
	svc := &service.Container{
		Repo:  repos,
		Media: service.NewMediaService(&config.Config{}, zap.NewNop(), repos),
	}

	all := requestLibraries(t, svc, "admin", "admin", "/api/libraries?include_hidden=1")
	if len(all) != 2 {
		t.Fatalf("include_hidden list = %#v, want root plus auto category library", all)
	}
	ids := map[string]bool{}
	for _, lib := range all {
		ids[lib.ID] = true
	}
	if !ids[root.ID] || !ids[auto.ID] {
		t.Fatalf("include_hidden list = %#v, want root %s and auto category %s", all, root.ID, auto.ID)
	}

	visible := requestLibraries(t, svc, "user-1", "user", "/api/libraries")
	if len(visible) != 2 {
		t.Fatalf("visible list = %#v, want root plus auto category library", visible)
	}
	ids = map[string]bool{}
	for _, lib := range visible {
		ids[lib.ID] = true
	}
	if !ids[root.ID] || !ids[auto.ID] {
		t.Fatalf("visible list = %#v, want root %s and auto category %s", visible, root.ID, auto.ID)
	}

	got := requestLibrary(t, svc, "user-1", "user", "/api/libraries/"+auto.ID, auto.ID)
	if got.ID != auto.ID || got.Name != auto.Name {
		t.Fatalf("auto category detail = %#v, want accessible category library", got)
	}
}

func TestGetLibraryAllowsEmptyLibrary(t *testing.T) {
	gin.SetMode(gin.TestMode)
	db, err := gorm.Open(sqlite.Open(":memory:"), &gorm.Config{})
	if err != nil {
		t.Fatal(err)
	}
	if err := db.AutoMigrate(&model.User{}, &model.Library{}, &model.Media{}, &model.Setting{}, &model.PlayProfile{}); err != nil {
		t.Fatal(err)
	}
	repos := repository.New(db)
	lib := model.Library{Name: "空媒体库", Path: "/media/empty", Type: "movie", Enabled: true}
	if err := repos.Library.Create(t.Context(), &lib); err != nil {
		t.Fatal(err)
	}
	svc := &service.Container{
		Repo:  repos,
		Media: service.NewMediaService(&config.Config{}, zap.NewNop(), repos),
	}

	got := requestLibrary(t, svc, "user-1", "user", "/api/libraries/"+lib.ID, lib.ID)
	if got.ID != lib.ID || got.Name != "空媒体库" {
		t.Fatalf("library detail = %#v, want empty library detail", got)
	}
	media := requestMediaList(t, svc, "/api/libraries/"+lib.ID+"/media", lib.ID)
	if media.Total != 0 || len(media.Items) != 0 {
		t.Fatalf("empty library media = %#v, want no items", media)
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
	}).Error; err != nil {
		t.Fatal(err)
	}
	svc := &service.Container{
		Repo:  repos,
		Media: service.NewMediaService(&config.Config{}, zap.NewNop(), repos),
	}

	grouped := requestMediaList(t, svc, "/api/libraries/"+lib.ID+"/media", lib.ID)
	if grouped.Total != 1 || len(grouped.Items) != 1 {
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

	raw := requestMediaList(t, svc, "/api/libraries/"+lib.ID+"/media?group_versions=0", lib.ID)
	if raw.Total != 2 || len(raw.Items) != 2 {
		t.Fatalf("raw response total=%d len=%d body=%#v", raw.Total, len(raw.Items), raw)
	}
}

func TestListLibrarySeriesDoesNotTruncateLargeEpisodeLibraries(t *testing.T) {
	gin.SetMode(gin.TestMode)
	db, err := gorm.Open(sqlite.Open(":memory:"), &gorm.Config{})
	if err != nil {
		t.Fatal(err)
	}
	if err := db.AutoMigrate(&model.User{}, &model.Library{}, &model.Media{}, &model.Setting{}, &model.PlayProfile{}); err != nil {
		t.Fatal(err)
	}
	repos := repository.New(db)
	lib := model.Library{Name: "国漫", Path: "cloud://openlist/国漫", Type: "anime", Enabled: true}
	if err := repos.Library.Create(t.Context(), &lib); err != nil {
		t.Fatal(err)
	}
	rows := make([]model.Media, 0, 2001)
	for i := 1; i <= 2001; i++ {
		rows = append(rows, model.Media{
			Base:       model.Base{ID: fmt.Sprintf("ep-%04d", i), CreatedAt: time.Now().Add(time.Duration(i) * time.Second)},
			LibraryID:  lib.ID,
			Title:      "大剧",
			Path:       fmt.Sprintf("cloud://openlist/国漫/大剧 (2026) {tmdb-123}/Season 1/大剧.S01E%04d.mkv", i),
			SeasonNum:  1,
			EpisodeNum: i,
		})
	}
	if err := repos.DB.CreateInBatches(rows, 100).Error; err != nil {
		t.Fatal(err)
	}
	svc := &service.Container{
		Repo:  repos,
		Media: service.NewMediaService(&config.Config{}, zap.NewNop(), repos),
	}

	series := requestLibrarySeries(t, svc, "/api/libraries/"+lib.ID+"/series", lib.ID)
	if series.Total != 1 || len(series.Items) != 1 {
		t.Fatalf("series response total=%d len=%d body=%#v", series.Total, len(series.Items), series)
	}
	if series.Items[0].Count != 2001 {
		t.Fatalf("series count = %d, want 2001", series.Items[0].Count)
	}
	if !strings.HasPrefix(series.Items[0].Key, "series:") ||
		strings.Contains(series.Items[0].Key, "lib:") ||
		strings.Contains(series.Items[0].Key, "show:") {
		t.Fatalf("series key = %q, want compact non-raw key", series.Items[0].Key)
	}
	episodes := requestLibrarySeriesEpisodes(t, svc, "/api/libraries/"+lib.ID+"/series/episodes?key="+url.QueryEscape(series.Items[0].Key), lib.ID)
	if episodes.Total != 2001 || len(episodes.Items) != 2001 {
		t.Fatalf("episodes total=%d len=%d, want 2001", episodes.Total, len(episodes.Items))
	}
	if episodes.Items[0].EpisodeNum != 1 || episodes.Items[len(episodes.Items)-1].EpisodeNum != 2001 {
		t.Fatalf("episode order first=%d last=%d", episodes.Items[0].EpisodeNum, episodes.Items[len(episodes.Items)-1].EpisodeNum)
	}
}

func TestScrapeOptionsFromRequestIgnoresRemovedEpisodeImagesOption(t *testing.T) {
	gin.SetMode(gin.TestMode)
	w := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(w)
	c.Request = httptest.NewRequest(http.MethodPost, "/api/media/ep-1/scrape", bytes.NewBufferString(`{"episode_images":false,"refresh_matched":true}`))
	c.Request.Header.Set("Content-Type", "application/json")

	options, err := scrapeOptionsFromRequest(c, false)
	if err != nil {
		t.Fatal(err)
	}
	if !options.IncludeMatched {
		t.Fatal("IncludeMatched = false, want true from refresh_matched")
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

func requestLibrary(t *testing.T, svc *service.Container, userID, role, path, libraryID string) model.Library {
	t.Helper()
	w := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(w)
	c.Set(middleware.CtxUserID, userID)
	c.Set(middleware.CtxUserRole, role)
	c.Params = gin.Params{{Key: "id", Value: libraryID}}
	c.Request = httptest.NewRequest(http.MethodGet, path, nil)
	getLibraryHandler(svc)(c)
	if w.Code != http.StatusOK {
		t.Fatalf("GET %s status = %d body=%s", path, w.Code, w.Body.String())
	}
	var lib model.Library
	if err := json.Unmarshal(w.Body.Bytes(), &lib); err != nil {
		t.Fatalf("decode library: %v", err)
	}
	return lib
}

type mediaListResponse struct {
	Items []service.MediaItem `json:"items"`
	Total int64               `json:"total"`
}

type seriesListResponse struct {
	Items []service.SeriesCard `json:"items"`
	Total int64                `json:"total"`
}

type seriesEpisodesResponse struct {
	Items []model.Media `json:"items"`
	Total int64         `json:"total"`
}

func TestSearchMediaGroupSeriesReturnsWorkPage(t *testing.T) {
	gin.SetMode(gin.TestMode)
	db, err := gorm.Open(sqlite.Open(":memory:"), &gorm.Config{})
	if err != nil {
		t.Fatal(err)
	}
	if err := db.AutoMigrate(&model.User{}, &model.Library{}, &model.Media{}, &model.Setting{}, &model.PlayProfile{}); err != nil {
		t.Fatal(err)
	}
	repos := repository.New(db)
	lib := model.Library{Name: "电影", Path: "/media/movies", Type: "movie", Enabled: true}
	if err := repos.Library.Create(t.Context(), &lib); err != nil {
		t.Fatal(err)
	}
	for _, media := range []model.Media{
		{LibraryID: lib.ID, Title: "黑衣人", Path: "/media/movies/men-in-black/4k.mkv", TMDbID: 607},
		{LibraryID: lib.ID, Title: "黑衣人", Path: "/media/movies/men-in-black/1080p.mkv", TMDbID: 607},
	} {
		row := media
		if err := repos.Media.Upsert(t.Context(), &row); err != nil {
			t.Fatal(err)
		}
	}
	svc := &service.Container{
		Repo:  repos,
		Media: service.NewMediaService(&config.Config{}, zap.NewNop(), repos),
	}

	w := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(w)
	c.Request = httptest.NewRequest(http.MethodGet, "/api/media?q=%E9%BB%91%E8%A1%A3%E4%BA%BA&page=1&page_size=1&group_series=1", nil)
	searchMediaHandler(svc)(c)
	if w.Code != http.StatusOK {
		t.Fatalf("search status = %d body=%s", w.Code, w.Body.String())
	}
	var payload seriesListResponse
	if err := json.Unmarshal(w.Body.Bytes(), &payload); err != nil {
		t.Fatal(err)
	}
	if payload.Total != 1 || len(payload.Items) != 1 || payload.Items[0].Count != 2 {
		t.Fatalf("grouped search payload = %#v", payload)
	}
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

func requestLibrarySeries(t *testing.T, svc *service.Container, path, libraryID string) seriesListResponse {
	t.Helper()
	w := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(w)
	c.Set(middleware.CtxUserID, "user-1")
	c.Set(middleware.CtxUserRole, "user")
	c.Params = gin.Params{{Key: "id", Value: libraryID}}
	c.Request = httptest.NewRequest(http.MethodGet, path, nil)
	listLibrarySeriesHandler(svc)(c)
	if w.Code != http.StatusOK {
		t.Fatalf("GET %s status = %d body=%s", path, w.Code, w.Body.String())
	}
	var payload seriesListResponse
	if err := json.Unmarshal(w.Body.Bytes(), &payload); err != nil {
		t.Fatalf("decode series list: %v", err)
	}
	return payload
}

func requestLibrarySeriesEpisodes(t *testing.T, svc *service.Container, path, libraryID string) seriesEpisodesResponse {
	t.Helper()
	w := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(w)
	c.Set(middleware.CtxUserID, "user-1")
	c.Set(middleware.CtxUserRole, "user")
	c.Params = gin.Params{{Key: "id", Value: libraryID}}
	c.Request = httptest.NewRequest(http.MethodGet, path, nil)
	listLibrarySeriesEpisodesHandler(svc)(c)
	if w.Code != http.StatusOK {
		t.Fatalf("GET %s status = %d body=%s", path, w.Code, w.Body.String())
	}
	var payload seriesEpisodesResponse
	if err := json.Unmarshal(w.Body.Bytes(), &payload); err != nil {
		t.Fatalf("decode series episodes: %v", err)
	}
	return payload
}

// 空库的 media / series 列表必须返回 "items":[]（而不是 Go nil slice 序列化出的 null）。
// 前端 [].concat(null) 会得到 [null]，随后在渲染期解引用 null 崩溃，导致「空库点进去
// 报错且无法返回」。这条测试钉死该 JSON 契约，防止再退化。
func TestEmptyLibraryListsReturnEmptyArraysNotNull(t *testing.T) {
	gin.SetMode(gin.TestMode)
	db, err := gorm.Open(sqlite.Open(":memory:"), &gorm.Config{})
	if err != nil {
		t.Fatal(err)
	}
	if err := db.AutoMigrate(&model.Library{}, &model.Media{}); err != nil {
		t.Fatal(err)
	}
	repos := repository.New(db)
	movie := model.Library{Name: "空电影库", Path: "/media/empty-movie", Type: "movie", Enabled: true}
	tv := model.Library{Name: "空剧集库", Path: "/media/empty-tv", Type: "tv", Enabled: true}
	for _, lib := range []*model.Library{&movie, &tv} {
		if err := repos.Library.Create(t.Context(), lib); err != nil {
			t.Fatal(err)
		}
	}
	svc := &service.Container{
		Repo:  repos,
		Media: service.NewMediaService(&config.Config{}, zap.NewNop(), repos),
	}

	cases := []struct {
		name    string
		path    string
		lib     string
		handler gin.HandlerFunc
	}{
		{"media", "/api/libraries/" + movie.ID + "/media", movie.ID, listMediaHandler(svc)},
		{"series", "/api/libraries/" + tv.ID + "/series", tv.ID, listLibrarySeriesHandler(svc)},
	}
	for _, tc := range cases {
		w := httptest.NewRecorder()
		c, _ := gin.CreateTestContext(w)
		c.Set(middleware.CtxUserID, "user-1")
		c.Set(middleware.CtxUserRole, "user")
		c.Params = gin.Params{{Key: "id", Value: tc.lib}}
		c.Request = httptest.NewRequest(http.MethodGet, tc.path, nil)
		tc.handler(c)
		if w.Code != http.StatusOK {
			t.Fatalf("%s: status = %d body=%s", tc.name, w.Code, w.Body.String())
		}
		body := w.Body.String()
		if strings.Contains(body, `"items":null`) {
			t.Fatalf("%s: empty library returned items:null (crashes frontend): %s", tc.name, body)
		}
		if !strings.Contains(body, `"items":[]`) {
			t.Fatalf("%s: expected items:[] for empty library, got %s", tc.name, body)
		}
	}
}
