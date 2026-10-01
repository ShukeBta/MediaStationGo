package service

import (
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"net/url"
	"reflect"
	"strconv"
	"testing"
	"time"

	"github.com/ShukeBta/MediaStationGo/internal/config"
	"go.uber.org/zap"
)

func TestChineseTMDbSectionWindowsKeepDomesticFiltersAndMediaType(t *testing.T) {
	for _, mediaType := range []string{"movie", "tv"} {
		t.Run(mediaType, func(t *testing.T) {
			var requestedPages []int
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				if want := "/3/discover/" + mediaType; r.URL.Path != want {
					t.Errorf("path = %q, want %q", r.URL.Path, want)
				}
				query := r.URL.Query()
				for key, want := range map[string]string{
					"api_key": "test-key", "language": "zh-CN", "with_origin_country": "CN",
					"sort_by": "popularity.desc", "include_adult": "false",
				} {
					if got := query.Get(key); got != want {
						t.Errorf("query %s = %q, want %q", key, got, want)
					}
				}
				dateKey := "primary_release_date.lte"
				if mediaType == "tv" {
					dateKey = "first_air_date.lte"
					if query.Get("include_null_first_air_dates") != "false" {
						t.Error("TV discovery must exclude unknown first air dates")
					}
				}
				if _, err := time.Parse("2006-01-02", query.Get(dateKey)); err != nil {
					t.Errorf("missing or invalid release bound: %v", err)
				}
				page, _ := strconv.Atoi(query.Get("page"))
				requestedPages = append(requestedPages, page)
				results := make([]map[string]any, 0, 20)
				for id := (page-1)*20 + 1; id <= page*20 && id <= 39; id++ {
					item := map[string]any{"id": id}
					if mediaType == "tv" {
						item["name"] = fmt.Sprintf("国产剧 %d", id)
						item["first_air_date"] = "2026-01-15"
					} else {
						item["title"] = fmt.Sprintf("国产电影 %d", id)
						item["release_date"] = "2026-01-15"
					}
					results = append(results, item)
				}
				_ = json.NewEncoder(w).Encode(map[string]any{"results": results})
			}))
			t.Cleanup(server.Close)
			discover := newTMDbDiscoverTestService(server)
			for _, test := range []struct {
				page, firstID, count int
				upstreamPages        []int
			}{
				{1, 1, 19, []int{1}},
				{2, 19, 19, []int{1, 2}},
				{3, 37, 3, []int{2}},
			} {
				requestedPages = nil
				items, err := discover.TMDbSectionWindow(t.Context(), "tmdb_chinese_"+mediaType, test.page, 18)
				if err != nil {
					t.Fatal(err)
				}
				if len(items) != test.count {
					t.Fatalf("page %d: got %d items, want %d", test.page, len(items), test.count)
				}
				for index, item := range items {
					if item.TMDbID != test.firstID+index || item.MediaType != mediaType || item.Title == "" || item.ReleaseDate != "2026-01-15" {
						t.Errorf("page %d item %d: unexpected metadata %#v", test.page, index, item)
					}
				}
				if !reflect.DeepEqual(requestedPages, test.upstreamPages) {
					t.Errorf("page %d: requested %v, want %v", test.page, requestedPages, test.upstreamPages)
				}
			}
		})
	}
}

func TestChineseTMDbDiscoveryBoundsReleaseDate(t *testing.T) {
	now := time.Date(2026, time.October, 1, 12, 0, 0, 0, time.UTC)
	for _, test := range []struct{ mediaType, dateKey string }{
		{"movie", "primary_release_date.lte"},
		{"tv", "first_air_date.lte"},
	} {
		endpoint, err := url.Parse(tmdbChineseDiscoverPath(test.mediaType, now))
		if err != nil {
			t.Fatal(err)
		}
		if got := endpoint.Query().Get(test.dateKey); got != "2026-10-01" {
			t.Errorf("%s release bound = %q, want today's date", test.mediaType, got)
		}
	}
}

func TestDiscoverFetchMergesFiltersAndOverridesControlledQuery(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/3/discover/movie" {
			t.Errorf("path = %q", r.URL.Path)
		}
		want := url.Values{
			"api_key": {"test-key"}, "language": {"zh-CN"}, "page": {"3"},
			"with_origin_country": {"CN"}, "sort_by": {"popularity.desc"},
		}
		if got := r.URL.Query(); !reflect.DeepEqual(got, want) {
			t.Errorf("query = %v, want %v", got, want)
		}
		_, _ = w.Write([]byte(`{"results":[{"id":1,"title":"国产电影"}]}`))
	}))
	t.Cleanup(server.Close)
	discover := newTMDbDiscoverTestService(server)
	items, err := discover.Fetch(t.Context(), "/discover/movie?with_origin_country=CN&sort_by=popularity.desc&api_key=wrong&api_key=other&language=en-US&page=999", 3)
	if err != nil || len(items) != 1 || items[0].Title != "国产电影" {
		t.Fatalf("fetch items = %#v, error = %v", items, err)
	}
}

func TestTMDbDiscoveryMediaTypeIgnoresQuery(t *testing.T) {
	for _, test := range []struct{ path, want string }{
		{"/discover/tv?with_origin_country=CN", "tv"},
		{"/tv/popular", "tv"},
		{"/trending/tv/week", "tv"},
		{"/discover/movie?query=/tv/", "movie"},
		{"/movie/popular?next=/tv/popular", "movie"},
	} {
		items := tmdbMatchesToExternal(test.path, []Match{{TMDbID: 1, Title: "Title"}})
		if items[0].MediaType != test.want {
			t.Errorf("%s media type = %q, want %q", test.path, items[0].MediaType, test.want)
		}
	}
}

func newTMDbDiscoverTestService(server *httptest.Server) *DiscoverService {
	provider := NewTMDbProvider(&config.Config{Secrets: config.SecretsConfig{
		TMDbAPIKey: "test-key", TMDbAPIProxy: server.URL + "/3/",
	}}, zap.NewNop(), nil)
	discover := NewDiscoverService(zap.NewNop(), provider)
	discover.client = server.Client()
	return discover
}
