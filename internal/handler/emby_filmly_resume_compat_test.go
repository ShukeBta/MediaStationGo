package handler

import (
	"net/http/httptest"
	"testing"

	"github.com/gin-gonic/gin"
)

func TestEmbyApplyFilmlyResumeCompatibilityUsesEpisodeCard(t *testing.T) {
	gin.SetMode(gin.TestMode)
	w := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(w)
	c.Request = httptest.NewRequest("GET", "/emby/Users/user-1/Items/Resume", nil)
	c.Request.Header.Set("User-Agent", "Filmly/162 CFNetwork/3896.100.1.2.1 Darwin/27.0.0")

	episode := map[string]any{
		"Id":                      "episode-209",
		"Type":                    "Episode",
		"Name":                    "幽海分身",
		"SeriesName":              "吞噬星空",
		"SeasonName":              "第 1 季",
		"IndexNumber":             209,
		"SeriesId":                "series-1",
		"SeasonId":                "season-1",
		"ImageTags":               map[string]string{"Primary": "episode-primary", "Thumb": "episode-thumb"},
		"PrimaryImageItemId":      "episode-209",
		"ParentBackdropItemId":    "series-1",
		"BackdropImageItemId":     "series-1",
		"ParentBackdropImageTags": []string{"series-backdrop"},
		"PrimaryImageAspectRatio": 16.0 / 9.0,
	}
	out := map[string]any{"Items": []map[string]any{episode}}

	embyApplyFilmlyResumeCompatibility(c, out)

	if got := episode["Name"]; got != "吞噬星空 第209集 幽海分身" {
		t.Fatalf("Filmly title = %#v", got)
	}
	for _, key := range []string{"SeriesName", "SeasonName", "BackdropImageItemId", "ParentBackdropItemId", "ParentBackdropImageTags"} {
		if _, exists := episode[key]; exists {
			t.Fatalf("Filmly episode must omit %s: %#v", key, episode)
		}
	}
	if got := episode["PrimaryImageItemId"]; got != "episode-209" {
		t.Fatalf("Filmly primary owner = %#v", got)
	}
	if tags := episode["ImageTags"].(map[string]string); tags["Thumb"] != "episode-thumb" {
		t.Fatalf("Filmly episode Thumb = %#v", tags)
	}
}

func TestEmbyApplyFilmlyResumeCompatibilityLeavesEmbyClientStandard(t *testing.T) {
	gin.SetMode(gin.TestMode)
	w := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(w)
	c.Request = httptest.NewRequest("GET", "/emby/Users/user-1/Items/Resume", nil)
	c.Request.Header.Set("User-Agent", "Emby/4.8.0")

	episode := map[string]any{
		"Type":                    "Episode",
		"Name":                    "幽海分身",
		"SeriesName":              "吞噬星空",
		"SeasonName":              "第 1 季",
		"IndexNumber":             209,
		"ParentBackdropItemId":    "series-1",
		"ParentBackdropImageTags": []string{"series-backdrop"},
	}
	out := map[string]any{"Items": []map[string]any{episode}}

	embyApplyFilmlyResumeCompatibility(c, out)

	if got := episode["Name"]; got != "幽海分身" {
		t.Fatalf("standard client title changed: %#v", got)
	}
	if got := episode["SeasonName"]; got != "第 1 季" {
		t.Fatalf("standard client season name changed: %#v", got)
	}
	if got := episode["ParentBackdropItemId"]; got != "series-1" {
		t.Fatalf("standard client backdrop owner changed: %#v", got)
	}
}
