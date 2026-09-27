package service

import (
	"context"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"github.com/ShukeBta/MediaStationGo/internal/service/cloud"
)

type cloudResolveTestRoundTripper func(*http.Request) (*http.Response, error)

func (f cloudResolveTestRoundTripper) RoundTrip(r *http.Request) (*http.Response, error) {
	return f(r)
}

func TestCloudResolveHotCacheHitDoesNotRefresh(t *testing.T) {
	var resolves atomic.Int32
	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/api/fs/list" {
			writeOpenListCacheList(w)
			return
		}
		if r.URL.Path != "/api/fs/get" {
			t.Fatalf("unexpected path %s", r.URL.Path)
		}
		n := resolves.Add(1)
		w.Header().Set("Content-Type", "application/json")
		_, _ = fmt.Fprintf(w, `{"code":200,"data":{"raw_url":"http://cdn.local/%d.mkv"}}`, n)
	}))
	defer upstream.Close()

	_, storage := newStorageUploadTestService(t)
	if _, err := storage.Save(t.Context(), StorageInput{
		Type: "openlist",
		Config: map[string]any{
			"server": upstream.URL,
			"token":  "token",
		},
	}); err != nil {
		t.Fatal(err)
	}

	link, err := storage.CloudResolve(t.Context(), "openlist", "/Movies/f1.mkv", "Player/1")
	if err != nil {
		t.Fatal(err)
	}
	if link.URL != "http://cdn.local/1.mkv" || resolves.Load() != 1 {
		t.Fatalf("first resolve link=%#v resolves=%d", link, resolves.Load())
	}
	for i := 0; i < 3; i++ {
		link, err = storage.CloudResolve(t.Context(), "openlist", "/Movies/f1.mkv", "Player/1")
		if err != nil {
			t.Fatal(err)
		}
		if link.URL != "http://cdn.local/1.mkv" || resolves.Load() != 1 {
			t.Fatalf("cached resolve link=%#v resolves=%d", link, resolves.Load())
		}
	}

	key := storage.resolveCacheKey("openlist", "/Movies/f1.mkv", "Player/1")
	storage.resolveMu.Lock()
	entry := storage.resolveCache[key]
	entry.expiresAt = time.Now().Add(5 * time.Second)
	storage.resolveCache[key] = entry
	storage.resolveMu.Unlock()

	link, err = storage.CloudResolve(t.Context(), "openlist", "/Movies/f1.mkv", "Player/1")
	if err != nil {
		t.Fatal(err)
	}
	if link.URL != "http://cdn.local/1.mkv" {
		t.Fatalf("hot hit should return cached link, got %s", link.URL)
	}
	time.Sleep(50 * time.Millisecond)
	if resolves.Load() != 1 {
		t.Fatalf("hot cache hit triggered an unexpected refresh, resolves=%d", resolves.Load())
	}

	storage.resolveMu.Lock()
	entry = storage.resolveCache[key]
	entry.expiresAt = time.Now().Add(-time.Second)
	storage.resolveCache[key] = entry
	storage.resolveMu.Unlock()
	link, err = storage.CloudResolve(t.Context(), "openlist", "/Movies/f1.mkv", "Player/1")
	if err != nil {
		t.Fatal(err)
	}
	if link.URL != "http://cdn.local/2.mkv" {
		t.Fatalf("refreshed link = %s, want second URL", link.URL)
	}
}

func TestCloudResolveCacheTTLUsesShortTTLForCloudPlaybackLinks(t *testing.T) {
	for _, typ := range []string{"cloud115", "clouddrive2", "openlist"} {
		if got := cloudResolveCacheTTL(typ); got != 2*time.Minute {
			t.Fatalf("%s cloud resolve cache ttl = %v, want 2m", typ, got)
		}
	}
}

func TestCloudResolveCacheUses115DirectLinkExpiry(t *testing.T) {
	expires := time.Now().Add(time.Hour).Unix()
	link := &cloud.DirectLink{URL: fmt.Sprintf("https://cdnfhnfile.115cdn.net/hash/movie.mp4?t=%d&k=secret", expires)}
	ttl, staleTTL := cloudResolveCacheDurations("openlist", link)
	if ttl <= 2*time.Minute || ttl > 30*time.Minute {
		t.Fatalf("ttl = %v, want extended ttl within 30m", ttl)
	}
	if staleTTL != ttl {
		t.Fatalf("openlist stale ttl = %v, want %v", staleTTL, ttl)
	}
}

