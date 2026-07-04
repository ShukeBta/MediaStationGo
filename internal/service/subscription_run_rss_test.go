package service

import (
	"net/http"
	"net/http/httptest"
	"sync/atomic"
	"testing"

	"go.uber.org/zap"

	"github.com/ShukeBta/MediaStationGo/internal/model"
	"github.com/ShukeBta/MediaStationGo/internal/repository"
)

func TestSubscriptionRunOneDeduplicatesDuplicateRSSGUIDInSameFeed(t *testing.T) {
	rss := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "application/rss+xml")
		_, _ = w.Write([]byte(`<?xml version="1.0"?>
<rss><channel>
  <item>
    <title>Some Show S01E01 1080p</title>
    <guid>episode-1</guid>
    <link>magnet:?xt=urn:btih:1111111111111111111111111111111111111111&amp;dn=Some+Show+S01E01</link>
  </item>
  <item>
    <title>Some Show S01E01 1080p</title>
    <guid>episode-1</guid>
    <link>magnet:?xt=urn:btih:1111111111111111111111111111111111111111&amp;dn=Some+Show+S01E01</link>
  </item>
</channel></rss>`))
	}))
	defer rss.Close()

	var addCalls int32
	qb := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/api/v2/auth/login":
			_, _ = w.Write([]byte("Ok."))
		case "/api/v2/torrents/info":
			if atomic.LoadInt32(&addCalls) > 0 {
				_, _ = w.Write([]byte(`[{"hash":"abc123","name":"Some Show S01E01 1080p","state":"downloading","progress":0.1}]`))
				return
			}
			_, _ = w.Write([]byte(`[]`))
		case "/api/v2/torrents/add":
			atomic.AddInt32(&addCalls, 1)
			_, _ = w.Write([]byte("Ok."))
		default:
			http.NotFound(w, r)
		}
	}))
	defer qb.Close()

	db := newServiceTestDB(t, &model.Subscription{}, &model.Setting{}, &model.DownloadTask{}, &model.Media{}, &model.DownloadClient{})
	repos := repository.New(db)
	configureTestDefaultQB(t, repos, qb.URL)
	downloads := NewDownloadService(zap.NewNop(), repos, NewHub(zap.NewNop()), nil)
	svc := NewSubscriptionService(nil, zap.NewNop(), repos, downloads, nil, NewHub(zap.NewNop()))

	sub := &model.Subscription{
		Name:      "Some Show 自动订阅",
		FeedURL:   rss.URL,
		Filter:    "Some Show",
		MediaType: "tv",
		SavePath:  "/downloads/tv",
	}
	if err := repos.Subscription.Create(t.Context(), sub); err != nil {
		t.Fatal(err)
	}
	queued, err := svc.runOne(t.Context(), sub)
	if err != nil {
		t.Fatal(err)
	}
	if queued != 1 {
		t.Fatalf("queued = %d, want 1", queued)
	}
	if got := atomic.LoadInt32(&addCalls); got != 1 {
		t.Fatalf("qb add calls = %d, want 1", got)
	}
	rows, err := repos.Download.List(t.Context())
	if err != nil {
		t.Fatal(err)
	}
	if len(rows) != 1 {
		t.Fatalf("download rows = %d, want 1", len(rows))
	}
}

func TestSubscriptionRunOneSkipsSameEpisodeAddedEarlierInFeed(t *testing.T) {
	rss := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "application/rss+xml")
		_, _ = w.Write([]byte(`<?xml version="1.0"?>
<rss><channel>
  <item>
    <title>Some Show S01E01 1080p</title>
    <guid>episode-1-a</guid>
    <link>magnet:?xt=urn:btih:aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa&amp;dn=Some+Show+S01E01+1080p</link>
  </item>
  <item>
    <title>Some Show S01E01 WEB-DL</title>
    <guid>episode-1-b</guid>
    <link>magnet:?xt=urn:btih:bbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbb&amp;dn=Some+Show+S01E01+WEB-DL</link>
  </item>
</channel></rss>`))
	}))
	defer rss.Close()

	var addCalls int32
	qb := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/api/v2/auth/login":
			_, _ = w.Write([]byte("Ok."))
		case "/api/v2/torrents/info":
			if atomic.LoadInt32(&addCalls) > 0 {
				_, _ = w.Write([]byte(`[{"hash":"abc123","name":"Some Show S01E01 1080p","state":"downloading","progress":0.1}]`))
				return
			}
			_, _ = w.Write([]byte(`[]`))
		case "/api/v2/torrents/add":
			atomic.AddInt32(&addCalls, 1)
			_, _ = w.Write([]byte("Ok."))
		default:
			http.NotFound(w, r)
		}
	}))
	defer qb.Close()

	db := newServiceTestDB(t, &model.Subscription{}, &model.Setting{}, &model.DownloadTask{}, &model.Media{}, &model.DownloadClient{})
	repos := repository.New(db)
	configureTestDefaultQB(t, repos, qb.URL)
	downloads := NewDownloadService(zap.NewNop(), repos, NewHub(zap.NewNop()), nil)
	svc := NewSubscriptionService(nil, zap.NewNop(), repos, downloads, nil, NewHub(zap.NewNop()))

	sub := &model.Subscription{
		Name:          "Some Show 自动订阅",
		FeedURL:       rss.URL,
		Filter:        "Some Show",
		MediaType:     "tv",
		SavePath:      "/downloads/tv",
		TotalEpisodes: 12,
	}
	if err := repos.Subscription.Create(t.Context(), sub); err != nil {
		t.Fatal(err)
	}
	queued, err := svc.runOne(t.Context(), sub)
	if err != nil {
		t.Fatal(err)
	}
	if queued != 1 {
		t.Fatalf("queued = %d, want 1", queued)
	}
	if got := atomic.LoadInt32(&addCalls); got != 1 {
		t.Fatalf("qb add calls = %d, want 1", got)
	}
	rows, err := repos.Download.List(t.Context())
	if err != nil {
		t.Fatal(err)
	}
	if len(rows) != 1 {
		t.Fatalf("download rows = %d, want 1", len(rows))
	}
}
