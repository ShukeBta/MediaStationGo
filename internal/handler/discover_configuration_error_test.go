package handler

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/ShukeBta/MediaStationGo/internal/config"
	"github.com/ShukeBta/MediaStationGo/internal/service"
	"github.com/gin-gonic/gin"
	"go.uber.org/zap"
)

func TestDiscoverMissingTMDbConfigurationIsVisibleAndNotCached(t *testing.T) {
	for _, provider := range []*service.TMDbProvider{nil, service.NewTMDbProvider(&config.Config{}, zap.NewNop(), nil)} {
		discover := service.NewDiscoverService(zap.NewNop(), provider)
		router := gin.New()
		router.GET("/feed", discoverFeedHandler(&service.Container{Discover: discover, Log: zap.NewNop()}))
		response := httptest.NewRecorder()
		router.ServeHTTP(response, httptest.NewRequest(http.MethodGet, "/feed?sections=tmdb_chinese_movie", nil))
		var body struct {
			Meta map[string]struct {
				Error string `json:"error"`
			} `json:"_meta"`
		}
		if err := json.Unmarshal(response.Body.Bytes(), &body); err != nil {
			t.Fatal(err)
		}
		if response.Code != http.StatusOK || !strings.Contains(body.Meta["tmdb_chinese_movie"].Error, "API Key") {
			t.Fatalf("missing actionable configuration error: %s", response.Body.String())
		}
		if _, ok := discover.CachedSection("tmdb_chinese_movie", 1); ok {
			t.Fatal("configuration failure cached as a successful empty result")
		}
	}
}
