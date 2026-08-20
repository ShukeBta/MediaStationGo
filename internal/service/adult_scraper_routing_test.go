package service

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/glebarez/sqlite"
	"go.uber.org/zap"
	"gorm.io/gorm"

	"github.com/ShukeBta/MediaStationGo/internal/model"
	"github.com/ShukeBta/MediaStationGo/internal/repository"
)

func TestAdultProviderRouting(t *testing.T) {
	db, err := gorm.Open(sqlite.Open(":memory:"), &gorm.Config{})
	if err != nil {
		t.Fatalf("failed to open sqlite: %v", err)
	}
	_ = db.AutoMigrate(&model.Setting{}, &model.ApiConfig{})

	repos := repository.New(db)

	mtServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/v1/movies/search" {
			q := r.URL.Query().Get("q")
			if q == "SSIS-001" {
				results := struct {
					Data []MetaTubeSearchResult `json:"data"`
				}{
					Data: []MetaTubeSearchResult{
						{
							ID:          "999",
							Number:      "SSIS-001",
							Title:       "河北彩花 専属デビュー",
							Provider:    "javdb",
							Actors:      []string{"河北彩花"},
							ReleaseDate: "2021-06-19",
							Score:       4.9,
						},
					},
				}
				_ = json.NewEncoder(w).Encode(results)
				return
			}
		}
		http.NotFound(w, r)
	}))
	defer mtServer.Close()

	// 1. Configure metatube settings
	_ = repos.Setting.Set(context.Background(), "adult.scraper.engine", "metatube")
	_ = repos.Setting.Set(context.Background(), "adult.scraper.metatube_server", mtServer.URL)

	adultProvider := NewAdultProvider(zap.NewNop(), nil, repos)

	// Test Search
	match, err := adultProvider.Search(context.Background(), "SSIS-001")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if match == nil {
		t.Fatal("expected match, got nil")
	}
	if match.OriginalName != "SSIS-001" {
		t.Errorf("expected SSIS-001, got %s", match.OriginalName)
	}
	if match.Year != 2021 {
		t.Errorf("expected 2021, got %d", match.Year)
	}

	// Test SearchCandidates
	candidates, err := adultProvider.SearchCandidates(context.Background(), "SSIS-001")
	if err != nil {
		t.Fatalf("unexpected error on candidates: %v", err)
	}
	if len(candidates) != 1 {
		t.Fatalf("expected 1 candidate, got %d", len(candidates))
	}

	// 2. Test auto mode with failing metatube query
	_ = repos.Setting.Set(context.Background(), "adult.scraper.engine", "auto")
	// Search for something not on mock metatube server; will attempt fallback
	mUnknown, _ := adultProvider.Search(context.Background(), "NONEXISTENT-999")
	if mUnknown != nil {
		t.Errorf("expected nil for nonexistent in auto mode when sources unavailable")
	}
}
