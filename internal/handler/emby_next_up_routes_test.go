package handler

import (
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/glebarez/sqlite"
	"go.uber.org/zap"
	"gorm.io/gorm"

	"github.com/ShukeBta/MediaStationGo/internal/config"
	"github.com/ShukeBta/MediaStationGo/internal/model"
	"github.com/ShukeBta/MediaStationGo/internal/repository"
	"github.com/ShukeBta/MediaStationGo/internal/service"
)

func TestEmbyNextUpRouteReturnsInProgressEpisode(t *testing.T) {
	gin.SetMode(gin.TestMode)
	db, err := gorm.Open(sqlite.Open(":memory:"), &gorm.Config{})
	if err != nil {
		t.Fatalf("open db: %v", err)
	}
	if sqlDB, err := db.DB(); err == nil {
		sqlDB.SetMaxOpenConns(1)
	}
	if err := db.AutoMigrate(model.AllModels()...); err != nil {
		t.Fatalf("migrate: %v", err)
	}
	repos := repository.New(db)
	if err := repos.User.Create(t.Context(), &model.User{
		Base:         model.Base{ID: "user-1"},
		Username:     "tester",
		PasswordHash: "x",
		Role:         "admin",
		Tier:         "plus",
		IsActive:     true,
	}); err != nil {
		t.Fatalf("create user: %v", err)
	}
	lib := model.Library{Name: "动漫", Path: `/media/anime`, Type: "anime", Enabled: true}
	if err := repos.Library.Create(t.Context(), &lib); err != nil {
		t.Fatalf("create library: %v", err)
	}
	for ep := 1; ep <= 12; ep++ {
		media := model.Media{
			Base:        model.Base{ID: epTestID(ep)},
			LibraryID:   lib.ID,
			Title:       "测试番",
			Path:        fmt.Sprintf(`/media/anime/测试番/S01E%02d.mkv`, ep),
			SeasonNum:   1,
			EpisodeNum:  ep,
			DurationSec: 1200,
		}
		if err := repos.DB.Create(&media).Error; err != nil {
			t.Fatalf("create episode %d: %v", ep, err)
		}
	}
	base := time.Now().UTC()
	for _, row := range []model.PlaybackHistory{
		{UserID: "user-1", MediaID: epTestID(3), PositionMs: 1_200_000, DurationMs: 1_200_000, WatchedAt: base.Add(-2 * time.Hour), Completed: true},
		{UserID: "user-1", MediaID: epTestID(4), PositionMs: 45_000, DurationMs: 1_200_000, WatchedAt: base.Add(-30 * time.Minute), Completed: false},
	} {
		if err := repos.DB.Create(&row).Error; err != nil {
			t.Fatalf("create history: %v", err)
		}
	}

	const secret = "test-secret"
	router := gin.New()
	registerEmbyRoutes(router, secret, &service.Container{
		Repo: repos,
		Emby: service.NewEmbyService(&config.Config{}, zap.NewNop(), repos),
	})
	token := signedTestToken(t, secret)

	groups, err := service.NewEmbyService(&config.Config{}, zap.NewNop(), repos).Items(t.Context(), service.ItemsParams{
		UserID: "user-1", IncludeItemTypes: []string{"Series"}, Limit: 10,
	})
	if err != nil {
		t.Fatalf("list series: %v", err)
	}
	seriesItems := groups["Items"].([]map[string]any)
	if len(seriesItems) == 0 {
		t.Fatalf("expected the seeded series to be listed, got %#v", groups)
	}
	seriesID, ok := seriesItems[0]["Id"].(string)
	if !ok || seriesID == "" {
		t.Fatalf("series id missing: %#v", seriesItems[0])
	}

	req := httptest.NewRequest(http.MethodGet, "/emby/Shows/NextUp?UserId=user-1&SeriesId="+seriesID+"&Limit=1", nil)
	req.Header.Set("X-Emby-Token", token)
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)
	if w.Code != http.StatusOK {
		t.Fatalf("next up status = %d body=%s", w.Code, w.Body.String())
	}
	var payload struct {
		Items            []map[string]any `json:"Items"`
		TotalRecordCount int              `json:"TotalRecordCount"`
	}
	if err := json.Unmarshal(w.Body.Bytes(), &payload); err != nil {
		t.Fatalf("decode response: %v body=%s", err, w.Body.String())
	}
	if payload.TotalRecordCount != 1 || len(payload.Items) != 1 {
		t.Fatalf("expected exactly one next-up episode, got %#v", payload)
	}
	if payload.Items[0]["Id"] != epTestID(4) {
		t.Fatalf("next up should resume the in-progress episode, got %#v", payload.Items[0]["Id"])
	}

	homeReq := httptest.NewRequest(http.MethodGet, "/emby/Shows/NextUp", nil)
	homeReq.Header.Set("X-Emby-Token", token)
	homeW := httptest.NewRecorder()
	router.ServeHTTP(homeW, homeReq)
	if homeW.Code != http.StatusOK {
		t.Fatalf("home next up status = %d body=%s", homeW.Code, homeW.Body.String())
	}
	if err := json.Unmarshal(homeW.Body.Bytes(), &payload); err != nil {
		t.Fatalf("decode home response: %v body=%s", err, homeW.Body.String())
	}
	if len(payload.Items) != 1 || payload.Items[0]["Id"] != epTestID(4) {
		t.Fatalf("home next up should contain the in-progress episode, got %#v", payload)
	}
}

func epTestID(ep int) string {
	return fmt.Sprintf("episode-%02d", ep)
}
