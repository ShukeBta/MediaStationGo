package service

import (
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync/atomic"
	"testing"

	"go.uber.org/zap"

	"github.com/ShukeBta/MediaStationGo/internal/model"
	"github.com/ShukeBta/MediaStationGo/internal/repository"
)

func TestSubscriptionRunOneRSSWashQueuesOnlyBestMovieVariant(t *testing.T) {
	rss := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "application/rss+xml")
		_, _ = w.Write([]byte(`<?xml version="1.0"?>
<rss><channel>
  <item>
    <title>Dune 2021 1080p WEB-DL</title>
    <guid>dune-1080-web</guid>
    <link>magnet:?xt=urn:btih:dddddddddddddddddddddddddddddddddddddddd&amp;dn=Dune+2021+1080p+WEB-DL</link>
  </item>
  <item>
    <title>Dune 2021 2160p UHD BluRay REMUX HDR</title>
    <guid>dune-2160-remux</guid>
    <link>magnet:?xt=urn:btih:eeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeee&amp;dn=Dune+2021+2160p+REMUX</link>
  </item>
  <item>
    <title>Dune 2021 720p HDTV</title>
    <guid>dune-720-hdtv</guid>
    <link>magnet:?xt=urn:btih:ffffffffffffffffffffffffffffffffffffffff&amp;dn=Dune+2021+720p+HDTV</link>
  </item>
</channel></rss>`))
	}))
	defer rss.Close()

	var addCalls int32
	var addedTitles []string
	addedHashes := make([]string, 0, 3)
	qb := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/api/v2/auth/login":
			_, _ = w.Write([]byte("Ok."))
		case "/api/v2/torrents/info":
			if len(addedHashes) == 0 {
				_, _ = w.Write([]byte(`[]`))
				return
			}
			var items []string
			for _, hash := range addedHashes {
				items = append(items, `{"hash":"`+hash+`","name":"Dune 2021","state":"downloading","progress":0.1}`)
			}
			_, _ = w.Write([]byte(`[` + strings.Join(items, ",") + `]`))
		case "/api/v2/torrents/add":
			call := atomic.AddInt32(&addCalls, 1)
			_ = r.ParseMultipartForm(10 << 20)
			addedTitles = append(addedTitles, r.FormValue("urls"))
			addedHashes = append(addedHashes, strings.Repeat(fmt.Sprintf("%x", call), 40))
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
		Name:         "Dune 自动订阅",
		FeedURL:      rss.URL,
		Filter:       "Dune 2021",
		MediaType:    "movie",
		Resolution:   "2160p",
		Quality:      "remux",
		WashEnabled:  true,
		WashPriority: "resolution",
		SavePath:     "/downloads/movies",
	}
	if err := repos.Subscription.Create(t.Context(), sub); err != nil {
		t.Fatal(err)
	}
	queued, err := svc.runOne(t.Context(), sub)
	if err != nil {
		t.Fatal(err)
	}
	if queued != 1 {
		t.Fatalf("queued = %d, want 1 best movie variant", queued)
	}
	if got := atomic.LoadInt32(&addCalls); got != 1 {
		t.Fatalf("qb add calls = %d, want 1", got)
	}
	if len(addedTitles) != 1 || !strings.Contains(addedTitles[0], "eeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeee") {
		t.Fatalf("added %#v, want 2160p REMUX variant only", addedTitles)
	}
}

