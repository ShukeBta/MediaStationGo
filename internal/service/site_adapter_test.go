package service

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"github.com/ShukeBta/MediaStationGo/internal/model"
	"github.com/ShukeBta/MediaStationGo/internal/repository"
)

func TestMTeamAuthenticateRequiresAPIKey(t *testing.T) {
	adapter := NewMTeamAdapter()
	err := adapter.Authenticate(context.Background(), SiteConfig{
		URL:      "https://api.m-team.cc",
		AuthType: "api_key",
	})
	if err == nil || !strings.Contains(err.Error(), "API Access Token") {
		t.Fatalf("Authenticate error = %v, want API Access Token hint", err)
	}
}

func TestMTeamAuthenticateUsesOpenAPIKeyHeader(t *testing.T) {
	var gotPath string
	var gotKey string
	var gotPayload map[string]any
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotPath = r.URL.Path
		gotKey = r.Header.Get("x-api-key")
		if err := json.NewDecoder(r.Body).Decode(&gotPayload); err != nil {
			t.Fatalf("decode request body: %v", err)
		}
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"code":"0","message":"SUCCESS","data":{"total":"0","data":[]}}`))
	}))
	defer server.Close()

	adapter := NewMTeamAdapter()
	err := adapter.Authenticate(context.Background(), SiteConfig{
		URL:      server.URL,
		AuthType: "api_key",
		APIKey:   "token-123",
		Timeout:  5 * time.Second,
	})
	if err != nil {
		t.Fatalf("Authenticate returned error: %v", err)
	}
	if gotPath != "/api/torrent/search" {
		t.Fatalf("path = %q, want /api/torrent/search", gotPath)
	}
	if gotKey != "token-123" {
		t.Fatalf("x-api-key = %q, want token-123", gotKey)
	}
	if gotPayload["mode"] != "all" || gotPayload["keyword"] != nil {
		t.Fatalf("payload = %#v, want mode all without keyword probe", gotPayload)
	}
}

func TestMTeamAuthenticateReportsAPIMessage(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"code":1,"message":"key無效","data":null}`))
	}))
	defer server.Close()

	adapter := NewMTeamAdapter()
	err := adapter.Authenticate(context.Background(), SiteConfig{
		URL:      server.URL,
		AuthType: "api_key",
		APIKey:   "bad-token",
		Timeout:  5 * time.Second,
	})
	if err == nil || !strings.Contains(err.Error(), "key無效") {
		t.Fatalf("Authenticate error = %v, want key invalid message", err)
	}
}

func TestMTeamAuthenticateHonorsConfiguredTimeout(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		time.Sleep(2 * time.Second)
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"code":"0","message":"SUCCESS","data":{"total":"0","data":[]}}`))
	}))
	defer server.Close()

	adapter := NewMTeamAdapter()
	started := time.Now()
	err := adapter.Authenticate(context.Background(), SiteConfig{
		URL:      server.URL,
		AuthType: "api_key",
		APIKey:   "token-123",
		Timeout:  time.Second,
	})
	if err == nil {
		t.Fatal("Authenticate error = nil, want timeout")
	}
	if elapsed := time.Since(started); elapsed >= 1500*time.Millisecond {
		t.Fatalf("Authenticate elapsed = %s, want configured timeout to stop before upstream response", elapsed)
	}
	if !strings.Contains(err.Error(), "M-Team API request timed out") {
		t.Fatalf("Authenticate error = %v, want M-Team timeout hint", err)
	}
}

func TestAPISiteDefaultTimeoutIsRaised(t *testing.T) {
	if got := siteRequestTimeout("mteam", 15); got != 45*time.Second {
		t.Fatalf("mteam timeout = %s, want 45s", got)
	}
	if got := siteRequestTimeout("yemapt", 0); got != 45*time.Second {
		t.Fatalf("yemapt timeout = %s, want 45s", got)
	}
	if got := siteRequestTimeout("nexusphp", 15); got != 15*time.Second {
		t.Fatalf("nexusphp timeout = %s, want 15s", got)
	}
	if got := siteRequestTimeout("mteam", 60); got != 60*time.Second {
		t.Fatalf("custom mteam timeout = %s, want 60s", got)
	}
}

func TestYemaPTAuthenticateUsesAuthorizationHeader(t *testing.T) {
	var gotPath string
	var gotAuth string
	var gotXAPIKey string
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotPath = r.URL.Path
		gotAuth = r.Header.Get("Authorization")
		gotXAPIKey = r.Header.Get("x-api-key")
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"success":true,"showType":0,"data":{"id":10,"name":"tester"}}`))
	}))
	defer server.Close()

	adapter := NewYemaPTAdapter()
	err := adapter.Authenticate(context.Background(), SiteConfig{
		Type:     "yemapt",
		URL:      server.URL,
		AuthType: "api_key",
		APIKey:   "auth-123",
		Timeout:  5 * time.Second,
	})
	if err != nil {
		t.Fatalf("Authenticate returned error: %v", err)
	}
	if gotPath != "/openApi/user/fetchBasicInfo.json" {
		t.Fatalf("path = %q, want /openApi/user/fetchBasicInfo.json", gotPath)
	}
	if gotAuth != "auth-123" {
		t.Fatalf("Authorization = %q, want auth-123", gotAuth)
	}
	if gotXAPIKey != "" {
		t.Fatalf("x-api-key = %q, want empty", gotXAPIKey)
	}
}