func TestOpenListCacheTTLRespects115ExpirySafetyMargin(t *testing.T) {
	nearExpiry := time.Now().Add(90 * time.Second).Unix()
	link := &cloud.DirectLink{URL: fmt.Sprintf("https://cdnfhnfile.115cdn.net/hash/movie.mp4?t=%d&k=secret", nearExpiry)}
	ttl, staleTTL := cloudResolveCacheDurations("openlist", link)
	if ttl <= 0 || ttl > 30*time.Second {
		t.Fatalf("ttl = %v, want a positive value no greater than the 30s safety window", ttl)
	}
	if staleTTL != ttl {
		t.Fatalf("openlist stale ttl = %v, want %v", staleTTL, ttl)
	}

	expiringNow := time.Now().Add(30 * time.Second).Unix()
	link.URL = fmt.Sprintf("https://cdnfhnfile.115cdn.net/hash/movie.mp4?t=%d&k=secret", expiringNow)
	ttl, staleTTL = cloudResolveCacheDurations("openlist", link)
	if ttl != 0 || staleTTL != 0 {
		t.Fatalf("near-expiry link cache ttl = (%v, %v), want (0, 0)", ttl, staleTTL)
	}
}

func TestCloudResolveCachePrunesLeastRecentlyUsedEntries(t *testing.T) {
	storage := &StorageConfigService{resolveCache: make(map[string]cloudResolveCacheEntry)}
	now := time.Now()
	for i := 0; i < cloudResolveCacheMaxEntries; i++ {
		key := fmt.Sprintf("openlist\x00/file-%04d\x00Player", i)
		storage.resolveCache[key] = cloudResolveCacheEntry{
			link:      &cloud.DirectLink{URL: fmt.Sprintf("https://cdn.example/%d", i)},
			expiresAt: now.Add(time.Minute), staleUntil: now.Add(time.Minute), lastHit: now.Add(time.Duration(i) * time.Millisecond),
		}
	}
	oldest := "openlist\x00/file-0000\x00Player"
	storage.storeResolvedLinkLocked("openlist\x00/new-file\x00Player", &cloud.DirectLink{URL: "https://cdn.example/new"}, time.Minute, time.Minute)
	if len(storage.resolveCache) != cloudResolveCacheMaxEntries {
		t.Fatalf("cache size=%d, want %d", len(storage.resolveCache), cloudResolveCacheMaxEntries)
	}
	if _, ok := storage.resolveCache[oldest]; ok {
		t.Fatal("least recently used cache entry was not pruned")
	}
}

func TestCloudResolveCacheRefreshAtCapacityDoesNotEvict(t *testing.T) {
	storage := &StorageConfigService{resolveCache: make(map[string]cloudResolveCacheEntry)}
	now := time.Now()
	for i := 0; i < cloudResolveCacheMaxEntries; i++ {
		key := fmt.Sprintf("key-%04d", i)
		storage.resolveCache[key] = cloudResolveCacheEntry{
			link:      &cloud.DirectLink{URL: fmt.Sprintf("https://cdn.example/%d", i)},
			expiresAt: now.Add(time.Minute), staleUntil: now.Add(time.Minute), lastHit: now,
		}
	}
	storage.storeResolvedLinkLocked("key-0000", &cloud.DirectLink{URL: "https://cdn.example/refreshed"}, time.Minute, time.Minute)
	if len(storage.resolveCache) != cloudResolveCacheMaxEntries || storage.resolveCache["key-0000"].link.URL != "https://cdn.example/refreshed" {
		t.Fatalf("cache refresh changed capacity or missed replacement")
	}
}

func TestCloudResolveExpiredLinkDoesNotReturnStaleOnResolveFailure(t *testing.T) {
	var resolves atomic.Int32
	expiry := time.Now().Add(time.Hour).Unix()
	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/api/fs/list" {
			writeOpenListCacheList(w)
			return
		}
		if r.URL.Path != "/api/fs/get" {
			t.Fatalf("unexpected path %s", r.URL.Path)
		}
		n := resolves.Add(1)
		w.Header().Set("Content-Type", "application/json")
		if n == 1 {
			_, _ = fmt.Fprintf(w, `{"code":200,"data":{"raw_url":"https://cdnfhnfile.115cdn.net/hash/movie.mp4?t=%d"}}`, expiry)
			return
		}
		_, _ = fmt.Fprint(w, `{"code":500,"message":"failed get link"}`)
	}))
	defer upstream.Close()

	_, storage := newStorageUploadTestService(t)
	if _, err := storage.Save(t.Context(), StorageInput{
		Type: "openlist",
		Config: map[string]any{
			"server": upstream.URL,
			"token":  "token",
		},
	}); err != nil {
		t.Fatal(err)
	}

	link, err := storage.CloudResolve(t.Context(), "openlist", "/Movies/f1.mkv", "Player/1")
	if err != nil {
		t.Fatal(err)
	}
	if link.URL == "" || resolves.Load() != 1 {
		t.Fatalf("first resolve link=%#v resolves=%d", link, resolves.Load())
	}

	key := storage.resolveCacheKey("openlist", "/Movies/f1.mkv", "Player/1")
	storage.resolveMu.Lock()
	entry := storage.resolveCache[key]
	entry.expiresAt = time.Now().Add(-time.Second)
	storage.resolveCache[key] = entry
	storage.resolveMu.Unlock()

	link, err = storage.CloudResolve(t.Context(), "openlist", "/Movies/f1.mkv", "Player/1")
	if err == nil {
		t.Fatalf("expired link unexpectedly succeeded: %#v", link)
	}
	if link != nil {
		t.Fatalf("expired link returned stale URL: %#v", link)
	}
	if resolves.Load() != 2 {
		t.Fatalf("resolves = %d, want one synchronous re-resolve", resolves.Load())
	}
}

