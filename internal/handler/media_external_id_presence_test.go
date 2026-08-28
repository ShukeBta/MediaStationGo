package handler

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"reflect"
	"testing"

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

func TestExternalIDPresenceReturnsOnlyVisibleRequestedIDs(t *testing.T) {
	gin.SetMode(gin.TestMode)
	db, err := gorm.Open(sqlite.Open(":memory:"), &gorm.Config{})
	if err != nil {
		t.Fatal(err)
	}
	if err := db.AutoMigrate(&model.User{}, &model.Library{}, &model.Media{}, &model.Setting{}, &model.PlayProfile{}); err != nil {
		t.Fatal(err)
	}
	repos := repository.New(db)
	viewer := &model.User{Base: model.Base{ID: "viewer"}, Username: "viewer", PasswordHash: "hash", Role: "admin", HideAdult: true}
	if err := repos.User.Create(t.Context(), viewer); err != nil {
		t.Fatal(err)
	}
	safe := model.Library{Base: model.Base{ID: "safe"}, Name: "电影", Path: "/media/movies", Type: "movie", Enabled: true}
	adult := model.Library{Base: model.Base{ID: "adult"}, Name: "成人", Path: "/media/adult", Type: "adult", Enabled: true}
	if err := db.Create(&[]model.Library{safe, adult}).Error; err != nil {
		t.Fatal(err)
	}
	rows := []model.Media{
		{Base: model.Base{ID: "safe-media"}, LibraryID: safe.ID, Title: "可见", Path: "/media/movies/visible.mkv", TMDbID: 101, DoubanID: "db-visible"},
		{Base: model.Base{ID: "adult-media"}, LibraryID: adult.ID, Title: "隐藏", Path: "/media/adult/hidden.mkv", TMDbID: 202, DoubanID: "db-hidden", NSFW: true},
	}
	if err := db.Create(&rows).Error; err != nil {
		t.Fatal(err)
	}
	svc := &service.Container{Repo: repos, Media: service.NewMediaService(&config.Config{}, zap.NewNop(), repos)}

	body, err := json.Marshal(externalIDPresenceRequest{
		TMDbIDs:   []int{101, 202, 404, 101},
		DoubanIDs: []string{"db-visible", "db-hidden", "db-missing", "db-visible"},
	})
	if err != nil {
		t.Fatal(err)
	}
	w := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(w)
	c.Set(middleware.CtxUserID, viewer.ID)
	c.Set(middleware.CtxUserRole, viewer.Role)
	c.Request = httptest.NewRequest(http.MethodPost, "/api/media/external-id-presence", bytes.NewReader(body))
	c.Request.Header.Set("Content-Type", "application/json")
	externalIDPresenceHandler(svc)(c)
	if w.Code != http.StatusOK {
		t.Fatalf("status=%d body=%s", w.Code, w.Body.String())
	}
	var response externalIDPresenceResponse
	if err := json.Unmarshal(w.Body.Bytes(), &response); err != nil {
		t.Fatalf("decode response: %v", err)
	}
	if want := []int{101}; !reflect.DeepEqual(response.TMDbIDs, want) {
		t.Fatalf("tmdb_ids=%v want=%v", response.TMDbIDs, want)
	}
	if want := []string{"db-visible"}; !reflect.DeepEqual(response.DoubanIDs, want) {
		t.Fatalf("douban_ids=%v want=%v", response.DoubanIDs, want)
	}
}

func TestExternalIDPresenceRejectsInvalidAndOversizedBatches(t *testing.T) {
	gin.SetMode(gin.TestMode)
	handler := externalIDPresenceHandler(&service.Container{})
	for _, requestBody := range []externalIDPresenceRequest{
		{TMDbIDs: []int{0}},
		{DoubanIDs: []string{""}},
		{TMDbIDs: func() []int {
			ids := make([]int, maxExternalIDPresenceBatch+1)
			for index := range ids {
				ids[index] = index + 1
			}
			return ids
		}()},
	} {
		body, err := json.Marshal(requestBody)
		if err != nil {
			t.Fatal(err)
		}
		w := httptest.NewRecorder()
		c, _ := gin.CreateTestContext(w)
		c.Request = httptest.NewRequest(http.MethodPost, "/api/media/external-id-presence", bytes.NewReader(body))
		c.Request.Header.Set("Content-Type", "application/json")
		handler(c)
		if w.Code != http.StatusBadRequest {
			t.Fatalf("body=%s status=%d want=%d", body, w.Code, http.StatusBadRequest)
		}
	}
}

