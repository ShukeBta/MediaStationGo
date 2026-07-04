package service

import (
	"bytes"
	"net/http"
	"net/http/httptest"
	"testing"

	"go.uber.org/zap"

	"github.com/ShukeBta/MediaStationGo/internal/config"
	"github.com/ShukeBta/MediaStationGo/internal/model"
	"github.com/ShukeBta/MediaStationGo/internal/repository"
)

func TestScanCloudLibraryCachesFileLevelRemoteArtwork(t *testing.T) {
	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.Method {
		case "PROPFIND":
			if r.URL.Path != "/dav/Movies" {
				t.Fatalf("unexpected propfind path %s", r.URL.Path)
			}
			w.Header().Set("Content-Type", "application/xml")
			w.WriteHeader(http.StatusMultiStatus)
			_, _ = w.Write([]byte(`<?xml version="1.0" encoding="utf-8"?>
<d:multistatus xmlns:d="DAV:">
  <d:response><d:href>/dav/Movies/</d:href><d:propstat><d:prop><d:resourcetype><d:collection/></d:resourcetype></d:prop></d:propstat></d:response>
  <d:response><d:href>/dav/Movies/Movie.mkv</d:href><d:propstat><d:prop><d:displayname>Movie.mkv</d:displayname><d:getcontentlength>4096</d:getcontentlength><d:resourcetype/></d:prop></d:propstat></d:response>
  <d:response><d:href>/dav/Movies/Movie.nfo</d:href><d:propstat><d:prop><d:displayname>Movie.nfo</d:displayname><d:getcontentlength>128</d:getcontentlength><d:resourcetype/></d:prop></d:propstat></d:response>
  <d:response><d:href>/dav/Movies/Movie.jpg</d:href><d:propstat><d:prop><d:displayname>Movie.jpg</d:displayname><d:getcontentlength>1024</d:getcontentlength><d:resourcetype/></d:prop></d:propstat></d:response>
</d:multistatus>`))
		case http.MethodGet:
			switch r.URL.Path {
			case "/dav/Movies/Movie.nfo":
				_, _ = w.Write([]byte(`<movie><title>Sidecar Movie</title><year>2026</year></movie>`))
			case "/dav/Movies/Movie.jpg":
				w.Header().Set("Content-Type", "image/jpeg")
				_, _ = w.Write(testJPEG)
			default:
				t.Fatalf("unexpected get path %s", r.URL.Path)
			}
		default:
			t.Fatalf("unexpected method %s", r.Method)
		}
	}))
	defer upstream.Close()

	db := newServiceTestDB(t, &model.Library{}, &model.Media{}, &model.Setting{}, &model.StorageConfig{})
	repos := repository.New(db)
	log := zap.NewNop()
	storage := NewStorageConfigService(log, repos, NewCryptoService("", log))
	if _, err := storage.Save(t.Context(), StorageInput{
		Type: "openlist",
		Config: map[string]any{
			"url": upstream.URL,
		},
	}); err != nil {
		t.Fatal(err)
	}
	lib := model.Library{Name: "OpenList · Movies", Path: "cloud://openlist/Movies", Type: "movie", Enabled: true}
	if err := repos.Library.Create(t.Context(), &lib); err != nil {
		t.Fatal(err)
	}
	scanner := NewScannerService(&config.Config{}, log, repos, NewHub(log), nil, nil)
	scanner.SetStorageConfig(storage)
	imageProxy := NewImageProxy(&config.Config{Cache: config.CacheConfig{CacheDir: t.TempDir()}}, log)
	scanner.SetImageProxy(imageProxy)

	res, err := scanner.ScanLibrary(t.Context(), lib.ID)
	if err != nil {
		t.Fatalf("scan cloud: %v", err)
	}
	if res.Added != 1 || res.LocalMetadata != 1 {
		t.Fatalf("scan result = %#v, want added=1 local_metadata=1", res)
	}
	var media model.Media
	if err := repos.DB.First(&media).Error; err != nil {
		t.Fatal(err)
	}
	if media.Title != "Sidecar Movie" || media.Year != 2026 {
		t.Fatalf("metadata not applied: %#v", media)
	}
	if media.PosterURL != "/api/img/cloud/openlist?ref=%2FMovies%2FMovie.jpg" {
		t.Fatalf("poster url = %q", media.PosterURL)
	}
	rec := httptest.NewRecorder()
	if !imageProxy.ServeCloudCached(rec, httptest.NewRequest(http.MethodGet, media.PosterURL, nil), "openlist:/Movies/Movie.jpg") {
		t.Fatal("file-level cloud poster should be cached locally during scan before media is exposed")
	}
	if got := rec.Body.Bytes(); !bytes.Equal(got, testJPEG) {
		t.Fatalf("cached poster body = %x", got)
	}
}