func TestCloudResolveExpiredLinkReturnsCircuitErrorInsteadOfStale(t *testing.T) {
	var resolves atomic.Int32
	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/api/fs/list" {
			writeOpenListCacheList(w)
			return
		}
		if r.URL.Path != "/api/fs/get" {
			t.Fatalf("unexpected path %s", r.URL.Path)
		}
		resolves.Add(1)
		w.Header().Set("Content-Type", "application/json")
		_, _ = fmt.Fprint(w, `{"code":200,"data":{"raw_url":"http://cdn.local/movie.mkv"}}`)
	}))
	defer upstream.Close()

	_, storage := newStorageUploadTestService(t)
	if _, err := storage.Save(t.Context(), StorageInput{Type: "openlist", Config: map[string]any{"server": upstream.URL, "token": "token"}}); err != nil {
		t.Fatal(err)
	}
	if _, err := storage.CloudResolve(t.Context(), "openlist", "/Movies/f1.mkv", "Player/1"); err != nil {
		t.Fatal(err)
	}
	key := storage.resolveCacheKey("openlist", "/Movies/f1.mkv", "Player/1")
	storage.resolveMu.Lock()
	entry := storage.resolveCache[key]
	entry.expiresAt = time.Now().Add(-time.Second)
	storage.resolveCache[key] = entry
	storage.resolveCircuit[cloud.TypeOpenList] = cloudResolveCircuitState{openUntil: time.Now().Add(time.Minute)}
	storage.resolveMu.Unlock()

	resolved, err := storage.CloudResolve(t.Context(), "openlist", "/Movies/f1.mkv", "Player/1")
	var circuitErr *cloudResolveCircuitOpenError
	if !errors.As(err, &circuitErr) {
		t.Fatalf("expired resolve link=%#v err=%v, want circuit error", resolved, err)
	}
	if resolved != nil {
		t.Fatalf("expired link returned stale URL: %#v", resolved)
	}
	if resolves.Load() != 1 {
		t.Fatalf("resolves = %d, want no provider call while circuit is open", resolves.Load())
	}
}

func TestCloudResolveCachesFastFailedGetLinkWithoutRetry(t *testing.T) {
	var resolves atomic.Int32
	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/api/fs/list" {
			writeOpenListCacheList(w)
			return
		}
		if r.URL.Path != "/api/fs/get" {
			t.Fatalf("unexpected path %s", r.URL.Path)
		}
		resolves.Add(1)
		w.Header().Set("Content-Type", "application/json")
		_, _ = fmt.Fprint(w, `{"code":500,"message":"failed get link"}`)
	}))
	defer upstream.Close()

	_, storage := newStorageUploadTestService(t)
	if _, err := storage.Save(t.Context(), StorageInput{
		Type: "openlist",
		Config: map[string]any{
			"server": upstream.URL,
			"token":  "token",
		},
	}); err != nil {
		t.Fatal(err)
	}

	if _, err := storage.CloudResolve(t.Context(), "openlist", "/Movies/f1.mkv", "Player/1"); err == nil {
		t.Fatal("first resolve unexpectedly succeeded")
	}
	if _, err := storage.CloudResolve(t.Context(), "openlist", "/Movies/f1.mkv", "Player/1"); err == nil {
		t.Fatal("cached failure unexpectedly succeeded")
	}
	if resolves.Load() != 1 {
		t.Fatalf("resolves = %d, want one call without retry", resolves.Load())
	}
}

