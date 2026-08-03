package service

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"go.uber.org/zap"

	"github.com/ShukeBta/MediaStationGo/internal/config"
)

func TestApplyTMDbChineseTitlePrefersMainlandAlternative(t *testing.T) {
	match := &Match{Title: "The Rookie", OriginalName: "The Rookie"}
	applyTMDbChineseTitle(match, []tmdbAlternativeTitle{
		{Country: "TW", Title: "菜鳥新移民"},
		{Country: "CN", Title: "菜鸟老警"},
	}, nil)

	if match.Title != "菜鸟老警" || match.OriginalName != "The Rookie" {
		t.Fatalf("title=%q original=%q, want mainland Chinese title with original preserved", match.Title, match.OriginalName)
	}
}

func TestApplyTMDbChineseTitleFallsBackToSingaporeTranslation(t *testing.T) {
	match := &Match{Title: "English Title", OriginalName: "English Title"}
	translation := tmdbTranslation{Country: "SG", Language: "zh"}
	translation.Data.Name = "新加坡中文名"
	applyTMDbChineseTitle(match, nil, []tmdbTranslation{translation})

	if match.Title != "新加坡中文名" {
		t.Fatalf("title=%q, want Singapore Chinese translation", match.Title)
	}
}

func TestGetMovieMatchUsesChineseAlternativeTitle(t *testing.T) {
	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/movie/1292695" {
			http.NotFound(w, r)
			return
		}
		if got := r.URL.Query().Get("append_to_response"); !strings.Contains(got, "alternative_titles") {
			t.Fatalf("append_to_response=%q, want alternative_titles", got)
		}
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(map[string]any{
			"id":                1292695,
			"title":             "They Will Kill You",
			"original_title":    "They Will Kill You",
			"original_language": "en",
			"release_date":      "2026-03-27",
			"alternative_titles": map[string]any{
				"titles": []map[string]any{{
					"iso_3166_1": "CN",
					"title":      "杀的就是你",
				}},
			},
		})
	}))
	defer upstream.Close()

	cfg := &config.Config{}
	cfg.Secrets.TMDbAPIKey = "test-key"
	cfg.Secrets.TMDbAPIProxy = upstream.URL
	provider := NewTMDbProvider(cfg, zap.NewNop(), nil)
	match, err := provider.GetMovieMatch(t.Context(), 1292695)
	if err != nil {
		t.Fatal(err)
	}
	if match == nil || match.Title != "杀的就是你" || match.OriginalName != "They Will Kill You" {
		t.Fatalf("match=%#v, want Chinese movie title with English original", match)
	}
}
