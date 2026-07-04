package service

import (
	"errors"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"testing"

	"go.uber.org/zap"

	"github.com/ShukeBta/MediaStationGo/internal/config"
	"github.com/ShukeBta/MediaStationGo/internal/model"
	"github.com/ShukeBta/MediaStationGo/internal/repository"
)

func TestWithAuthTokenPropagatesToInternalRedirect(t *testing.T) {
	// <video src=/api/stream/{id}?token=JWT> follows the 302 to the cloud
	// play endpoint, which must stay authenticated.
	r := &http.Request{Header: http.Header{}, URL: &url.URL{RawQuery: "token=jwt123&profile=p"}}
	got := withAuthToken("/api/cloud/play/cloud115?ref=abc", r)
	u, err := url.Parse(got)
	if err != nil {
		t.Fatalf("parse: %v", err)
	}
	if u.Query().Get("token") != "jwt123" {
		t.Fatalf("token not propagated: %q", got)
	}
	if u.Query().Get("ref") != "abc" {
		t.Fatalf("existing query lost: %q", got)
	}
}

func TestWithAuthTokenNeverLeaksToAbsoluteURL(t *testing.T) {
	// An absolute external direct link (e.g. cloud CDN) must NOT receive the JWT.
	r := &http.Request{Header: http.Header{}, URL: &url.URL{RawQuery: "token=jwt123"}}
	got := withAuthToken("https://cdn.115.example/x.mp4?sig=1", r)
	if strings.Contains(got, "jwt123") {
		t.Fatalf("JWT leaked to external URL: %q", got)
	}
	if got != "https://cdn.115.example/x.mp4?sig=1" {
		t.Fatalf("external URL mutated: %q", got)
	}
}

func TestWithAuthTokenPropagatesToSameOriginAbsoluteInternalURL(t *testing.T) {
	r := httptest.NewRequest(http.MethodGet, "http://media.example/Videos/m-1/stream?api_key=jwt123", nil)
	got := withAuthTokenForInternalRedirect("http://media.example/api/cloud/play/openlist?ref=abc", r, "http://media.example")
	u, err := url.Parse(got)
	if err != nil {
		t.Fatalf("parse: %v", err)
	}
	if u.Query().Get("token") != "jwt123" || u.Query().Get("ref") != "abc" {
		t.Fatalf("same-origin internal URL should keep ref and receive token: %q", got)
	}
}

func TestWithAuthTokenAddsMediaIDToCloudPlaybackRedirect(t *testing.T) {
	req := httptest.NewRequest(http.MethodGet, "http://media.example/api/stream/media-1?token=jwt123", nil)
	got := withAuthTokenForInternalRedirect("/api/cloud/play/openlist?ref=abc", req, "")
	u, err := url.Parse(got)
	if err != nil {
		t.Fatalf("parse: %v", err)
	}
	if u.Query().Get("token") != "jwt123" || u.Query().Get("media_id") != "media-1" {
		t.Fatalf("cloud redirect should carry token and media_id, got %q", got)
	}
}

func TestServeFileRedirectsInternalSTRMAsAbsoluteURLWithToken(t *testing.T) {
	repos := newStreamTestRepo(t)
	if err := repos.DB.Create(&model.Media{
		Base:    model.Base{ID: "cloud-1"},
		Title:   "Cloud",
		Path:    "cloud://openlist/Movie.mkv",
		STRMURL: "/api/cloud/play/openlist?ref=movie",
	}).Error; err != nil {
		t.Fatal(err)
	}
	svc := NewStreamService(&config.Config{}, zap.NewNop(), repos, nil)
	req := httptest.NewRequest(http.MethodGet, "http://nas.local:18080/api/stream/cloud-1?api_key=jwt123", nil)
	w := httptest.NewRecorder()

	if err := svc.ServeFile(w, req, "cloud-1"); err != nil {
		t.Fatal(err)
	}
	if w.Code != http.StatusFound {
		t.Fatalf("status = %d, want 302", w.Code)
	}
	loc := w.Header().Get("Location")
	if !strings.HasPrefix(loc, "http://nas.local:18080/api/cloud/play/openlist?") ||
		!strings.Contains(loc, "ref=movie") ||
		!strings.Contains(loc, "token=jwt123") {
		t.Fatalf("redirect Location should be absolute and tokenized, got %q", loc)
	}
	if got := w.Header().Get("Cache-Control"); !strings.Contains(got, "no-store") {
		t.Fatalf("cloud redirect Cache-Control = %q, want no-store", got)
	}
}

