package service

import (
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"go.uber.org/zap"

	"github.com/ShukeBta/MediaStationGo/internal/model"
	"github.com/ShukeBta/MediaStationGo/internal/repository"
)

func TestSiteUpdateKeepsSecretsWhenPatchIsBlank(t *testing.T) {
	db := newServiceTestDB(t, &model.Site{})
	svc := NewSiteService(zap.NewNop(), &repository.Container{DB: db}, "")
	site := &model.Site{
		Name:     "M-Team",
		Type:     "mteam",
		URL:      "https://api.m-team.cc",
		AuthType: "api_key",
		APIKey:   "token-123",
		Enabled:  true,
	}
	if err := svc.Create(context.Background(), site); err != nil {
		t.Fatal(err)
	}

	if err := svc.Update(context.Background(), site.ID, map[string]any{
		"url":     "https://api.m-team.cc/",
		"api_key": "",
		"cookie":  "",
	}); err != nil {
		t.Fatal(err)
	}

	got, err := svc.FindByID(context.Background(), site.ID)
	if err != nil {
		t.Fatal(err)
	}
	if got.APIKey != "token-123" {
		t.Fatalf("APIKey = %q, want original token", got.APIKey)
	}
	if got.URL != "https://api.m-team.cc" {
		t.Fatalf("URL = %q, want trimmed URL", got.URL)
	}
}

func TestSitePortalRateLimitProtectsMTeamByDefault(t *testing.T) {
	mteam := model.Site{Type: "mteam"}
	if got := sitePortalMinInterval(mteam); got < 3*time.Second {
		t.Fatalf("mteam min interval = %s, want conservative throttle", got)
	}

	plain := model.Site{Type: "nexusphp"}
	if got := sitePortalMinInterval(plain); got != 0 {
		t.Fatalf("plain site min interval = %s, want no throttle unless enabled", got)
	}

	limited := model.Site{Type: "nexusphp", RateLimit: true}
	if got := sitePortalMinInterval(limited); got <= 0 {
		t.Fatalf("rate-limited site min interval = %s, want throttle", got)
	}
}

func TestSitePortalRateLimitErrorMatchesMTeamMessage(t *testing.T) {
	if !isSitePortalRateLimitError(errors.New("mteam: 請求過於頻繁")) {
		t.Fatal("traditional Chinese M-Team rate limit message should be detected")
	}
	if !isSitePortalRateLimitError(errors.New("browse failed: status 429")) {
		t.Fatal("HTTP 429 should be detected")
	}
	if isSitePortalRateLimitError(errors.New("authentication failed")) {
		t.Fatal("unrelated errors should not be treated as rate limits")
	}
}

func TestYemaPTTestConnectionDoesNotFallbackAfterAuthFailure(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"success":false,"errorCode":403,"errorMessage":"need api auth"}`))
	}))
	defer server.Close()

	db := newServiceTestDB(t, &model.Site{})
	repos := repository.New(db)
	svc := NewSiteService(zap.NewNop(), repos, "")
	site := &model.Site{
		Name:     "YemaPT",
		Type:     "yemapt",
		URL:      server.URL,
		AuthType: "api_key",
		APIKey:   "bad-auth",
		Enabled:  true,
	}
	if err := svc.Create(context.Background(), site); err != nil {
		t.Fatal(err)
	}

	ok, msg, err := svc.TestConnection(context.Background(), site.ID)
	if err != nil {
		t.Fatal(err)
	}
	if ok {
		t.Fatal("TestConnection succeeded after YemaPT auth failure")
	}
	if !strings.Contains(msg, "need api auth") {
		t.Fatalf("message = %q, want need api auth", msg)
	}
}

func TestRedactSensitiveDownloadURL(t *testing.T) {
	tests := []struct {
		name string
		raw  string
		want string
	}{
		{
			name: "query secrets",
			raw:  "https://pt.example/download.php?id=123&passkey=secret#frag",
			want: "https://pt.example/download.php",
		},
		{
			name: "magnet",
			raw:  "magnet:?xt=urn:btih:abc&dn=movie",
			want: "magnet:?xt=***",
		},
		{
			name: "invalid",
			raw:  "not a url",
			want: "[redacted-download-url]",
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := redactSensitiveDownloadURL(tt.raw); got != tt.want {
				t.Fatalf("redactSensitiveDownloadURL() = %q, want %q", got, tt.want)
			}
		})
	}
}

func TestSiteSearchReturnsErrorWhenAllEnabledSitesFail(t *testing.T) {
	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		http.Error(w, "upstream timeout simulation", http.StatusGatewayTimeout)
	}))
	defer upstream.Close()

	db := newServiceTestDB(t, &model.Site{})
	repos := repository.New(db)
	svc := NewSiteService(zap.NewNop(), repos, "")
	site := &model.Site{
		Name:     "馒头",
		Type:     "mteam",
		URL:      upstream.URL,
		AuthType: "api_key",
		APIKey:   "token-123",
		Enabled:  true,
		Timeout:  5,
	}
	if err := svc.Create(context.Background(), site); err != nil {
		t.Fatal(err)
	}

	results, err := svc.Search(context.Background(), "南部档案 2026")
	if err == nil {
		t.Fatalf("Search error = nil, want all-sites-failed error; results=%#v", results)
	}
	if len(results) != 0 {
		t.Fatalf("results = %#v, want none on all-sites failure", results)
	}
	if !strings.Contains(err.Error(), "all enabled sites failed") || !strings.Contains(err.Error(), "馒头") {
		t.Fatalf("error = %q, want site failure context", err.Error())
	}
}

func TestSearchSiteQueriesSelectedSiteEvenWhenDisabled(t *testing.T) {
	var gotQuery string
	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotQuery = r.URL.RawQuery
		_, _ = w.Write([]byte(`<table><tr><td><a href="details.php?id=321" title="Selected Site Result">Selected Site Result</a><a href="download.php?id=321">下载</a></td></tr></table>`))
	}))
	defer upstream.Close()

	db := newServiceTestDB(t, &model.Site{})
	repos := repository.New(db)
	svc := NewSiteService(zap.NewNop(), repos, "")
	site := &model.Site{
		Name:     "Selected Nexus",
		Type:     "nexusphp",
		URL:      upstream.URL,
		AuthType: "cookie",
		Cookie:   "uid=1; pass=token",
		Enabled:  false,
		Timeout:  5,
	}
	if err := svc.Create(context.Background(), site); err != nil {
		t.Fatal(err)
	}

	results, err := svc.SearchSite(context.Background(), site.ID, "Selected", 1)
	if err != nil {
		t.Fatalf("SearchSite returned error: %v", err)
	}
	if !strings.Contains(gotQuery, "searchstr=Selected") {
		t.Fatalf("query = %q, want searchstr=Selected", gotQuery)
	}
	if len(results) != 1 || results[0].SiteID != site.ID || results[0].Title != "Selected Site Result" {
		t.Fatalf("results = %#v", results)
	}
}
