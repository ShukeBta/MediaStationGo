package service

import (
	"testing"

	"go.uber.org/zap"

	"github.com/ShukeBta/MediaStationGo/internal/config"
	"github.com/ShukeBta/MediaStationGo/internal/model"
	"github.com/ShukeBta/MediaStationGo/internal/repository"
)

func TestClassifyMediaCategoryMatchesSmartRules(t *testing.T) {
	tests := []struct {
		name  string
		input mediaClassifyInput
		want  string
	}{
		{
			name: "movie animation first",
			input: mediaClassifyInput{
				MediaType: "movie",
				Title:     "Robot Dreams",
				Countries: []string{"ES"},
				Genres:    []string{"Animation"},
			},
			want: "动画电影",
		},
		{
			name: "translated tmdb animation title uses genre before title script",
			input: mediaClassifyInput{
				MediaType: "movie",
				Title:     "寻龙记",
				Languages: []string{"en"},
				Countries: []string{"NL"},
				Genres:    []string{"16"},
			},
			want: "动画电影",
		},
		{
			name: "translated western movie title does not become chinese movie",
			input: mediaClassifyInput{
				MediaType: "movie",
				Title:     "大雄兔",
				Languages: []string{"en"},
				Countries: []string{"NL"},
				Genres:    []string{"Comedy"},
			},
			want: "欧美电影",
		},
		{
			name: "movie animation source category fallback",
			input: mediaClassifyInput{
				MediaType: "movie",
				Title:     "Sintel 2010 1080p",
				Category:  "动画电影",
			},
			want: "动画电影",
		},
		{
			name: "tv variety by genre",
			input: mediaClassifyInput{
				MediaType: "tv",
				Title:     "声生不息",
				Countries: []string{"CN"},
				Genres:    []string{"Reality"},
			},
			want: "综艺",
		},
		{
			name: "anime china",
			input: mediaClassifyInput{
				MediaType: "anime",
				Countries: []string{"CN"},
				Genres:    []string{"Animation"},
			},
			want: "国漫",
		},
		{
			name: "tv documentary before region",
			input: mediaClassifyInput{
				MediaType: "tv",
				Countries: []string{"US"},
				Genres:    []string{"Documentary"},
			},
			want: "纪录片",
		},
		{
			name: "chinese movie title without metadata",
			input: mediaClassifyInput{
				MediaType: "movie",
				Title:     "流浪地球2 2023 2160p",
			},
			want: "华语电影",
		},
		{
			name: "latin movie title without metadata",
			input: mediaClassifyInput{
				MediaType: "movie",
				Title:     "Dune 2021 2160p",
			},
			want: "欧美电影",
		},
		{
			name: "chinese tv title without metadata",
			input: mediaClassifyInput{
				MediaType: "tv",
				Title:     "狂飙 S01E01 1080p",
			},
			want: "国产剧",
		},
		{
			name: "latin tv title without metadata falls back to western tv",
			input: mediaClassifyInput{
				MediaType: "tv",
				Title:     "The Last of Us S01E01 1080p",
			},
			want: "欧美剧",
		},
		{
			name: "latin tv keeps explicit western source category",
			input: mediaClassifyInput{
				MediaType: "tv",
				Title:     "Blades.of.the.Guardians.S02E01.1080p",
				Category:  "downloads 欧美剧 Blades.of.the.Guardians",
			},
			want: "欧美剧",
		},
		{
			name: "generic tv folder is not treated as chinese category",
			input: mediaClassifyInput{
				MediaType: "tv",
				Title:     "The Last of Us S01E01 1080p",
				Category:  "downloads 电视剧",
			},
			want: "欧美剧",
		},
		{
			name: "gala title overrides wrong western source category",
			input: mediaClassifyInput{
				MediaType: "tv",
				Title:     "HNTV Spring Festival Gala 2026 2160p WEB-DL",
				Category:  "欧美剧",
			},
			want: "综艺",
		},
		{
			name: "platform token alone does not classify romanized drama",
			input: mediaClassifyInput{
				MediaType: "tv",
				Title:     "Motherhood.of.Taihang.S01E01.2026.1080p.iQIYI.WEB-DL",
			},
			want: "欧美剧",
		},
		{
			name: "metadata classifies romanized chinese drama",
			input: mediaClassifyInput{
				MediaType: "tv",
				Title:     "Ashes.to.Crown.S01E15.2160p.YOUKU.WEB-DL",
				Countries: []string{"CN"},
			},
			want: "国产剧",
		},
		{
			name: "gala is variety",
			input: mediaClassifyInput{
				MediaType: "tv",
				Title:     "HNTV Spring Festival Gala 2026 2160p WEB-DL",
			},
			want: "综艺",
		},
		{
			name: "japanese anime localized chinese title without metadata falls back to other",
			input: mediaClassifyInput{
				MediaType: "anime",
				Title:     "葬送的芙莉莲",
			},
			want: "其他",
		},
		{
			name: "chinese anime explicit marker without metadata",
			input: mediaClassifyInput{
				MediaType: "anime",
				Title:     "斗破苍穹 国漫",
			},
			want: "国漫",
		},
		{
			name: "anime with JP country metadata is jp even from wrong source folder",
			input: mediaClassifyInput{
				MediaType: "anime",
				Title:     "间谍过家家 SPY×FAMILY",
				Countries: []string{"JP"},
				Genres:    []string{"16"},
				Category:  "国产剧",
			},
			want: "日番",
		},
		{
			name: "western anime metadata uses us anime category",
			input: mediaClassifyInput{
				MediaType: "anime",
				Title:     "Family Guy",
				Countries: []string{"US"},
				Genres:    []string{"16"},
				Category:  "日番",
			},
			want: "美漫",
		},
		{
			name: "tv animation with western metadata uses us anime category",
			input: mediaClassifyInput{
				MediaType: "tv",
				Title:     "The Simpsons",
				Countries: []string{"US"},
				Genres:    []string{"Animation"},
			},
			want: "美漫",
		},
		{
			name: "western anime legacy source category maps to us anime without metadata",
			input: mediaClassifyInput{
				MediaType: "anime",
				Title:     "The Simpsons S01E01 1080p",
				Category:  "downloads 欧美动漫",
			},
			want: "美漫",
		},
		{
			name: "anime with CN country metadata is cn",
			input: mediaClassifyInput{
				MediaType: "anime",
				Title:     "某番",
				Countries: []string{"CN"},
				Genres:    []string{"16"},
			},
			want: "国漫",
		},
		{
			name: "western movie source category remains western movie",
			input: mediaClassifyInput{
				MediaType: "movie",
				Title:     "Dune 2021 2160p",
				Category:  "downloads 欧美电影",
			},
			want: "欧美电影",
		},
		{
			name: "movie concert by music genre",
			input: mediaClassifyInput{
				MediaType: "movie",
				Title:     "Taylor Swift The Eras Tour",
				Genres:    []string{"10402"},
			},
			want: "演唱会",
		},
		{
			name: "movie documentary before region",
			input: mediaClassifyInput{
				MediaType: "movie",
				Title:     "Planet Earth",
				Languages: []string{"en"},
				Countries: []string{"GB"},
				Genres:    []string{"99"},
			},
			want: "纪录片",
		},
		{
			name: "movie korean language uses jk movie",
			input: mediaClassifyInput{
				MediaType: "movie",
				Title:     "Parasite",
				Languages: []string{"ko"},
				Countries: []string{"KR"},
				Genres:    []string{"18"},
			},
			want: "日韩电影",
		},
		{
			name: "anime korean metadata uses korean anime category",
			input: mediaClassifyInput{
				MediaType: "anime",
				Title:     "Korean Animation",
				Countries: []string{"KR"},
				Genres:    []string{"16"},
			},
			want: "韩漫",
		},
		{
			name: "jav code is adult",
			input: mediaClassifyInput{
				MediaType: "movie",
				Title:     "IPZZ-293-UC 1080p",
			},
			want: "成人",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := classifyMediaCategory(tt.input, nil); got != tt.want {
				t.Fatalf("category = %q, want %q", got, tt.want)
			}
		})
	}
}

