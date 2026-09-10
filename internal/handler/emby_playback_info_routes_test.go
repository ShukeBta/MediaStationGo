package handler

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/gin-gonic/gin"
	"github.com/glebarez/sqlite"
	"go.uber.org/zap"
	"gorm.io/gorm"

	"github.com/ShukeBta/MediaStationGo/internal/config"
	"github.com/ShukeBta/MediaStationGo/internal/model"
	"github.com/ShukeBta/MediaStationGo/internal/repository"
	"github.com/ShukeBta/MediaStationGo/internal/service"
)

func TestEmbyLowercasePlaybackInfoRouteReturnsJSON(t *testing.T) {
	gin.SetMode(gin.TestMode)
	db, err := gorm.Open(sqlite.Open(":memory:"), &gorm.Config{})
	if err != nil {
		t.Fatalf("open db: %v", err)
	}
	if err := db.AutoMigrate(model.AllModels()...); err != nil {
		t.Fatalf("migrate: %v", err)
	}
	repos := repository.New(db)
	if err := repos.User.Create(t.Context(), &model.User{
		Base:         model.Base{ID: "user-1"},
		Username:     "tester",
		PasswordHash: "x",
		Role:         "admin",
		Tier:         "plus",
		IsActive:     true,
	}); err != nil {
		t.Fatalf("create user: %v", err)
	}
	lib := model.Library{Name: "电影", Path: t.TempDir(), Type: "movie", Enabled: true}
	if err := repos.Library.Create(t.Context(), &lib); err != nil {
		t.Fatalf("create library: %v", err)
	}
	if err := db.Create(&model.Media{
		Base:      model.Base{ID: "media-1"},
		LibraryID: lib.ID,
		Title:     "Lowercase Playback",
		Path:      filepath.Join(lib.Path, "lowercase-playback.mp4"),
		Container: "mp4",
	}).Error; err != nil {
		t.Fatalf("create media: %v", err)
	}

	const secret = "test-secret"
	router := gin.New()
	registerEmbyRoutes(router, secret, &service.Container{
		Repo: repos,
		Emby: service.NewEmbyService(&config.Config{}, zap.NewNop(), repos),
	})

	req := httptest.NewRequest(http.MethodGet, "/users/user-1/items/media-1/playbackinfo", nil)
	req.Header.Set("X-Emby-Token", signedTestToken(t, secret))
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("unexpected status: %d body=%s", w.Code, w.Body.String())
	}
	var body map[string]any
	if err := json.Unmarshal(w.Body.Bytes(), &body); err != nil {
		t.Fatalf("decode playback info: %v", err)
	}
	if _, ok := body["MediaSources"]; !ok {
		t.Fatalf("missing MediaSources: %#v", body)
	}
	sources, ok := body["MediaSources"].([]any)
	if !ok || len(sources) == 0 {
		t.Fatalf("unexpected MediaSources: %#v", body["MediaSources"])
	}
	source, ok := sources[0].(map[string]any)
	if !ok {
		t.Fatalf("unexpected MediaSource: %#v", sources[0])
	}
	directURL, _ := source["DirectStreamUrl"].(string)
	if !strings.Contains(directURL, "api_key=") {
		t.Fatalf("DirectStreamUrl should carry api_key for clients that do not repeat auth headers: %#v", source)
	}
	transcodeURL, _ := source["TranscodingUrl"].(string)
	if transcodeURL != "" && !strings.Contains(transcodeURL, "api_key=") {
		t.Fatalf("TranscodingUrl should carry api_key: %#v", source)
	}
}