func TestExternalIDPresenceDistinguishesMovieAndSeriesTMDbIDs(t *testing.T) {
	gin.SetMode(gin.TestMode)
	db, err := gorm.Open(sqlite.Open(":memory:"), &gorm.Config{})
	if err != nil {
		t.Fatal(err)
	}
	if err := db.AutoMigrate(&model.User{}, &model.Library{}, &model.Media{}, &model.Setting{}, &model.PlayProfile{}); err != nil {
		t.Fatal(err)
	}
	repos := repository.New(db)
	viewer := &model.User{Base: model.Base{ID: "viewer"}, Username: "viewer", PasswordHash: "hash", Role: "admin"}
	if err := repos.User.Create(t.Context(), viewer); err != nil {
		t.Fatal(err)
	}
	movieLibrary := model.Library{Base: model.Base{ID: "movie"}, Name: "电影", Path: "/media/movies", Type: "movie", Enabled: true}
	tvLibrary := model.Library{Base: model.Base{ID: "tv"}, Name: "剧集", Path: "/media/tv", Type: "tv", Enabled: true}
	animeLibrary := model.Library{Base: model.Base{ID: "anime"}, Name: "动漫", Path: "/media/anime", Type: "anime", Enabled: true}
	varietyLibrary := model.Library{Base: model.Base{ID: "variety"}, Name: "综艺", Path: "/media/variety", Type: "variety", Enabled: true}
	if err := db.Create(&[]model.Library{movieLibrary, tvLibrary, animeLibrary, varietyLibrary}).Error; err != nil {
		t.Fatal(err)
	}
	rows := []model.Media{
		{Base: model.Base{ID: "movie-607"}, LibraryID: movieLibrary.ID, Title: "黑衣人", Path: "/media/movies/men-in-black.mkv", TMDbID: 607},
		// Moving a misclassified movie-library row into a TV library updates its
		// library without inventing a SeriesID, so the target library still has
		// to classify it as TV for external-ID presence checks.
		{Base: model.Base{ID: "tv-607"}, LibraryID: tvLibrary.ID, Title: "飞天小女警", Path: "/media/tv/powerpuff-girls/Season 1/E01.mkv", TMDbID: 607},
		{Base: model.Base{ID: "anime-106449"}, LibraryID: animeLibrary.ID, Title: "凡人修仙传", Path: "/media/anime/fanren/Season 1/E01.mkv", TMDbID: 106449},
		{Base: model.Base{ID: "variety-95396"}, LibraryID: varietyLibrary.ID, Title: "综艺测试", Path: "/media/variety/show/Season 1/E01.mkv", TMDbID: 95396},
	}
	if err := db.Create(&rows).Error; err != nil {
		t.Fatal(err)
	}
	svc := &service.Container{Repo: repos, Media: service.NewMediaService(&config.Config{}, zap.NewNop(), repos)}
	body, err := json.Marshal(externalIDPresenceRequest{
		TMDbRefs: []externalIDPresenceTMDbRef{
			{ID: 607, MediaType: "movie"},
			{ID: 607, MediaType: "tv"},
			{ID: 106449, MediaType: "tv"},
			{ID: 106449, MediaType: "movie"},
			{ID: 95396, MediaType: "tv"},
		},
	})
	if err != nil {
		t.Fatal(err)
	}
	w := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(w)
	c.Set(middleware.CtxUserID, viewer.ID)
	c.Set(middleware.CtxUserRole, viewer.Role)
	c.Request = httptest.NewRequest(http.MethodPost, "/api/media/external-id-presence", bytes.NewReader(body))
	c.Request.Header.Set("Content-Type", "application/json")
	externalIDPresenceHandler(svc)(c)
	if w.Code != http.StatusOK {
		t.Fatalf("status=%d body=%s", w.Code, w.Body.String())
	}
	var response externalIDPresenceResponse
	if err := json.Unmarshal(w.Body.Bytes(), &response); err != nil {
		t.Fatalf("decode response: %v", err)
	}
	want := []externalIDPresenceTMDbRef{
		{ID: 607, MediaType: "movie"},
		{ID: 607, MediaType: "tv"},
		{ID: 106449, MediaType: "tv"},
		{ID: 95396, MediaType: "tv"},
	}
	if !reflect.DeepEqual(response.TMDbRefs, want) {
		t.Fatalf("tmdb_refs=%v want=%v", response.TMDbRefs, want)
	}
	if len(response.TMDbIDs) != 0 {
		t.Fatalf("legacy tmdb_ids=%v want empty", response.TMDbIDs)
	}
}