func TestSubscriptionRunOneRSSDefaultQueuesOnlyBestEpisodeVariant(t *testing.T) {
	rss := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "application/rss+xml")
		_, _ = w.Write([]byte(`<?xml version="1.0"?>
<rss><channel>
  <item>
    <title>House of the Dragon S03E01 2160p BluRay H264 AAC</title>
    <guid>hotd-e01-bluray</guid>
    <link>magnet:?xt=urn:btih:aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa&amp;dn=House+of+the+Dragon+S03E01+2160p+BluRay</link>
  </item>
  <item>
    <title>House of the Dragon S03E01 1080p WEB-DL H264 AAC</title>
    <guid>hotd-e01-webdl</guid>
    <link>magnet:?xt=urn:btih:bbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbb&amp;dn=House+of+the+Dragon+S03E01+1080p+WEB-DL</link>
  </item>
  <item>
    <title>House of the Dragon S03E01 720p HDTV H264 AAC</title>
    <guid>hotd-e01-hdtv</guid>
    <link>magnet:?xt=urn:btih:cccccccccccccccccccccccccccccccccccccccc&amp;dn=House+of+the+Dragon+S03E01+720p+HDTV</link>
  </item>
</channel></rss>`))
	}))
	defer rss.Close()

	var addCalls int32
	var addedURLs []string
	addedHashes := make([]string, 0, 3)
	qb := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/api/v2/auth/login":
			_, _ = w.Write([]byte("Ok."))
		case "/api/v2/torrents/info":
			if len(addedHashes) == 0 {
				_, _ = w.Write([]byte(`[]`))
				return
			}
			var items []string
			for _, hash := range addedHashes {
				items = append(items, `{"hash":"`+hash+`","name":"House of the Dragon S03E01","state":"downloading","progress":0.1}`)
			}
			_, _ = w.Write([]byte(`[` + strings.Join(items, ",") + `]`))
		case "/api/v2/torrents/add":
			call := atomic.AddInt32(&addCalls, 1)
			_ = r.ParseMultipartForm(10 << 20)
			addedURLs = append(addedURLs, r.FormValue("urls"))
			addedHashes = append(addedHashes, strings.Repeat(fmt.Sprintf("%x", call), 40))
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
		Name:      "House of the Dragon 自动订阅",
		FeedURL:   rss.URL,
		Filter:    "House of the Dragon",
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
		t.Fatalf("queued = %d, want one best episode variant", queued)
	}
	if got := atomic.LoadInt32(&addCalls); got != 1 {
		t.Fatalf("qb add calls = %d, want 1", got)
	}
	if len(addedURLs) != 1 || !strings.Contains(addedURLs[0], "bbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbb") {
		t.Fatalf("added %#v, want 1080p WEB-DL variant only", addedURLs)
	}
}

func TestSubscriptionRunOneRSSCustomExcludeStillSkipsIncompatibleVariants(t *testing.T) {
	rss := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "application/rss+xml")
		_, _ = w.Write([]byte(`<?xml version="1.0"?>
<rss><channel>
  <item>
    <title>House of the Dragon S03E01 2160p WEB-DL HEVC 10bit DoVi Atmos</title>
    <guid>hotd-e01-dovi</guid>
    <link>magnet:?xt=urn:btih:aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa&amp;dn=House+of+the+Dragon+S03E01+2160p+DoVi</link>
  </item>
  <item>
    <title>House of the Dragon S03E01 1080p WEB-DL H264 AAC</title>
    <guid>hotd-e01-webdl</guid>
    <link>magnet:?xt=urn:btih:bbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbb&amp;dn=House+of+the+Dragon+S03E01+1080p+WEB-DL</link>
  </item>
</channel></rss>`))
	}))
	defer rss.Close()

	var addCalls int32
	var addedURLs []string
	addedHashes := make([]string, 0, 2)
	qb := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/api/v2/auth/login":
			_, _ = w.Write([]byte("Ok."))
		case "/api/v2/torrents/info":
			if len(addedHashes) == 0 {
				_, _ = w.Write([]byte(`[]`))
				return
			}
			var items []string
			for _, hash := range addedHashes {
				items = append(items, `{"hash":"`+hash+`","name":"House of the Dragon S03E01","state":"downloading","progress":0.1}`)
			}
			_, _ = w.Write([]byte(`[` + strings.Join(items, ",") + `]`))
		case "/api/v2/torrents/add":
			call := atomic.AddInt32(&addCalls, 1)
			_ = r.ParseMultipartForm(10 << 20)
			addedURLs = append(addedURLs, r.FormValue("urls"))
			addedHashes = append(addedHashes, strings.Repeat(fmt.Sprintf("%x", call), 40))
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
		Name:         "House of the Dragon 自动订阅",
		FeedURL:      rss.URL,
		Filter:       "House of the Dragon",
		MediaType:    "tv",
		SavePath:     "/downloads/tv",
		ExcludeWords: "官中,无字幕",
	}
	if err := repos.Subscription.Create(t.Context(), sub); err != nil {
		t.Fatal(err)
	}
	queued, err := svc.runOne(t.Context(), sub)
	if err != nil {
		t.Fatal(err)
	}
	if queued != 1 {
		t.Fatalf("queued = %d, want one compatible WEB-DL variant", queued)
	}
	if got := atomic.LoadInt32(&addCalls); got != 1 {
		t.Fatalf("qb add calls = %d, want 1", got)
	}
	if len(addedURLs) != 1 || !strings.Contains(addedURLs[0], "bbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbb") {
		t.Fatalf("added %#v, want compatible 1080p WEB-DL only", addedURLs)
	}
}
