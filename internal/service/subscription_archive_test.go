package service

import (
	"net/http"
	"net/http/httptest"
	"sync/atomic"
	"testing"
	"time"

	"go.uber.org/zap"

	"github.com/ShukeBta/MediaStationGo/internal/model"
	"github.com/ShukeBta/MediaStationGo/internal/repository"
)

func TestSubscriptionRunOneArchivesCompletedMovieRSS(t *testing.T) {
	rss := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "application/rss+xml")
		_, _ = w.Write([]byte(`<?xml version="1.0"?>
<rss><channel>
  <item>
    <title>Dune 2021 1080p WEB-DL</title>
    <guid>dune-1080-web</guid>
    <link>magnet:?xt=urn:btih:dddddddddddddddddddddddddddddddddddddddd&amp;dn=Dune+2021+1080p+WEB-DL</link>
  </item>
</channel></rss>`))
	}))
	defer rss.Close()

	var addCalls int32
	var added bool
	qb := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/api/v2/auth/login":
			_, _ = w.Write([]byte("Ok."))
		case "/api/v2/torrents/info":
			if added {
				_, _ = w.Write([]byte(`[{"hash":"dunehash","name":"Dune 2021 1080p WEB-DL","state":"downloading","progress":0.1}]`))
				return
			}
			_, _ = w.Write([]byte(`[]`))
		case "/api/v2/torrents/add":
			added = true
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
		Name:      "Dune 自动订阅",
		FeedURL:   rss.URL,
		Filter:    "Dune 2021",
		MediaType: "movie",
		SavePath:  "/downloads/movies",
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
	active, err := repos.Subscription.List(t.Context())
	if err != nil {
		t.Fatal(err)
	}
	if len(active) != 0 {
		t.Fatalf("active subscriptions = %d, want 0 after completion", len(active))
	}
	history, err := repos.Subscription.History(t.Context())
	if err != nil {
		t.Fatal(err)
	}
	if len(history) != 1 || history[0].ArchivedAt == nil {
		t.Fatalf("history = %#v, want one archived subscription", history)
	}
}

func TestSubscriptionArchiveCompletedSingleEpisodeTV(t *testing.T) {
	db := newServiceTestDB(t, &model.Subscription{})
	repos := repository.New(db)
	svc := NewSubscriptionService(nil, zap.NewNop(), repos, nil, nil, NewHub(zap.NewNop()))
	sub := &model.Subscription{
		Name:      "Some Show S01E01 自动订阅",
		FeedURL:   "site-search://search?keyword=Some%20Show%20S01E01",
		Filter:    "Some Show S01E01",
		MediaType: "tv",
		Enabled:   true,
	}
	if err := repos.Subscription.Create(t.Context(), sub); err != nil {
		t.Fatal(err)
	}

	if err := svc.archiveCompletedSubscription(t.Context(), sub, LocalAvailability{
		DownloadedEpisodes: 1,
		LocalMediaCount:    1,
		InLibrary:          true,
		ExistingEpisodeKeys: map[string]struct{}{
			episodeKey(1, 1): {},
		},
	}); err != nil {
		t.Fatal(err)
	}
	active, err := repos.Subscription.List(t.Context())
	if err != nil {
		t.Fatal(err)
	}
	if len(active) != 0 {
		t.Fatalf("active subscriptions = %d, want 0", len(active))
	}
	history, err := repos.Subscription.History(t.Context())
	if err != nil {
		t.Fatal(err)
	}
	if len(history) != 1 || history[0].ArchiveReason == "" {
		t.Fatalf("history = %#v, want archived single episode", history)
	}
}

func TestSubscriptionArchiveKeepsGenericUnknownTotalSeriesActive(t *testing.T) {
	sub := &model.Subscription{
		Name:      "Some Show 自动订阅",
		Filter:    "Some Show",
		MediaType: "tv",
	}
	availability := LocalAvailability{
		DownloadedEpisodes: 1,
		LocalMediaCount:    1,
		InLibrary:          true,
		ExistingEpisodeKeys: map[string]struct{}{
			episodeKey(1, 1): {},
		},
	}
	if subscriptionShouldArchive(sub, availability) {
		t.Fatal("generic series with unknown total should stay active for incremental episodes")
	}
}