func TestEmbyPlaybackInfoUsesAndRemembersSelectedMediaSource(t *testing.T) {
	gin.SetMode(gin.TestMode)
	db, err := gorm.Open(sqlite.Open(":memory:"), &gorm.Config{})
	if err != nil {
		t.Fatalf("open db: %v", err)
	}
	if err := db.AutoMigrate(model.AllModels()...); err != nil {
		t.Fatalf("migrate: %v", err)
	}
	repos := repository.New(db)
	for _, user := range []model.User{
		{Base: model.Base{ID: "user-1"}, Username: "viewer-1", PasswordHash: "x", Role: "admin", Tier: "plus", IsActive: true},
		{Base: model.Base{ID: "user-2"}, Username: "viewer-2", PasswordHash: "x", Role: "admin", Tier: "plus", IsActive: true},
	} {
		if err := repos.User.Create(t.Context(), &user); err != nil {
			t.Fatalf("create user: %v", err)
		}
	}
	lib := model.Library{Name: "电影", Path: t.TempDir(), Type: "movie", Enabled: true}
	if err := repos.Library.Create(t.Context(), &lib); err != nil {
		t.Fatalf("create library: %v", err)
	}
	media := []model.Media{
		{
			Base: model.Base{ID: "version-primary"}, LibraryID: lib.ID, Title: "特立独行",
			Path: filepath.Join(lib.Path, "primary.mkv"), Container: "mkv", VersionGroupKey: "independent-versions", Width: 3840,
		},
		{
			Base: model.Base{ID: "version-selected"}, LibraryID: lib.ID, Title: "特立独行",
			Path: filepath.Join(lib.Path, "selected.mkv"), Container: "mkv", VersionGroupKey: "independent-versions", Width: 1920,
		},
	}
	if err := db.Create(&media).Error; err != nil {
		t.Fatalf("create media: %v", err)
	}

	const secret = "test-secret"
	emby := service.NewEmbyService(&config.Config{}, zap.NewNop(), repos)
	router := gin.New()
	registerEmbyRoutes(router, secret, &service.Container{Repo: repos, Emby: emby})
	token := signedTestToken(t, secret)

	req := httptest.NewRequest(
		http.MethodPost,
		"/emby/Items/version-primary/PlaybackInfo?MediaSourceId=version-selected&IsPlayback=true",
		nil,
	)
	req.Header.Set("X-Emby-Token", token)
	req.Header.Set("User-Agent", "Filmly/2.6.18-382")
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)
	if w.Code != http.StatusOK {
		t.Fatalf("selected playback status = %d body=%s", w.Code, w.Body.String())
	}
	assertFirstPlaybackSource(t, w.Body.Bytes(), "version-selected", "/Videos/version-selected/")

	req = httptest.NewRequest(http.MethodGet, "/emby/Users/user-1/Items/version-primary?Fields=MediaSources", nil)
	req.Header.Set("X-Emby-Token", token)
	w = httptest.NewRecorder()
	router.ServeHTTP(w, req)
	if w.Code != http.StatusOK {
		t.Fatalf("detail status = %d body=%s", w.Code, w.Body.String())
	}
	assertFirstPlaybackSource(t, w.Body.Bytes(), "version-selected", "")

	playback, err := emby.PlaybackInfo(t.Context(), "version-primary", "user-1")
	if err != nil {
		t.Fatalf("saved playback preference: %v", err)
	}
	if sources := playback["MediaSources"].([]map[string]any); sources[0]["Id"] != "version-selected" {
		t.Fatalf("saved playback first source = %#v", sources)
	}
	otherDetail, err := emby.Item(t.Context(), "version-primary", "user-2")
	if err != nil {
		t.Fatalf("other user detail: %v", err)
	}
	if sources := otherDetail["MediaSources"].([]map[string]any); sources[0]["Id"] != "version-primary" {
		t.Fatalf("other user should retain default source order: %#v", sources)
	}

	req = httptest.NewRequest(
		http.MethodPost,
		"/emby/Items/version-primary/PlaybackInfo?MediaSourceId=not-a-sibling&IsPlayback=true",
		nil,
	)
	req.Header.Set("X-Emby-Token", token)
	w = httptest.NewRecorder()
	router.ServeHTTP(w, req)
	if w.Code != http.StatusBadRequest {
		t.Fatalf("invalid source status = %d body=%s", w.Code, w.Body.String())
	}
}

