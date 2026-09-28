package handler

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/ShukeBta/MediaStationGo/internal/middleware"
	"github.com/ShukeBta/MediaStationGo/internal/model"
	"github.com/ShukeBta/MediaStationGo/internal/repository"
	"github.com/ShukeBta/MediaStationGo/internal/service"
	"github.com/gin-gonic/gin"
	"github.com/glebarez/sqlite"
	"gorm.io/gorm"
	"gorm.io/gorm/logger"
)

func TestTMDbCatalogHTTPRespectsLibraryAndAdultVisibility(t *testing.T) {
	gin.SetMode(gin.TestMode)
	db, err := gorm.Open(sqlite.Open(":memory:"), &gorm.Config{Logger: logger.Default.LogMode(logger.Silent)})
	if err != nil {
		t.Fatal(err)
	}
	sqlDB, _ := db.DB()
	sqlDB.SetMaxOpenConns(1)
	t.Cleanup(func() { _ = sqlDB.Close() })
	if err := db.AutoMigrate(&model.Media{}, &model.Library{}, &model.User{}, &model.Setting{}, &model.PlayProfile{}, &model.TMDbCatalogItem{}, &model.TMDbCatalogJob{}); err != nil {
		t.Fatal(err)
	}
	repos := repository.New(db)
	svc := &service.Container{Repo: repos, TMDbCatalog: service.NewTMDbCatalogService(repos, nil, nil)}
	user := model.User{Base: model.Base{ID: "viewer"}, Username: "viewer", Role: "user", AllowedLibraryIDs: []string{"visible"}, HideAdult: true}
	if err := db.Create(&user).Error; err != nil {
		t.Fatal(err)
	}
	for _, item := range []model.Media{
		{Base: model.Base{ID: "local-visible"}, LibraryID: "visible", Path: "/visible/1.mkv", Title: "Visible", TMDbID: 1, SeasonNum: 1, EpisodeNum: 1},
		{Base: model.Base{ID: "secret-library-media"}, LibraryID: "secret", Path: "/secret/2.mkv", Title: "Secret", TMDbID: 1, SeasonNum: 1, EpisodeNum: 2},
		{Base: model.Base{ID: "hidden-adult-media"}, LibraryID: "visible", Path: "/visible/3.mkv", Title: "Adult", TMDbID: 1, SeasonNum: 1, EpisodeNum: 3, NSFW: true},
	} {
		if err := db.Create(&item).Error; err != nil {
			t.Fatal(err)
		}
	}
	router := gin.New()
	router.Use(func(c *gin.Context) { c.Set(middleware.CtxUserID, "viewer"); c.Set(middleware.CtxUserRole, "user") })
	router.GET("/media/:id/tmdb-catalog", tmdbSeriesCatalogHandler(svc, false))
	router.POST("/media/:id/tmdb-catalog/refresh", middleware.AdminRequired(), tmdbSeriesCatalogHandler(svc, true))
	for _, id := range []string{"secret-library-media", "hidden-adult-media", "missing"} {
		response := httptest.NewRecorder()
		router.ServeHTTP(response, httptest.NewRequest(http.MethodGet, "/media/"+id+"/tmdb-catalog", nil))
		if response.Code != http.StatusNotFound {
			t.Fatalf("inaccessible %s status %d: %s", id, response.Code, response.Body.String())
		}
	}
	response := httptest.NewRecorder()
	router.ServeHTTP(response, httptest.NewRequest(http.MethodGet, "/media/local-visible/tmdb-catalog", nil))
	if response.Code != http.StatusOK || !strings.Contains(response.Body.String(), "local-visible") || strings.Contains(response.Body.String(), "secret-library-media") || strings.Contains(response.Body.String(), "hidden-adult-media") {
		t.Fatalf("catalog leaked availability: %d %s", response.Code, response.Body.String())
	}
	response = httptest.NewRecorder()
	router.ServeHTTP(response, httptest.NewRequest(http.MethodPost, "/media/local-visible/tmdb-catalog/refresh", nil))
	if response.Code != http.StatusForbidden {
		t.Fatalf("ordinary user can refresh: %d", response.Code)
	}
}
