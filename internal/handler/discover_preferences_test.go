package handler

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"slices"
	"testing"

	"github.com/gin-gonic/gin"
	"github.com/glebarez/sqlite"
	"gorm.io/gorm"

	"github.com/ShukeBta/MediaStationGo/internal/middleware"
	"github.com/ShukeBta/MediaStationGo/internal/model"
	"github.com/ShukeBta/MediaStationGo/internal/repository"
	"github.com/ShukeBta/MediaStationGo/internal/service"
)

func TestNormalizeDiscoverPreferenceSectionsPreservesUserOrderAndValidates(t *testing.T) {
	sections := []discoverSectionDef{
		{Key: "first"},
		{Key: "second"},
		{Key: "adult", Group: "adult"},
	}
	got, err := normalizeDiscoverPreferenceSections([]string{"second", "first", "second"}, sections, true, true)
	if err != nil || len(got) != 2 || got[0] != "second" || got[1] != "first" {
		t.Fatalf("got = %#v err=%v", got, err)
	}
	if _, err := normalizeDiscoverPreferenceSections([]string{"missing"}, sections, true, true); err == nil {
		t.Fatal("unknown section should be rejected")
	}
	got, err = normalizeDiscoverPreferenceSections([]string{"adult", "first"}, sections, false, false)
	if err != nil || len(got) != 1 || got[0] != "first" {
		t.Fatalf("adult filtered result = %#v err=%v", got, err)
	}
}

func TestNormalizeDiscoverPreferenceSectionsMigratesLegacyPerformerSection(t *testing.T) {
	sections := []discoverSectionDef{
		{Key: "adult_javdb_performers_new", Group: "adult"},
		{Key: "adult_javdb_performers_monthly", Group: "adult"},
		{Key: "adult_javdb_performers_fanza", Group: "adult"},
	}
	got, err := normalizeDiscoverPreferenceSections(
		[]string{"adult_javdb_performers"}, sections, true, false,
	)
	if err != nil || len(got) != 1 || got[0] != "adult_javdb_performers_monthly" {
		t.Fatalf("got = %#v err=%v", got, err)
	}
}

func TestNormalizeDiscoverPreferenceFD2PPVSort(t *testing.T) {
	for _, value := range []string{"release", "views", "likes", "favorites", "comments"} {
		got, err := normalizeDiscoverPreferenceFD2PPVSort(value)
		if err != nil || got != value {
			t.Fatalf("value=%q got=%q err=%v", value, got, err)
		}
	}
	if got, err := normalizeDiscoverPreferenceFD2PPVSort(""); err != nil || got != defaultDiscoverFD2PPVSort {
		t.Fatalf("default got=%q err=%v", got, err)
	}
	if _, err := normalizeDiscoverPreferenceFD2PPVSort("score"); err == nil {
		t.Fatal("unsupported sort should be rejected")
	}
}

func TestDiscoverPreferenceFD2PPVSortPersistsPerUser(t *testing.T) {
	gin.SetMode(gin.TestMode)
	db, err := gorm.Open(sqlite.Open(":memory:"), &gorm.Config{})
	if err != nil {
		t.Fatal(err)
	}
	if err := db.AutoMigrate(&model.UserDiscoverPreference{}); err != nil {
		t.Fatal(err)
	}
	svc := &service.Container{Repo: repository.New(db)}
	newRouter := func(userID string) *gin.Engine {
		router := gin.New()
		router.Use(func(c *gin.Context) {
			c.Set(middleware.CtxUserID, userID)
			c.Next()
		})
		router.GET("/discover/preferences", getDiscoverPreferenceHandler(svc))
		router.PUT("/discover/preferences", updateDiscoverPreferenceHandler(svc))
		return router
	}

	userOne := newRouter("user-1")
	putDiscoverPreference(t, userOne, `{"selected_sections":[],"adult_fd2ppv_sort":"views"}`)
	putDiscoverPreference(t, userOne, `{"adult_fd2ppv_sort":"favorites"}`)
	preference := getDiscoverPreference(t, userOne)
	if !preference.Configured || preference.AdultFD2PPVSort != "favorites" || len(preference.SelectedSections) != 0 {
		t.Fatalf("user-1 preference = %#v", preference)
	}

	preference = getDiscoverPreference(t, newRouter("user-2"))
	if preference.Configured || preference.AdultFD2PPVSort != defaultDiscoverFD2PPVSort {
		t.Fatalf("user-2 preference = %#v", preference)
	}
}

type discoverPreferenceResponse struct {
	Configured       bool     `json:"configured"`
	SelectedSections []string `json:"selected_sections"`
	AdultFD2PPVSort  string   `json:"adult_fd2ppv_sort"`
	SectionsVersion  int      `json:"sections_version"`
}