func TestServeFileRedirectUsesForwardedTunnelHost(t *testing.T) {
	repos := newStreamTestRepo(t)
	if err := repos.DB.Create(&model.Media{
		Base:    model.Base{ID: "cloud-1"},
		Title:   "Cloud",
		Path:    "cloud://openlist/Movie.mkv",
		STRMURL: "/api/cloud/play/openlist?ref=movie",
	}).Error; err != nil {
		t.Fatal(err)
	}
	svc := NewStreamService(&config.Config{}, zap.NewNop(), repos, nil)
	req := httptest.NewRequest(http.MethodGet, "http://127.0.0.1:8080/api/stream/cloud-1?api_key=jwt123", nil)
	req.Header.Set("X-Forwarded-Host", "media.example.com")
	req.Header.Set("X-Forwarded-Proto", "https")
	w := httptest.NewRecorder()

	if err := svc.ServeFile(w, req, "cloud-1"); err != nil {
		t.Fatal(err)
	}
	if w.Code != http.StatusFound {
		t.Fatalf("status = %d, want 302", w.Code)
	}
	loc := w.Header().Get("Location")
	if !strings.HasPrefix(loc, "https://media.example.com/api/cloud/play/openlist?") ||
		!strings.Contains(loc, "ref=movie") ||
		!strings.Contains(loc, "token=jwt123") {
		t.Fatalf("redirect Location should use forwarded tunnel host and token, got %q", loc)
	}
}

func TestServeFileRedirectsCloudMediaForVideoStreamMode(t *testing.T) {
	repos := newStreamTestRepo(t)
	if err := repos.Setting.Set(t.Context(), CloudPlaybackModeSettingKey, CloudPlaybackModeRedirectProxy); err != nil {
		t.Fatal(err)
	}
	if err := repos.DB.Create(&model.Media{
		Base:    model.Base{ID: "cloud-1"},
		Title:   "Cloud",
		Path:    "cloud://openlist/Movie.mkv",
		STRMURL: "/api/cloud/play/openlist?ref=movie",
	}).Error; err != nil {
		t.Fatal(err)
	}
	svc := NewStreamService(&config.Config{}, zap.NewNop(), repos, nil)
	req := httptest.NewRequest(http.MethodGet, "http://nas.local:18080/api/stream/cloud-1?api_key=jwt123", nil)
	w := httptest.NewRecorder()

	err := svc.ServeFile(w, req, "cloud-1")
	if err != nil {
		t.Fatalf("video stream mode should still reach cloud playback endpoint: %v", err)
	}
	if w.Code != http.StatusFound {
		t.Fatalf("status = %d, want 302", w.Code)
	}
	loc := w.Header().Get("Location")
	if !strings.Contains(loc, "/api/cloud/play/openlist?") || !strings.Contains(loc, "token=jwt123") {
		t.Fatalf("redirect Location should target tokenized cloud play endpoint, got %q", loc)
	}
}

func TestServeFileRedirectsCloudMediaExternalHTTPSTRMURL(t *testing.T) {
	repos := newStreamTestRepo(t)
	target := "https://cdn.example.test/Movie.mkv?sign=direct"
	if err := repos.DB.Create(&model.Media{
		Base:    model.Base{ID: "cloud-http"},
		Title:   "Cloud HTTP",
		Path:    "cloud://openlist/Movie.mkv",
		STRMURL: target,
	}).Error; err != nil {
		t.Fatal(err)
	}
	svc := NewStreamService(&config.Config{}, zap.NewNop(), repos, nil)
	req := httptest.NewRequest(http.MethodGet, "http://nas.local:18080/api/stream/cloud-http?token=jwt123", nil)
	w := httptest.NewRecorder()

	if err := svc.ServeFile(w, req, "cloud-http"); err != nil {
		t.Fatalf("external HTTP STRM target should redirect: %v", err)
	}
	if w.Code != http.StatusFound {
		t.Fatalf("status = %d, want 302", w.Code)
	}
	loc := w.Header().Get("Location")
	if loc != target {
		t.Fatalf("Location = %q, want %q", loc, target)
	}
	if strings.Contains(loc, "jwt123") || strings.Contains(loc, "media_id=") {
		t.Fatalf("external direct link must not receive internal auth query, got %q", loc)
	}
}

func TestServeFileRedirectsLocalSTRMFileTargetByDefault(t *testing.T) {
	repos := newStreamTestRepo(t)
	target := "https://cdn.example.test/LocalMovie.mkv?sign=direct"
	if err := repos.DB.Create(&model.Media{
		Base:      model.Base{ID: "local-strm"},
		Title:     "Local STRM",
		Path:      "D:/media/LocalMovie.strm",
		Container: "strm",
		STRMURL:   target,
	}).Error; err != nil {
		t.Fatal(err)
	}
	svc := NewStreamService(&config.Config{}, zap.NewNop(), repos, nil)
	req := httptest.NewRequest(http.MethodGet, "http://nas.local:18080/api/stream/local-strm?token=jwt123", nil)
	w := httptest.NewRecorder()

	if err := svc.ServeFile(w, req, "local-strm"); err != nil {
		t.Fatalf("local .strm media should redirect to its target: %v", err)
	}
	if w.Code != http.StatusFound {
		t.Fatalf("status = %d, want 302", w.Code)
	}
	if loc := w.Header().Get("Location"); loc != target {
		t.Fatalf("Location = %q, want %q", loc, target)
	}
}

