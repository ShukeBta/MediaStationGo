package handler

import (
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/ShukeBta/MediaStationGo/internal/service"
	"github.com/gin-gonic/gin"
	"go.uber.org/zap"
)

func TestDiscoverCachedRecommendationsFilterShortsWithoutLosingNextPage(t *testing.T) {
	discover := service.NewDiscoverService(zap.NewNop(), nil)
	items := make([]service.ExternalMediaResult, discoverWorkPageSize+1)
	for i := range items {
		items[i] = service.ExternalMediaResult{Title: fmt.Sprintf("国产正剧%d", i), MediaType: "tv", Countries: []string{"CN"}}
	}
	items[0].Genres = []string{"短剧"}
	discover.RememberSection("tmdb_chinese_tv", 1, items)
	discover.RememberSection("tmdb_chinese_tv", 2, items[discoverWorkPageSize:])
	router := gin.New()
	router.GET("/discover/feed", discoverFeedHandler(&service.Container{Discover: discover, Log: zap.NewNop()}))
	for _, page := range []int{1, 2} {
		recorder := httptest.NewRecorder()
		router.ServeHTTP(recorder, httptest.NewRequest(http.MethodGet, fmt.Sprintf("/discover/feed?sections=tmdb_chinese_tv&page=%d", page), nil))
		var body struct {
			Items []service.ExternalMediaResult `json:"tmdb_chinese_tv"`
			Meta  map[string]struct {
				HasNext bool `json:"has_next"`
			} `json:"_meta"`
		}
		if recorder.Code != http.StatusOK {
			t.Fatalf("status %d: %s", recorder.Code, recorder.Body.String())
		}
		if err := json.Unmarshal(recorder.Body.Bytes(), &body); err != nil {
			t.Fatal(err)
		}
		if page == 1 && (len(body.Items) != 17 || body.Items[0].Title != "国产正剧1" || body.Items[16].Title != "国产正剧17" || !body.Meta["tmdb_chinese_tv"].HasNext) {
			t.Fatalf("first page = %s", recorder.Body.String())
		}
		if page == 2 && (len(body.Items) != 1 || body.Items[0].Title != "国产正剧18" || body.Meta["tmdb_chinese_tv"].HasNext) {
			t.Fatalf("next page = %s", recorder.Body.String())
		}
	}
	if cached, _ := discover.CachedSection("tmdb_chinese_tv", 1); len(cached) != 19 {
		t.Fatal("filter modified underlying page cache")
	}
}
