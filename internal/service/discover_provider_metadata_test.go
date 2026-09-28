package service

import (
	"net/http"
	"net/http/httptest"
	"testing"

	"go.uber.org/zap"

	"github.com/ShukeBta/MediaStationGo/internal/config"
)

func TestDoubanDiscoverKeepsProviderURL(t *testing.T) {
	provider := NewDoubanProvider(&config.Config{}, zap.NewNop())
	provider.client = &http.Client{Transport: discoverSearchRoundTripFunc(func(req *http.Request) (*http.Response, error) {
		server := httptest.NewRecorder()
		server.Header().Set("Content-Type", "application/json")
		_, _ = server.WriteString(`{"subjects":[{"id":"1292052","title":"肖申克的救赎","rate":"9.7","cover":"https://img.example/poster.jpg","url":"https://movie.douban.com/subject/1292052/"}]}`)
		return server.Result(), nil
	})}

	items, err := provider.Discover(t.Context(), "douban_hot_movie")
	if err != nil {
		t.Fatal(err)
	}
	if len(items) != 1 || items[0].ProviderURL != "https://movie.douban.com/subject/1292052/" {
		t.Fatalf("items = %#v", items)
	}
}

func TestBangumiCalendarKeepsReleaseDateAndProviderURL(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`[{"items":[{"id":42,"name":"Original","name_cn":"中文名","summary":"简介","air_date":"2026-07-19","images":{"large":"https://img.example/poster.jpg"},"rating":{"score":8.5}}]}]`))
	}))
	defer server.Close()

	provider := NewBangumiProvider(&config.Config{}, zap.NewNop())
	provider.base = server.URL
	items, err := provider.Calendar(t.Context())
	if err != nil {
		t.Fatal(err)
	}
	if len(items) != 1 || items[0].ReleaseDate != "2026-07-19" || items[0].ProviderURL != "https://bgm.tv/subject/42" {
		t.Fatalf("items = %#v", items)
	}
}