func assertFirstPlaybackSource(t *testing.T, raw []byte, wantID, wantURLFragment string) {
	t.Helper()
	var body map[string]any
	if err := json.Unmarshal(raw, &body); err != nil {
		t.Fatalf("decode response: %v", err)
	}
	sources, ok := body["MediaSources"].([]any)
	if !ok || len(sources) == 0 {
		t.Fatalf("MediaSources = %#v", body["MediaSources"])
	}
	first, ok := sources[0].(map[string]any)
	if !ok || first["Id"] != wantID {
		t.Fatalf("first source = %#v, want %q", sources[0], wantID)
	}
	if wantURLFragment != "" {
		directURL, _ := first["DirectStreamUrl"].(string)
		if !strings.Contains(directURL, wantURLFragment) {
			t.Fatalf("DirectStreamUrl = %q, want fragment %q", directURL, wantURLFragment)
		}
	}
}

func TestEmbyPlaybackInfoSubtitleDeliveryURLServesNativeSRT(t *testing.T) {
	gin.SetMode(gin.TestMode)
	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, "Movie.zh.srt"), []byte("1\n00:00:01,000 --> 00:00:02,000\n你好\n"), 0o644); err != nil {
		t.Fatalf("write subtitle: %v", err)
	}
	db, err := gorm.Open(sqlite.Open(":memory:"), &gorm.Config{})
	if err != nil {
		t.Fatalf("open db: %v", err)
	}
	if err := db.AutoMigrate(model.AllModels()...); err != nil {
		t.Fatalf("migrate: %v", err)
	}
	repos := repository.New(db)
	if err := repos.User.Create(t.Context(), &model.User{
		Base:         model.Base{ID: "user-1"},
		Username:     "tester",
		PasswordHash: "x",
		Role:         "admin",
		Tier:         "plus",
		IsActive:     true,
	}); err != nil {
		t.Fatalf("create user: %v", err)
	}
	lib := model.Library{Name: "电影", Path: dir, Type: "movie", Enabled: true}
	if err := repos.Library.Create(t.Context(), &lib); err != nil {
		t.Fatalf("create library: %v", err)
	}
	if err := db.Create(&model.Media{
		Base:       model.Base{ID: "media-sub"},
		LibraryID:  lib.ID,
		Title:      "Movie",
		Path:       filepath.Join(dir, "Movie.mkv"),
		Container:  "mkv",
		VideoCodec: "h264",
		AudioCodec: "aac",
	}).Error; err != nil {
		t.Fatalf("create media: %v", err)
	}

	const secret = "test-secret"
	emby := service.NewEmbyService(&config.Config{}, zap.NewNop(), repos)
	subtitle := service.NewSubtitleService(zap.NewNop(), repos)
	emby.SetSubtitleService(subtitle)
	router := gin.New()
	registerEmbyRoutes(router, secret, &service.Container{
		Repo:     repos,
		Emby:     emby,
		Subtitle: subtitle,
	})

	token := signedTestToken(t, secret)
	detailReq := httptest.NewRequest(http.MethodGet, "/emby/Users/user-1/Items/media-sub?Fields=MediaSources", nil)
	detailReq.Header.Set("X-Emby-Token", token)
	detailRecorder := httptest.NewRecorder()
	router.ServeHTTP(detailRecorder, detailReq)
	if detailRecorder.Code != http.StatusOK {
		t.Fatalf("unexpected item detail status: %d body=%s", detailRecorder.Code, detailRecorder.Body.String())
	}
	var detail map[string]any
	if err := json.Unmarshal(detailRecorder.Body.Bytes(), &detail); err != nil {
		t.Fatalf("decode item detail: %v", err)
	}
	detailSource := detail["MediaSources"].([]any)[0].(map[string]any)
	detailStreams := detailSource["MediaStreams"].([]any)
	var detailDeliveryURL string
	for _, raw := range detailStreams {
		stream := raw.(map[string]any)
		if stream["Type"] == "Subtitle" {
			detailDeliveryURL, _ = stream["DeliveryUrl"].(string)
			break
		}
	}
	if detailDeliveryURL == "" || !strings.Contains(detailDeliveryURL, "api_key=") {
		t.Fatalf("item detail should expose a tokenized external subtitle: %#v", detailStreams)
	}

	req := httptest.NewRequest(http.MethodGet, "/emby/Items/media-sub/PlaybackInfo", nil)
	req.Header.Set("X-Emby-Token", token)
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)
	if w.Code != http.StatusOK {
		t.Fatalf("unexpected playback status: %d body=%s", w.Code, w.Body.String())
	}
	var body map[string]any
	if err := json.Unmarshal(w.Body.Bytes(), &body); err != nil {
		t.Fatalf("decode playback info: %v", err)
	}
	source := body["MediaSources"].([]any)[0].(map[string]any)
	streams := source["MediaStreams"].([]any)
	var deliveryURL string
	for _, raw := range streams {
		stream := raw.(map[string]any)
		if stream["Type"] == "Subtitle" {
			deliveryURL, _ = stream["DeliveryUrl"].(string)
			break
		}
	}
	if deliveryURL == "" || !strings.Contains(deliveryURL, "api_key=") {
		t.Fatalf("subtitle DeliveryUrl should carry api_key: %#v", streams)
	}
	if strings.Contains(deliveryURL, "mp_track=") {
		t.Fatalf("subtitle DeliveryUrl should use the standard stream index route: %q", deliveryURL)
	}

	req = httptest.NewRequest(http.MethodGet, deliveryURL, nil)
	w = httptest.NewRecorder()
	router.ServeHTTP(w, req)
	if w.Code != http.StatusOK {
		t.Fatalf("unexpected subtitle status: %d body=%s", w.Code, w.Body.String())
	}
	if contentType := w.Header().Get("Content-Type"); !strings.Contains(contentType, "application/x-subrip") {
		t.Fatalf("subtitle content type = %q", contentType)
	}
	if body := w.Body.String(); strings.Contains(body, "WEBVTT") || !strings.Contains(body, "00:00:01,000 --> 00:00:02,000") {
		t.Fatalf("unexpected subtitle body: %q", body)
	}
}

