package cloud

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func TestOpenListResolveUsesAPIRawURLFor302Playback(t *testing.T) {
	var gotPath, gotAuth string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotPath = r.URL.Path
		gotAuth = r.Header.Get("Authorization")
		if r.Method != http.MethodPost || r.URL.Path != "/api/fs/get" {
			t.Fatalf("unexpected request %s %s", r.Method, r.URL.Path)
		}
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"code":200,"data":{"raw_url":"https://cdn.example.test/movie.mkv?sign=1"}}`))
	}))
	defer srv.Close()

	p, err := New(TypeOpenList, map[string]any{"server": srv.URL, "token": "alist-token"}, srv.Client())
	if err != nil {
		t.Fatal(err)
	}
	link, err := p.Resolve(context.Background(), "/Cloud/Movie.mkv")
	if err != nil {
		t.Fatalf("resolve: %v", err)
	}
	if gotPath != "/api/fs/get" {
		t.Fatalf("api path = %q, want /api/fs/get", gotPath)
	}
	if gotAuth != "alist-token" {
		t.Fatalf("Authorization = %q, want token", gotAuth)
	}
	if link.URL != "https://cdn.example.test/movie.mkv?sign=1" {
		t.Fatalf("url = %q", link.URL)
	}
	if link.Proxy {
		t.Fatalf("openlist raw_url without required headers should be 302 playback")
	}
}

func TestOpenListResolveWMVUsesAPIRawURLFor302Playback(t *testing.T) {
	var gotPath, gotAuth string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		switch r.URL.Path {
		case "/api/fs/list":
			_, _ = w.Write([]byte(`{"code":200,"data":{"content":[],"total":0}}`))
		case "/api/fs/get":
			gotPath = r.URL.Path
			gotAuth = r.Header.Get("Authorization")
			_, _ = w.Write([]byte(`{"code":200,"data":{"raw_url":"https://cdn.example.test/movie.wmv?sign=1"}}`))
		default:
			t.Fatalf("unexpected request %s %s", r.Method, r.URL.Path)
		}
	}))
	defer srv.Close()

	p, err := New(TypeOpenList, map[string]any{"server": srv.URL, "token": "alist-token"}, srv.Client())
	if err != nil {
		t.Fatal(err)
	}
	link, err := p.Resolve(context.Background(), "/Cloud/Movie.wmv")
	if err != nil {
		t.Fatalf("resolve: %v", err)
	}
	if gotPath != "/api/fs/get" {
		t.Fatalf("api path = %q, want /api/fs/get", gotPath)
	}
	if gotAuth != "alist-token" {
		t.Fatalf("Authorization = %q, want token", gotAuth)
	}
	if link.Proxy || link.URL != "https://cdn.example.test/movie.wmv?sign=1" {
		t.Fatalf("link = %#v, want direct WMV CDN URL", link)
	}
}

func TestOpenListResolveCollapsesHostedRawURLRedirectToCDN(t *testing.T) {
	var probeSeen bool
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/api/fs/get":
			w.Header().Set("Content-Type", "application/json")
			_, _ = w.Write([]byte(`{"code":200,"data":{"raw_url":"/d/Cloud/Movie.mkv?sign=1"}}`))
		case "/d/Cloud/Movie.mkv":
			probeSeen = true
			if r.Header.Get("Range") != "bytes=0-0" {
				t.Fatalf("probe Range = %q", r.Header.Get("Range"))
			}
			http.Redirect(w, r, "https://cdn.example.test/movie.mkv?sign=cdn", http.StatusFound)
		default:
			t.Fatalf("unexpected path %s", r.URL.Path)
		}
	}))
	defer srv.Close()

	p, err := New(TypeOpenList, map[string]any{"server": srv.URL, "token": "alist-token"}, srv.Client())
	if err != nil {
		t.Fatal(err)
	}
	link, err := p.Resolve(context.Background(), "/Cloud/Movie.mkv")
	if err != nil {
		t.Fatalf("resolve: %v", err)
	}
	if !probeSeen {
		t.Fatal("expected OpenList-hosted raw_url probe")
	}
	if link.URL != "https://cdn.example.test/movie.mkv?sign=cdn" || link.Proxy || len(link.Headers) != 0 {
		t.Fatalf("link = %#v, want collapsed CDN 302 playback", link)
	}
}

func TestOpenListResolveLogsInWithUsernamePasswordForAPIRawURL(t *testing.T) {
	var loginSeen bool
	var gotAuth string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		switch r.URL.Path {
		case "/api/auth/login":
			loginSeen = true
			var body map[string]string
			if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
				t.Fatalf("decode login body: %v", err)
			}
			if body["username"] != "alice" || body["password"] != "secret" {
				t.Fatalf("login body = %#v", body)
			}
			_, _ = w.Write([]byte(`{"code":200,"data":{"token":"api-token"}}`))
		case "/api/fs/get":
			gotAuth = r.Header.Get("Authorization")
			_, _ = w.Write([]byte(`{"code":200,"data":{"raw_url":"https://cdn.example.test/movie.mkv?sign=1"}}`))
		default:
			t.Fatalf("unexpected path %s", r.URL.Path)
		}
	}))
	defer srv.Close()

	p, err := New(TypeOpenList, map[string]any{"server": srv.URL, "username": "alice", "password": "secret"}, srv.Client())
	if err != nil {
		t.Fatal(err)
	}
	link, err := p.Resolve(context.Background(), "/Cloud/Movie.mkv")
	if err != nil {
		t.Fatalf("resolve: %v", err)
	}
	if !loginSeen {
		t.Fatalf("expected api login before fs/get")
	}
	if gotAuth != "api-token" {
		t.Fatalf("Authorization = %q, want api-token", gotAuth)
	}
	if link.URL != "https://cdn.example.test/movie.mkv?sign=1" || link.Proxy {
		t.Fatalf("link = %#v, want raw_url 302 playback", link)
	}
}

func TestOpenListResolveFallsBackToProxyWhenAPIRawURLNeedsHeaders(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/api/fs/get" {
			t.Fatalf("unexpected path %s", r.URL.Path)
		}
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"code":200,"data":{"raw_url":"/dav/Cloud/Movie.mkv","header":{"Cookie":"sid=abc"}}}`))
	}))
	defer srv.Close()

	p, err := New(TypeOpenList, map[string]any{"server": srv.URL, "token": "alist-token"}, srv.Client())
	if err != nil {
		t.Fatal(err)
	}
	link, err := p.Resolve(context.Background(), "/Cloud/Movie.mkv")
	if err != nil {
		t.Fatalf("resolve error = %v, want WebDAV proxy fallback", err)
	}
	if link == nil || !link.Proxy || !strings.Contains(link.URL, "/dav/Cloud/Movie.mkv") {
		t.Fatalf("link = %#v, want proxy WebDAV fallback link", link)
	}
}