func TestNormalizeMediaTypeAcceptsChineseLibraryTypes(t *testing.T) {
	tests := map[string]string{
		"华语电影": "movie",
		"欧美剧":  "tv",
		"国产剧":  "tv",
		"日漫":   "anime",
		"国漫":   "anime",
		"综艺":   "variety",
		"成人":   "adult",
	}
	for input, want := range tests {
		if got := normalizeMediaType(input, "测试标题", ""); got != want {
			t.Fatalf("normalizeMediaType(%q) = %q, want %q", input, got, want)
		}
	}
}

func TestNormalizeMediaTypeDoesNotTreatReleaseTokensAsTV(t *testing.T) {
	tests := []string{
		"They Will Kill You 2026 1080p HDTV x264",
		"Some Movie 2026 2160p AppleTV WEB-DL",
		"Some Movie 2026 2160p ATVP WEB-DL",
	}
	for _, input := range tests {
		t.Run(input, func(t *testing.T) {
			if got := normalizeMediaType("", input, ""); got != "movie" {
				t.Fatalf("normalizeMediaType(%q) = %q, want movie", input, got)
			}
		})
	}

	if got := normalizeMediaType("", "The Last of Us", `F:\media\tv\The Last of Us`); got != "tv" {
		t.Fatalf("standalone tv path token = %q, want tv", got)
	}
}

func TestSubscriptionResolveClassifiedSavePath(t *testing.T) {
	db := newServiceTestDB(t, &model.Setting{})
	repos := repository.New(db)
	if err := repos.Setting.Set(t.Context(), "organizer.smart_classify", "true"); err != nil {
		t.Fatal(err)
	}
	if err := repos.Setting.Set(t.Context(), "qbittorrent.savepath", `D:\Downloads`); err != nil {
		t.Fatal(err)
	}
	svc := NewSubscriptionService(&config.Config{}, zap.NewNop(), repos, nil, nil, nil)
	sub := &model.Subscription{Name: "声生不息 自动订阅", MediaType: "tv"}

	mediaType, category := svc.classifySubscriptionItem(t.Context(), sub, "声生不息 S01E01", "综艺")
	if mediaType != "tv" || category != "综艺" {
		t.Fatalf("classification = %q/%q, want tv/综艺", mediaType, category)
	}
	got := svc.resolveSubscriptionSavePath(t.Context(), sub, mediaType, category)
	want := `D:\Downloads\综艺`
	if got != want {
		t.Fatalf("save path = %q, want %q", got, want)
	}
}
