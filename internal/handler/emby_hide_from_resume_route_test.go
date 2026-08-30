package handler

import (
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

func TestEmbyHideFromResumeRoutePersistsHiddenState(t *testing.T) {
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
	lib := model.Library{Name: "电影", Path: `/media/movies`, Type: "movie", Enabled: true}
	if err := repos.Library.Create(t.Context(), &lib); err != nil {
		t.Fatalf("create library: %v", err)
	}
	for i, id := range []string{"movie-a", "movie-b"} {
		media := model.Media{
			Base:        model.Base{ID: id},
			LibraryID:   lib.ID,
			Title:       id,
			Path:        fmt.Sprintf(`/media/movies/%s.mkv`, id),
			DurationSec: 1200,
		}
		if err := repos.DB.Create(&media).Error; err != nil {
			t.Fatalf("create media: %v", err)
		}
		if err := repos.DB.Create(&model.PlaybackHistory{
			UserID:     "user-1",
			MediaID:    id,
			PositionMs: int64(120_000 + i),
			DurationMs: 1_200_000,
			WatchedAt:  time.Now().UTC().Add(time.Duration(i) * time.Minute),
			Completed:  false,
		}).Error; err != nil {
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

	req := httptest.NewRequest(http.MethodPost, "/emby/Users/user-1/Items/movie-a/HideFromResume?Hide=true", nil)
	req.Header.Set("X-Emby-Token", token)
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)
	if w.Code != http.StatusOK {
		t.Fatalf("hide from resume status = %d body=%s", w.Code, w.Body.String())
	}

	svc := service.NewEmbyService(&config.Config{}, zap.NewNop(), repos)
	out, err := svc.ResumeItems(t.Context(), "user-1")
	if err != nil {
		t.Fatalf("resume items: %v", err)
	}
	if out["TotalRecordCount"] != 1 {
		t.Fatalf("resume total after hide = %#v, want 1", out["TotalRecordCount"])
	}
	if items := out["Items"].([]map[string]any); items[0]["Id"] != "movie-b" {
		t.Fatalf("only movie-b should remain: %#v", items[0]["Id"])
	}

	req = httptest.NewRequest(http.MethodPost, "/emby/Users/user-1/Items/movie-a/HideFromResume?Hide=false", nil)
	req.Header.Set("X-Emby-Token", token)
	w = httptest.NewRecorder()
	router.ServeHTTP(w, req)
	if w.Code != http.StatusOK {
		t.Fatalf("restore to resume status = %d body=%s", w.Code, w.Body.String())
	}
	out, err = svc.ResumeItems(t.Context(), "user-1")
	if err != nil {
		t.Fatalf("resume items after restore: %v", err)
	}
	if out["TotalRecordCount"] != 2 {
		t.Fatalf("resume total after Hide=false = %#v, want 2", out["TotalRecordCount"])
	}

	// Filmly sends this route without Hide while synchronizing Resume. Missing
	// is not an instruction to hide and must therefore preserve the row.
	req = httptest.NewRequest(http.MethodPost, "/emby/Users/user-1/Items/movie-a/HideFromResume", nil)
	req.Header.Set("X-Emby-Token", token)
	w = httptest.NewRecorder()
	router.ServeHTTP(w, req)
	if w.Code != http.StatusOK {
		t.Fatalf("missing Hide status = %d body=%s", w.Code, w.Body.String())
	}
	out, err = svc.ResumeItems(t.Context(), "user-1")
	if err != nil {
		t.Fatalf("resume items after missing Hide: %v", err)
	}
	if out["TotalRecordCount"] != 2 {
		t.Fatalf("resume total after missing Hide = %#v, want 2", out["TotalRecordCount"])
	}
}
