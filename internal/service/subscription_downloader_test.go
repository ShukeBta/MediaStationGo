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

func TestSubscriptionRunOneDoesNotUseDeletedDownloader(t *testing.T) {
	rss := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "application/rss+xml")
		_, _ = w.Write([]byte(`<?xml version="1.0"?>
<rss><channel>
  <item>
    <title>Deleted Downloader Show S01E01 1080p</title>
    <guid>deleted-downloader-episode-1</guid>
    <link>magnet:?xt=urn:btih:cccccccccccccccccccccccccccccccccccccccc&amp;dn=Deleted+Downloader+Show+S01E01</link>
  </item>
</channel></rss>`))
	}))
	defer rss.Close()

	var qbCalls int32
	qb := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		atomic.AddInt32(&qbCalls, 1)
		switch r.URL.Path {
		case "/api/v2/auth/login":
			_, _ = w.Write([]byte("Ok."))
		case "/api/v2/torrents/info":
			_, _ = w.Write([]byte(`[]`))
		case "/api/v2/torrents/add":
			_, _ = w.Write([]byte("Ok."))
		default:
			http.NotFound(w, r)
		}
	}))
	defer qb.Close()

	db := newServiceTestDB(t, &model.Subscription{}, &model.Setting{}, &model.DownloadTask{}, &model.Media{}, &model.DownloadClient{})
	repos := repository.New(db)
	client := &model.DownloadClient{Name: "qB deleted", Type: "qbittorrent", Host: qb.URL, Username: "admin", Password: "admin", IsDefault: true, Enabled: true}
	if err := repos.DownloadClient.Create(t.Context(), client); err != nil {
		t.Fatal(err)
	}
	if err := repos.Setting.Set(t.Context(), settingDownloadClientsManaged, "true"); err != nil {
		t.Fatal(err)
	}
	if err := repos.DownloadClient.Delete(t.Context(), client.ID); err != nil {
		t.Fatal(err)
	}

	downloads := NewDownloadService(zap.NewNop(), repos, NewHub(zap.NewNop()), nil)
	svc := NewSubscriptionService(nil, zap.NewNop(), repos, downloads, nil, NewHub(zap.NewNop()))
	sub := &model.Subscription{
		Name:      "Deleted Downloader Show 自动订阅",
		FeedURL:   rss.URL,
		Filter:    "Deleted Downloader Show",
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
	if queued != 0 {
		t.Fatalf("queued = %d, want 0 when default downloader was deleted", queued)
	}
	if got := atomic.LoadInt32(&qbCalls); got != 0 {
		t.Fatalf("qB calls = %d, want 0 after downloader deletion", got)
	}
	rows, err := repos.Download.List(t.Context())
	if err != nil {
		t.Fatal(err)
	}
	if len(rows) != 0 {
		t.Fatalf("download rows = %d, want 0", len(rows))
	}
}