func TestEmbyStandardVideoSubtitleRoutesServeVTT(t *testing.T) {
	gin.SetMode(gin.TestMode)
	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, "Movie.zh.srt"), []byte("1\n00:00:01,000 --> 00:00:02,000\nhello\n"), 0o644); err != nil {
		t.Fatalf("write subtitle: %v", err)
	}
	db, err := gorm.Open(sqlite.Open(":memory:"), &gorm.Config{})
	if err != nil {
		t.Fatalf("open db: %v", err)
	}
	if err := db.AutoMigrate(model.AllModels()...); err != nil {
		t.Fatalf("migrate: %v", err)
	}
	repos := repository.New(db)
	if err := repos.User.Create(t.Context(), &model.User{
		Base:         model.Base{ID: "user-1"},
		Username:     "tester",
		PasswordHash: "x",
		Role:         "admin",
		Tier:         "plus",
		IsActive:     true,
	}); err != nil {
		t.Fatalf("create user: %v", err)
	}
	lib := model.Library{Name: "Movies", Path: dir, Type: "movie", Enabled: true}
	if err := repos.Library.Create(t.Context(), &lib); err != nil {
		t.Fatalf("create library: %v", err)
	}
	if err := db.Create(&model.Media{
		Base:       model.Base{ID: "media-sub"},
		LibraryID:  lib.ID,
		Title:      "Movie",
		Path:       filepath.Join(dir, "Movie.mkv"),
		Container:  "mkv",
		VideoCodec: "h264",
	}).Error; err != nil {
		t.Fatalf("create media: %v", err)
	}

	const secret = "test-secret"
	emby := service.NewEmbyService(&config.Config{}, zap.NewNop(), repos)
	subtitle := service.NewSubtitleService(zap.NewNop(), repos)
	emby.SetSubtitleService(subtitle)
	router := gin.New()
	registerEmbyRoutes(router, secret, &service.Container{
		Repo:     repos,
		Emby:     emby,
		Subtitle: subtitle,
	})

	token := signedTestToken(t, secret)
	for _, path := range []string{
		"/emby/Videos/media-sub/media-sub/Subtitles/1/Stream.vtt",
		"/emby/Videos/media-sub/media-sub/Subtitles/1/0/Stream.vtt",
	} {
		req := httptest.NewRequest(http.MethodGet, path, nil)
		req.Header.Set("X-Emby-Token", token)
		w := httptest.NewRecorder()
		router.ServeHTTP(w, req)

		if w.Code != http.StatusOK {
			t.Fatalf("unexpected standard subtitle status for %s: %d body=%s", path, w.Code, w.Body.String())
		}
		if contentType := w.Header().Get("Content-Type"); !strings.Contains(contentType, "text/vtt") {
			t.Fatalf("standard subtitle content type for %s = %q", path, contentType)
		}
		if body := w.Body.String(); !strings.Contains(body, "WEBVTT") || !strings.Contains(body, "hello") {
			t.Fatalf("unexpected standard subtitle body for %s: %q", path, body)
		}
	}
}