func TestYemaPTAuthenticateReportsAPIMessage(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"success":false,"errorCode":403,"errorMessage":"need api auth"}`))
	}))
	defer server.Close()

	adapter := NewYemaPTAdapter()
	err := adapter.Authenticate(context.Background(), SiteConfig{
		Type:     "yemapt",
		URL:      server.URL,
		AuthType: "api_key",
		APIKey:   "bad-auth",
		Timeout:  5 * time.Second,
	})
	if err == nil || !strings.Contains(err.Error(), "need api auth") {
		t.Fatalf("Authenticate error = %v, want need api auth", err)
	}
}

func TestNewSiteAdapterDetectsYemaPTURL(t *testing.T) {
	adapter := NewSiteAdapter(&model.Site{
		Type: "nexusphp",
		URL:  "https://www.yemapt.org",
	})
	if _, ok := adapter.(*YemaPTAdapter); !ok {
		t.Fatalf("adapter = %T, want *YemaPTAdapter", adapter)
	}
}

func TestBuildRequestAPIKeyHeaderBySite(t *testing.T) {
	yemaReq, err := buildRequest(context.Background(), http.MethodGet, "https://www.yemapt.org/openApi/user/fetchBasicInfo.json", SiteConfig{
		Type:     "yemapt",
		URL:      "https://www.yemapt.org",
		AuthType: "api_key",
		APIKey:   "yema-auth",
	}, nil)
	if err != nil {
		t.Fatal(err)
	}
	if got := yemaReq.Header.Get("Authorization"); got != "yema-auth" {
		t.Fatalf("YemaPT Authorization = %q, want yema-auth", got)
	}
	if got := yemaReq.Header.Get("x-api-key"); got != "" {
		t.Fatalf("YemaPT x-api-key = %q, want empty", got)
	}

	mteamReq, err := buildRequest(context.Background(), http.MethodGet, "https://api.m-team.cc/api/torrent/search", SiteConfig{
		Type:     "mteam",
		URL:      "https://api.m-team.cc",
		AuthType: "api_key",
		APIKey:   "mteam-auth",
	}, nil)
	if err != nil {
		t.Fatal(err)
	}
	if got := mteamReq.Header.Get("x-api-key"); got != "mteam-auth" {
		t.Fatalf("M-Team x-api-key = %q, want mteam-auth", got)
	}
	if got := mteamReq.Header.Get("Authorization"); got != "" {
		t.Fatalf("M-Team Authorization = %q, want empty", got)
	}
}

func TestNexusPHPSearchUsesSearchstr(t *testing.T) {
	var gotQuery string
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotQuery = r.URL.RawQuery
		_, _ = w.Write([]byte(`<table class="torrents"><tr><td><a href="details.php?id=123" title="测试资源">测试资源</a></td><td><a href="download.php?id=123">下载</a></td></tr></table>`))
	}))
	defer server.Close()

	adapter := NewNexusPHPAdapter()
	result, err := adapter.Search(t.Context(), SiteConfig{
		Name:     "Nexus",
		URL:      server.URL,
		AuthType: "cookie",
		Cookie:   "uid=1; pass=token",
		Timeout:  5 * time.Second,
	}, "测试", 2)
	if err != nil {
		t.Fatalf("Search returned error: %v", err)
	}
	values, err := url.ParseQuery(gotQuery)
	if err != nil {
		t.Fatal(err)
	}
	if values.Get("searchstr") != "测试" || values.Get("search") != "测试" || values.Get("page") != "2" {
		t.Fatalf("query = %q", gotQuery)
	}
	if len(result.Items) != 1 || result.Items[0].Title != "测试资源" {
		t.Fatalf("items = %#v", result.Items)
	}
}

func TestNexusPHPSearchReportsExpiredCookieLoginPage(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_, _ = w.Write([]byte(`<html><form id="loginform" action="takelogin.php"><input type="password" /></form></html>`))
	}))
	defer server.Close()

	adapter := NewNexusPHPAdapter()
	_, err := adapter.Search(t.Context(), SiteConfig{
		Name:     "Nexus",
		URL:      server.URL,
		AuthType: "cookie",
		Cookie:   "uid=1; pass=expired",
		Timeout:  5 * time.Second,
	}, "测试", 1)
	if err == nil || !strings.Contains(err.Error(), "cookie expired") {
		t.Fatalf("Search error = %v, want cookie expired hint", err)
	}
}

func TestParseNexusPHPHTMLModernRows(t *testing.T) {
	page := `
