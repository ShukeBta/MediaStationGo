package service

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"go.uber.org/zap"

	"github.com/ShukeBta/MediaStationGo/internal/model"
	"github.com/ShukeBta/MediaStationGo/internal/repository"
)

func TestResourceImportSubscriptionStoppedNotification(t *testing.T) {
	sub := &model.Subscription{Name: "吞噬星空", Filter: "Swallowed Star", MediaType: "anime"}
	job := model.ResourceImportJob{CandidateTitle: "Swallowed Star - 148 Final", PublicError: "无法识别视频集数", SeasonNumber: 1}
	event := resourceImportSubscriptionStoppedNotification(sub, job)
	if event.Type != EventSubscriptionStopped || event.Title != "MediaStationGo 自动追更已停止" {
		t.Fatalf("notification event = %+v", event)
	}
	if !strings.Contains(event.Message, "剧集：吞噬星空") || !strings.Contains(event.Message, "失败集数：E148") || !strings.Contains(event.Message, "无法识别视频集数") || !strings.Contains(event.Message, "Swallowed Star - 148 Final") {
		t.Fatalf("notification message = %q", event.Message)
	}
}

func TestResourceImportSubscriptionCompletedNotification(t *testing.T) {
	sub := &model.Subscription{Name: "吞噬星空", Filter: "Swallowed Star", MediaType: "anime"}
	job := model.ResourceImportJob{CandidateTitle: "Swallowed Star - 148 Final", SeasonNumber: 1, Status: ResourceImportStatusCompleted, ResultJSON: `{"subscription_follow":{"selected_episodes":[148],"moved_episodes":[148],"verified_episodes":[148]}}`}
	event := resourceImportSubscriptionCompletedNotification(sub, job)
	if event.Type != EventSubscriptionCompleted || event.Title != "MediaStationGo 自动追更入库完成" {
		t.Fatalf("notification event = %+v", event)
	}
	if !strings.Contains(event.Message, "剧集：吞噬星空") || !strings.Contains(event.Message, "入库集数：E148") || !strings.Contains(event.Message, "已成功入库") {
		t.Fatalf("notification message = %q", event.Message)
	}
}

func TestFailedResourceImportSubscriptionStopsBeforeAnotherAutomaticRun(t *testing.T) {
	db := newServiceTestDB(t, &model.Subscription{}, &model.ResourceImportJob{})
	repos := repository.New(db)
	svc := NewSubscriptionService(nil, nil, repos, nil, nil, nil)
	sub := model.Subscription{
		Name: "吞噬星空", Filter: "Swallowed Star", FeedURL: "resource-import://default",
		DeliveryMode: subscriptionDeliveryResourceImport, Enabled: true, CatchUpActive: true,
	}
	if err := repos.DB.Create(&sub).Error; err != nil {
		t.Fatal(err)
	}
	job := model.ResourceImportJob{
		UserID: "user", SubscriptionID: sub.ID, SubscriptionFollow: true, LibraryID: "library", LibraryRootID: "root",
		SearchSessionID: "search", CandidateJSON: `{}`, CandidateTitle: "Swallowed Star - 148 Final",
		IdempotencyKey: "failed-follow", Status: ResourceImportStatusFailed, Stage: "failed", PublicError: "无法识别视频集数",
	}
	if err := repos.DB.Create(&job).Error; err != nil {
		t.Fatal(err)
	}
	stopped, err := svc.stopFailedResourceImportSubscription(t.Context(), &sub)
	if err != nil || !stopped {
		t.Fatalf("stop failed subscription = stopped:%v err:%v", stopped, err)
	}
	var stored model.Subscription
	if err := repos.DB.First(&stored, "id = ?", sub.ID).Error; err != nil {
		t.Fatal(err)
	}
	if stored.Enabled || stored.CatchUpActive {
		t.Fatalf("failed subscription stayed runnable: %+v", stored)
	}
}

