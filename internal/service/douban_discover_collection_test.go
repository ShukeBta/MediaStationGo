package service

import (
	"net/http"
	"strings"
	"testing"
)

func TestDoubanDiscoverRecoversWithSameSourceCollection(t *testing.T) {
	for _, key := range []string{"douban_hot_movie", "douban_hot_tv"} {
		t.Run(key, func(t *testing.T) {
			calls := 0
			provider := NewDoubanProvider(nil, nil)
			provider.client.Transport = roundTripFunc(func(req *http.Request) (*http.Response, error) {
				calls++
				if req.URL.Host == "movie.douban.com" {
					return doubanFixtureResponse(req, 403, `{}`), nil
				}
				collection := "movie_hot_gaia"
				if key == "douban_hot_tv" {
					collection = "tv_hot"
				}
				if req.URL.Host != "m.douban.com" || !strings.Contains(req.URL.Path, "/"+collection+"/items") || req.URL.Query().Get("start") != "18" || req.URL.Query().Get("count") != "19" {
					t.Fatalf("unexpected collection request: %s", req.URL)
				}
				return doubanFixtureResponse(req, 200, `{"subject_collection_items":[{"id":"123","title":"测试作品","year":"2026","cover":{"url":"https://img.doubanio.com/poster.jpg"},"rating":{"value":8.2}}]}`), nil
			})
			items, err := provider.DiscoverWindow(t.Context(), key, 2, 18)
			if err != nil || len(items) != 1 || calls != 2 {
				t.Fatalf("items=%#v err=%v calls=%d", items, err, calls)
			}
			item := items[0]
			mediaType := "movie"
			if key == "douban_hot_tv" {
				mediaType = "tv"
			}
			if item.Source != "douban" || item.DoubanID != "123" || item.TMDbID != 0 || item.MediaType != mediaType || item.Year != 2026 || item.Rating != 8.2 || item.ProviderURL != "https://movie.douban.com/subject/123/" {
				t.Fatalf("incorrect identity/metadata: %#v", item)
			}
		})
	}
}

func TestDoubanDiscoverDistinguishesEmptyPageFromInvalidResponse(t *testing.T) {
	for _, tt := range []struct {
		name, body string
		wantCalls  int
		wantError  bool
	}{
		{"empty end page", `{"subjects":[]}`, 1, false},
		{"missing subjects", `{"msg":"permission denied"}`, 2, true},
		{"null subjects", `{"subjects":null}`, 2, true},
		{"html login", `<html>login</html>`, 2, true},
	} {
		t.Run(tt.name, func(t *testing.T) {
			calls := 0
			provider := NewDoubanProvider(nil, nil)
			provider.client.Transport = roundTripFunc(func(req *http.Request) (*http.Response, error) {
				calls++
				return doubanFixtureResponse(req, 200, tt.body), nil
			})
			_, err := provider.Discover(t.Context(), "douban_hot_movie")
			if (err != nil) != tt.wantError || calls != tt.wantCalls {
				t.Fatalf("err=%v calls=%d", err, calls)
			}
		})
	}
}
