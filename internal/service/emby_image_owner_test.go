package service

import (
	"testing"
	"time"

	"github.com/ShukeBta/MediaStationGo/internal/model"
)

func TestEmbyMoviePayloadIncludesImageOwnerFields(t *testing.T) {
	svc := newTestEmbyService(t)
	lib := model.Library{Name: "Movies", Path: `/media/movies`, Type: "movie", Enabled: true}
	if err := svc.repo.Library.Create(t.Context(), &lib); err != nil {
		t.Fatalf("create library: %v", err)
	}
	media := model.Media{
		Base:        model.Base{ID: "movie-1"},
		LibraryID:   lib.ID,
		Title:       "Movie",
		Path:        `/media/movies/movie.mkv`,
		PosterURL:   `https://img.example/poster.jpg`,
		BackdropURL: `https://img.example/backdrop.jpg`,
	}

	item := svc.itemPayload(t.Context(), &media, false, 0)

	wantPrimary := embyImageTag(media.ID, "primary", media.PosterURL, media.UpdatedAt)
	if item["PrimaryImageItemId"] != media.ID || item["PrimaryImageTag"] != wantPrimary {
		t.Fatalf("primary image owner fields missing: %#v", item)
	}
	wantBackdrop := embyImageTag(media.ID, "backdrop", media.BackdropURL, media.UpdatedAt)
	if tags, ok := item["BackdropImageTags"].([]string); !ok || len(tags) != 1 || tags[0] != wantBackdrop {
		t.Fatalf("BackdropImageTags = %#v", item["BackdropImageTags"])
	}
	for _, key := range []string{"BackdropImageItemId", "ParentBackdropItemId", "ParentBackdropImageTags"} {
		if _, ok := item[key]; ok {
			t.Fatalf("own backdrop must not populate %s: %#v", key, item)
		}
	}
}

func TestEmbyImageTagChangesWithArtworkURLAndUpdatedAt(t *testing.T) {
	svc := newTestEmbyService(t)
	updatedAt := time.Date(2026, 7, 11, 10, 0, 0, 0, time.UTC)
	media := model.Media{
		Base:      model.Base{ID: "movie-1", UpdatedAt: updatedAt},
		Title:     "Movie",
		Path:      `/media/movies/movie.mkv`,
		PosterURL: `https://img.example/poster-old.jpg`,
	}

	oldTag := svc.itemPayload(t.Context(), &media, false, 0)["PrimaryImageTag"]
	media.PosterURL = `https://img.example/poster-new.jpg`
	newURLTag := svc.itemPayload(t.Context(), &media, false, 0)["PrimaryImageTag"]
	if oldTag == newURLTag {
		t.Fatalf("primary image tag should change when poster url changes: old=%#v new=%#v", oldTag, newURLTag)
	}

	media.UpdatedAt = updatedAt.Add(time.Minute)
	newTimeTag := svc.itemPayload(t.Context(), &media, false, 0)["PrimaryImageTag"]
	if newURLTag == newTimeTag {
		t.Fatalf("primary image tag should change when updated_at changes: old=%#v new=%#v", newURLTag, newTimeTag)
	}
}

func TestEmbyItemPayloadIncludesCacheMetadata(t *testing.T) {
	svc := newTestEmbyService(t)
	createdAt := time.Date(2026, 7, 11, 9, 0, 0, 0, time.UTC)
	updatedAt := time.Date(2026, 7, 11, 10, 0, 0, 0, time.UTC)
	media := model.Media{
		Base:        model.Base{ID: "movie-1", CreatedAt: createdAt, UpdatedAt: updatedAt},
		Title:       "Movie",
		Path:        `/media/movies/movie.mkv`,
		PosterURL:   `https://img.example/poster.jpg`,
		BackdropURL: `https://img.example/backdrop.jpg`,
	}

	item := svc.itemPayload(t.Context(), &media, false, 0)

	if item["DateModified"] != updatedAt {
		t.Fatalf("DateModified = %#v, want %s", item["DateModified"], updatedAt)
	}
	if item["Etag"] == "" {
		t.Fatalf("Etag missing: %#v", item)
	}
	if item["PrimaryImageAspectRatio"] != 2.0/3.0 {
		t.Fatalf("PrimaryImageAspectRatio = %#v, want 2/3", item["PrimaryImageAspectRatio"])
	}
}

