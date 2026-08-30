package service

import (
	"testing"
	"time"

	"github.com/ShukeBta/MediaStationGo/internal/config"
	"github.com/ShukeBta/MediaStationGo/internal/model"
	"github.com/ShukeBta/MediaStationGo/internal/repository"
	"go.uber.org/zap"
)

func TestEmbySystemInfoAdvertisesVersionedProtocolExtensions(t *testing.T) {
	db := newServiceTestDB(t, &model.Library{}, &model.Series{}, &model.Media{}, &model.Person{}, &model.Favorite{}, &model.PlaybackHistory{}, &model.User{}, &model.Setting{})
	repos := repository.New(db)
	svc := NewEmbyService(&config.Config{App: config.AppConfig{
		WindowsUpdateDownloadSources:     "https://one.example/,direct",
		WindowsUpdatePolicyMaxAgeSeconds: 3600,
	}}, zap.NewNop(), repos)
	for name, payload := range map[string]map[string]any{
		"authenticated": svc.SystemInfo(),
		"public":        svc.SystemInfoPublic(),
	} {
		t.Run(name, func(t *testing.T) {
			extensions, ok := payload["ProtocolExtensions"].([]map[string]any)
			if !ok || len(extensions) != 2 {
				t.Fatalf("ProtocolExtensions = %#v, want two extensions", payload["ProtocolExtensions"])
			}
			if extensions[0]["Id"] != "playback-preferences" || extensions[0]["Version"] != 1 {
				t.Fatalf("ProtocolExtensions[0] = %#v, want playback-preferences v1", extensions[0])
			}
			if extensions[1]["Id"] != "update-download-sources" || extensions[1]["Version"] != 1 || extensions[1]["MaxAgeSeconds"] != 3600 {
				t.Fatalf("ProtocolExtensions[1] = %#v, want update-download-sources v1", extensions[1])
			}
			sources, ok := extensions[1]["Sources"].([]string)
			if !ok || len(sources) != 2 || sources[0] != "https://one.example/" || sources[1] != "direct" {
				t.Fatalf("ProtocolExtensions[1].Sources = %#v", extensions[1]["Sources"])
			}
		})
	}
}

func TestEmbyStandardCatalogTypesGroupSeriesWithoutEpisodeLeak(t *testing.T) {
	svc := newTestEmbyService(t)
	lib := model.Library{
		Base:    model.Base{ID: "library-tv"},
		Name:    "剧集",
		Path:    "/media/tv",
		Type:    "tv",
		Enabled: true,
	}
	if err := svc.repo.Library.Create(t.Context(), &lib); err != nil {
		t.Fatalf("create library: %v", err)
	}
	for _, row := range []model.Media{
		{
			Base:       model.Base{ID: "episode-1"},
			LibraryID:  lib.ID,
			SeriesID:   "series-1",
			Title:      "示例剧",
			Path:       "/media/tv/示例剧/Season 01/示例剧.S01E01.mkv",
			SeasonNum:  1,
			EpisodeNum: 1,
		},
		{
			Base:       model.Base{ID: "episode-2"},
			LibraryID:  lib.ID,
			SeriesID:   "series-1",
			Title:      "示例剧",
			Path:       "/media/tv/示例剧/Season 01/示例剧.S01E02.mkv",
			SeasonNum:  1,
			EpisodeNum: 2,
		},
	} {
		if err := svc.repo.DB.Create(&row).Error; err != nil {
			t.Fatalf("create media: %v", err)
		}
	}

	out, err := svc.Items(t.Context(), ItemsParams{
		ParentID:         lib.ID,
		Recursive:        true,
		IncludeItemTypes: []string{"Movie", "Series", "Video", "MusicVideo", "BoxSet"},
		Limit:            50,
	})
	if err != nil {
		t.Fatalf("items: %v", err)
	}
	items := out["Items"].([]map[string]any)
	if len(items) != 1 || items[0]["Type"] != "Series" || items[0]["Id"] != "series-1" {
		t.Fatalf("standard catalog query should return one Series without Episode leakage: %#v", out)
	}
}

