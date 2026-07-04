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

func TestDeleteSubscriptionRemovesDownloaderTaskAndSeenState(t *testing.T) {
	const title = "Delete Subscription Show S01E01 1080p"
	const hash = "abcdef1234567890abcdef1234567890abcdef12"
	var deleteCalls atomic.Int32
	qb := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/api/v2/auth/login":
			_, _ = w.Write([]byte("Ok."))
		case "/api/v2/torrents/info":
			_, _ = w.Write([]byte(`[{"hash":"` + hash + `","name":"` + title + `","state":"downloading","progress":0.2}]`))
		case "/api/v2/torrents/delete":
			deleteCalls.Add(1)
			if got := r.FormValue("deleteFiles"); got != "false" {
				t.Fatalf("deleteFiles = %q, want false", got)
			}
			_, _ = w.Write([]byte("Ok."))
		default:
			http.NotFound(w, r)
		}
	}))
	defer qb.Close()

	db := newServiceTestDB(t, &model.Subscription{}, &model.DownloadTask{}, &model.DownloadClient{}, &model.Setting{})
	repos := repository.New(db)
	configureTestDefaultQB(t, repos, qb.URL)
	downloads := NewDownloadService(zap.NewNop(), repos, NewHub(zap.NewNop()), nil)
	if err := downloads.ReloadConfig(t.Context()); err != nil {
		t.Fatal(err)
	}
	svc := NewSubscriptionService(nil, zap.NewNop(), repos, downloads, nil, NewHub(zap.NewNop()))
	sub := &model.Subscription{Name: "Delete Subscription Show 自动订阅", Filter: "Delete Subscription Show", FeedURL: "https://rss.example/feed", UserID: "u1", SavePath: "/downloads/tv"}
	if err := repos.Subscription.Create(t.Context(), sub); err != nil {
		t.Fatal(err)
	}
	task := &model.DownloadTask{
		UserID:         "u1",
		SubscriptionID: sub.ID,
		Source:         "qbittorrent",
		URL:            "https://pt.example/download?id=1",
		Title:          title,
		SavePath:       "/downloads/tv",
		Status:         "downloading",
		Progress:       0.2,
	}
	if err := repos.Download.Create(t.Context(), task); err != nil {
		t.Fatal(err)
	}
	if err := repos.Setting.Set(t.Context(), "subscription."+sub.ID+".seen", "guid-1"); err != nil {
		t.Fatal(err)
	}

	if err := svc.Delete(t.Context(), sub.ID); err != nil {
		t.Fatalf("delete subscription: %v", err)
	}
	if got := deleteCalls.Load(); got != 1 {
		t.Fatalf("qb delete calls = %d, want 1", got)
	}
	var updated model.DownloadTask
	if err := db.Where("id = ?", task.ID).First(&updated).Error; err != nil {
		t.Fatal(err)
	}
	if updated.Status != "deleted" {
		t.Fatalf("download task status = %q, want deleted", updated.Status)
	}
	seen, err := repos.Setting.Get(t.Context(), "subscription."+sub.ID+".seen")
	if err != nil {
		t.Fatal(err)
	}
	if seen != "" {
		t.Fatalf("seen state = %q, want cleared", seen)
	}
	var count int64
	if err := db.Model(&model.Subscription{}).Where("id = ?", sub.ID).Count(&count).Error; err != nil {
		t.Fatal(err)
	}
	if count != 0 {
		t.Fatalf("active subscription count = %d, want 0", count)
	}
	var deleted model.Subscription
	if err := db.Unscoped().Where("id = ?", sub.ID).First(&deleted).Error; err != nil {
		t.Fatal(err)
	}
	if deleted.Enabled {
		t.Fatal("deleted subscription stayed enabled; active legacy compatibility would show it again")
	}
	active, err := repos.Subscription.List(t.Context())
	if err != nil {
		t.Fatal(err)
	}
	if len(active) != 0 {
		t.Fatalf("active subscriptions = %#v, want deleted subscription hidden", active)
	}
}

func TestDeletedDownloadTaskDoesNotBlockSubscriptionReadd(t *testing.T) {
	if downloadTaskBlocksReadd("deleted") {
		t.Fatal("deleted download task must not block subscription re-add")
	}
	if downloadTaskBlocksReadd("removed") {
		t.Fatal("removed download task must not block subscription re-add")
	}
}

func TestListIncludesEnabledSoftDeletedActiveSubscription(t *testing.T) {
	db := newServiceTestDB(t, &model.Subscription{})
	repos := repository.New(db)
	sub := &model.Subscription{
		Name:    "Hidden Active 自动订阅",
		FeedURL: "site-search://search?keyword=Hidden%20Active",
		Filter:  "Hidden Active",
		Enabled: true,
	}
	if err := repos.Subscription.Create(t.Context(), sub); err != nil {
		t.Fatal(err)
	}
	if err := db.Where("id = ?", sub.ID).Delete(&model.Subscription{}).Error; err != nil {
		t.Fatal(err)
	}

	active, err := repos.Subscription.List(t.Context())
	if err != nil {
		t.Fatal(err)
	}
	if len(active) != 1 || active[0].ID != sub.ID {
		t.Fatalf("active subscriptions = %#v, want soft-deleted enabled subscription recovered", active)
	}
}

func TestDeleteRecoveredSoftDeletedSubscriptionClearsSeenAndHidesIt(t *testing.T) {
	db := newServiceTestDB(t, &model.Subscription{}, &model.Setting{}, &model.DownloadTask{})
	repos := repository.New(db)
	svc := NewSubscriptionService(nil, zap.NewNop(), repos, nil, nil, NewHub(zap.NewNop()))
	sub := &model.Subscription{
		Name:    "Recovered Hidden 自动订阅",
		FeedURL: "site-search://search?keyword=Recovered%20Hidden",
		Filter:  "Recovered Hidden",
		Enabled: true,
	}
	if err := repos.Subscription.Create(t.Context(), sub); err != nil {
		t.Fatal(err)
	}
	if err := repos.Setting.Set(t.Context(), "subscription."+sub.ID+".seen", "old-guid"); err != nil {
		t.Fatal(err)
	}
	if err := db.Where("id = ?", sub.ID).Delete(&model.Subscription{}).Error; err != nil {
		t.Fatal(err)
	}

	if err := svc.Delete(t.Context(), sub.ID); err != nil {
		t.Fatal(err)
	}
	active, err := repos.Subscription.List(t.Context())
	if err != nil {
		t.Fatal(err)
	}
	if len(active) != 0 {
		t.Fatalf("active subscriptions = %#v, want recovered deleted subscription hidden", active)
	}
	seen, err := repos.Setting.Get(t.Context(), "subscription."+sub.ID+".seen")
	if err != nil {
		t.Fatal(err)
	}
	if seen != "" {
		t.Fatalf("seen state = %q, want cleared", seen)
	}
}