func TestEmbyEpisodeWithoutArtworkDoesNotAdvertiseUnresolvedParentBackdrop(t *testing.T) {
	svc := newTestEmbyService(t)
	lib := model.Library{Name: "TV", Path: `/media/tv`, Type: "tv", Enabled: true}
	if err := svc.repo.Library.Create(t.Context(), &lib); err != nil {
		t.Fatalf("create library: %v", err)
	}
	media := model.Media{
		Base:       model.Base{ID: "episode-1"},
		LibraryID:  lib.ID,
		Title:      "Show",
		Path:       `/media/tv/Show/Season 01/Show - S01E01.mkv`,
		SeasonNum:  1,
		EpisodeNum: 1,
	}

	item := svc.itemPayload(t.Context(), &media, false, 0)
	seriesID, _ := item["SeriesId"].(string)
	if seriesID == "" {
		t.Fatalf("episode payload missing SeriesId: %#v", item)
	}
	if item["PrimaryImageItemId"] != seriesID {
		t.Fatalf("PrimaryImageItemId = %#v, want series id %q", item["PrimaryImageItemId"], seriesID)
	}
	if _, ok := item["PrimaryImageTag"]; ok {
		t.Fatalf("episode without own image should not expose PrimaryImageTag: %#v", item)
	}
	for _, key := range []string{"BackdropImageItemId", "ParentBackdropItemId", "ParentBackdropImageTags"} {
		if _, ok := item[key]; ok {
			t.Fatalf("episode without resolved parent artwork must omit %s: %#v", key, item)
		}
	}
}

func TestEmbySeriesPayloadIncludesImageOwnerFields(t *testing.T) {
	svc := newTestEmbyService(t)
	lib := model.Library{Name: "TV", Path: `/media/tv`, Type: "tv", Enabled: true}
	if err := svc.repo.Library.Create(t.Context(), &lib); err != nil {
		t.Fatalf("create library: %v", err)
	}
	media := model.Media{
		Base:        model.Base{ID: "episode-1"},
		LibraryID:   lib.ID,
		Title:       "Show",
		Path:        `/media/tv/Show/Season 01/Show - S01E01.mkv`,
		PosterURL:   `https://img.example/poster.jpg`,
		BackdropURL: `https://img.example/backdrop.jpg`,
		SeasonNum:   1,
		EpisodeNum:  1,
	}
	if err := svc.repo.DB.Create(&media).Error; err != nil {
		t.Fatalf("create media: %v", err)
	}
	root, err := svc.Items(t.Context(), ItemsParams{ParentID: lib.ID, Limit: 50})
	if err != nil {
		t.Fatalf("items: %v", err)
	}
	items := root["Items"].([]map[string]any)
	if len(items) != 1 {
		t.Fatalf("items = %#v", items)
	}
	series := items[0]
	seriesID, _ := series["Id"].(string)
	primaryTag, _ := series["PrimaryImageTag"].(string)
	if series["PrimaryImageItemId"] != seriesID || primaryTag == "" || primaryTag == seriesID {
		t.Fatalf("series primary owner fields missing: %#v", series)
	}
	backdropTags, _ := series["BackdropImageTags"].([]string)
	if len(backdropTags) != 1 || backdropTags[0] == seriesID || backdropTags[0] == seriesID+"-bd" {
		t.Fatalf("series backdrop tags should be dynamic: %#v", series)
	}
	for _, key := range []string{"BackdropImageItemId", "ParentBackdropItemId", "ParentBackdropImageTags"} {
		if _, ok := series[key]; ok {
			t.Fatalf("series own backdrop must not populate %s: %#v", key, series)
		}
	}
}