func TestScanCloudLibraryUsesArtworkReferencedByRemoteNFO(t *testing.T) {
	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.Method {
		case "PROPFIND":
			if r.URL.Path != "/dav/Movies" {
				t.Fatalf("unexpected propfind path %s", r.URL.Path)
			}
			w.Header().Set("Content-Type", "application/xml")
			w.WriteHeader(http.StatusMultiStatus)
			_, _ = w.Write([]byte(`<?xml version="1.0" encoding="utf-8"?>
<d:multistatus xmlns:d="DAV:">
  <d:response><d:href>/dav/Movies/</d:href><d:propstat><d:prop><d:resourcetype><d:collection/></d:resourcetype></d:prop></d:propstat></d:response>
  <d:response><d:href>/dav/Movies/Movie.mkv</d:href><d:propstat><d:prop><d:displayname>Movie.mkv</d:displayname><d:getcontentlength>4096</d:getcontentlength><d:resourcetype/></d:prop></d:propstat></d:response>
  <d:response><d:href>/dav/Movies/Movie.nfo</d:href><d:propstat><d:prop><d:displayname>Movie.nfo</d:displayname><d:getcontentlength>256</d:getcontentlength><d:resourcetype/></d:prop></d:propstat></d:response>
  <d:response><d:href>/dav/Movies/Artwork.Custom.tbn</d:href><d:propstat><d:prop><d:displayname>Artwork.Custom.tbn</d:displayname><d:getcontentlength>1024</d:getcontentlength><d:resourcetype/></d:prop></d:propstat></d:response>
  <d:response><d:href>/dav/Movies/Scene.Still.png</d:href><d:propstat><d:prop><d:displayname>Scene.Still.png</d:displayname><d:getcontentlength>1024</d:getcontentlength><d:resourcetype/></d:prop></d:propstat></d:response>
</d:multistatus>`))
		case http.MethodGet:
			switch r.URL.Path {
			case "/dav/Movies/Movie.nfo":
				_, _ = w.Write([]byte(`<movie><title>NFO Custom Artwork</title><thumb aspect="poster">Artwork.Custom.tbn</thumb><fanart><thumb>Scene.Still.png?version=1</thumb></fanart></movie>`))
			case "/dav/Movies/Artwork.Custom.tbn":
				w.Header().Set("Content-Type", "image/jpeg")
				_, _ = w.Write(testJPEG)
			case "/dav/Movies/Scene.Still.png":
				w.Header().Set("Content-Type", "image/jpeg")
				_, _ = w.Write(testJPEG)
			default:
				t.Fatalf("unexpected get path %s", r.URL.Path)
			}
		default:
			t.Fatalf("unexpected method %s", r.Method)
		}
	}))
	defer upstream.Close()

	db := newServiceTestDB(t, &model.Library{}, &model.Media{}, &model.Setting{}, &model.StorageConfig{})
	repos := repository.New(db)
	log := zap.NewNop()
	storage := NewStorageConfigService(log, repos, NewCryptoService("", log))
	if _, err := storage.Save(t.Context(), StorageInput{
		Type: "openlist",
		Config: map[string]any{
			"url": upstream.URL,
		},
	}); err != nil {
		t.Fatal(err)
	}
	lib := model.Library{Name: "OpenList · Movies", Path: "cloud://openlist/Movies", Type: "movie", Enabled: true}
	if err := repos.Library.Create(t.Context(), &lib); err != nil {
		t.Fatal(err)
	}
	scanner := NewScannerService(&config.Config{}, log, repos, NewHub(log), nil, nil)
	scanner.SetStorageConfig(storage)
	imageProxy := NewImageProxy(&config.Config{Cache: config.CacheConfig{CacheDir: t.TempDir()}}, log)
	scanner.SetImageProxy(imageProxy)

	res, err := scanner.ScanLibrary(t.Context(), lib.ID)
	if err != nil {
		t.Fatalf("scan cloud: %v", err)
	}
	if res.Added != 1 || res.LocalMetadata != 1 {
		t.Fatalf("scan result = %#v, want added=1 local_metadata=1", res)
	}
	var media model.Media
	if err := repos.DB.First(&media).Error; err != nil {
		t.Fatal(err)
	}
	if media.Title != "NFO Custom Artwork" {
		t.Fatalf("metadata title = %q", media.Title)
	}
	if media.PosterURL != "/api/img/cloud/openlist?ref=%2FMovies%2FArtwork.Custom.tbn" {
		t.Fatalf("poster url = %q", media.PosterURL)
	}
	if media.BackdropURL != "/api/img/cloud/openlist?ref=%2FMovies%2FScene.Still.png" {
		t.Fatalf("backdrop url = %q", media.BackdropURL)
	}
	rec := httptest.NewRecorder()
	if !imageProxy.ServeCloudCached(rec, httptest.NewRequest(http.MethodGet, media.PosterURL, nil), "openlist:/Movies/Artwork.Custom.tbn") {
		t.Fatal("NFO-referenced cloud poster should be cached locally during scan")
	}
	if got := rec.Body.Bytes(); !bytes.Equal(got, testJPEG) {
		t.Fatalf("cached poster body = %x", got)
	}
}

