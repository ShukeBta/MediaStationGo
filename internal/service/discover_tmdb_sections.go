package service

import (
	"net/url"
	"strings"
	"time"
)

func tmdbDiscoverPath(key string) string {
	switch key {
	case "tmdb_trending_day", "trending_day":
		return "/trending/movie/day"
	case "tmdb_trending_week", "trending_week":
		return "/trending/movie/week"
	case "tmdb_latest_movie", "latest_movie":
		return "/movie/now_playing"
	case "tmdb_latest_tv", "latest_tv":
		return "/tv/on_the_air"
	case "tmdb_popular_movie", "popular_movie":
		return "/movie/popular"
	case "tmdb_popular_tv", "popular_tv":
		return "/tv/popular"
	case "tmdb_chinese_movie", "chinese_movie":
		return tmdbChineseDiscoverPath("movie", time.Now())
	case "tmdb_chinese_tv", "chinese_tv":
		return tmdbChineseDiscoverPath("tv", time.Now())
	case "tmdb_chinese_anime", "chinese_anime":
		return tmdbChineseDiscoverPath("tv", time.Now(), "16")
	case "tmdb_chinese_variety", "chinese_variety":
		return tmdbChineseDiscoverPath("tv", time.Now(), "10764|10767")
	case "tmdb_top_rated_movie", "top_rated_movie":
		return "/movie/top_rated"
	case "tmdb_upcoming_movie", "upcoming_movie":
		return "/movie/upcoming"
	default:
		return ""
	}
}

// Origin filtering includes mainland, Hong Kong, Taiwan and Macao productions;
// a translated Chinese title alone does not establish a Chinese production.
func tmdbChineseDiscoverPath(mediaType string, now time.Time, genres ...string) string {
	query := url.Values{
		"with_origin_country": {"CN|HK|TW|MO"},
		"sort_by":             {"popularity.desc"},
		"include_adult":       {"false"},
	}
	if len(genres) > 0 && genres[0] != "" {
		query.Set("with_genres", genres[0])
	}
	date := now.Format("2006-01-02")
	if mediaType == "tv" {
		query.Set("first_air_date.lte", date)
		query.Set("include_null_first_air_dates", "false")
	} else {
		query.Set("primary_release_date.lte", date)
	}
	return "/discover/" + mediaType + "?" + query.Encode()
}

func tmdbDiscoverMediaType(path string) string {
	endpoint, err := url.Parse(path)
	if err == nil && strings.Contains(endpoint.Path+"/", "/tv/") {
		return "tv"
	}
	return "movie"
}
