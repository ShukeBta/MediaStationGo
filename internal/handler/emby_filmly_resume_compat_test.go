package handler

import (
	"net/http/httptest"
	"reflect"
	"testing"

	"github.com/gin-gonic/gin"

	"github.com/ShukeBta/MediaStationGo/internal/service"
)

func TestEmbyApplyFilmlyResumeCompatibilityKeepsEpisodePresentation(t *testing.T) {
	gin.SetMode(gin.TestMode)
	w := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(w)
	c.Request = httptest.NewRequest("GET", "/emby/Users/user-1/Items/Resume", nil)
	c.Request.Header.Set("User-Agent", "Filmly/162 CFNetwork/3896.100.1.2.1 Darwin/27.0.0")

	episode := map[string]any{
		"Id":                      "episode-211",
		"Type":                    "Episode",
		"Name":                    "碾压宿敌，硬撼尊者",
		"ParentId":                "season-1",
		"ParentIndexNumber":       1,
		"SeriesName":              "吞噬星空",
		"SeasonName":              "第 1 季",
		"IndexNumber":             211,
		"SeriesId":                "series-1",
		"SeasonId":                "season-1",
		"ImageTags":               map[string]string{"Primary": "episode-primary", "Thumb": "episode-thumb"},
		"PrimaryImageItemId":      "episode-211",
		"PrimaryImageTag":         "episode-primary",
		"BackdropImageTags":       []string{"episode-backdrop"},
		"BackdropImageItemId":     "series-1",
		"ParentBackdropItemId":    "series-1",
		"ParentBackdropImageTags": []string{"series-backdrop"},
		"PrimaryImageAspectRatio": 16.0 / 9.0,
		"Etag":                    "episode-etag",
		"UserData": map[string]any{
			"PlaybackPositionTicks": int64(2_320_000_000),
		},
	}
	out := map[string]any{"Items": []map[string]any{episode}}

	if err := embyApplyFilmlyResumeCompatibility(c, out); err != nil {
		t.Fatalf("apply Filmly compatibility: %v", err)
	}

	if got := episode["Name"]; got != "吞噬星空 第211集 碾压宿敌，硬撼尊者" {
		t.Fatalf("Filmly title = %#v", got)
	}
	for _, key := range []string{
		"ParentId", "ParentIndexNumber", "SeasonId", "SeasonName", "SeriesId", "SeriesName",
		"BackdropImageItemId", "ParentBackdropItemId", "ParentBackdropImageTags",
	} {
		if _, exists := episode[key]; exists {
			t.Fatalf("Filmly Resume episode must omit %s: %#v", key, episode)
		}
	}
	if got := episode["Etag"]; got != "episode-etag"+embyFilmlyResumeETagSuffix {
		t.Fatalf("Filmly Etag = %#v", got)
	}
	if got := episode["PrimaryImageItemId"]; got != "episode-211" {
		t.Fatalf("Filmly primary owner = %#v", got)
	}
	if tags := episode["ImageTags"].(map[string]string); tags["Primary"] != "episode-primary" || tags["Thumb"] != "episode-thumb" {
		t.Fatalf("Filmly episode ImageTags = %#v", tags)
	}
	if tags := episode["BackdropImageTags"].([]string); len(tags) != 1 || tags[0] != "episode-backdrop" {
		t.Fatalf("Filmly episode BackdropImageTags = %#v", tags)
	}
	userData := episode["UserData"].(map[string]any)
	if userData["Key"] != "episode-211" || userData["ItemId"] != "episode-211" {
		t.Fatalf("Filmly episode UserData identity = %#v", userData)
	}
	if userData["PlaybackPositionTicks"] != int64(2_320_000_000) {
		t.Fatalf("Filmly episode playback position changed: %#v", userData)
	}
}

func TestEmbyApplyFilmlyResumeCompatibilityLeavesStandardClientUnchanged(t *testing.T) {
	gin.SetMode(gin.TestMode)
	w := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(w)
	c.Request = httptest.NewRequest("GET", "/emby/Users/user-1/Items/Resume", nil)
	c.Request.Header.Set("User-Agent", "Emby/4.8.0")

	episode := map[string]any{
		"Id":                      "episode-211",
		"Type":                    "Episode",
		"Name":                    "碾压宿敌，硬撼尊者",
		"SeriesName":              "吞噬星空",
		"SeasonName":              "第 1 季",
		"IndexNumber":             211,
		"SeriesId":                "series-1",
		"ParentBackdropItemId":    "series-1",
		"ParentBackdropImageTags": []string{"series-backdrop"},
		"Etag":                    "episode-etag",
		"UserData":                map[string]any{"PlaybackPositionTicks": int64(2_320_000_000)},
	}
	want := map[string]any{}
	for key, value := range episode {
		want[key] = value
	}
	want["UserData"] = map[string]any{"PlaybackPositionTicks": int64(2_320_000_000)}
	out := map[string]any{"Items": []map[string]any{episode}}

	if err := embyApplyFilmlyResumeCompatibility(c, out); err != nil {
		t.Fatalf("apply standard compatibility: %v", err)
	}

	if !reflect.DeepEqual(episode, want) {
		t.Fatalf("standard Episode changed: got %#v want %#v", episode, want)
	}
}

func TestEmbyApplyFilmlyResumeCompatibilityAddsIdentityWithoutChangingNonEpisodePresentation(t *testing.T) {
	gin.SetMode(gin.TestMode)
	w := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(w)
	c.Request = httptest.NewRequest("GET", "/emby/Users/user-1/Items/Resume", nil)
	c.Request.Header.Set("User-Agent", "Filmly/162")

	movie := map[string]any{
		"Id":       "movie-1",
		"Type":     "Movie",
		"Name":     "电影",
		"Etag":     "movie-etag",
		"UserData": map[string]any{"PlaybackPositionTicks": int64(1_200_000_000)},
	}
	out := map[string]any{"Items": []map[string]any{movie}}
	if err := embyApplyFilmlyResumeCompatibility(c, out); err != nil {
		t.Fatalf("apply Filmly movie compatibility: %v", err)
	}

	want := map[string]any{
		"Id":   "movie-1",
		"Type": "Movie",
		"Name": "电影",
		"Etag": "movie-etag",
		"UserData": map[string]any{
			"PlaybackPositionTicks": int64(1_200_000_000),
			"Key":                   "movie-1",
			"ItemId":                "movie-1",
		},
	}
	if !reflect.DeepEqual(movie, want) {
		t.Fatalf("Filmly movie changed: %#v", movie)
	}
}

func TestEmbyItemsParamsRequestResumeRows(t *testing.T) {
	if !embyItemsParamsRequestResumeRows(service.ItemsParams{Filters: []string{"IsFavorite", "isresumable"}}) {
		t.Fatal("IsResumable must select Filmly Resume compatibility")
	}
	if embyItemsParamsRequestResumeRows(service.ItemsParams{Filters: []string{"IsFavorite"}}) {
		t.Fatal("non-Resume filters must retain the standard Items payload")
	}
}

func TestEmbyFilmlyResumeEpisodeTitleAvoidsDuplicateEpisodeLabel(t *testing.T) {
	if got := embyFilmlyResumeEpisodeTitle("吞噬星空", 211, "第211集"); got != "吞噬星空 第211集" {
		t.Fatalf("title = %q", got)
	}
}