func TestDiscoverPreferencesAddChineseReleaseRailsOncePerAccount(t *testing.T) {
	for _, test := range []struct {
		name     string
		selected []string
		want     []string
	}{
		{"custom order", []string{"douban_hot_tv", "tmdb_chinese_tv", "tmdb_chinese_movie", "bangumi_calendar"},
			[]string{"douban_hot_tv", "tmdb_chinese_tv", "tmdb_chinese_latest_tv", "tmdb_chinese_upcoming_tv", "tmdb_chinese_movie", "tmdb_chinese_latest_movie", "tmdb_chinese_upcoming_movie", "bangumi_calendar"}},
		{"existing release position", []string{"tmdb_chinese_latest_movie", "douban_hot_tv", "tmdb_chinese_movie"},
			[]string{"tmdb_chinese_latest_movie", "douban_hot_tv", "tmdb_chinese_movie", "tmdb_chinese_upcoming_movie"}},
		{"custom without Chinese", []string{"douban_hot_tv", "tmdb_latest_movie"}, []string{"douban_hot_tv", "tmdb_latest_movie"}},
		{"empty", []string{}, []string{}},
	} {
		t.Run(test.name, func(t *testing.T) {
			svc, newRouter := newDiscoverPreferenceTestService(t)
			legacy := &model.UserDiscoverPreference{UserID: "legacy", SelectedSections: test.selected, AdultFD2PPVSort: "views"}
			if err := svc.Repo.DiscoverPreference.Upsert(t.Context(), legacy); err != nil {
				t.Fatal(err)
			}
			got := getDiscoverPreference(t, newRouter("legacy"))
			if !slices.Equal(got.SelectedSections, test.want) || got.SectionsVersion != model.DiscoverSectionsVersion || got.AdultFD2PPVSort != "views" {
				t.Fatalf("migrated preference = %+v, want %v", got, test.want)
			}
			persisted, err := svc.Repo.DiscoverPreference.FindByUserID(t.Context(), "legacy")
			if err != nil || persisted == nil || persisted.SectionsVersion != model.DiscoverSectionsVersion || !slices.Equal(persisted.SelectedSections, test.want) {
				t.Fatalf("migration was not persisted: %+v, %v", persisted, err)
			}
			// Removing the newly inserted sections must persist across requests
			// and devices, even when the Chinese popular rail remains enabled.
			body, _ := json.Marshal(map[string]any{"selected_sections": test.selected})
			putDiscoverPreference(t, newRouter("legacy"), string(body))
			got = getDiscoverPreference(t, newRouter("legacy"))
			if !slices.Equal(got.SelectedSections, test.selected) || got.SectionsVersion != model.DiscoverSectionsVersion {
				t.Fatalf("user's removal was undone: %+v", got)
			}
		})
	}
}

func TestDiscoverPreferenceExplicitSelectionDoesNotEnableExtraRails(t *testing.T) {
	_, newRouter := newDiscoverPreferenceTestService(t)
	router := newRouter("user")
	putDiscoverPreference(t, router, `{"selected_sections":["tmdb_chinese_movie"]}`)
	got := getDiscoverPreference(t, router)
	if !slices.Equal(got.SelectedSections, []string{"tmdb_chinese_movie"}) || got.SectionsVersion != model.DiscoverSectionsVersion {
		t.Fatalf("explicit selection was migrated: %+v", got)
	}
}

func newDiscoverPreferenceTestService(t *testing.T) (*service.Container, func(string) *gin.Engine) {
	t.Helper()
	gin.SetMode(gin.TestMode)
	db, err := gorm.Open(sqlite.Open(":memory:"), &gorm.Config{})
	if err != nil {
		t.Fatal(err)
	}
	if err := db.AutoMigrate(&model.UserDiscoverPreference{}, &model.APIConfig{}); err != nil {
		t.Fatal(err)
	}
	sqlDB, err := db.DB()
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = sqlDB.Close() })
	svc := &service.Container{Repo: repository.New(db)}
	return svc, func(userID string) *gin.Engine {
		router := gin.New()
		router.Use(func(c *gin.Context) {
			c.Set(middleware.CtxUserID, userID)
			c.Next()
		})
		router.GET("/discover/preferences", getDiscoverPreferenceHandler(svc))
		router.PUT("/discover/preferences", updateDiscoverPreferenceHandler(svc))
		return router
	}
}

func putDiscoverPreference(t *testing.T, router http.Handler, body string) {
	t.Helper()
	req := httptest.NewRequest(http.MethodPut, "/discover/preferences", bytes.NewBufferString(body))
	req.Header.Set("Content-Type", "application/json")
	recorder := httptest.NewRecorder()
	router.ServeHTTP(recorder, req)
	if recorder.Code != http.StatusOK {
		t.Fatalf("PUT status=%d body=%s", recorder.Code, recorder.Body.String())
	}
}

func getDiscoverPreference(t *testing.T, router http.Handler) discoverPreferenceResponse {
	t.Helper()
	recorder := httptest.NewRecorder()
	router.ServeHTTP(recorder, httptest.NewRequest(http.MethodGet, "/discover/preferences", nil))
	if recorder.Code != http.StatusOK {
		t.Fatalf("GET status=%d body=%s", recorder.Code, recorder.Body.String())
	}
	var preference discoverPreferenceResponse
	if err := json.Unmarshal(recorder.Body.Bytes(), &preference); err != nil {
		t.Fatal(err)
	}
	return preference
}