<table class="torrents">
  <tr class="torrent">
    <td class="cat"><a href="torrents.php?cat=401" title="电影">电影</a></td>
    <td>
      <a class="torrent-title" href="/details.php?id=456&hit=1" title="Some &amp; Movie 2026 2160p">ignored</a>
      <span class="subtitle">副标题 &amp; 描述</span>
      <a href="/download.php?id=456&passkey=abc">下载</a>
    </td>
    <td>12.5 GiB</td>
    <td class="seeders"><a>33</a></td>
    <td class="leechers"><span>4</span></td>
    <td class="snatched">99</td>
  </tr>
</table>`
	result, err := parseNexusPHPHTML(page, "Nexus", "https://pt.example")
	if err != nil {
		t.Fatal(err)
	}
	if len(result.Items) != 1 {
		t.Fatalf("items = %#v", result.Items)
	}
	item := result.Items[0]
	if item.ID != "456" || item.Title != "Some & Movie 2026 2160p" || item.Subtitle != "副标题 & 描述" {
		t.Fatalf("parsed item = %#v", item)
	}
	if item.DetailURL != "https://pt.example/details.php?id=456&hit=1" || item.DownloadURL != "https://pt.example/download.php?id=456&passkey=abc" {
		t.Fatalf("urls = detail %q download %q", item.DetailURL, item.DownloadURL)
	}
	if item.Seeders != 33 || item.Leechers != 4 || item.Snatched != 99 {
		t.Fatalf("stats = %#v", item)
	}
}

func TestParseNexusPHPHTMLCapturesRiskAndPromotionLabels(t *testing.T) {
	page := `
<table class="torrents">
  <tr class="torrent">
    <td><a href="/details.php?id=456" title="Some Show S01E01 1080p WEB-DL">Some Show</a></td>
    <td><img class="pro_free" alt="免费" /><span title="HR">H&R</span></td>
    <td><a href="/download.php?id=456">下载</a></td>
  </tr>
</table>`
	result, err := parseNexusPHPHTML(page, "Nexus", "https://pt.example")
	if err != nil {
		t.Fatal(err)
	}
	if len(result.Items) != 1 {
		t.Fatalf("items = %#v", result.Items)
	}
	item := result.Items[0]
	if !item.Free {
		t.Fatalf("item.Free = false, want free promotion detected: %#v", item)
	}
	if !strings.Contains(item.Labels, "HR") || !strings.Contains(item.Labels, "free") {
		t.Fatalf("labels = %q, want HR and free", item.Labels)
	}
}

func TestParseNexusPHPHTMLIgnoresUserDetailsLinks(t *testing.T) {
	page := `
<table>
  <tr><td><a href="userdetails.php?id=31044">shukBeta</a></td><td>15.5 GiB</td></tr>
  <tr><td><a href="/details.php?id=789" title="问心 S01 1080p">问心</a><a href="/download.php?id=789">下载</a></td><td>1.5 GiB</td></tr>
