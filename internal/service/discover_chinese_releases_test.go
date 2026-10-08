package service

import (
	"net/url"
	"reflect"
	"testing"
	"time"
)

func TestChineseTMDbReleaseDiscoveryBoundsAndOrder(t *testing.T) {
	for _, dates := range []struct{ today, from, tomorrow, through string }{
		{"2026-10-08", "2025-10-08", "2026-10-09", "2027-10-08"},
		{"2026-12-31", "2025-12-31", "2027-01-01", "2027-12-31"},
	} {
		now, _ := time.Parse(time.DateOnly, dates.today)
		for _, mediaType := range []string{"movie", "tv"} {
			for _, upcoming := range []bool{false, true} {
				path, err := url.Parse(tmdbChineseReleaseDiscoverPath(mediaType, now, upcoming))
				if err != nil {
					t.Fatal(err)
				}
				dateKey := "primary_release_date"
				if mediaType == "tv" {
					dateKey = "first_air_date"
				}
				from, through, order := dates.from, dates.today, dateKey+".desc"
				if upcoming {
					from, through, order = dates.tomorrow, dates.through, dateKey+".asc"
				}
				for key, want := range map[string]string{
					dateKey + ".gte": from, dateKey + ".lte": through,
					"sort_by": order, "with_origin_country": "CN|HK|TW|MO",
					"include_adult": "false", "vote_count.gte": "", "vote_average.gte": "",
				} {
					if got := path.Query().Get(key); got != want {
						t.Errorf("%s upcoming=%v %s=%q, want %q", mediaType, upcoming, key, got, want)
					}
				}
			}
		}
	}
}

func TestChineseReleaseRecommendationsEnforceDateRangesAndSort(t *testing.T) {
	now := time.Date(2026, 10, 8, 12, 0, 0, 0, time.UTC)
	items := []ExternalMediaResult{
		{TMDbID: 1, ReleaseDate: "2025-10-08", Countries: []string{"CN"}},
		{TMDbID: 2, ReleaseDate: "2026-10-08", Countries: []string{"HK"}},
		{TMDbID: 3, ReleaseDate: "2026-10-09", Countries: []string{"TW"}},
		{TMDbID: 4, ReleaseDate: "2027-10-08", Countries: []string{"MO"}},
		{TMDbID: 5, ReleaseDate: "2025-10-07", Countries: []string{"CN"}},
		{TMDbID: 6, ReleaseDate: "2027-10-09", Countries: []string{"CN"}},
		{TMDbID: 7, Title: "中文译名", ReleaseDate: "2026-10-08", Countries: []string{"US"}},
		{TMDbID: 8, Title: "中文译名", ReleaseDate: "2026-10-09", Countries: []string{"JP"}},
		{TMDbID: 9, ReleaseDate: "", Countries: []string{"CN"}},
		{TMDbID: 10, ReleaseDate: "2026-99-99", Countries: []string{"CN"}},
		{TMDbID: 11, ReleaseDate: "2026-10-01", Countries: []string{"US", "CN"}},
		// TMDb movie list responses may omit origin_country; the query's
		// with_origin_country still supplies the production-country constraint.
		{TMDbID: 12, ReleaseDate: "2026-10-10"},
	}
	for _, mediaType := range []string{"movie", "tv"} {
		for _, test := range []struct {
			mode string
			want []int
		}{
			{"latest", []int{2, 11, 1}},
			{"upcoming", []int{3, 12, 4}},
		} {
			for _, prefix := range []string{"tmdb_", ""} {
				key := prefix + "chinese_" + test.mode + "_" + mediaType
				got := FilterChineseReleaseRecommendations(key, items, now)
				ids := make([]int, 0, len(got))
				for _, item := range got {
					ids = append(ids, item.TMDbID)
				}
				if !reflect.DeepEqual(ids, test.want) {
					t.Errorf("%s ids=%v, want %v", key, ids, test.want)
				}
			}
		}
	}
	if got := FilterChineseReleaseRecommendations("tmdb_chinese_movie", items, now); !reflect.DeepEqual(got, items) {
		t.Fatal("release-date filtering changed the popular rail")
	}
}