func TestEmbyExternalSubtitleTrackIndexUsesRequestedMediaSource(t *testing.T) {
	item := map[string]any{
		"MediaSources": []map[string]any{
			{
				"Id": "source-a",
				"MediaStreams": []map[string]any{
					{"Index": 0, "Type": "Video"},
					{"Index": 1, "Type": "Subtitle", "IsExternal": true},
				},
			},
			{
				"Id": "source-b",
				"MediaStreams": []map[string]any{
					{"Index": 0, "Type": "Video"},
					{"Index": 1, "Type": "Audio"},
					{"Index": 2, "Type": "Subtitle", "IsExternal": true},
					{"Index": 3, "Type": "Subtitle", "IsExternal": true},
				},
			},
		},
	}
	trackIndex, ok := embyExternalSubtitleTrackIndex(item, "source-b", 3)
	if !ok || trackIndex != 1 {
		t.Fatalf("unexpected source-specific subtitle mapping: index=%d ok=%v", trackIndex, ok)
	}
	if _, ok := embyExternalSubtitleTrackIndex(item, "source-a", 3); ok {
		t.Fatal("stream index from another media source must not be accepted")
	}
}

func TestEmbyPlaybackInfoDoesNotExposeTokenInCloudPath(t *testing.T) {
	gin.SetMode(gin.TestMode)
	db, err := gorm.Open(sqlite.Open(":memory:"), &gorm.Config{})
	if err != nil {
		t.Fatalf("open db: %v", err)
	}
	if err := db.AutoMigrate(model.AllModels()...); err != nil {
		t.Fatalf("migrate: %v", err)
	}
	repos := repository.New(db)
	if err := repos.Setting.Set(t.Context(), service.CloudPlaybackModeSettingKey, service.CloudPlaybackModeSTRM); err != nil {
		t.Fatalf("set cloud playback mode: %v", err)
	}
	if err := repos.User.Create(t.Context(), &model.User{
		Base:         model.Base{ID: "user-1"},
		Username:     "tester",
		PasswordHash: "x",
		Role:         "admin",
		Tier:         "plus",
		IsActive:     true,
	}); err != nil {
		t.Fatalf("create user: %v", err)
	}
	lib := model.Library{Name: "OpenList", Path: "cloud://openlist/Movies", Type: "movie", Enabled: true}
	if err := repos.Library.Create(t.Context(), &lib); err != nil {
		t.Fatalf("create library: %v", err)
	}
	if err := db.Create(&model.Media{
		Base:      model.Base{ID: "cloud-1"},
		LibraryID: lib.ID,
		Title:     "Cloud Movie",
		Path:      "cloud://openlist/Movies/Movie.mkv",
		STRMURL:   "/api/cloud/play/openlist?ref=%2FMovies%2FMovie.mkv",
		Container: "mkv",
	}).Error; err != nil {
		t.Fatalf("create media: %v", err)
	}

	const secret = "test-secret"
	router := gin.New()
	registerEmbyRoutes(router, secret, &service.Container{
		Repo: repos,
		Emby: service.NewEmbyService(&config.Config{}, zap.NewNop(), repos),
	})

	req := httptest.NewRequest(http.MethodGet, "/users/user-1/items/cloud-1/playbackinfo", nil)
	req.Header.Set("X-Emby-Token", signedTestToken(t, secret))
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("unexpected status: %d body=%s", w.Code, w.Body.String())
	}
	var body map[string]any
	if err := json.Unmarshal(w.Body.Bytes(), &body); err != nil {
		t.Fatalf("decode playback info: %v", err)
	}
	source := body["MediaSources"].([]any)[0].(map[string]any)
	pathURL, _ := source["Path"].(string)
	if pathURL != "/Videos/cloud-1/stream.mkv" {
		t.Fatalf("cloud Path should stay as the non-tokenized standard stream URL, got %#v", source)
	}
	if strings.Contains(pathURL, "api_key=") || strings.Contains(pathURL, "token=") {
		t.Fatalf("cloud Path must not expose auth key/token: %#v", source)
	}
	if strings.Contains(pathURL, "/api/cloud/play/") {
		t.Fatalf("cloud Path should not expose naked cloud play URL: %#v", source)
	}
	directURL, _ := source["DirectStreamUrl"].(string)
	if !strings.HasPrefix(directURL, "/Videos/cloud-1/stream.mkv") || !strings.Contains(directURL, "api_key=") {
		t.Fatalf("standard DirectStreamUrl should stay tokenized: %#v", source)
	}
	if source["SupportsDirectPlay"] != true {
		t.Fatalf("cloud media should advertise DirectPlay when tokenized Path is playable: %#v", source)
	}
	if source["SupportsTranscoding"] != false {
		t.Fatalf("cloud media should not advertise host transcoding: %#v", source)
	}
}