func TestSubscriptionArchiveKeepsPartialSeriesWithParentRowActive(t *testing.T) {
	sub := &model.Subscription{
		Name:   "南部档案 自动订阅",
		Filter: "南部档案",
	}
	availability := LocalAvailability{
		DownloadedEpisodes: 6,
		TotalEpisodes:      1,
		LocalMediaCount:    7,
		InLibrary:          true,
		HasSeriesPack:      true,
		ExistingEpisodeKeys: map[string]struct{}{
			episodeKey(1, 1): {},
			episodeKey(1, 2): {},
			episodeKey(1, 3): {},
			episodeKey(1, 4): {},
			episodeKey(1, 5): {},
			episodeKey(1, 6): {},
		},
	}
	if subscriptionShouldArchive(sub, availability) {
		t.Fatal("partial series with a parent/collection row should stay active")
	}
}

func TestSubscriptionArchiveKeepsWashSubscriptionActive(t *testing.T) {
	db := newServiceTestDB(t, &model.Subscription{})
	repos := repository.New(db)
	svc := NewSubscriptionService(nil, zap.NewNop(), repos, nil, nil, NewHub(zap.NewNop()))
	sub := &model.Subscription{
		Name:        "Dune 自动订阅",
		FeedURL:     "site-search://search?keyword=Dune",
		Filter:      "Dune 2021",
		MediaType:   "movie",
		Resolution:  "2160p",
		WashEnabled: true,
		Enabled:     true,
	}
	if err := repos.Subscription.Create(t.Context(), sub); err != nil {
		t.Fatal(err)
	}

	if err := svc.archiveCompletedSubscription(t.Context(), sub, LocalAvailability{
		DownloadedEpisodes: 1,
		LocalMediaCount:    1,
		InLibrary:          true,
	}); err != nil {
		t.Fatal(err)
	}
	active, err := repos.Subscription.List(t.Context())
	if err != nil {
		t.Fatal(err)
	}
	if len(active) != 1 {
		t.Fatalf("active subscriptions = %d, want wash subscription to stay active", len(active))
	}
	history, err := repos.Subscription.History(t.Context())
	if err != nil {
		t.Fatal(err)
	}
	if len(history) != 0 {
		t.Fatalf("history subscriptions = %d, want 0", len(history))
	}
}

func TestRestoreArchivedSubscriptionReturnsToActiveAndClearsSeenState(t *testing.T) {
	db := newServiceTestDB(t, &model.Subscription{}, &model.Setting{})
	repos := repository.New(db)
	svc := NewSubscriptionService(nil, zap.NewNop(), repos, nil, nil, NewHub(zap.NewNop()))
	sub := &model.Subscription{
		Name:          "南部档案 自动订阅",
		FeedURL:       "https://rss.example/feed",
		Filter:        "南部档案",
		MediaType:     "tv",
		TotalEpisodes: 33,
	}
	if err := repos.Subscription.Create(t.Context(), sub); err != nil {
		t.Fatal(err)
	}
	archivedAt := time.Now()
	if err := repos.Subscription.Archive(t.Context(), sub.ID, "已下载 1/33 集，缺 33 集", archivedAt); err != nil {
		t.Fatal(err)
	}
	if err := repos.Setting.Set(t.Context(), "subscription."+sub.ID+".seen", "old-guid"); err != nil {
		t.Fatal(err)
	}
	restored, err := svc.Restore(t.Context(), sub.ID)
	if err != nil {
		t.Fatal(err)
	}
	if restored.ArchivedAt != nil || restored.ArchiveReason != "" || !restored.Enabled {
		t.Fatalf("restored subscription not active: archived=%v reason=%q enabled=%v", restored.ArchivedAt, restored.ArchiveReason, restored.Enabled)
	}
	if restored.TotalEpisodes != 0 {
		t.Fatalf("restored total_episodes = %d, want 0 so it gets recomputed from authoritative metadata", restored.TotalEpisodes)
	}
	active, err := repos.Subscription.List(t.Context())
	if err != nil {
		t.Fatal(err)
	}
	if len(active) != 1 || active[0].ID != sub.ID {
		t.Fatalf("active subscriptions = %#v, want restored subscription", active)
	}
	history, err := repos.Subscription.History(t.Context())
	if err != nil {
		t.Fatal(err)
	}
	if len(history) != 0 {
		t.Fatalf("history subscriptions = %d, want 0 after restore", len(history))
	}
	seen, err := repos.Setting.Get(t.Context(), "subscription."+sub.ID+".seen")
	if err != nil {
		t.Fatal(err)
	}
	if seen != "" {
		t.Fatalf("seen state = %q, want cleared", seen)
	}
}