func TestEmbyEpisodeExposesScrapedTitleAndOwnPrimaryStill(t *testing.T) {
	svc := newTestEmbyService(t)
	lib := model.Library{
		Base:    model.Base{ID: "library-anime"},
		Name:    "番剧",
		Path:    "/media/anime",
		Type:    "anime",
		Enabled: true,
	}
	if err := svc.repo.Library.Create(t.Context(), &lib); err != nil {
		t.Fatalf("create library: %v", err)
	}
	media := model.Media{
		Base:         model.Base{ID: "episode-scraped"},
		LibraryID:    lib.ID,
		Title:        "间谍过家家",
		EpisodeTitle: "任务代号：猫",
		Path:         "/media/anime/间谍过家家/Season 02/间谍过家家.S02E01.mkv",
		PosterURL:    "https://image.example/series-poster.jpg",
		BackdropURL:  "https://image.example/episode-still.jpg",
		SeasonNum:    2,
		EpisodeNum:   1,
		Width:        3840,
		Height:       1600,
	}
	if err := svc.repo.DB.Create(&media).Error; err != nil {
		t.Fatalf("create media: %v", err)
	}

	item, err := svc.Item(t.Context(), media.ID, "")
	if err != nil {
		t.Fatalf("item: %v", err)
	}
	if item["Type"] != "Episode" || item["Name"] != media.EpisodeTitle {
		t.Fatalf("episode identity = type %#v name %#v, want scraped title %q", item["Type"], item["Name"], media.EpisodeTitle)
	}
	imageTags, ok := item["ImageTags"].(map[string]string)
	if !ok || imageTags["Primary"] == "" {
		t.Fatalf("episode ImageTags = %#v, want own Primary image", item["ImageTags"])
	}
	if item["PrimaryImageItemId"] != media.ID || item["PrimaryImageTag"] != imageTags["Primary"] {
		t.Fatalf("episode primary image ownership = %#v", item)
	}
	wantBackdrop := embyImageTag(media.ID, "backdrop", media.BackdropURL, media.UpdatedAt)
	if backdropTags, ok := item["BackdropImageTags"].([]string); !ok || len(backdropTags) != 1 || backdropTags[0] != wantBackdrop {
		t.Fatalf("episode Backdrop tags = %#v, want own still %q", item["BackdropImageTags"], wantBackdrop)
	}
	if item["PrimaryImageAspectRatio"] != 16.0/9.0 {
		t.Fatalf("PrimaryImageAspectRatio = %#v, want 16/9 episode still", item["PrimaryImageAspectRatio"])
	}
	imageURL, err := svc.ImageURL(t.Context(), media.ID, "Primary")
	if err != nil {
		t.Fatalf("primary image url: %v", err)
	}
	if imageURL != media.BackdropURL {
		t.Fatalf("episode Primary image = %q, want scraped still %q", imageURL, media.BackdropURL)
	}
}

func TestEmbyResumeItemsPageHonorsPaginationAndTotal(t *testing.T) {
	svc := newTestEmbyService(t)
	lib := model.Library{
		Base:    model.Base{ID: "library-movies"},
		Name:    "电影",
		Path:    "/media/movies",
		Type:    "movie",
		Enabled: true,
	}
	if err := svc.repo.Library.Create(t.Context(), &lib); err != nil {
		t.Fatalf("create library: %v", err)
	}
	if err := svc.repo.User.Create(t.Context(), &model.User{
		Base:         model.Base{ID: "user-1"},
		Username:     "viewer",
		PasswordHash: "hash",
		Role:         "admin",
		Tier:         "plus",
		IsActive:     true,
	}); err != nil {
		t.Fatalf("create user: %v", err)
	}
	now := time.Now()
	for i, id := range []string{"movie-1", "movie-2", "movie-3"} {
		if err := svc.repo.DB.Create(&model.Media{
			Base:      model.Base{ID: id},
			LibraryID: lib.ID,
			Title:     id,
			Path:      "/media/movies/" + id + ".mkv",
		}).Error; err != nil {
			t.Fatalf("create media: %v", err)
		}
		if err := svc.repo.DB.Create(&model.PlaybackHistory{
			Base:       model.Base{ID: "history-" + id},
			UserID:     "user-1",
			MediaID:    id,
			PositionMs: 60_000,
			DurationMs: 600_000,
			WatchedAt:  now.Add(-time.Duration(i) * time.Minute),
			Completed:  false,
		}).Error; err != nil {
			t.Fatalf("create playback history: %v", err)
		}
	}

	out, err := svc.ResumeItemsPage(t.Context(), "user-1", 1, 1)
	if err != nil {
		t.Fatalf("resume items page: %v", err)
	}
	items := out["Items"].([]map[string]any)
	if len(items) != 1 || items[0]["Id"] != "movie-2" {
		t.Fatalf("resume page = %#v, want movie-2", out)
	}
	if out["TotalRecordCount"] != 3 || out["StartIndex"] != 1 {
		t.Fatalf("resume envelope = %#v, want total=3 start=1", out)
	}
}
