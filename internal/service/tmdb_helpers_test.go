package service

import (
	"reflect"
	"testing"
)

func TestTMDbGenreNamesConvertKnownIDsAndIgnoreUnknown(t *testing.T) {
	tests := []struct {
		name      string
		mediaType string
		ids       []int
		want      []string
	}{
		{
			name:      "movie genres",
			mediaType: "movie",
			ids:       []int{28, 12, 16, 999999, 28},
			want:      []string{"动作", "冒险", "动画"},
		},
		{
			name:      "tv genres",
			mediaType: "tv",
			ids:       []int{10759, 10765, 18, 999999},
			want:      []string{"动作冒险", "科幻奇幻", "剧情"},
		},
		{
			name:      "anime uses tv genres",
			mediaType: "anime",
			ids:       []int{16, 10759, 35},
			want:      []string{"动画", "动作冒险", "喜剧"},
		},
		{
			name:      "missing type uses known ids",
			mediaType: "",
			ids:       []int{28, 10765, 999999},
			want:      []string{"动作", "科幻奇幻"},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := tmdbGenreNames(tt.mediaType, tt.ids); !reflect.DeepEqual(got, tt.want) {
				t.Fatalf("tmdbGenreNames(%q, %v) = %v, want %v", tt.mediaType, tt.ids, got, tt.want)
			}
		})
	}
}

func TestTMDbSearchResultsExposeChineseGenreNames(t *testing.T) {
	provider := &TMDbProvider{}
	movie := provider.movieSearchResultToMatch(tmdbMovieSearchResult{GenreIDs: []int{28, 12, 999999}})
	if want := []string{"动作", "冒险"}; !reflect.DeepEqual(movie.Genres, want) {
		t.Fatalf("movie search genres = %v, want %v", movie.Genres, want)
	}
	tv := provider.tvSearchResultToMatch(tmdbTVSearchResult{GenreIDs: []int{10759, 10765, 999999}})
	if want := []string{"动作冒险", "科幻奇幻"}; !reflect.DeepEqual(tv.Genres, want) {
		t.Fatalf("tv search genres = %v, want %v", tv.Genres, want)
	}
}

func TestNormalizeTMDbGenreValuesConvertsNumericAndPreservesNames(t *testing.T) {
	got := normalizeTMDbGenreValues("tv", []string{"10759", "18", "999999", "剧情", "10759", ""})
	want := []string{"动作冒险", "剧情"}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("normalizeTMDbGenreValues() = %v, want %v", got, want)
	}
}