func TestRestoreSoftDeletedArchivedSubscriptionReturnsToActive(t *testing.T) {
	db := newServiceTestDB(t, &model.Subscription{}, &model.Setting{})
	repos := repository.New(db)
	svc := NewSubscriptionService(nil, zap.NewNop(), repos, nil, nil, NewHub(zap.NewNop()))
	sub := &model.Subscription{
		Name:          "Legacy Hidden History 自动订阅",
		FeedURL:       "https://rss.example/feed",
		Filter:        "Legacy Hidden History",
		MediaType:     "tv",
		TotalEpisodes: 12,
	}
	if err := repos.Subscription.Create(t.Context(), sub); err != nil {
		t.Fatal(err)
	}
	archivedAt := time.Now()
	if err := repos.Subscription.Archive(t.Context(), sub.ID, "订阅完成：12/12", archivedAt); err != nil {
		t.Fatal(err)
	}
	if err := db.Where("id = ?", sub.ID).Delete(&model.Subscription{}).Error; err != nil {
		t.Fatal(err)
	}

	restored, err := svc.Restore(t.Context(), sub.ID)
	if err != nil {
		t.Fatal(err)
	}
	if restored.ArchivedAt != nil || restored.ArchiveReason != "" || !restored.Enabled || restored.TotalEpisodes != 0 {
		t.Fatalf("restored subscription not reset: %#v", restored)
	}
	active, err := repos.Subscription.List(t.Context())
	if err != nil {
		t.Fatal(err)
	}
	if len(active) != 1 || active[0].ID != sub.ID {
		t.Fatalf("active subscriptions = %#v, want restored legacy subscription", active)
	}
	var deletedCount int64
	if err := db.Unscoped().Model(&model.Subscription{}).
		Where("id = ? AND deleted_at IS NOT NULL", sub.ID).
		Count(&deletedCount).Error; err != nil {
		t.Fatal(err)
	}
	if deletedCount != 0 {
		t.Fatal("restored subscription kept deleted_at set")
	}
}

func TestSubscriptionHistoryIncludesExistingResourceImportAttempts(t *testing.T) {
	db := newServiceTestDB(t, &model.Subscription{}, &model.ResourceImportJob{})
	repos := repository.New(db)
	archivedAt := time.Now().Add(-time.Hour)
	sub := model.Subscription{
		Name: "凡人修仙传", FeedURL: "resource-import://pansou", DeliveryMode: subscriptionDeliveryResourceImport,
		Filter: "凡人修仙传", Enabled: false, ArchivedAt: &archivedAt,
	}
	if err := db.Create(&sub).Error; err != nil {
		t.Fatal(err)
	}
	rows := []model.ResourceImportJob{
		{SubscriptionID: sub.ID, SubscriptionFollow: true, UserID: "user", LibraryID: "lib", LibraryRootID: "root", SearchSessionID: "search-1", CandidateIndex: 0, CandidateJSON: `{}`, CandidateSource: "default", TitleClass: "cumulative_pack", IdempotencyKey: "attempt-1", Attempt: 1, Status: ResourceImportStatusFailed, Stage: "failed", Outcome: "rejected", PublicError: "unknown video", ResultJSON: `{"subscription_follow":{"selected_episodes":[115,116],"source_block":{"reason":"invalid_episode_layout"}}}`},
		{SubscriptionID: sub.ID, SubscriptionFollow: true, UserID: "user", LibraryID: "lib", LibraryRootID: "root", SearchSessionID: "search-1", CandidateIndex: 0, CandidateJSON: `{}`, CandidateSource: "default", TitleClass: "range", IdempotencyKey: "attempt-2", Attempt: 2, RetryOfJobID: "attempt-one", Status: ResourceImportStatusCompleted, Stage: "completed", Outcome: "imported", ResultJSON: `{"subscription_follow":{"selected_episodes":[115,116],"moved_episodes":[115,116],"verified_episodes":[115,116],"scan_added":2}}`},
		{SubscriptionID: sub.ID, SubscriptionFollow: false, UserID: "user", LibraryID: "lib", LibraryRootID: "root", SearchSessionID: "manual-search", CandidateIndex: 0, CandidateJSON: `{}`, IdempotencyKey: "manual-import", Attempt: 1, Status: ResourceImportStatusCompleted, Stage: "completed", Outcome: "imported"},
	}
	if err := db.Create(&rows).Error; err != nil {
		t.Fatal(err)
	}
	svc := NewSubscriptionService(nil, nil, repos, nil, nil, nil)
	history, err := svc.History(t.Context())
	if err != nil {
		t.Fatal(err)
	}
	if len(history) != 1 || len(history[0].ImportJobs) != 2 {
		t.Fatalf("history = %+v", history)
	}
	if history[0].ImportJobs[0].Attempt != 2 || history[0].ImportJobs[1].Outcome != "rejected" {
		t.Fatalf("attempts = %+v", history[0].ImportJobs)
	}
	if got := history[0].ImportJobs[0]; got.CandidateSource != "default" || got.CandidateGranularity != "range" || len(got.MovedEpisodes) != 2 || len(got.VerifiedEpisodes) != 2 || got.ScanAdded != 2 {
		t.Fatalf("completed audit projection = %+v", got)
	}
	if got := history[0].ImportJobs[1]; got.BlockReason != "invalid_episode_layout" || len(got.SelectedEpisodes) != 2 {
		t.Fatalf("rejected audit projection = %+v", got)
	}
}

