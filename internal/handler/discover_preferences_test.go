package handler

import (
	"bytes"
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
