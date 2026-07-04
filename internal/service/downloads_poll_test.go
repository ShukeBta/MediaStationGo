package service

import (
	"testing"
	"time"

	"go.uber.org/zap"

	"github.com/ShukeBta/MediaStationGo/internal/model"
)

func TestDownloadPollBaselinesAlreadyCompletedTorrents(t *testing.T) {
	repos := newOrganizerTestRepo(t)
	svc := NewDownloadService(zap.NewNop(), repos, NewHub(zap.NewNop()), nil)

	svc.processDownloadSnapshot(t.Context(), []QBitTorrent{{
		Hash:     "already-complete",
		Name:     "Already Complete S01E01",
		Progress: 1,
		State:    "stalledUP",
	}}, nil)

	if got := len(svc.organizeQueue); got != 0 {
		t.Fatalf("first poll queued %d organize jobs, want 0", got)
	}
	if !svc.prevStates["already-complete"] {
		t.Fatal("first poll should remember completed baseline state")
	}

	svc.processDownloadSnapshot(t.Context(), []QBitTorrent{{
		Hash:     "late-complete",
		Name:     "Late Complete S01E01",
		Progress: 1,
		State:    "stalledUP",
	}}, nil)
	if got := len(svc.organizeQueue); got != 0 {
		t.Fatalf("newly discovered completed torrent queued %d organize jobs, want 0", got)
	}

	svc.processDownloadSnapshot(t.Context(), []QBitTorrent{{
		Hash:     "new-download",
		Name:     "New Download S01E01",
		Progress: 0.5,
	}}, nil)
	svc.processDownloadSnapshot(t.Context(), []QBitTorrent{{
		Hash:     "new-download",
		Name:     "New Download S01E01",
		Progress: 1,
		State:    "stalledUP",
	}}, nil)

	if got := len(svc.organizeQueue); got != 1 {
		t.Fatalf("completion transition queued %d organize jobs, want 1", got)
	}
}

func TestDownloadPollCatchesUpRecentlyCompletedTorrents(t *testing.T) {
	repos := newOrganizerTestRepo(t)
	if err := repos.DB.AutoMigrate(&model.DownloadTask{}); err != nil {
		t.Fatal(err)
	}
	task := &model.DownloadTask{
		Source:   "qbittorrent",
		URL:      "magnet:?xt=urn:btih:fresh",
		Title:    "Fresh Complete S01E01",
		SavePath: "/downloads",
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

	svc.processDownloadSnapshot(t.Context(), []QBitTorrent{
		{Hash: "fresh-complete", Name: "Fresh Complete S01E01", Progress: 1, State: "stalledUP", CompletionOn: time.Now().Add(-time.Hour).Unix()},
		{Hash: "stale-complete", Name: "Stale Complete S01E01", Progress: 1, State: "stalledUP", CompletionOn: time.Now().Add(-48 * time.Hour).Unix()},
		{Hash: "no-timestamp", Name: "No Timestamp S01E01", Progress: 1, State: "stalledUP"},
	}, tasksByTorrentIdentity([]model.DownloadTask{*task}))

	// 只有补整理时间窗内、且存在本地追踪任务的种子会被补整理；无 completion_on 的保守跳过。
	if got := len(svc.organizeQueue); got != 1 {
		t.Fatalf("first poll queued %d organize jobs, want 1 (recent tracked completion only)", got)
	}
}

func TestDownloadPollDoesNotCatchUpWhenAutoOrganizeDisabled(t *testing.T) {
	repos := newOrganizerTestRepo(t)
	if err := repos.DB.AutoMigrate(&model.DownloadTask{}); err != nil {
		t.Fatal(err)
	}
	task := &model.DownloadTask{
		Source:   "qbittorrent",
		URL:      "magnet:?xt=urn:btih:fresh",
		Title:    "Fresh Complete S01E01",
		SavePath: "/downloads",
		Status:   "queued",
		Progress: 0,
	}
	if err := repos.Download.Create(t.Context(), task); err != nil {
		t.Fatal(err)
	}
	svc := NewDownloadService(zap.NewNop(), repos, NewHub(zap.NewNop()), nil)
	torrent := QBitTorrent{
		Hash:         "fresh-complete",
		Name:         "Fresh Complete S01E01",
		Progress:     1,
		State:        "stalledUP",
		CompletionOn: time.Now().Add(-time.Hour).Unix(),
	}

	svc.processDownloadSnapshot(t.Context(), []QBitTorrent{torrent}, tasksByTorrentIdentity([]model.DownloadTask{*task}))
	svc.processDownloadSnapshot(t.Context(), []QBitTorrent{torrent}, tasksByTorrentIdentity([]model.DownloadTask{*task}))

	if got := len(svc.organizeQueue); got != 0 {
		t.Fatalf("auto-organize disabled queued %d completed jobs, want 0", got)
	}
}

func TestDownloadPollSkipsRecordedCompletedTorrentCatchup(t *testing.T) {
	repos := newOrganizerTestRepo(t)
	torrent := QBitTorrent{
		Hash:         "fresh-complete",
		Name:         "Fresh Complete S01E01",
		Progress:     1,
		State:        "stalledUP",
		CompletionOn: time.Now().Add(-time.Hour).Unix(),
	}
	if err := repos.Setting.Set(t.Context(), completedTorrentCatchupSettingKey(torrent), "true"); err != nil {
		t.Fatal(err)
	}
	svc := NewDownloadService(zap.NewNop(), repos, NewHub(zap.NewNop()), nil)

	svc.processDownloadSnapshot(t.Context(), []QBitTorrent{torrent}, nil)

	if got := len(svc.organizeQueue); got != 0 {
		t.Fatalf("recorded completed torrent queued %d organize jobs, want 0", got)
	}
}