func TestCloudResolveOpensCircuitAfterConsecutiveTransportFailures(t *testing.T) {
	var resolves atomic.Int32
	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/api/fs/list" {
			writeOpenListCacheList(w)
			return
		}
		if r.URL.Path != "/api/fs/get" {
			t.Fatalf("unexpected path %s", r.URL.Path)
		}
		resolves.Add(1)
		w.Header().Set("Content-Type", "application/json")
		_, _ = fmt.Fprint(w, `{"code":500,"message":"Post https://proapi.115.com/open/ufile/downurl: net/http: TLS handshake timeout; failed get link"}`)
	}))
	defer upstream.Close()

	_, storage := newStorageUploadTestService(t)
	if _, err := storage.Save(t.Context(), StorageInput{Type: "openlist", Config: map[string]any{"server": upstream.URL, "token": "token"}}); err != nil {
		t.Fatal(err)
	}
	for _, ref := range []string{"/Movies/f1.mkv", "/Movies/f2.mkv"} {
		if _, err := storage.CloudResolve(t.Context(), "openlist", ref, "Player/1"); err == nil {
			t.Fatalf("resolve %s unexpectedly succeeded", ref)
		}
	}
	_, err := storage.CloudResolve(t.Context(), "openlist", "/Movies/f3.mkv", "Player/1")
	var circuitErr *cloudResolveCircuitOpenError
	if !errors.As(err, &circuitErr) {
		t.Fatalf("third resolve error = %v, want open circuit", err)
	}
	if resolves.Load() != 2 {
		t.Fatalf("resolves = %d, want two upstream calls before circuit opened", resolves.Load())
	}
}

func TestCloudResolveRateLimitOpensCircuitImmediately(t *testing.T) {
	var resolves atomic.Int32
	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/api/fs/list" {
			writeOpenListCacheList(w)
			return
		}
		if r.URL.Path != "/api/fs/get" {
			t.Fatalf("unexpected path %s", r.URL.Path)
		}
		resolves.Add(1)
		w.Header().Set("Content-Type", "application/json")
		_, _ = fmt.Fprint(w, `{"code":500,"message":"40140117: refresh too frequently"}`)
	}))
	defer upstream.Close()

	_, storage := newStorageUploadTestService(t)
	if _, err := storage.Save(t.Context(), StorageInput{Type: "openlist", Config: map[string]any{"server": upstream.URL, "token": "token"}}); err != nil {
		t.Fatal(err)
	}
	if _, err := storage.CloudResolve(t.Context(), "openlist", "/Movies/f1.mkv", "Player/1"); err == nil || !strings.Contains(err.Error(), "40140117") {
		t.Fatalf("first resolve error = %v, want provider rate limit", err)
	}
	_, err := storage.CloudResolve(t.Context(), "openlist", "/Movies/f2.mkv", "Player/1")
	var circuitErr *cloudResolveCircuitOpenError
	if !errors.As(err, &circuitErr) {
		t.Fatalf("second resolve error = %v, want open circuit", err)
	}
	if resolves.Load() != 1 {
		t.Fatalf("resolves = %d, want one upstream call before rate-limit cooldown", resolves.Load())
	}
}

func TestCloudResolveCircuitAllowsSingleHalfOpenProbe(t *testing.T) {
	var resolves atomic.Int32
	probeStarted := make(chan struct{})
	releaseProbe := make(chan struct{}, 1)
	defer func() {
		select {
		case releaseProbe <- struct{}{}:
		default:
		}
	}()
	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/api/fs/list" {
			writeOpenListCacheList(w)
			return
		}
		if r.URL.Path != "/api/fs/get" {
			t.Fatalf("unexpected path %s", r.URL.Path)
		}
		call := resolves.Add(1)
		w.Header().Set("Content-Type", "application/json")
		if call <= 2 {
			_, _ = fmt.Fprint(w, `{"code":500,"message":"net/http: TLS handshake timeout; failed get link"}`)
			return
		}
		if call == 3 {
			close(probeStarted)
			<-releaseProbe
		}
		_, _ = fmt.Fprintf(w, `{"code":200,"data":{"raw_url":"http://cdn.local/%d.mkv"}}`, call)
	}))
	defer upstream.Close()

	_, storage := newStorageUploadTestService(t)
	if _, err := storage.Save(t.Context(), StorageInput{Type: "openlist", Config: map[string]any{"server": upstream.URL, "token": "token"}}); err != nil {
		t.Fatal(err)
	}
	for _, ref := range []string{"/Movies/f1.mkv", "/Movies/f2.mkv"} {
		if _, err := storage.CloudResolve(t.Context(), "openlist", ref, "Player/1"); err == nil {
			t.Fatalf("resolve %s unexpectedly succeeded", ref)
		}
	}
	storage.resolveMu.Lock()
	state := storage.resolveCircuit[cloud.TypeOpenList]
	state.openUntil = time.Now().Add(-time.Second)
	storage.resolveCircuit[cloud.TypeOpenList] = state
	storage.resolveMu.Unlock()

	probeLink := make(chan *cloud.DirectLink, 1)
	probeErr := make(chan error, 1)
	go func() {
		link, err := storage.CloudResolve(t.Context(), "openlist", "/Movies/f3.mkv", "Player/1")
		probeLink <- link
		probeErr <- err
	}()
	select {
	case <-probeStarted:
	case <-time.After(time.Second):
		t.Fatal("half-open probe did not start")
	}
	_, err := storage.CloudResolve(t.Context(), "openlist", "/Movies/f4.mkv", "Player/1")
	var circuitErr *cloudResolveCircuitOpenError
	if !errors.As(err, &circuitErr) {
		t.Fatalf("parallel half-open resolve error = %v, want open circuit", err)
	}
	if resolves.Load() != 3 {
		t.Fatalf("resolves = %d, want only one half-open probe", resolves.Load())
	}
	releaseProbe <- struct{}{}
	if err := <-probeErr; err != nil {
		t.Fatal(err)
	}
	if link := <-probeLink; link == nil || link.URL != "http://cdn.local/3.mkv" {
		t.Fatalf("half-open probe link = %#v", link)
	}
	link, err := storage.CloudResolve(t.Context(), "openlist", "/Movies/f5.mkv", "Player/1")
	if err != nil || link == nil || link.URL != "http://cdn.local/4.mkv" {
		t.Fatalf("resolve after successful probe link=%#v err=%v", link, err)
	}
}