func TestScanCloudLibraryReadsRemoteNFOAndArtwork(t *testing.T) {
	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.Method {
		case "PROPFIND":
			w.Header().Set("Content-Type", "application/xml")
			w.WriteHeader(http.StatusMultiStatus)
			switch r.URL.Path {
			case "/dav/Anime/JianLai":
				_, _ = w.Write([]byte(`<?xml version="1.0" encoding="utf-8"?>
<d:multistatus xmlns:d="DAV:">
  <d:response><d:href>/dav/Anime/JianLai/</d:href><d:propstat><d:prop><d:resourcetype><d:collection/></d:resourcetype></d:prop></d:propstat></d:response>
  <d:response><d:href>/dav/Anime/JianLai/tvshow.nfo</d:href><d:propstat><d:prop><d:displayname>tvshow.nfo</d:displayname><d:getcontentlength>64</d:getcontentlength><d:resourcetype/></d:prop></d:propstat></d:response>
  <d:response><d:href>/dav/Anime/JianLai/poster.jpg</d:href><d:propstat><d:prop><d:displayname>poster.jpg</d:displayname><d:getcontentlength>1024</d:getcontentlength><d:resourcetype/></d:prop></d:propstat></d:response>
  <d:response><d:href>/dav/Anime/JianLai/Season1/</d:href><d:propstat><d:prop><d:displayname>Season1</d:displayname><d:resourcetype><d:collection/></d:resourcetype></d:prop></d:propstat></d:response>
</d:multistatus>`))
			case "/dav/Anime/JianLai/Season1":
				_, _ = w.Write([]byte(`<?xml version="1.0" encoding="utf-8"?>
<d:multistatus xmlns:d="DAV:">
  <d:response><d:href>/dav/Anime/JianLai/Season1/</d:href><d:propstat><d:prop><d:resourcetype><d:collection/></d:resourcetype></d:prop></d:propstat></d:response>
  <d:response><d:href>/dav/Anime/JianLai/Season1/JianLai.S01E01.mkv</d:href><d:propstat><d:prop><d:displayname>JianLai.S01E01.mkv</d:displayname><d:getcontentlength>2048</d:getcontentlength><d:resourcetype/></d:prop></d:propstat></d:response>
  <d:response><d:href>/dav/Anime/JianLai/Season1/JianLai.S01E01.nfo</d:href><d:propstat><d:prop><d:displayname>JianLai.S01E01.nfo</d:displayname><d:getcontentlength>128</d:getcontentlength><d:resourcetype/></d:prop></d:propstat></d:response>
</d:multistatus>`))
			default:
				t.Fatalf("unexpected propfind path %s", r.URL.Path)
			}
		case http.MethodGet:
			switch r.URL.Path {
			case "/dav/Anime/JianLai/tvshow.nfo":
				_, _ = w.Write([]byte(`<tvshow><title>剑来</title><year>2024</year><plot>天地有剑气</plot></tvshow>`))
			case "/dav/Anime/JianLai/Season1/JianLai.S01E01.nfo":
				_, _ = w.Write([]byte(`<episodedetails><showtitle>剑来</showtitle><title>第一集</title><season>1</season><episode>1</episode></episodedetails>`))
			case "/dav/Anime/JianLai/poster.jpg":
				w.Header().Set("Content-Type", "image/jpeg")
				_, _ = w.Write(testJPEG)
			default:
				t.Fatalf("unexpected get path %s", r.URL.Path)
			}
		default:
			t.Fatalf("unexpected method %s", r.Method)
		}
	}))
	defer upstream.Close()

	db := newServiceTestDB(t, &model.Library{}, &model.Media{}, &model.Setting{}, &model.StorageConfig{})
	repos := repository.New(db)
	log := zap.NewNop()
	storage := NewStorageConfigService(log, repos, NewCryptoService("", log))
	if _, err := storage.Save(t.Context(), StorageInput{
		Type: "openlist",
		Config: map[string]any{
			"url": upstream.URL,
		},
	}); err != nil {
		t.Fatal(err)
	}
	lib := model.Library{Name: "OpenList · 国漫 · 剑来", Path: "cloud://openlist/Anime/JianLai", Type: "anime", Enabled: true}
	if err := repos.Library.Create(t.Context(), &lib); err != nil {
		t.Fatal(err)
	}
	scanner := NewScannerService(&config.Config{}, log, repos, NewHub(log), nil, nil)
	scanner.SetStorageConfig(storage)
	imageProxy := NewImageProxy(&config.Config{Cache: config.CacheConfig{CacheDir: t.TempDir()}}, log)
	scanner.SetImageProxy(imageProxy)

	res, err := scanner.ScanLibrary(t.Context(), lib.ID)
	if err != nil {
		t.Fatalf("scan cloud: %v", err)
	}
	if res.Added != 1 || res.LocalMetadata != 1 {
		t.Fatalf("scan result = %#v, want added=1 local_metadata=1", res)
	}
	var media model.Media
	if err := repos.DB.First(&media).Error; err != nil {
		t.Fatal(err)
	}
	// 单集名(episode <title>「第一集」)不得写入 OriginalName(整剧原名/分组键)。
	// tvshow.nfo 未提供 originaltitle, 故 OriginalName 应为空。
	if media.Title != "剑来" || media.OriginalName != "" || media.Year != 2024 {
		t.Fatalf("metadata not applied: %#v", media)
	}
	if media.SeasonNum != 1 || media.EpisodeNum != 1 {
		t.Fatalf("episode numbers = %d/%d", media.SeasonNum, media.EpisodeNum)
	}
	if media.PosterURL != "/api/img/cloud/openlist?ref=%2FAnime%2FJianLai%2Fposter.jpg" {
		t.Fatalf("poster url = %q", media.PosterURL)
	}
	rec := httptest.NewRecorder()
	if !imageProxy.ServeCloudCached(rec, httptest.NewRequest(http.MethodGet, media.PosterURL, nil), "openlist:/Anime/JianLai/poster.jpg") {
		t.Fatal("cloud poster should be cached locally during scan before media is exposed")
	}
	if got := rec.Body.Bytes(); !bytes.Equal(got, testJPEG) {
		t.Fatalf("cached poster body = %x", got)
	}
	if media.ScrapeStatus != "matched" {
		t.Fatalf("scrape status = %q", media.ScrapeStatus)
	}
}

