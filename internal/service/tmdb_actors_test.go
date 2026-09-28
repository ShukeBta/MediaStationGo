package service

import (
	"net/http"
	"net/http/httptest"
	"testing"

	"go.uber.org/zap"

	"github.com/ShukeBta/MediaStationGo/internal/config"
)

func TestTMDbGetDetailsIncludesTopCast(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if got := r.URL.Query().Get("append_to_response"); got != "credits" {
			t.Fatalf("append_to_response = %q", got)
		}
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{
			"original_language":"zh",
			"production_countries":[{"iso_3166_1":"CN"}],
			"spoken_languages":[{"iso_639_1":"zh"}],
			"genres":[{"name":"剧情"}],
			"credits":{"cast":[{"name":"演员甲"},{"name":"演员乙"},{"name":"演员甲"}]}
		}`))
	}))
	defer server.Close()

	cfg := &config.Config{}
	cfg.Secrets.TMDbAPIKey = "test-key"
	cfg.Secrets.TMDbAPIProxy = server.URL
	provider := NewTMDbProvider(cfg, zap.NewNop(), nil)
	details, err := provider.GetDetails(t.Context(), 1, "movie")
	if err != nil {
		t.Fatal(err)
	}
	if len(details.Actors) != 2 || details.Actors[0] != "演员甲" || details.Actors[1] != "演员乙" {
		t.Fatalf("actors = %#v", details.Actors)
	}
}