func TestCloudResolveColdMissUsesBoundedWorkerDeadline(t *testing.T) {
	started := make(chan struct{})
	_, storage := newStorageUploadTestService(t)
	storage.client = &http.Client{
		Transport: cloudResolveTestRoundTripper(func(r *http.Request) (*http.Response, error) {
			switch r.URL.Path {
			case "/api/fs/list":
				return &http.Response{
					StatusCode: http.StatusOK,
					Header:     http.Header{"Content-Type": []string{"application/json"}},
					Body:       io.NopCloser(strings.NewReader(`{"code":200,"data":{"content":[],"total":0}}`)),
					Request:    r,
				}, nil
			case "/api/fs/get":
				select {
				case <-started:
				default:
					close(started)
				}
				<-r.Context().Done()
				return nil, r.Context().Err()
			default:
				return nil, fmt.Errorf("unexpected path %s", r.URL.Path)
			}
		}),
	}

	if _, err := storage.Save(t.Context(), StorageInput{
		Type: "openlist",
		Config: map[string]any{
			"server": "http://openlist.test",
			"token":  "token",
		},
	}); err != nil {
		t.Fatal(err)
	}

	start := time.Now()
	_, err := storage.CloudResolve(t.Context(), "openlist", "/Movies/f1.mkv", "Player/1")
	elapsed := time.Since(start)
	if !errors.Is(err, context.DeadlineExceeded) {
		t.Fatalf("cold resolve error = %v, want context deadline exceeded", err)
	}
	select {
	case <-started:
	case <-time.After(time.Second):
		t.Fatal("OpenList get request did not start")
	}
	if elapsed >= cloudResolveColdMaxDuration+500*time.Millisecond {
		t.Fatalf("cold resolve took %s, want less than %s", elapsed, cloudResolveColdMaxDuration+500*time.Millisecond)
	}
	key := storage.resolveCacheKey("openlist", "/Movies/f1.mkv", "Player/1")
	storage.resolveMu.Lock()
	_, inFlight := storage.resolveFlight[key]
	storage.resolveMu.Unlock()
	if inFlight {
		t.Fatal("cold resolve left a single-flight request behind")
	}
}

func TestStorageConfigSaveNotifiesChangeHandler(t *testing.T) {
	_, storage := newStorageUploadTestService(t)
	var changed string
	storage.SetChangeHandler(func(typ string) {
		changed = typ
	})
	if _, err := storage.Save(t.Context(), StorageInput{
		Type: "openlist",
		Config: map[string]any{
			"server": "http://openlist.test",
			"token":  "token",
		},
	}); err != nil {
		t.Fatal(err)
	}
	if changed != "openlist" {
		t.Fatalf("changed=%q, want openlist", changed)
	}
}