func TestEmbyItemsDoNotExposeTokenInEmbeddedCloudPath(t *testing.T) {
	gin.SetMode(gin.TestMode)
	db, err := gorm.Open(sqlite.Open(":memory:"), &gorm.Config{})
	if err != nil {
		t.Fatalf("open db: %v", err)
	}
	if err := db.AutoMigrate(model.AllModels()...); err != nil {
		t.Fatalf("migrate: %v", err)
	}
	repos := repository.New(db)
	if err := repos.Setting.Set(t.Context(), service.CloudPlaybackModeSettingKey, service.CloudPlaybackModeSTRM); err != nil {
		t.Fatalf("set cloud playback mode: %v", err)
	}
	if err := repos.User.Create(t.Context(), &model.User{
		Base:         model.Base{ID: "user-1"},
		Username:     "tester",
		PasswordHash: "x",
		Role:         "admin",
		Tier:         "plus",
		IsActive:     true,
	}); err != nil {
		t.Fatalf("create user: %v", err)
	}
	lib := model.Library{Name: "OpenList", Path: "cloud://openlist/Movies", Type: "movie", Enabled: true}
	if err := repos.Library.Create(t.Context(), &lib); err != nil {
		t.Fatalf("create library: %v", err)
	}
	if err := db.Create(&model.Media{
		Base:      model.Base{ID: "cloud-1"},
		LibraryID: lib.ID,
		Title:     "Cloud Movie",
		Path:      "cloud://openlist/Movies/Movie.mkv",
		STRMURL:   "/api/cloud/play/openlist?ref=%2FMovies%2FMovie.mkv",
		Container: "mkv",
	}).Error; err != nil {
		t.Fatalf("create media: %v", err)
	}

	const secret = "test-secret"
	token := signedTestToken(t, secret)
	router := gin.New()
	registerEmbyRoutes(router, secret, &service.Container{
		Repo: repos,
		Emby: service.NewEmbyService(&config.Config{}, zap.NewNop(), repos),
	})

	req := httptest.NewRequest(http.MethodGet, "/emby/Users/user-1/Items?IncludeItemTypes=Movie&Recursive=true&Limit=5&X-Emby-Token="+token, nil)
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("unexpected status: %d body=%s", w.Code, w.Body.String())
	}
	var body map[string]any
	if err := json.Unmarshal(w.Body.Bytes(), &body); err != nil {
		t.Fatalf("decode items: %v", err)
	}
	items := body["Items"].([]any)
	if len(items) != 1 {
		t.Fatalf("unexpected items: %#v", body["Items"])
	}
	source := items[0].(map[string]any)["MediaSources"].([]any)[0].(map[string]any)
	pathURL, _ := source["Path"].(string)
	if pathURL != "/Videos/cloud-1/stream.mkv" {
		t.Fatalf("embedded cloud Path should stay as the non-tokenized standard stream URL, got %#v", source)
	}
	if strings.Contains(pathURL, "api_key=") || strings.Contains(pathURL, "token=") {
		t.Fatalf("embedded cloud Path must not expose auth key/token: %#v", source)
	}
}
