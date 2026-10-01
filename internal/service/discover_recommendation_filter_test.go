package service

import "testing"

func TestDiscoverRecommendationsExcludeOnlyIdentifiedShortDramas(t *testing.T) {
	for _, tc := range []struct {
		name   string
		item   ExternalMediaResult
		hidden bool
	}{
		{"explicit genre", ExternalMediaResult{Title: "示例作品", MediaType: "tv", Genres: []string{"剧情", "微短剧"}}, true},
		{"title label", ExternalMediaResult{Title: "示例作品（短剧）", MediaType: "tv"}, true},
		{"title prefix", ExternalMediaResult{Title: "【竖屏短剧】示例作品", MediaType: "tv"}, true},
		{"english label", ExternalMediaResult{Title: "Example (Short Drama)", MediaType: "tv"}, true},
		{"format description", ExternalMediaResult{Title: "示例作品", MediaType: "tv", Overview: "本剧是一部都市微短剧，讲述重逢的故事。"}, true},
		{"english format", ExternalMediaResult{Title: "Example", MediaType: "tv", Overview: "An original vertical drama about a reunion."}, true},
		{"Chinese television", ExternalMediaResult{Title: "国产正剧", MediaType: "tv", Countries: []string{"CN"}, TotalEpisodes: 100}, false},
		{"Chinese film", ExternalMediaResult{Title: "国产电影", MediaType: "movie", Countries: []string{"CN"}}, false},
		{"Chinese animation", ExternalMediaResult{Title: "国漫", MediaType: "tv", Genres: []string{"动画"}, DurationMinutes: 5, TotalEpisodes: 200}, false},
		{"variety", ExternalMediaResult{Title: "综艺", MediaType: "tv", Genres: []string{"真人秀"}, Overview: "本剧是一部短剧，选手据此展开讨论。"}, false},
		{"documentary", ExternalMediaResult{Title: "微短剧发展史", MediaType: "tv", Genres: []string{"纪录"}}, false},
		{"actor history", ExternalMediaResult{Title: "普通剧集", MediaType: "tv", Overview: "主演曾出演微短剧，如今回归荧屏。"}, false},
		{"plot mentions drama", ExternalMediaResult{Title: "短剧演员的生活", MediaType: "tv", Overview: "讲述一位短剧演员的成长。"}, false},
		{"brief episodes alone", ExternalMediaResult{Title: "儿童节目", MediaType: "tv", DurationMinutes: 3}, false},
		{"foreign television", ExternalMediaResult{Title: "Other Show", MediaType: "tv", Countries: []string{"US"}}, false},
	} {
		t.Run(tc.name, func(t *testing.T) {
			items := []ExternalMediaResult{tc.item}
			got := FilterDiscoverRecommendations(items)
			if (len(got) == 0) != tc.hidden {
				t.Fatalf("hidden = %v, want %v", len(got) == 0, tc.hidden)
			}
			if items[0].Title != tc.item.Title {
				t.Fatal("filter mutated source cache")
			}
		})
	}
}