func TestCloudResolveContinuesAfterOwnerContextCanceled(t *testing.T) {
	var resolves atomic.Int32
	getStarted := make(chan struct{})
	releaseGet := make(chan struct{})
	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/api/fs/list":
			writeOpenListCacheList(w)
		case "/api/fs/get":
			if resolves.Add(1) == 1 {
				close(getStarted)
			}
			<-releaseGet
			w.Header().Set("Content-Type", "application/json")
			_, _ = fmt.Fprint(w, `{"code":200,"data":{"raw_url":"http://cdn.local/movie.mkv"}}`)
		default:
			t.Fatalf("unexpected path %s", r.URL.Path)
		}
	}))
	defer upstream.Close()

	_, storage := newStorageUploadTestService(t)
	if _, err := storage.Save(t.Context(), StorageInput{Type: "openlist", Config: map[string]any{"server": upstream.URL, "token": "token"}}); err != nil {
		t.Fatal(err)
	}

	ownerCtx, cancelOwner := context.WithCancel(t.Context())
	ownerErr := make(chan error, 1)
	go func() {
		_, err := storage.CloudResolve(ownerCtx, "openlist", "/Movies/f1.mkv", "Player/1")
		ownerErr <- err
	}()
	select {
	case <-getStarted:
	case <-time.After(time.Second):
		t.Fatal("cold resolve did not start")
	}
	cancelOwner()
	if err := <-ownerErr; !errors.Is(err, context.Canceled) {
		t.Fatalf("owner error = %v, want context canceled", err)
	}

	waiterResult := make(chan *cloud.DirectLink, 1)
	waiterErr := make(chan error, 1)
	go func() {
		link, err := storage.CloudResolve(t.Context(), "openlist", "/Movies/f1.mkv", "Player/1")
		waiterResult <- link
		waiterErr <- err
	}()
	time.Sleep(30 * time.Millisecond)
	if resolves.Load() != 1 {
		t.Fatalf("resolve calls = %d, want one shared flight", resolves.Load())
	}
	close(releaseGet)
	if err := <-waiterErr; err != nil {
		t.Fatal(err)
	}
	if link := <-waiterResult; link == nil || link.URL != "http://cdn.local/movie.mkv" {
		t.Fatalf("waiter link = %#v", link)
	}
	link, err := storage.CloudResolve(t.Context(), "openlist", "/Movies/f1.mkv", "Player/1")
	if err != nil || link == nil || link.URL != "http://cdn.local/movie.mkv" || resolves.Load() != 1 {
		t.Fatalf("cached link=%#v err=%v resolves=%d", link, err, resolves.Load())
	}
}

func TestCloudResolveKeepsDifferentUserAgentsIndependent(t *testing.T) {
	var resolves atomic.Int32
	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/api/fs/list":
			writeOpenListCacheList(w)
		case "/api/fs/get":
			resolves.Add(1)
			w.Header().Set("Content-Type", "application/json")
			_, _ = fmt.Fprintf(w, `{"code":200,"data":{"raw_url":"http://cdn.local/%s.mkv"}}`, url.PathEscape(r.UserAgent()))
		default:
			t.Fatalf("unexpected path %s", r.URL.Path)
		}
	}))
	defer upstream.Close()

	_, storage := newStorageUploadTestService(t)
	if _, err := storage.Save(t.Context(), StorageInput{Type: "openlist", Config: map[string]any{"server": upstream.URL, "token": "token"}}); err != nil {
		t.Fatal(err)
	}
	first, err := storage.CloudResolve(t.Context(), "openlist", "/Movies/f1.mkv", "Player/1")
	if err != nil {
		t.Fatal(err)
	}
	second, err := storage.CloudResolve(t.Context(), "openlist", "/Movies/f1.mkv", "Player/2")
	if err != nil {
		t.Fatal(err)
	}
	if first.URL == second.URL || resolves.Load() != 2 {
		t.Fatalf("first=%q second=%q resolves=%d, want UA-specific links", first.URL, second.URL, resolves.Load())
	}
}

