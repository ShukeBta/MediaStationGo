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
	for i, id := range []string{"episode-209", "episode-210", "episode-211"} {
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
		if id == "episode-211" {
			continue
		}
		if err := repos.DB.Create(&model.PlaybackHistory{
			UserID:     "user-1",
			MediaID:    id,
			PositionMs: int64(120_000 + i),
			DurationMs: 1_200_000,
			WatchedAt:  time.Now().UTC().Add(-time.Hour + time.Duration(i)*time.Minute),
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

	req := httptest.NewRequest(http.MethodPost, "/emby/Users/user-1/Items/episode-210/HideFromResume?Hide=true", nil)
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
	if items := out["Items"].([]map[string]any); items[0]["Id"] != "episode-209" {
		t.Fatalf("only episode-209 should remain: %#v", items[0]["Id"])
	}

	req = httptest.NewRequest(http.MethodPost, "/emby/Users/user-1/Items/episode-210/HideFromResume?Hide=false", nil)
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

	// A clear with an explicit Hide value persists normally.
	for _, id := range []string{"episode-209", "episode-210"} {
		req = httptest.NewRequest(http.MethodPost, "/emby/Users/user-1/Items/"+id+"/HideFromResume?Hide=true", nil)
		req.Header.Set("X-Emby-Token", token)
		req.Header.Set("User-Agent", "Filmly/3.0")
		w = httptest.NewRecorder()
		router.ServeHTTP(w, req)
		if w.Code != http.StatusOK {
			t.Fatalf("clear %s status = %d body=%s", id, w.Code, w.Body.String())
		}
	}

	// Filmly also emits ambiguous parameterless calls while synchronizing
	// Resume. They must be ignored instead of restoring or hiding a row.
	for _, id := range []string{"episode-209", "episode-210"} {
		req = httptest.NewRequest(http.MethodPost, "/emby/Users/user-1/Items/"+id+"/HideFromResume", nil)
		req.Header.Set("X-Emby-Token", token)
		req.Header.Set("User-Agent", "Filmly/3.0")
		w = httptest.NewRecorder()
		router.ServeHTTP(w, req)
		if w.Code != http.StatusOK {
			t.Fatalf("clear %s without Hide status = %d body=%s", id, w.Code, w.Body.String())
		}
		var responseUserData struct {
			Key    string `json:"Key"`
			ItemID string `json:"ItemId"`
		}
		if err := json.Unmarshal(w.Body.Bytes(), &responseUserData); err != nil {
			t.Fatalf("decode Filmly parameterless response for %s: %v body=%s", id, err, w.Body.String())
		}
		if responseUserData.Key != id || responseUserData.ItemID != id {
			t.Fatalf("Filmly parameterless response identity for %s = %#v", id, responseUserData)
		}
	}
	out, err = svc.ResumeItems(t.Context(), "user-1")
	if err != nil {
		t.Fatalf("resume items after ignored Filmly calls: %v", err)
	}
	if out["TotalRecordCount"] != 0 {
		t.Fatalf("resume total after ignored Filmly calls = %#v, want 0", out["TotalRecordCount"])
	}

	// After the clear, playing episode 211 must leave both the home Resume query
	// and Filmly's full Resume list with the same single row.
	if err := svc.RecordProgress(t.Context(), "user-1", "episode-211", 232_000*10_000, 1_322_000*10_000); err != nil {
		t.Fatalf("record episode-211 progress: %v", err)
	}
	req = httptest.NewRequest(http.MethodGet, "/emby/Users/user-1/Items/Resume?Limit=72&MediaTypes=Video&StartIndex=0", nil)
	req.Header.Set("X-Emby-Token", token)
	req.Header.Set("User-Agent", "Filmly/3.0")
	w = httptest.NewRecorder()
	router.ServeHTTP(w, req)
	if w.Code != http.StatusOK {
		t.Fatalf("Filmly Resume status = %d body=%s", w.Code, w.Body.String())
	}
	var page struct {
		Items []struct {
			ID       string `json:"Id"`
			UserData struct {
				Key    string `json:"Key"`
				ItemID string `json:"ItemId"`
			} `json:"UserData"`
		} `json:"Items"`
		TotalRecordCount int `json:"TotalRecordCount"`
	}
	if err := json.Unmarshal(w.Body.Bytes(), &page); err != nil {
		t.Fatalf("decode Filmly Resume: %v body=%s", err, w.Body.String())
	}
	if page.TotalRecordCount != 1 || len(page.Items) != 1 || page.Items[0].ID != "episode-211" {
		t.Fatalf("Filmly Resume after replay = %#v, want only episode-211", page)
	}
	if page.Items[0].UserData.Key != "episode-211" || page.Items[0].UserData.ItemID != "episode-211" {
		t.Fatalf("Filmly Resume episode-211 identity = %#v", page.Items[0].UserData)
	}

	// Reproduce Filmly's N-1 synchronization: after the newest row, it calls
	// parameterless HideFromResume for every remaining row. Those responses must
	// identify their own items and the subsequent Resume page must stay stable.
	for _, id := range []string{"episode-209", "episode-210"} {
		req = httptest.NewRequest(http.MethodPost, "/emby/Users/user-1/Items/"+id+"/HideFromResume?Hide=false", nil)
		req.Header.Set("X-Emby-Token", token)
		req.Header.Set("User-Agent", "Filmly/3.0")
		w = httptest.NewRecorder()
		router.ServeHTTP(w, req)
		if w.Code != http.StatusOK {
			t.Fatalf("restore %s before Filmly N-1 sync status = %d body=%s", id, w.Code, w.Body.String())
		}
	}

	assertFilmlyResumeOrder := func(stage string) {
		t.Helper()
		req = httptest.NewRequest(http.MethodGet, "/emby/Users/user-1/Items/Resume?Limit=12&MediaTypes=Video&StartIndex=0", nil)
		req.Header.Set("X-Emby-Token", token)
		req.Header.Set("User-Agent", "Filmly/3.0")
		w = httptest.NewRecorder()
		router.ServeHTTP(w, req)
		if w.Code != http.StatusOK {
			t.Fatalf("Filmly Resume %s status = %d body=%s", stage, w.Code, w.Body.String())
		}
		page = struct {
			Items []struct {
				ID       string `json:"Id"`
				UserData struct {
					Key    string `json:"Key"`
					ItemID string `json:"ItemId"`
				} `json:"UserData"`
			} `json:"Items"`
			TotalRecordCount int `json:"TotalRecordCount"`
		}{}
		if err := json.Unmarshal(w.Body.Bytes(), &page); err != nil {
			t.Fatalf("decode Filmly Resume %s: %v body=%s", stage, err, w.Body.String())
		}
		wantIDs := []string{"episode-211", "episode-210", "episode-209"}
		if page.TotalRecordCount != len(wantIDs) || len(page.Items) != len(wantIDs) {
			t.Fatalf("Filmly Resume %s count = %#v, want %d", stage, page, len(wantIDs))
		}
		for i, wantID := range wantIDs {
			item := page.Items[i]
			if item.ID != wantID || item.UserData.Key != wantID || item.UserData.ItemID != wantID {
				t.Fatalf("Filmly Resume %s item %d = %#v, want identity %s", stage, i, item, wantID)
			}
		}
	}

	assertFilmlyResumeOrder("before N-1 sync")
	for _, id := range []string{"episode-210", "episode-209"} {
		req = httptest.NewRequest(http.MethodPost, "/emby/Users/user-1/Items/"+id+"/HideFromResume", nil)
		req.Header.Set("X-Emby-Token", token)
		req.Header.Set("User-Agent", "Filmly/3.0")
		w = httptest.NewRecorder()
		router.ServeHTTP(w, req)
		if w.Code != http.StatusOK {
			t.Fatalf("Filmly N-1 sync %s status = %d body=%s", id, w.Code, w.Body.String())
		}
		var responseUserData struct {
			Key    string `json:"Key"`
			ItemID string `json:"ItemId"`
		}
		if err := json.Unmarshal(w.Body.Bytes(), &responseUserData); err != nil {
			t.Fatalf("decode Filmly N-1 sync response for %s: %v body=%s", id, err, w.Body.String())
		}
		if responseUserData.Key != id || responseUserData.ItemID != id {
			t.Fatalf("Filmly N-1 sync response identity for %s = %#v", id, responseUserData)
		}
	}
	assertFilmlyResumeOrder("after N-1 sync")
}