func TestScanCloudLibraryRefreshesExistingRemoteNFOAndArtwork(t *testing.T) {
	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.Method {
		case "PROPFIND":
			w.Header().Set("Content-Type", "application/xml")
			w.WriteHeader(http.StatusMultiStatus)
			switch r.URL.Path {
			case "/dav/Anime/JianLai":
				_, _ = w.Write([]byte(`<?xml version="1.0" encoding="utf-8"?>
<d:multistatus xmlns:d="DAV:">
  <d:response><d:href>/dav/Anime/JianLai/</d:href><d:propstat><d:prop><d:resourcetype><d:collection/></d:resourcetype></d:prop></d:propstat></d:response>
  <d:response><d:href>/dav/Anime/JianLai/tvshow.nfo</d:href><d:propstat><d:prop><d:displayname>tvshow.nfo</d:displayname><d:getcontentlength>64</d:getcontentlength><d:resourcetype/></d:prop></d:propstat></d:response>
  <d:response><d:href>/dav/Anime/JianLai/poster.jpg</d:href><d:propstat><d:prop><d:displayname>poster.jpg</d:displayname><d:getcontentlength>1024</d:getcontentlength><d:resourcetype/></d:prop></d:propstat></d:response>
  <d:response><d:href>/dav/Anime/JianLai/Season1/</d:href><d:propstat><d:prop><d:displayname>Season1</d:displayname><d:resourcetype><d:collection/></d:resourcetype></d:prop></d:propstat></d:response>
</d:multistatus>`))
			case "/dav/Anime/JianLai/Season1":
				_, _ = w.Write([]byte(`<?xml version="1.0" encoding="utf-8"?>
<d:multistatus xmlns:d="DAV:">
  <d:response><d:href>/dav/Anime/JianLai/Season1/</d:href><d:propstat><d:prop><d:resourcetype><d:collection/></d:resourcetype></d:prop></d:propstat></d:response>
  <d:response><d:href>/dav/Anime/JianLai/Season1/JianLai.S01E01.mkv</d:href><d:propstat><d:prop><d:displayname>JianLai.S01E01.mkv</d:displayname><d:getcontentlength>2048</d:getcontentlength><d:resourcetype/></d:prop></d:propstat></d:response>
  <d:response><d:href>/dav/Anime/JianLai/Season1/JianLai.S01E01.nfo</d:href><d:propstat><d:prop><d:displayname>JianLai.S01E01.nfo</d:displayname><d:getcontentlength>128</d:getcontentlength><d:resourcetype/></d:prop></d:propstat></d:response>
</d:multistatus>`))
			default:
				t.Fatalf("unexpected propfind path %s", r.URL.Path)
			}
		case http.MethodGet:
			switch r.URL.Path {
			case "/dav/Anime/JianLai/tvshow.nfo":
				_, _ = w.Write([]byte(`<tvshow><title>剑来</title><year>2024</year><plot>天地有剑气</plot><uniqueid type="tmdb">296753</uniqueid></tvshow>`))
			case "/dav/Anime/JianLai/Season1/JianLai.S01E01.nfo":
				_, _ = w.Write([]byte(`<episodedetails><showtitle>剑来</showtitle><title>第一集</title><season>1</season><episode>1</episode></episodedetails>`))
			case "/dav/Anime/JianLai/poster.jpg":
				w.Header().Set("Content-Type", "image/jpeg")
				_, _ = w.Write(testJPEG)
			default:
				t.Fatalf("unexpected get path %s", r.URL.Path)
			}
		default:
			t.Fatalf("unexpected method %s", r.Method)
		}
	}))
	defer upstream.Close()

	db := newServiceTestDB(t, &model.Library{}, &model.Media{}, &model.Setting{}, &model.StorageConfig{})
	repos := repository.New(db)
	log := zap.NewNop()
	storage := NewStorageConfigService(log, repos, NewCryptoService("", log))
	if _, err := storage.Save(t.Context(), StorageInput{
		Type: "openlist",
		Config: map[string]any{
			"url": upstream.URL,
		},
	}); err != nil {
		t.Fatal(err)
	}
	lib := model.Library{Name: "OpenList · 国漫 · 剑来", Path: "cloud://openlist/Anime/JianLai", Type: "anime", Enabled: true}
	if err := repos.Library.Create(t.Context(), &lib); err != nil {
		t.Fatal(err)
	}

	mediaPath := "cloud://openlist/Anime/JianLai/Season1/JianLai.S01E01.mkv"
	old := model.Media{
		LibraryID:    lib.ID,
		Title:        "JianLai.S01E01",
		Path:         mediaPath,
		SizeBytes:    2048,
		Container:    "mkv",
		PosterURL:    "https://image.tmdb.org/t/p/w500/old.jpg",
		STRMURL:      BuildRelativeCloudPlayURL("openlist", "/Anime/JianLai/Season1/JianLai.S01E01.mkv"),
		ScrapeStatus: "no_match",
	}
	if err := repos.Media.Upsert(t.Context(), &old); err != nil {
		t.Fatal(err)
	}

	scanner := NewScannerService(&config.Config{}, log, repos, NewHub(log), nil, nil)
	scanner.SetStorageConfig(storage)
	imageProxy := NewImageProxy(&config.Config{Cache: config.CacheConfig{CacheDir: t.TempDir()}}, log)
	scanner.SetImageProxy(imageProxy)

	res, err := scanner.ScanLibrary(t.Context(), lib.ID)
	if err != nil {
		t.Fatalf("scan cloud: %v", err)
	}
	if res.Updated != 1 || res.LocalMetadata != 1 {
		t.Fatalf("scan result = %#v, want updated=1 local_metadata=1", res)
	}
	var media model.Media
	if err := repos.DB.First(&media, "path = ?", mediaPath).Error; err != nil {
		t.Fatal(err)
	}
	if media.Title != "剑来" || media.Year != 2024 || media.TMDbID != 296753 {
		t.Fatalf("metadata not refreshed: %#v", media)
	}
	if media.PosterURL != "/api/img/cloud/openlist?ref=%2FAnime%2FJianLai%2Fposter.jpg" {
		t.Fatalf("poster url = %q", media.PosterURL)
	}
	if media.ScrapeStatus != "matched" {
		t.Fatalf("scrape status = %q", media.ScrapeStatus)
	}
	rec := httptest.NewRecorder()
	if !imageProxy.ServeCloudCached(rec, httptest.NewRequest(http.MethodGet, media.PosterURL, nil), "openlist:/Anime/JianLai/poster.jpg") {
		t.Fatal("refreshed cloud poster should be cached locally during scan")
	}
	if got := rec.Body.Bytes(); !bytes.Equal(got, testJPEG) {
		t.Fatalf("cached poster body = %x", got)
	}
}