func TestAcknowledgedResourceImportFailureDoesNotStopReenabledSubscription(t *testing.T) {
	db := newServiceTestDB(t, &model.Subscription{}, &model.ResourceImportJob{})
	repos := repository.New(db)
	svc := NewSubscriptionService(nil, nil, repos, nil, nil, nil)
	acknowledgedAt := time.Now()
	finishedAt := acknowledgedAt.Add(-time.Minute)
	sub := model.Subscription{
		Name: "吞噬星空", Filter: "Swallowed Star", FeedURL: "resource-import://default",
		DeliveryMode: subscriptionDeliveryResourceImport, Enabled: true, CatchUpActive: true, LastRunAt: &acknowledgedAt,
	}
	if err := repos.DB.Create(&sub).Error; err != nil {
		t.Fatal(err)
	}
	job := model.ResourceImportJob{
		UserID: "user", SubscriptionID: sub.ID, SubscriptionFollow: true, LibraryID: "library", LibraryRootID: "root",
		SearchSessionID: "search", CandidateJSON: `{}`, CandidateTitle: "Swallowed Star - 148 Final",
		IdempotencyKey: "acknowledged-failed-follow", Status: ResourceImportStatusFailed, Stage: "failed", FinishedAt: &finishedAt,
	}
	if err := repos.DB.Create(&job).Error; err != nil {
		t.Fatal(err)
	}
	stopped, err := svc.stopFailedResourceImportSubscription(t.Context(), &sub)
	if err != nil || stopped {
		t.Fatalf("acknowledged failed subscription = stopped:%v err:%v", stopped, err)
	}
}

func TestNormalizeSubscriptionDefaultsUsesOrdinaryResourceSearch(t *testing.T) {
	sub := &model.Subscription{DeliveryMode: subscriptionDeliveryResourceImport}

	normalizeSubscriptionDefaults(sub)

	if sub.ResourceSource != "default" || sub.FeedURL != "resource-import://default" {
		t.Fatalf("resource defaults = source %q feed %q", sub.ResourceSource, sub.FeedURL)
	}
}

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
	if deleted.ArchivedAt == nil || deleted.ArchiveReason != "手动删除" {
		t.Fatalf("deleted subscription archive fields = %#v, %q", deleted.ArchivedAt, deleted.ArchiveReason)
	}
	active, err := repos.Subscription.List(t.Context())
	if err != nil {
		t.Fatal(err)
	}
	if len(active) != 0 {
		t.Fatalf("active subscriptions = %#v, want deleted subscription hidden", active)
	}
}

func TestDeleteSubscriptionKeepsResourceImportAuditLink(t *testing.T) {
	db := newServiceTestDB(t, &model.Subscription{}, &model.DownloadTask{}, &model.ResourceImportJob{})
	repos := repository.New(db)
	svc := NewSubscriptionService(nil, zap.NewNop(), repos, nil, nil, NewHub(zap.NewNop()))
	sub := &model.Subscription{
		Name: "Audit Show", Filter: "Audit Show", FeedURL: "resource-import://pansou",
		DeliveryMode: subscriptionDeliveryResourceImport, Enabled: true,
	}
	if err := repos.Subscription.Create(t.Context(), sub); err != nil {
		t.Fatal(err)
	}
	job := model.ResourceImportJob{
		SubscriptionID: sub.ID, SubscriptionFollow: true, UserID: "user", LibraryID: "library", LibraryRootID: "root",
		SearchSessionID: "search", CandidateJSON: `{}`, CandidateTitle: "Audit Show S01E02",
		IdempotencyKey: "audit-job", Attempt: 1, Status: ResourceImportStatusFailed, Stage: "failed",
	}
	if err := db.Create(&job).Error; err != nil {
		t.Fatal(err)
	}

	if err := svc.Delete(t.Context(), sub.ID); err != nil {
		t.Fatal(err)
	}
	var persisted model.ResourceImportJob
	if err := db.First(&persisted, "id = ?", job.ID).Error; err != nil {
		t.Fatal(err)
	}
	if persisted.SubscriptionID != sub.ID {
		t.Fatalf("subscription audit link = %q, want %q", persisted.SubscriptionID, sub.ID)
	}
	history, err := svc.History(t.Context())
	if err != nil {
		t.Fatal(err)
	}
	if len(history) != 1 || history[0].ID != sub.ID || history[0].ArchiveReason != "手动删除" {
		t.Fatalf("history = %#v, want manually deleted subscription", history)
	}
	if len(history[0].ImportJobs) != 1 || history[0].ImportJobs[0].ID != job.ID {
		t.Fatalf("history import jobs = %#v, want %q", history[0].ImportJobs, job.ID)
	}
	restored, err := svc.Restore(t.Context(), sub.ID)
	if err != nil {
		t.Fatal(err)
	}
	if !restored.Enabled || restored.ArchivedAt != nil || restored.ArchiveReason != "" {
		t.Fatalf("restored subscription = %#v", restored)
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