func TestSubscriptionHistoryGroupsEquivalentArchivedRules(t *testing.T) {
	db := newServiceTestDB(t, &model.Subscription{}, &model.ResourceImportJob{})
	repos := repository.New(db)
	archivedAt := time.Now()
	first := model.Subscription{Name: "吞噬星空", Filter: "Swallowed Star", FeedURL: "resource-import://default", DeliveryMode: subscriptionDeliveryResourceImport, LibraryID: "library", LibraryRootID: "root", SeasonNumber: 1, ArchivedAt: &archivedAt}
	second := first
	second.ID = "second-history"
	second.ArchivedAt = ptrTime(archivedAt.Add(time.Minute))
	if err := db.Create(&first).Error; err != nil {
		t.Fatal(err)
	}
	if err := db.Create(&second).Error; err != nil {
		t.Fatal(err)
	}
	jobs := []model.ResourceImportJob{
		resourceSubscriptionHistoryJob(first.ID, "first-job"),
		resourceSubscriptionHistoryJob(second.ID, "second-job"),
	}
	if err := db.Create(&jobs).Error; err != nil {
		t.Fatal(err)
	}
	history, err := NewSubscriptionService(nil, nil, repos, nil, nil, nil).History(t.Context())
	if err != nil {
		t.Fatal(err)
	}
	if len(history) != 1 || len(history[0].HistoryIDs) != 2 || len(history[0].ImportJobs) != 2 {
		t.Fatalf("grouped history = %+v", history)
	}
}