</table>`
	result, err := parseNexusPHPHTML(page, "Nexus", "https://pt.example")
	if err != nil {
		t.Fatal(err)
	}
	if len(result.Items) != 1 {
		t.Fatalf("items = %#v, want only real torrent details row", result.Items)
	}
	if result.Items[0].ID != "789" || result.Items[0].Title != "问心 S01 1080p" {
		t.Fatalf("parsed item = %#v", result.Items[0])
	}
}

func TestMTeamAPIRateLimits(t *testing.T) {
	search := mteamAPIRateLimits(mteamAPIEndpointSearch)
	if len(search) != 1 || search[0].Limit != 1500 || search[0].Window != 24*time.Hour {
		t.Fatalf("search limits = %#v, want 1500/24h", search)
	}
	detail := mteamAPIRateLimits(mteamAPIEndpointDetail)
	if len(detail) != 1 || detail[0].Limit != 100 || detail[0].Window != time.Hour {
		t.Fatalf("detail limits = %#v, want 100/1h", detail)
	}
	download := mteamAPIRateLimits(mteamAPIEndpointDownload)
	if len(download) != 2 ||
		download[0].Limit != 100 || download[0].Window != time.Hour ||
		download[1].Limit != 1000 || download[1].Window != 24*time.Hour {
		t.Fatalf("download limits = %#v, want 100/1h and 1000/24h", download)
	}
}

func TestPersistentSiteAPIRateLimiterPersistsSlidingWindow(t *testing.T) {
	db := newServiceTestDB(t, &model.Setting{})
	repos := repository.New(db)
	now := time.Date(2026, 6, 20, 12, 0, 0, 0, time.UTC)
	limiter := newPersistentSiteAPIRateLimiter(repos)
	limiter.now = func() time.Time { return now }
	limit := siteAPIRateLimit{Bucket: "test_1h", Limit: 2, Window: time.Hour}

	if err := limiter.Allow(t.Context(), "mteam:test", limit); err != nil {
		t.Fatalf("first allow: %v", err)
	}
	if err := limiter.Allow(t.Context(), "mteam:test", limit); err != nil {
		t.Fatalf("second allow: %v", err)
	}
	err := limiter.Allow(t.Context(), "mteam:test", limit)
	var limited *siteAPIRateLimitError
	if !errors.As(err, &limited) {
		t.Fatalf("third allow error = %v, want siteAPIRateLimitError", err)
	}
	if limited.RetryAfter != time.Hour {
		t.Fatalf("retry_after = %v, want 1h", limited.RetryAfter)
	}

	restarted := newPersistentSiteAPIRateLimiter(repos)
	restarted.now = func() time.Time { return now.Add(30 * time.Minute) }
	if err := restarted.Allow(t.Context(), "mteam:test", limit); !errors.As(err, &limited) {
		t.Fatalf("restarted allow error = %v, want persisted limit", err)
	}

	restarted.now = func() time.Time { return now.Add(time.Hour + time.Second) }
	if err := restarted.Allow(t.Context(), "mteam:test", limit); err != nil {
		t.Fatalf("allow after window: %v", err)
	}
}

func TestMTeamRateLimitStopsRequestBeforeHTTP(t *testing.T) {
	var requests atomic.Int32
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		requests.Add(1)
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"code":"0","message":"SUCCESS","data":{"total":"0","data":[]}}`))
	}))
	defer server.Close()

	adapter := NewMTeamAdapter()
	limiter := &staticSiteAPIRateLimiter{err: &siteAPIRateLimitError{
		Bucket:     "torrent_search_24h",
		Limit:      1000,
		Window:     24 * time.Hour,
		RetryAfter: time.Hour,
	}}
	_, err := adapter.Search(t.Context(), SiteConfig{
		URL:         server.URL,
		AuthType:    "api_key",
		APIKey:      "token-123",
		Timeout:     5 * time.Second,
		rateLimiter: limiter,
	}, "show", 1)
	if err == nil || !strings.Contains(err.Error(), "rate limit") {
		t.Fatalf("Search error = %v, want rate limit", err)
	}
	if got := requests.Load(); got != 0 {
		t.Fatalf("HTTP requests = %d, want 0", got)
	}
	if limiter.calls != 1 {
		t.Fatalf("limiter calls = %d, want 1", limiter.calls)
	}
}

type staticSiteAPIRateLimiter struct {
	err   error
	calls int
}

func (l *staticSiteAPIRateLimiter) Allow(context.Context, string, ...siteAPIRateLimit) error {
	l.calls++
	return l.err
}
