package service

import (
	"testing"
	"time"

	"go.uber.org/zap"

	"github.com/ShukeBta/MediaStationGo/internal/model"
	"github.com/ShukeBta/MediaStationGo/internal/repository"
)

func TestSyncDownloadTaskProgressSkipsUnchangedCompletedTask(t *testing.T) {
	db := newServiceTestDB(t, &model.DownloadTask{})
	repos := repository.New(db)
	task := &model.DownloadTask{
		Source:   "qbittorrent",
		URL:      "magnet:?xt=urn:btih:test",
		Title:    "Already.Done.S01E01",
		SavePath: "/downloads",
		Status:   "completed",
		Progress: 1,
	}
	if err := repos.Download.Create(t.Context(), task); err != nil {
		t.Fatal(err)
	}
	var before model.DownloadTask
	if err := db.First(&before, "id = ?", task.ID).Error; err != nil {
		t.Fatal(err)
	}
	time.Sleep(10 * time.Millisecond)
	svc := NewDownloadService(zap.NewNop(), repos, NewHub(zap.NewNop()), nil)
	svc.syncDownloadTaskProgress(t.Context(), QBitTorrent{
		Name:     task.Title,
		Progress: 1,
		State:    "completed",
	}, tasksByIdentity([]model.DownloadTask{before}))
	var after model.DownloadTask
	if err := db.First(&after, "id = ?", task.ID).Error; err != nil {
		t.Fatal(err)
	}
	if !after.UpdatedAt.Equal(before.UpdatedAt) {
		t.Fatalf("unchanged completed torrent touched updated_at: before=%s after=%s", before.UpdatedAt, after.UpdatedAt)
	}
}

func TestSyncDownloadTaskProgressMatchesSeasonFolderTorrentName(t *testing.T) {
	db := newServiceTestDB(t, &model.DownloadTask{})
	repos := repository.New(db)
	task := &model.DownloadTask{
		Source:   "qbittorrent",
		URL:      "magnet:?xt=urn:btih:test",
		Title:    "The First Jasmine S01E01 1080p TX WEB-DL AAC2.0 H.264-MWeb",
		SavePath: "/downloads/欧美剧",
		Status:   "queued",
		Progress: 0.5,
	}
	if err := repos.Download.Create(t.Context(), task); err != nil {
		t.Fatal(err)
	}

	svc := NewDownloadService(zap.NewNop(), repos, NewHub(zap.NewNop()), nil)
	svc.syncDownloadTaskProgress(t.Context(), QBitTorrent{
		Name:     "The.First.Jasmine.S01.1080p.TX.WEB-DL.AAC2.0.H.264-MWeb",
		Progress: 1,
		State:    "stalledUP",
	}, tasksByTorrentIdentity([]model.DownloadTask{*task}))

	var after model.DownloadTask
	if err := db.First(&after, "id = ?", task.ID).Error; err != nil {
		t.Fatal(err)
	}
	if after.Status != "completed" || after.Progress != 1 {
		t.Fatalf("task completion = %s/%v, want completed/1", after.Status, after.Progress)
	}
}

func TestProcessDownloadSnapshotQueuesCompletedPendingTaskOnFirstSnapshot(t *testing.T) {
	db := newServiceTestDB(t, &model.DownloadTask{}, &model.Setting{})
	repos := repository.New(db)
	task := &model.DownloadTask{
		Source:   "qbittorrent",
		URL:      "magnet:?xt=urn:btih:test",
		Title:    "Blades of the Guardians S02E01 1080p TX WEB-DL AAC2.0 H.264-MWeb",
		SavePath: "/downloads/欧美剧",
		Status:   "queued",
		Progress: 0,
	}
	if err := repos.Download.Create(t.Context(), task); err != nil {
		t.Fatal(err)
	}
	if err := repos.Setting.Set(t.Context(), "organizer.auto_after_download", "true"); err != nil {
		t.Fatal(err)
	}

	svc := NewDownloadService(zap.NewNop(), repos, NewHub(zap.NewNop()), nil)
	svc.processDownloadSnapshot(t.Context(), []QBitTorrent{{
		Hash:     "quickdone",
		Name:     "Blades.of.the.Guardians.S02.1080p.TX.WEB-DL.AAC2.0.H.264-MWeb",
		Progress: 1,
		State:    "stalledUP",
	}}, tasksByTorrentIdentity([]model.DownloadTask{*task}))

	if got := len(svc.organizeQueue); got != 1 {
		t.Fatalf("queued completed organize jobs = %d, want 1", got)
	}
}