func TestReuseArchivedResourceSubscriptionMergesHistory(t *testing.T) {
	db := newServiceTestDB(t, &model.Subscription{}, &model.Setting{}, &model.ResourceImportJob{})
	repos := repository.New(db)
	archivedAt := time.Now()
	old := model.Subscription{Name: "吞噬星空", Filter: "Swallowed Star", FeedURL: "resource-import://default", DeliveryMode: subscriptionDeliveryResourceImport, LibraryID: "library", LibraryRootID: "root", SeasonNumber: 1, ArchivedAt: &archivedAt}
	duplicate := old
	duplicate.ID = "duplicate-history"
	duplicate.ArchivedAt = ptrTime(archivedAt.Add(time.Minute))
	if err := db.Create(&old).Error; err != nil {
		t.Fatal(err)
	}
	if err := db.Create(&duplicate).Error; err != nil {
		t.Fatal(err)
	}
	jobs := []model.ResourceImportJob{
		resourceSubscriptionHistoryJob(old.ID, "old-job"),
		resourceSubscriptionHistoryJob(duplicate.ID, "duplicate-job"),
	}
	if err := db.Create(&jobs).Error; err != nil {
		t.Fatal(err)
	}
	desired := model.Subscription{Name: "吞噬星空", Filter: "Swallowed Star", FeedURL: "resource-import://default?alias=吞噬星空", DeliveryMode: subscriptionDeliveryResourceImport, LibraryID: "library", LibraryRootID: "root", SeasonNumber: 1, Enabled: true, PollIntervalMinutes: 15}
	reused, err := NewSubscriptionService(nil, nil, repos, nil, nil, nil).reuseArchivedResourceImportSubscription(t.Context(), &desired)
	if err != nil || !reused {
		t.Fatalf("reused=%v err=%v", reused, err)
	}
	if desired.ID != duplicate.ID || desired.ArchivedAt != nil || !desired.Enabled {
		t.Fatalf("restored rule = %+v", desired)
	}
	var subscriptions []model.Subscription
	if err := db.Unscoped().Find(&subscriptions).Error; err != nil {
		t.Fatal(err)
	}
	if len(subscriptions) != 1 || subscriptions[0].ID != duplicate.ID {
		t.Fatalf("subscription rows = %+v", subscriptions)
	}
	var mergedJobs []model.ResourceImportJob
	if err := db.Find(&mergedJobs).Error; err != nil {
		t.Fatal(err)
	}
	if len(mergedJobs) != 2 || mergedJobs[0].SubscriptionID != duplicate.ID || mergedJobs[1].SubscriptionID != duplicate.ID {
		t.Fatalf("merged jobs = %+v", mergedJobs)
	}
}

func TestPurgeSubscriptionHistoryRemovesAuditButNotOtherRules(t *testing.T) {
	db := newServiceTestDB(t, &model.Subscription{}, &model.ResourceImportJob{})
	repos := repository.New(db)
	archivedAt := time.Now()
	target := model.Subscription{Name: "吞噬星空", Filter: "Swallowed Star", FeedURL: "resource-import://default", DeliveryMode: subscriptionDeliveryResourceImport, LibraryID: "library", LibraryRootID: "root", SeasonNumber: 1, ArchivedAt: &archivedAt}
	other := model.Subscription{Name: "遮天", Filter: "遮天", FeedURL: "resource-import://default", DeliveryMode: subscriptionDeliveryResourceImport, LibraryID: "library", LibraryRootID: "root", SeasonNumber: 1, ArchivedAt: &archivedAt}
	if err := db.Create(&target).Error; err != nil {
		t.Fatal(err)
	}
	if err := db.Create(&other).Error; err != nil {
		t.Fatal(err)
	}
	if err := db.Create(&[]model.ResourceImportJob{resourceSubscriptionHistoryJob(target.ID, "target-job"), resourceSubscriptionHistoryJob(other.ID, "other-job")}).Error; err != nil {
		t.Fatal(err)
	}
	if err := NewSubscriptionService(nil, nil, repos, nil, nil, nil).PurgeHistory(t.Context(), target.ID); err != nil {
		t.Fatal(err)
	}
	var rows []model.Subscription
	if err := db.Unscoped().Find(&rows).Error; err != nil {
		t.Fatal(err)
	}
	if len(rows) != 1 || rows[0].ID != other.ID {
		t.Fatalf("subscription rows = %+v", rows)
	}
	var jobs []model.ResourceImportJob
	if err := db.Find(&jobs).Error; err != nil {
		t.Fatal(err)
	}
	if len(jobs) != 1 || jobs[0].SubscriptionID != other.ID {
		t.Fatalf("audit rows = %+v", jobs)
	}
}

func ptrTime(value time.Time) *time.Time { return &value }

func resourceSubscriptionHistoryJob(subscriptionID, key string) model.ResourceImportJob {
	return model.ResourceImportJob{
		SubscriptionID: subscriptionID, SubscriptionFollow: true, UserID: "user", LibraryID: "library", LibraryRootID: "root",
		SearchSessionID: "search", CandidateIndex: 0, CandidateJSON: `{}`, IdempotencyKey: key,
		Status: ResourceImportStatusCompleted, Stage: "completed", Outcome: "imported",
	}
}