func TestCloudResolveSerializesDifferentUserAgentsForSameFile(t *testing.T) {
	var resolves atomic.Int32
	var active atomic.Int32
	var maxActive atomic.Int32
	firstStarted := make(chan struct{})
	releaseFirst := make(chan struct{})
	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/api/fs/list":
			writeOpenListCacheList(w)
		case "/api/fs/get":
			call := resolves.Add(1)
			current := active.Add(1)
			for {
				previous := maxActive.Load()
				if current <= previous || maxActive.CompareAndSwap(previous, current) {
					break
				}
			}
			defer active.Add(-1)
			if call == 1 {
				close(firstStarted)
				<-releaseFirst
			}
			w.Header().Set("Content-Type", "application/json")
			_, _ = fmt.Fprintf(w, `{"code":200,"data":{"raw_url":"http://cdn.local/%s.mkv"}}`, url.PathEscape(r.UserAgent()))
		default:
			t.Fatalf("unexpected path %s", r.URL.Path)
		}
	}))
	defer upstream.Close()

	_, storage := newStorageUploadTestService(t)
	if _, err := storage.Save(t.Context(), StorageInput{Type: "openlist", Config: map[string]any{"server": upstream.URL, "token": "token"}}); err != nil {
		t.Fatal(err)
	}
	type result struct {
		link *cloud.DirectLink
		err  error
	}
	firstResult := make(chan result, 1)
	go func() {
		link, err := storage.CloudResolve(t.Context(), "openlist", "/Movies/f1.mkv", "Player/1")
		firstResult <- result{link: link, err: err}
	}()
	select {
	case <-firstStarted:
	case <-time.After(time.Second):
		t.Fatal("first resolve did not start")
	}

	secondResult := make(chan result, 1)
	go func() {
		link, err := storage.CloudResolve(t.Context(), "openlist", "/Movies/f1.mkv", "Player/2")
		secondResult <- result{link: link, err: err}
	}()
	time.Sleep(50 * time.Millisecond)
	if resolves.Load() != 1 || maxActive.Load() != 1 {
		t.Fatalf("resolves=%d max_active=%d, want one in-flight resolve", resolves.Load(), maxActive.Load())
	}
	close(releaseFirst)

	first := <-firstResult
	second := <-secondResult
	if first.err != nil || second.err != nil || first.link == nil || second.link == nil {
		t.Fatalf("first=%#v second=%#v, want two successful links", first, second)
	}
	if first.link.URL == second.link.URL || resolves.Load() != 2 || maxActive.Load() != 1 {
		t.Fatalf("first=%q second=%q resolves=%d max_active=%d, want serialized UA-specific links", first.link.URL, second.link.URL, resolves.Load(), maxActive.Load())
	}
	storage.resolveMu.Lock()
	defer storage.resolveMu.Unlock()
	if len(storage.resolveFileGate) != 0 {
		t.Fatalf("resolve file gates leaked: %d", len(storage.resolveFileGate))
	}
}

func TestCloudResolveLimitsColdMissesAcrossFilesToTwo(t *testing.T) {
	var resolves atomic.Int32
	var active atomic.Int32
	var maxActive atomic.Int32
	started := make(chan struct{}, 2)
	releaseFirstTwo := make(chan struct{})
	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/api/fs/list":
			writeOpenListCacheList(w)
		case "/api/fs/get":
			call := resolves.Add(1)
			current := active.Add(1)
			for {
				previous := maxActive.Load()
				if current <= previous || maxActive.CompareAndSwap(previous, current) {
					break
				}
			}
			defer active.Add(-1)
			if call <= 2 {
				started <- struct{}{}
				<-releaseFirstTwo
			}
			w.Header().Set("Content-Type", "application/json")
			_, _ = fmt.Fprintf(w, `{"code":200,"data":{"raw_url":"http://cdn.local/%d.mkv"}}`, call)
		default:
			t.Fatalf("unexpected path %s", r.URL.Path)
		}
	}))
	defer upstream.Close()

	_, storage := newStorageUploadTestService(t)
	if _, err := storage.Save(t.Context(), StorageInput{Type: "openlist", Config: map[string]any{"server": upstream.URL, "token": "token"}}); err != nil {
		t.Fatal(err)
	}
	sqlDB, err := storage.repo.DB.DB()
	if err != nil {
		t.Fatal(err)
	}
	sqlDB.SetMaxOpenConns(1)
	type result struct {
		link *cloud.DirectLink
		err  error
	}
	results := make(chan result, 3)
	for i := 1; i <= 3; i++ {
		i := i
		go func() {
			link, err := storage.CloudResolve(t.Context(), "openlist", fmt.Sprintf("/Movies/f%d.mkv", i), fmt.Sprintf("Player/%d", i))
			results <- result{link: link, err: err}
		}()
	}
	for i := 0; i < 2; i++ {
		select {
		case <-started:
		case <-time.After(time.Second):
			close(releaseFirstTwo)
			t.Fatalf("two concurrent resolves did not start: resolves=%d max_active=%d", resolves.Load(), maxActive.Load())
		}
	}
	time.Sleep(50 * time.Millisecond)
	if resolves.Load() != 2 || maxActive.Load() != 2 {
		t.Fatalf("resolves=%d max_active=%d, want two in-flight resolves and one queued", resolves.Load(), maxActive.Load())
	}
	close(releaseFirstTwo)

	for i := 0; i < 3; i++ {
		got := <-results
		if got.err != nil || got.link == nil {
			t.Fatalf("result=%#v, want three successful links", got)
		}
	}
	if resolves.Load() != 3 || maxActive.Load() != 2 {
		t.Fatalf("resolves=%d max_active=%d, want at most two concurrent cold resolves", resolves.Load(), maxActive.Load())
	}
}