func TestCloudPlaybackModeUsesExplicitModeBeforeLegacySTRMFlag(t *testing.T) {
	repos := newStreamTestRepo(t)
	if got := CloudPlaybackMode(t.Context(), repos); got != CloudPlaybackModeRedirectProxy {
		t.Fatalf("default mode = %q, want %q", got, CloudPlaybackModeRedirectProxy)
	}
	if err := repos.Setting.Set(t.Context(), STRMEnabledSettingKey, "true"); err != nil {
		t.Fatal(err)
	}
	if got := CloudPlaybackMode(t.Context(), repos); got != CloudPlaybackModeSTRM {
		t.Fatalf("legacy strm.enabled=true mode = %q, want %q", got, CloudPlaybackModeSTRM)
	}
	if err := repos.Setting.Set(t.Context(), CloudPlaybackModeSettingKey, CloudPlaybackModeRedirectProxy); err != nil {
		t.Fatal(err)
	}
	if got := CloudPlaybackMode(t.Context(), repos); got != CloudPlaybackModeRedirectProxy {
		t.Fatalf("explicit mode should override legacy flag, got %q", got)
	}
	if err := repos.Setting.Set(t.Context(), CloudPlaybackModeSettingKey, CloudPlaybackModeSTRM); err != nil {
		t.Fatal(err)
	}
	if got := CloudPlaybackMode(t.Context(), repos); got != CloudPlaybackModeSTRM {
		t.Fatalf("explicit strm mode = %q, want %q", got, CloudPlaybackModeSTRM)
	}
	if err := repos.Setting.Set(t.Context(), CloudPlaybackSTRMEnabledSettingKey, "false"); err != nil {
		t.Fatal(err)
	}
	if err := repos.Setting.Set(t.Context(), CloudPlaybackRedirectEnabledSettingKey, "false"); err != nil {
		t.Fatal(err)
	}
	if got := CloudPlaybackMode(t.Context(), repos); got != "" {
		t.Fatalf("both disabled mode = %q, want empty", got)
	}
}

func TestServeFileRejectsCloudMediaWhenSelectedModeDisabled(t *testing.T) {
	repos := newStreamTestRepo(t)
	if err := repos.Setting.Set(t.Context(), CloudPlaybackSTRMEnabledSettingKey, "false"); err != nil {
		t.Fatal(err)
	}
	if err := repos.Setting.Set(t.Context(), CloudPlaybackRedirectEnabledSettingKey, "false"); err != nil {
		t.Fatal(err)
	}
	if err := repos.DB.Create(&model.Media{
		Base:    model.Base{ID: "cloud-1"},
		Title:   "Cloud",
		Path:    "cloud://openlist/Movie.mkv",
		STRMURL: "/api/cloud/play/openlist?ref=movie",
	}).Error; err != nil {
		t.Fatal(err)
	}
	svc := NewStreamService(&config.Config{}, zap.NewNop(), repos, nil)
	req := httptest.NewRequest(http.MethodGet, "http://nas.local:18080/api/stream/cloud-1?api_key=jwt123", nil)
	w := httptest.NewRecorder()

	err := svc.ServeFileWithCloudMode(w, req, "cloud-1", CloudPlaybackModeSTRM)
	if !errors.Is(err, ErrCloudPlaybackDisabled) {
		t.Fatalf("error = %v, want ErrCloudPlaybackDisabled", err)
	}
}

func newStreamTestRepo(t *testing.T) *repository.Container {
	t.Helper()
	db := newServiceTestDB(t, &model.Media{}, &model.Setting{})
	return repository.New(db)
}

func TestRequestTokenFromBearerHeader(t *testing.T) {
	h := http.Header{}
	h.Set("Authorization", "Bearer hdrtok")
	r := &http.Request{Header: h, URL: &url.URL{}}
	if got := requestToken(r); got != "hdrtok" {
		t.Fatalf("bearer token not extracted: %q", got)
	}
}

func TestRequestTokenFromMediaBrowserAuthorizationHeader(t *testing.T) {
	h := http.Header{}
	h.Set("X-MediaBrowser-Authorization", `MediaBrowser Client="Infuse", Device="PC", Token="mbtok"`)
	r := &http.Request{Header: h, URL: &url.URL{}}
	if got := requestToken(r); got != "mbtok" {
		t.Fatalf("MediaBrowser token not extracted: %q", got)
	}
}

func TestAppendQueryToHLSSegments(t *testing.T) {
	in := "#EXTM3U\n#EXTINF:4.0,\nseg_00000.ts\n#EXTINF:4.0,\nseg_00001.ts?old=1\n"
	got := appendQueryToHLSSegments(in, "token=abc")
	if !strings.Contains(got, "seg_00000.ts?token=abc") {
		t.Fatalf("missing tokenized segment: %q", got)
	}
	if !strings.Contains(got, "seg_00001.ts?old=1") {
		t.Fatalf("existing query should be preserved: %q", got)
	}
}
