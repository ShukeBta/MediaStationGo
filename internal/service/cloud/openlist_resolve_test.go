package cloud

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"
)

func TestOpenListResolveUsesAPIRawURLFor302Playback(t *testing.T) {
	var gotPath, gotAuth string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotPath = r.URL.Path
		gotAuth = r.Header.Get("Authorization")
		if r.Method != http.MethodPost {
			t.Fatalf("unexpected request %s %s", r.Method, r.URL.Path)
		}
		w.Header().Set("Content-Type", "application/json")
		switch r.URL.Path {
		case "/api/fs/list":
			_, _ = w.Write([]byte(`{"code":200,"data":{"content":[],"total":0}}`))
		case "/api/fs/get":
			_, _ = w.Write([]byte(`{"code":200,"data":{"raw_url":"https://cdn.example.test/movie.mkv?sign=1"}}`))
		default:
			t.Fatalf("unexpected request %s %s", r.Method, r.URL.Path)
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
		case "/api/fs/list":
			w.Header().Set("Content-Type", "application/json")
			_, _ = w.Write([]byte(`{"code":200,"data":{"content":[],"total":0}}`))
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
		case "/api/fs/list":
			if r.Header.Get("Authorization") != "api-token" {
				t.Fatalf("list Authorization = %q, want api-token", r.Header.Get("Authorization"))
			}
			_, _ = w.Write([]byte(`{"code":200,"data":{"content":[],"total":0}}`))
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
		w.Header().Set("Content-Type", "application/json")
		switch r.URL.Path {
		case "/api/fs/list":
			_, _ = w.Write([]byte(`{"code":200,"data":{"content":[],"total":0}}`))
		case "/api/fs/get":
			_, _ = w.Write([]byte(`{"code":200,"data":{"raw_url":"/dav/Cloud/Movie.mkv","header":{"Cookie":"sid=abc"}}}`))
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
}

func TestOpenListResolveFallsBackToProxyWithoutCDNRedirect(t *testing.T) {
	var probeSeen bool
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/api/fs/list":
			w.Header().Set("Content-Type", "application/json")
			_, _ = w.Write([]byte(`{"code":200,"data":{"content":[],"total":0}}`))
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
		case "/api/fs/list":
			w.Header().Set("Content-Type", "application/json")
			_, _ = w.Write([]byte(`{"code":200,"data":{"content":[],"total":0}}`))
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

func TestOpenListResolveListsExactParentAndRetriesOnObjectCacheMiss(t *testing.T) {
	var getCalls, listCalls int
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		switch r.URL.Path {
		case "/api/fs/get":
			getCalls++
			if getCalls == 1 {
				if listCalls != 0 {
					t.Fatal("target parent was warmed before the cache miss")
				}
				_, _ = w.Write([]byte(`{"code":500,"message":"failed to get obj: code: 430004, message: file not found"}`))
				return
			}
			if listCalls != 1 {
				t.Fatal("target parent was not warmed after the cache miss")
			}
			_, _ = w.Write([]byte(`{"code":200,"data":{"raw_url":"https://cdn.example.test/movie.mkv?sign=1"}}`))
		case "/api/fs/list":
			listCalls++
			var body map[string]any
			if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
				t.Fatalf("decode list body: %v", err)
			}
			if body["path"] != "/115/电影/Movie" || body["refresh"] != false {
				t.Fatalf("list body = %#v, want exact parent with refresh=false", body)
			}
			_, _ = w.Write([]byte(`{"code":200,"data":{"content":[],"total":0}}`))
		default:
			t.Fatalf("unexpected path %s", r.URL.Path)
		}
	}))
	defer srv.Close()

	p, err := New(TypeOpenList, map[string]any{"server": srv.URL, "token": "alist-token"}, srv.Client())
	if err != nil {
		t.Fatal(err)
	}
	link, err := p.Resolve(context.Background(), "/115/电影/Movie/Movie.mkv")
	if err != nil {
		t.Fatalf("resolve: %v", err)
	}
	if link.URL != "https://cdn.example.test/movie.mkv?sign=1" || getCalls != 2 || listCalls != 1 {
		t.Fatalf("link=%#v getCalls=%d listCalls=%d", link, getCalls, listCalls)
	}
}

func TestOpenListResolveDoesNotWarmParentWhenGetSucceeds(t *testing.T) {
	var getCalls, listCalls int
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		switch r.URL.Path {
		case "/api/fs/list":
			listCalls++
			_, _ = w.Write([]byte(`{"code":500,"message":"temporary list failure"}`))
		case "/api/fs/get":
			getCalls++
			_, _ = w.Write([]byte(`{"code":200,"data":{"raw_url":"https://cdn.example.test/movie.mkv?sign=1"}}`))
		default:
			t.Fatalf("unexpected path %s", r.URL.Path)
		}
	}))
	defer srv.Close()

	p, err := New(TypeOpenList, map[string]any{"server": srv.URL, "token": "alist-token"}, srv.Client())
	if err != nil {
		t.Fatal(err)
	}
	link, err := p.Resolve(context.Background(), "/115/Movies/Movie/Movie.mkv")
	if err != nil {
		t.Fatalf("resolve: %v", err)
	}
	if link.URL != "https://cdn.example.test/movie.mkv?sign=1" || listCalls != 0 || getCalls != 1 {
		t.Fatalf("link=%#v listCalls=%d getCalls=%d, want direct get without parent warmup", link, listCalls, getCalls)
	}
}