func TestCloudResolveQueuedUserAgentStopsAfterRateLimit(t *testing.T) {
	var resolves atomic.Int32
	firstStarted := make(chan struct{})
	releaseFirst := make(chan struct{})
	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/api/fs/list":
			writeOpenListCacheList(w)
		case "/api/fs/get":
			call := resolves.Add(1)
			if call == 1 {
				close(firstStarted)
				<-releaseFirst
			}
			w.Header().Set("Content-Type", "application/json")
			_, _ = fmt.Fprint(w, `{"code":500,"message":"40140117: refresh too frequently"}`)
		default:
			t.Fatalf("unexpected path %s", r.URL.Path)
		}
	}))
	defer upstream.Close()

	_, storage := newStorageUploadTestService(t)
	if _, err := storage.Save(t.Context(), StorageInput{Type: "openlist", Config: map[string]any{"server": upstream.URL, "token": "token"}}); err != nil {
		t.Fatal(err)
	}
	type result struct {
		err error
	}
	firstResult := make(chan result, 1)
	go func() {
		_, err := storage.CloudResolve(t.Context(), "openlist", "/Movies/f1.mkv", "Player/1")
		firstResult <- result{err: err}
	}()
	select {
	case <-firstStarted:
	case <-time.After(time.Second):
		t.Fatal("first resolve did not start")
	}
	secondResult := make(chan result, 1)
	go func() {
		_, err := storage.CloudResolve(t.Context(), "openlist", "/Movies/f1.mkv", "Player/2")
		secondResult <- result{err: err}
	}()
	time.Sleep(50 * time.Millisecond)
	if resolves.Load() != 1 {
		t.Fatalf("resolves=%d before first completes, want one queued request", resolves.Load())
	}
	close(releaseFirst)

	first := <-firstResult
	second := <-secondResult
	if first.err == nil || !strings.Contains(first.err.Error(), "40140117") {
		t.Fatalf("first error=%v, want provider rate limit", first.err)
	}
	var circuitErr *cloudResolveCircuitOpenError
	if !errors.As(second.err, &circuitErr) {
		t.Fatalf("second error=%v, want local circuit-open error", second.err)
	}
	if resolves.Load() != 1 {
		t.Fatalf("resolves=%d, want no upstream retry after rate limit", resolves.Load())
	}
}

func TestCloudResolveConfigChangePreventsOldFlightCacheWrite(t *testing.T) {
	var resolves atomic.Int32
	firstStarted := make(chan struct{})
	releaseFirst := make(chan struct{})
	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/api/fs/list":
			writeOpenListCacheList(w)
		case "/api/fs/get":
			call := resolves.Add(1)
			if call == 1 {
				close(firstStarted)
				<-releaseFirst
			}
			w.Header().Set("Content-Type", "application/json")
			_, _ = fmt.Fprintf(w, `{"code":200,"data":{"raw_url":"http://cdn.local/%d.mkv"}}`, call)
		default:
			t.Fatalf("unexpected path %s", r.URL.Path)
		}
	}))
	defer upstream.Close()

	_, storage := newStorageUploadTestService(t)
	if _, err := storage.Save(t.Context(), StorageInput{Type: "openlist", Config: map[string]any{"server": upstream.URL, "token": "token-1"}}); err != nil {
		t.Fatal(err)
	}
	oldResult := make(chan error, 1)
	go func() {
		_, err := storage.CloudResolve(t.Context(), "openlist", "/Movies/f1.mkv", "Player/1")
		oldResult <- err
	}()
	select {
	case <-firstStarted:
	case <-time.After(time.Second):
		t.Fatal("old flight did not start")
	}
	if _, err := storage.Save(t.Context(), StorageInput{Type: "openlist", Config: map[string]any{"server": upstream.URL, "token": "token-2"}}); err != nil {
		t.Fatal(err)
	}
	close(releaseFirst)
	if err := <-oldResult; err == nil || !strings.Contains(err.Error(), "storage config changed") {
		t.Fatalf("old flight error = %v, want config changed", err)
	}
	link, err := storage.CloudResolve(t.Context(), "openlist", "/Movies/f1.mkv", "Player/1")
	if err != nil {
		t.Fatal(err)
	}
	if link.URL != "http://cdn.local/2.mkv" || resolves.Load() != 2 {
		t.Fatalf("link=%#v resolves=%d, old result was cached", link, resolves.Load())
	}
}

func writeOpenListCacheList(w http.ResponseWriter) {
	w.Header().Set("Content-Type", "application/json")
	_, _ = fmt.Fprint(w, `{"code":200,"data":{"content":[],"total":0}}`)
}
