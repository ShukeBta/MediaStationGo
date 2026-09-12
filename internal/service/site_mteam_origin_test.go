package service

import (
	"io"
	"net/http"
	"strings"
	"testing"
	"time"
)

func TestMTeamAPIOrigin(t *testing.T) {
	for _, tt := range []struct {
		url, override, want string
	}{
		{"https://kp.m-team.cc/", "", defaultMTeamAPIBase},
		{"https://m-team.cc/torrents", "", defaultMTeamAPIBase},
		{"https://kp.m-team.io", "", defaultMTeamAPIBase},
		{"https://api.m-team.cc/api/", "", defaultMTeamAPIBase},
		{"http://127.0.0.1:1234", "", "http://127.0.0.1:1234"},
		{"https://proxy.example.test/mteam/api/", "", "https://proxy.example.test/mteam"},
		{"https://kp.m-team.cc", "https://proxy.example.test/mteam/api/", "https://proxy.example.test/mteam"},
	} {
		cfg := SiteConfig{URL: tt.url, Extra: map[string]string{"api_url": tt.override}}
		if got := mteamAPIBaseURL(cfg); got != tt.want {
			t.Errorf("origin for %#v = %q, want %q", tt, got, tt.want)
		}
	}
	front := SiteConfig{URL: "https://kp.m-team.cc", APIKey: "same-account"}
	api := SiteConfig{URL: defaultMTeamAPIBase, APIKey: "same-account"}
	if mteamAPIRateSiteKey(front) != mteamAPIRateSiteKey(api) {
		t.Fatal("website and API aliases must share the account quota")
	}
}

func TestMTeamRequestsUseAPIOriginAndKeepWebsiteLinks(t *testing.T) {
	for _, override := range []string{"", "https://proxy.example.test/mteam/api/"} {
		t.Run(override, func(t *testing.T) {
			cfg := SiteConfig{URL: "https://kp.m-team.cc", Type: "mteam", AuthType: "api_key", APIKey: "test-token", Extra: map[string]string{"api_url": override}, Timeout: 30 * time.Second}
			base := defaultMTeamAPIBase
			prefix := ""
			if override != "" {
				base, prefix = "https://proxy.example.test", "/mteam"
			}
			requests := 0
			adapter := NewMTeamAdapter()
			adapter.client.Transport = roundTripFunc(func(r *http.Request) (*http.Response, error) {
				requests++
				if r.URL.Scheme+"://"+r.URL.Host != base || r.Header.Get("x-api-key") != "test-token" {
					t.Errorf("wrong API origin or authentication: %s", r.URL.Host)
				}
				body := `{"code":"0","data":{"total":1,"data":[{"id":"42","name":"Movie"}]}}`
				switch r.URL.Path {
				case prefix + "/api/torrent/search":
				case prefix + "/api/torrent/detail":
					body = `{"code":"0","data":{"name":"Movie"}}`
				case prefix + "/api/torrent/genDlToken":
					if r.URL.Query().Get("id") != "42&mode=bad" || r.URL.Query().Has("mode") {
						t.Errorf("download ID was not escaped: %s", r.URL.RawQuery)
					}
					body = `{"code":"0","data":"https://download.example.test/torrent"}`
				default:
					t.Errorf("wrong API path: %s", r.URL.Path)
				}
				return &http.Response{StatusCode: 200, Header: make(http.Header), Body: io.NopCloser(strings.NewReader(body))}, nil
			})
			if err := adapter.Authenticate(t.Context(), cfg); err != nil {
				t.Fatal(err)
			}
			search, err := adapter.Search(t.Context(), cfg, "Movie", 1)
			if err != nil || len(search.Items) != 1 || search.Items[0].DetailURL != cfg.URL+"/detail/42" {
				t.Fatalf("search = %#v, %v", search, err)
			}
			if _, err := adapter.Browse(t.Context(), cfg, "movie", 1); err != nil {
				t.Fatal(err)
			}
			detail, err := adapter.GetDetail(t.Context(), cfg, "42")
			if err != nil || detail.DetailURL != cfg.URL+"/detail/42" {
				t.Fatalf("detail = %#v, %v", detail, err)
			}
			if _, err := adapter.GetDownloadURL(t.Context(), cfg, "42&mode=bad"); err != nil {
				t.Fatal(err)
			}
			if requests != 5 {
				t.Fatalf("requests = %d, want all 5 API operations", requests)
			}
		})
	}
}