func TestOpenListResolveDoesNotWarmParentBeforeOtherAPIErrors(t *testing.T) {
	var listCalls int
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		if r.URL.Path == "/api/fs/list" {
			listCalls++
		}
		_, _ = w.Write([]byte(`{"code":500,"message":"driver unavailable"}`))
	}))
	defer srv.Close()

	p, err := New(TypeOpenList, map[string]any{"server": srv.URL, "token": "alist-token"}, srv.Client())
	if err != nil {
		t.Fatal(err)
	}
	_, err = p.Resolve(context.Background(), "/115/电影/Movie/Movie.mkv")
	if err == nil || listCalls != 0 {
		t.Fatalf("err=%v listCalls=%d, want no parent warmup", err, listCalls)
	}
}

func TestOpenListResolveDoesNotListRootForObjectCacheMiss(t *testing.T) {
	var listCalls int
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		if r.URL.Path == "/api/fs/list" {
			listCalls++
		}
		_, _ = w.Write([]byte(`{"code":500,"message":"failed to get obj: code: 430004, message: file not found"}`))
	}))
	defer srv.Close()

	p, err := New(TypeOpenList, map[string]any{"server": srv.URL, "token": "alist-token"}, srv.Client())
	if err != nil {
		t.Fatal(err)
	}
	_, err = p.Resolve(context.Background(), "/Movie.mkv")
	if err == nil || listCalls != 0 {
		t.Fatalf("err=%v listCalls=%d, want root cache miss without broad list", err, listCalls)
	}
}

func TestOpenListResolveCollapsesConcurrentParentWarmup(t *testing.T) {
	for _, delayedSecond := range []bool{false, true} {
		name := "concurrent-cache-misses"
		if delayedSecond {
			name = "second-client-after-warmup"
		}
		t.Run(name, func(t *testing.T) {
			var getCalls, coldGets, listCalls atomic.Int32
			var parentWarm atomic.Bool
			initialGetCount := int32(2)
			if delayedSecond {
				initialGetCount = 1
			}
			initialGetsStarted := make(chan struct{})
			listStarted := make(chan struct{})
			releaseList := make(chan struct{})
			var releaseOnce sync.Once
			srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				w.Header().Set("Content-Type", "application/json")
				switch r.URL.Path {
				case "/api/fs/get":
					getCalls.Add(1)
					if !parentWarm.Load() {
						// Both cold GETs must reach the server before either can
						// trigger the list. A retry is successful because the parent
						// was warmed, never because it happens to be the third GET.
						if coldGets.Add(1) == initialGetCount {
							close(initialGetsStarted)
						}
						select {
						case <-initialGetsStarted:
						case <-r.Context().Done():
							return
						}
						_, _ = w.Write([]byte(`{"code":500,"message":"failed to get obj: code: 430004, message: file not found"}`))
						return
					}
					_, _ = w.Write([]byte(`{"code":200,"data":{"raw_url":"https://cdn.example.test/movie.mkv?sign=1"}}`))
				case "/api/fs/list":
					if listCalls.Add(1) == 1 {
						close(listStarted)
					}
					select {
					case <-releaseList:
					case <-r.Context().Done():
						return
					}
					parentWarm.Store(true)
					_, _ = w.Write([]byte(`{"code":200,"data":{"content":[],"total":0}}`))
				default:
					t.Errorf("unexpected path %s", r.URL.Path)
					w.WriteHeader(http.StatusNotFound)
				}
			}))
			t.Cleanup(srv.Close)
			providers := make([]Provider, 2)
			for i := range providers {
				p, err := New(TypeOpenList, map[string]any{"server": srv.URL, "token": "alist-token"}, srv.Client())
				if err != nil {
					t.Fatal(err)
				}
				providers[i] = p
			}
			ctx, cancel := context.WithTimeout(t.Context(), 5*time.Second)
			errCh := make(chan error, len(providers))
			firstResolved := make(chan struct{})
			var wg sync.WaitGroup
			t.Cleanup(func() {
				cancel()
				releaseOnce.Do(func() { close(releaseList) })
				wg.Wait()
			})
			for i, provider := range providers {
				wg.Add(1)
				go func(p Provider, index int) {
					defer wg.Done()
					if index == 0 {
						defer close(firstResolved)
					}
					if delayedSecond && index == 1 {
						select {
						case <-firstResolved:
						case <-ctx.Done():
							errCh <- ctx.Err()
							return
						}
					}
					link, err := p.Resolve(ctx, "/115/电影/Movie/Movie.mkv")
					if err == nil && (link == nil || link.URL != "https://cdn.example.test/movie.mkv?sign=1") {
						t.Errorf("unexpected direct link: %#v", link)
					}
					errCh <- err
				}(provider, i)
			}
			select {
			case <-listStarted:
			case <-ctx.Done():
				t.Fatal("parent list did not start")
			}
			releaseOnce.Do(func() { close(releaseList) })
			wg.Wait()
			close(errCh)
			for err := range errCh {
				if err != nil {
					t.Fatalf("resolve: %v", err)
				}
			}
			if listCalls.Load() != 1 || coldGets.Load() != initialGetCount || getCalls.Load() != initialGetCount+2 {
				t.Fatalf("listCalls=%d coldGets=%d getCalls=%d, want 1/%d/%d", listCalls.Load(), coldGets.Load(), getCalls.Load(), initialGetCount, initialGetCount+2)
			}
		})
	}
}
