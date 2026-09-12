package service

import (
	"errors"
	"net/http"
	"net/http/httptest"
	"net/url"
	"os"
	"path/filepath"
	"testing"

	"go.uber.org/zap"

	"github.com/ShukeBta/MediaStationGo/internal/config"
	"github.com/ShukeBta/MediaStationGo/internal/model"
)

func TestServeFileLocalSTRMTargets(t *testing.T) {
	root := t.TempDir()
	isoPath := filepath.Join(root, "Movie with spaces 100%.iso")
	if err := os.WriteFile(isoPath, []byte("0123456789ISO media bytes"), 0o600); err != nil {
		t.Fatal(err)
	}
	uriPath := filepath.ToSlash(isoPath)
	if uriPath[0] != '/' {
		uriPath = "/" + uriPath
	}
	fileURI := (&url.URL{Scheme: "file", Path: uriPath}).String()
	for _, target := range []string{isoPath, fileURI, "./Movie with spaces 100%.iso"} {
		t.Run(target, func(t *testing.T) {
			repos := newStreamTestRepo(t)
			strmPath := filepath.Join(root, "Movie.strm")
			if err := os.WriteFile(strmPath, []byte("\ufeff# source\n"+target+"\n"), 0o600); err != nil {
				t.Fatal(err)
			}
			if got, err := readLocalSTRMTarget(strmPath); err != nil || got != target {
				t.Fatalf("readLocalSTRMTarget = %q, %v; want %q", got, err, target)
			}
			// Old scan results have an empty STRMURL and must work without a rescan.
			media := model.Media{Base: model.Base{ID: "local-iso"}, Path: strmPath, Container: "strm"}
			if err := repos.DB.Create(&media).Error; err != nil {
				t.Fatal(err)
			}
			for _, key := range []string{CloudPlaybackSTRMEnabledSettingKey, CloudPlaybackRedirectEnabledSettingKey} {
				if err := repos.Setting.Set(t.Context(), key, "false"); err != nil {
					t.Fatal(err)
				}
			}
			svc := NewStreamService(&config.Config{}, zap.NewNop(), repos, nil)
			request := httptest.NewRequest(http.MethodGet, "/api/stream/local-iso", nil)
			request.Header.Set("Range", "bytes=2-5")
			response := httptest.NewRecorder()
			if err := svc.ServeFile(response, request, media.ID); err != nil {
				t.Fatal(err)
			}
			if response.Code != http.StatusPartialContent || response.Body.String() != "2345" || response.Header().Get("Location") != "" {
				t.Fatalf("local STRM returned %d, %q, Location=%q", response.Code, response.Body.String(), response.Header().Get("Location"))
			}
			if container := embyMediaContainer(&media); container != "iso" {
				t.Fatalf("Emby container = %q, want iso", container)
			}
		})
	}
}

func TestServeFileDoesNotServeInvalidSTRMContents(t *testing.T) {
	repos := newStreamTestRepo(t)
	path := filepath.Join(t.TempDir(), "Invalid.strm")
	if err := os.WriteFile(path, []byte("file:///etc/passwd\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	media := model.Media{Base: model.Base{ID: "invalid-strm"}, Path: path, Container: "strm"}
	if err := repos.DB.Create(&media).Error; err != nil {
		t.Fatal(err)
	}
	svc := NewStreamService(&config.Config{}, zap.NewNop(), repos, nil)
	response := httptest.NewRecorder()
	err := svc.ServeFile(response, httptest.NewRequest(http.MethodGet, "/api/stream/invalid-strm", nil), media.ID)
	if !errors.Is(err, ErrMediaNotFound) || response.Body.Len() != 0 {
		t.Fatalf("invalid STRM should fail without exposing its contents: err=%v, body=%q", err, response.Body.String())
	}
}

func TestEmbyMediaContainerUsesSTRMTarget(t *testing.T) {
	for _, target := range []string{
		"https://cdn.example.test/Movies/Disc.ISO?signature=example",
		"/api/cloud/play/openlist?ref=%2FMovies%2FDisc.iso",
	} {
		media := model.Media{Path: "/library/Disc.strm", Container: "strm", STRMURL: target}
		if got := embyMediaContainer(&media); got != "iso" {
			t.Errorf("container for %q = %q, want iso", target, got)
		}
	}
}

func TestEmbyPlaybackInfoForLocalISOSTRM(t *testing.T) {
	svc := newTestEmbyService(t)
	root := t.TempDir()
	isoPath := filepath.Join(root, "Disc.iso")
	strmPath := filepath.Join(root, "Disc.strm")
	writeTestFile(t, isoPath, "ISO bytes")
	writeTestFile(t, strmPath, "./Disc.iso")
	lib := model.Library{Name: "ISO", Path: root, Type: "movie", Enabled: true}
	if err := svc.repo.Library.Create(t.Context(), &lib); err != nil {
		t.Fatal(err)
	}
	media := model.Media{Base: model.Base{ID: "local-iso"}, LibraryID: lib.ID, Title: "Disc", Path: strmPath, Container: "strm"}
	if err := svc.repo.Media.Upsert(t.Context(), &media); err != nil {
		t.Fatal(err)
	}
	playback, err := svc.PlaybackInfo(t.Context(), media.ID, "user-1")
	if err != nil {
		t.Fatal(err)
	}
	source := playback["MediaSources"].([]map[string]any)[0]
	for key, want := range map[string]any{
		"Container": "iso", "Path": isoPath, "IsRemote": false,
		"Size":               int64(len("ISO bytes")),
		"DirectStreamUrl":    "/Videos/local-iso/stream.iso",
		"SupportsDirectPlay": true, "SupportsTranscoding": false,
	} {
		if source[key] != want {
			t.Errorf("%s = %#v, want %#v", key, source[key], want)
		}
	}
	if _, exists := source["TranscodingUrl"]; exists {
		t.Fatal("ISO must not advertise generic HLS transcoding")
	}
}

func TestCloudSTRMCannotAuthorizeLocalPlayback(t *testing.T) {
	repos := newStreamTestRepo(t)
	path := filepath.Join(t.TempDir(), "Private.iso")
	writeTestFile(t, path, "private media")
	media := model.Media{Base: model.Base{ID: "cloud-strm"}, Path: "cloud://openlist/Disc.strm", Container: "strm", STRMURL: path}
	if err := repos.DB.Create(&media).Error; err != nil {
		t.Fatal(err)
	}
	svc := NewStreamService(&config.Config{}, zap.NewNop(), repos, nil)
	response := httptest.NewRecorder()
	err := svc.ServeFile(response, httptest.NewRequest(http.MethodGet, "/api/stream/cloud-strm", nil), media.ID)
	if !errors.Is(err, ErrCloudPlaybackUnavailable) || response.Body.Len() != 0 {
		t.Fatalf("cloud STRM must not read local media: err=%v, body=%q", err, response.Body.String())
	}
}