func TestProcessDownloadSnapshotDoesNotQueueActiveDownloadAtFullProgress(t *testing.T) {
	db := newServiceTestDB(t, &model.DownloadTask{}, &model.Setting{})
	repos := repository.New(db)
	task := &model.DownloadTask{
		Source:   "qbittorrent",
		URL:      "magnet:?xt=urn:btih:test",
		Title:    "Still Downloading S01E01",
		SavePath: "/downloads/欧美剧",
		Status:   "downloading",
		Progress: 0.99,
	}
	if err := repos.Download.Create(t.Context(), task); err != nil {
		t.Fatal(err)
	}
	if err := repos.Setting.Set(t.Context(), "organizer.auto_after_download", "true"); err != nil {
		t.Fatal(err)
	}

	svc := NewDownloadService(zap.NewNop(), repos, NewHub(zap.NewNop()), nil)
	svc.processDownloadSnapshot(t.Context(), []QBitTorrent{{
		Hash:     "notdone",
		Name:     "Still.Downloading.S01E01",
		Progress: 1,
		State:    "downloading",
	}}, tasksByTorrentIdentity([]model.DownloadTask{*task}))

	if got := len(svc.organizeQueue); got != 0 {
		t.Fatalf("queued active download organize jobs = %d, want 0", got)
	}
	var after model.DownloadTask
	if err := db.First(&after, "id = ?", task.ID).Error; err != nil {
		t.Fatal(err)
	}
	if after.Status == "completed" {
		t.Fatalf("active download status = %q, should not be completed", after.Status)
	}
}

func TestProcessDownloadSnapshotDoesNotQueueFullProgressWithoutQBitState(t *testing.T) {
	db := newServiceTestDB(t, &model.DownloadTask{}, &model.Setting{})
	repos := repository.New(db)
	task := &model.DownloadTask{
		Source:   "qbittorrent",
		URL:      "magnet:?xt=urn:btih:test",
		Title:    "Missing State S01E01",
		SavePath: "/downloads/欧美剧",
		Status:   "downloading",
		Progress: 0.99,
	}
	if err := repos.Download.Create(t.Context(), task); err != nil {
		t.Fatal(err)
	}
	if err := repos.Setting.Set(t.Context(), "organizer.auto_after_download", "true"); err != nil {
		t.Fatal(err)
	}

	svc := NewDownloadService(zap.NewNop(), repos, NewHub(zap.NewNop()), nil)
	svc.processDownloadSnapshot(t.Context(), []QBitTorrent{{
		Hash:     "missing-state",
		Name:     "Missing.State.S01E01",
		Progress: 1,
	}}, tasksByTorrentIdentity([]model.DownloadTask{*task}))

	if got := len(svc.organizeQueue); got != 0 {
		t.Fatalf("queued full-progress torrent without state = %d, want 0", got)
	}
	var after model.DownloadTask
	if err := db.First(&after, "id = ?", task.ID).Error; err != nil {
		t.Fatal(err)
	}
	if after.Status == "completed" {
		t.Fatalf("missing-state torrent status = %q, should not be completed", after.Status)
	}
}

func TestProcessDownloadSnapshotDoesNotTrustCompletionOnForActiveDownload(t *testing.T) {
	db := newServiceTestDB(t, &model.DownloadTask{}, &model.Setting{})
	repos := repository.New(db)
	task := &model.DownloadTask{
		Source:   "qbittorrent",
		URL:      "magnet:?xt=urn:btih:test",
		Title:    "Still Downloading With Completion Timestamp S01E01",
		SavePath: "/downloads/欧美剧",
		Status:   "downloading",
		Progress: 0.5,
	}
	if err := repos.Download.Create(t.Context(), task); err != nil {
		t.Fatal(err)
	}
	if err := repos.Setting.Set(t.Context(), "organizer.auto_after_download", "true"); err != nil {
		t.Fatal(err)
	}

	svc := NewDownloadService(zap.NewNop(), repos, NewHub(zap.NewNop()), nil)
	svc.processDownloadSnapshot(t.Context(), []QBitTorrent{{
		Hash:         "notdone-completion-on",
		Name:         "Still.Downloading.With.Completion.Timestamp.S01E01",
		Progress:     0.5,
		State:        "downloading",
		CompletionOn: time.Now().Unix(),
	}}, tasksByTorrentIdentity([]model.DownloadTask{*task}))

	if got := len(svc.organizeQueue); got != 0 {
		t.Fatalf("queued active download organize jobs = %d, want 0", got)
	}
	var after model.DownloadTask
	if err := db.First(&after, "id = ?", task.ID).Error; err != nil {
		t.Fatal(err)
	}
	if after.Status == "completed" || after.Progress >= 1 {
		t.Fatalf("active download mutated to completed state: status=%q progress=%v", after.Status, after.Progress)
	}
}

func TestProcessDownloadSnapshotSkipsUntrackedCompletedTorrentOnFirstSnapshot(t *testing.T) {
	db := newServiceTestDB(t, &model.DownloadTask{}, &model.Setting{})
	repos := repository.New(db)
	svc := NewDownloadService(zap.NewNop(), repos, NewHub(zap.NewNop()), nil)

	svc.processDownloadSnapshot(t.Context(), []QBitTorrent{{
		Hash:         "historydone",
		Name:         "Large.History.Pack.2026.1080p",
		Progress:     1,
		State:        "stalledUP",
		CompletionOn: time.Now().Unix(),
	}}, tasksByTorrentIdentity(nil))

	if got := len(svc.organizeQueue); got != 0 {
		t.Fatalf("queued untracked historical torrents = %d, want 0", got)
	}
}