func TestOpenListResolveFallsBackToProxyWithoutCDNRedirect(t *testing.T) {
	var probeSeen bool
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/api/fs/get":
			w.Header().Set("Content-Type", "application/json")
			_, _ = w.Write([]byte(`{"code":200,"data":{"raw_url":"/d/Cloud/Movie.mkv?sign=1"}}`))
		case "/d/Cloud/Movie.mkv":
			probeSeen = true
			w.Header().Set("Content-Range", "bytes 0-0/10")
			w.WriteHeader(http.StatusPartialContent)
			_, _ = w.Write([]byte("x"))
		default:
			t.Fatalf("unexpected path %s", r.URL.Path)
		}
	}))
	defer srv.Close()

	p, err := New(TypeOpenList, map[string]any{"server": srv.URL, "token": "alist-token"}, srv.Client())
	if err != nil {
		t.Fatal(err)
	}
	link, err := p.Resolve(context.Background(), "/Cloud/Movie.mkv")
	if err != nil {
		t.Fatalf("resolve error = %v, want WebDAV proxy fallback", err)
	}
	if link == nil || !link.Proxy || !strings.Contains(link.URL, "/dav/Cloud/Movie.mkv") {
		t.Fatalf("link = %#v, want proxy WebDAV fallback link", link)
	}
	if !probeSeen {
		t.Fatal("expected OpenList-hosted raw_url probe")
	}
}

func TestOpenListResolveFallsBackToProxyWhenAPIRawURLFails(t *testing.T) {
	var davSeen bool
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/api/fs/get":
			w.Header().Set("Content-Type", "application/json")
			_, _ = w.Write([]byte(`{"code":500,"message":"driver cannot provide raw_url"}`))
		case "/dav/Cloud/Movie.mkv":
			davSeen = true
			w.WriteHeader(http.StatusOK)
		default:
			t.Fatalf("unexpected path %s", r.URL.Path)
		}
	}))
	defer srv.Close()

	p, err := New(TypeOpenList, map[string]any{"server": srv.URL, "token": "alist-token"}, srv.Client())
	if err != nil {
		t.Fatal(err)
	}
	link, err := p.Resolve(context.Background(), "/Cloud/Movie.mkv")
	if err != nil {
		t.Fatalf("resolve error = %v, want WebDAV proxy fallback", err)
	}
	if link == nil || !link.Proxy || !strings.Contains(link.URL, "/dav/Cloud/Movie.mkv") {
		t.Fatalf("link = %#v, want proxy WebDAV fallback link", link)
	}
	if davSeen {
		t.Fatal("resolve itself should not touch WebDAV; proxying happens at playback time")
	}
}
