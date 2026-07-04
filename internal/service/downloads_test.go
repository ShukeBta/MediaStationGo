package service

import (
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"github.com/glebarez/sqlite"
	"go.uber.org/zap"
	"gorm.io/gorm"

	"github.com/ShukeBta/MediaStationGo/internal/config"
	"github.com/ShukeBta/MediaStationGo/internal/model"
	"github.com/ShukeBta/MediaStationGo/internal/repository"
)

func TestDownloadViewsDoNotExposePrivateURL(t *testing.T) {
	rows := []model.DownloadTask{{
		UserID:   "u1",
		Source:   "qbittorrent",
		URL:      "https://tracker.example/download?id=1&passkey=private-token",
		Title:    "测试影片",
		SavePath: "/downloads",
		Status:   "queued",
	}}

	tasks, torrents := DownloadViews(rows, nil)
	data, err := json.Marshal(map[string]any{
		"tasks":    tasks,
		"torrents": torrents,
	})
	if err != nil {
		t.Fatal(err)
	}
	body := string(data)
	if strings.Contains(body, "private-token") || strings.Contains(body, "passkey") || strings.Contains(body, "tracker.example") {
		t.Fatalf("download views leaked private URL: %s", body)
	}
	if !strings.Contains(body, "测试影片") {
		t.Fatalf("download views should keep public title: %s", body)
	}
}

func TestSyncDownloadTaskProgressSkipsUnchangedCompletedTask(t *testing.T) {
	db, err := gorm.Open(sqlite.Open(":memory:"), &gorm.Config{})
	if err != nil {
		t.Fatal(err)
	}
	if err := db.AutoMigrate(&model.DownloadTask{}); err != nil {
		t.Fatal(err)
	}
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
	db, err := gorm.Open(sqlite.Open(":memory:"), &gorm.Config{})
	if err != nil {
		t.Fatal(err)
	}
	if err := db.AutoMigrate(&model.DownloadTask{}); err != nil {
		t.Fatal(err)
	}
	repos := repository.New(db)
	task := &model.DownloadTask{
		Source:   "qbittorrent",
		URL:      "magnet:?xt=urn:btih:test",
		Title:    "The First Jasmine S01E01 1080p TX WEB-DL AAC2.0 H.264-MWeb",
		SavePath: "/downloads/未分类",
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
	db, err := gorm.Open(sqlite.Open(":memory:"), &gorm.Config{})
	if err != nil {
		t.Fatal(err)
	}
	if err := db.AutoMigrate(&model.DownloadTask{}, &model.Setting{}); err != nil {
		t.Fatal(err)
	}
	repos := repository.New(db)
	task := &model.DownloadTask{
		Source:   "qbittorrent",
		URL:      "magnet:?xt=urn:btih:test",
		Title:    "Blades of the Guardians S02E01 1080p TX WEB-DL AAC2.0 H.264-MWeb",
		SavePath: "/downloads/未分类",
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

func TestProcessDownloadSnapshotSkipsUntrackedCompletedTorrentOnFirstSnapshot(t *testing.T) {
	db, err := gorm.Open(sqlite.Open(":memory:"), &gorm.Config{})
	if err != nil {
		t.Fatal(err)
	}
	if err := db.AutoMigrate(&model.DownloadTask{}, &model.Setting{}); err != nil {
		t.Fatal(err)
	}
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

func TestDownloadCompleteAutoOrganizesContentPath(t *testing.T) {
	root := t.TempDir()
	src := filepath.Join(root, "downloads", "国产剧", "狂飙.S01E01.2023.1080p.mkv")
	dest := filepath.Join(root, "media")
	if err := os.MkdirAll(filepath.Dir(src), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(src, []byte("episode"), 0o644); err != nil {
		t.Fatal(err)
	}

	repos := newOrganizerTestRepo(t)
	if err := repos.DB.AutoMigrate(&model.DownloadTask{}); err != nil {
		t.Fatal(err)
	}
	for key, value := range map[string]string{
		"organizer.auto_after_download": "true",
		"organize.target_dir":           dest,
		"organize.transfer_mode":        "copy",
	} {
		if err := repos.Setting.Set(t.Context(), key, value); err != nil {
			t.Fatal(err)
		}
	}
	org := NewOrganizerService(&config.Config{}, zap.NewNop(), repos)
	svc := NewDownloadService(zap.NewNop(), repos, NewHub(zap.NewNop()), org)
	svc.onTorrentComplete(t.Context(), QBitTorrent{
		Hash:        "done123",
		Name:        "狂飙.S01E01.2023.1080p",
		Progress:    1,
		SavePath:    filepath.Join(root, "downloads", "国产剧"),
		ContentPath: src,
	})

	want := filepath.Join(dest, "电视剧", "国产剧", "狂飙", "Season 01", "狂飙 - S01E01.mkv")
	if _, err := os.Stat(want); err != nil {
		t.Fatalf("auto organized file missing at %q: %v", want, err)
	}
	if _, err := os.Stat(src); err != nil {
		t.Fatalf("copy mode should keep source: %v", err)
	}
}

func TestDownloadCompleteAutoOrganizeUsesTaskMediaCategory(t *testing.T) {
	root := t.TempDir()
	src := filepath.Join(root, "downloads", "Motherhood.of.Taihang.S01E01.2026.1080p.mkv")
	dest := filepath.Join(root, "media")
	writeOrgFile(t, src, "episode")

	repos := newOrganizerTestRepo(t)
	if err := repos.DB.AutoMigrate(&model.DownloadTask{}); err != nil {
		t.Fatal(err)
	}
	for key, value := range map[string]string{
		"organizer.auto_after_download": "true",
		"organize.target_dir":           dest,
		"organize.transfer_mode":        "copy",
	} {
		if err := repos.Setting.Set(t.Context(), key, value); err != nil {
			t.Fatal(err)
		}
	}
	task := &model.DownloadTask{
		UserID:        "u1",
		Source:        "qbittorrent",
		URL:           "magnet:?xt=urn:btih:motherhood",
		Title:         "Motherhood.of.Taihang.S01E01.2026.1080p",
		SavePath:      filepath.Join(root, "downloads"),
		MediaType:     "tv",
		MediaCategory: "国产剧",
		Status:        "completed",
		Progress:      1,
	}
	if err := repos.Download.Create(t.Context(), task); err != nil {
		t.Fatal(err)
	}

	org := NewOrganizerService(&config.Config{}, zap.NewNop(), repos)
	svc := NewDownloadService(zap.NewNop(), repos, NewHub(zap.NewNop()), org)
	svc.onTorrentComplete(t.Context(), QBitTorrent{
		Hash:        "done-category",
		Name:        "Motherhood.of.Taihang.S01E01.2026.1080p",
		Progress:    1,
		SavePath:    filepath.Join(root, "downloads"),
		ContentPath: src,
	})

	categoryRoot := filepath.Join(dest, "电视剧", "国产剧")
	var organized string
	err := filepath.WalkDir(categoryRoot, func(path string, entry os.DirEntry, err error) error {
		if err != nil || entry.IsDir() {
			return err
		}
		if filepath.Ext(path) == ".mkv" {
			organized = path
		}
		return nil
	})
	if err != nil {
		t.Fatalf("walk organized category: %v", err)
	}
	if organized == "" {
		t.Fatalf("expected organized file under %q", categoryRoot)
	}
	wrongRoot := filepath.Join(dest, "电视剧", "Motherhood Of Taihang")
	if _, err := os.Stat(wrongRoot); !os.IsNotExist(err) {
		t.Fatalf("unexpected uncategorized organize root %q, err=%v", wrongRoot, err)
	}
}

func TestDownloadCompleteOnlyReplacesExistingWhenTaskAllowsWash(t *testing.T) {
	for _, tc := range []struct {
		name        string
		allowWash   bool
		wantContent string
	}{
		{name: "wash disabled keeps existing", allowWash: false, wantContent: "inception-1080p"},
		{name: "wash enabled replaces existing", allowWash: true, wantContent: "inception-2160p"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			root := t.TempDir()
			src := filepath.Join(root, "downloads", "Inception 2010 2160p BluRay.mkv")
			dest := filepath.Join(root, "media")
			existing := filepath.Join(dest, "电影", "Inception (2010)", "Inception (2010).mkv")
			writeOrgFile(t, src, "inception-2160p")
			writeOrgFile(t, existing, "inception-1080p")

			repos := newOrganizerTestRepo(t)
			if err := repos.DB.AutoMigrate(&model.DownloadTask{}); err != nil {
				t.Fatal(err)
			}
			for key, value := range map[string]string{
				"organizer.auto_after_download": "true",
				"organize.target_dir":           dest,
				"organize.transfer_mode":        "copy",
			} {
				if err := repos.Setting.Set(t.Context(), key, value); err != nil {
					t.Fatal(err)
				}
			}
			if err := repos.Media.Upsert(t.Context(), &model.Media{
				Title:     "Inception",
				Path:      existing,
				Year:      2010,
				Container: "mkv",
				Width:     1920,
				Height:    1080,
			}); err != nil {
				t.Fatal(err)
			}
			if err := repos.Download.Create(t.Context(), &model.DownloadTask{
				Source:               "qbittorrent",
				URL:                  "magnet:?xt=urn:btih:inception",
				Title:                "Inception 2010 2160p BluRay",
				SavePath:             filepath.Dir(src),
				Status:               "completed",
				Progress:             1,
				AllowExistingLibrary: tc.allowWash,
			}); err != nil {
				t.Fatal(err)
			}

			org := NewOrganizerService(&config.Config{}, zap.NewNop(), repos)
			svc := NewDownloadService(zap.NewNop(), repos, NewHub(zap.NewNop()), org)
			svc.onTorrentComplete(t.Context(), QBitTorrent{
				Hash:        "inception",
				Name:        "Inception 2010 2160p BluRay",
				Progress:    1,
				SavePath:    filepath.Dir(src),
				ContentPath: src,
			})

			got, err := os.ReadFile(existing)
			if err != nil {
				t.Fatal(err)
			}
			if string(got) != tc.wantContent {
				t.Fatalf("existing content = %q, want %q", string(got), tc.wantContent)
			}
		})
	}
}

func TestCompletedTorrentSourceDoesNotFallbackToSavePath(t *testing.T) {
	root := t.TempDir()
	savePath := filepath.Join(root, "downloads", "日番")
	if err := os.MkdirAll(savePath, 0o755); err != nil {
		t.Fatal(err)
	}
	svc := NewDownloadService(zap.NewNop(), newOrganizerTestRepo(t), NewHub(zap.NewNop()), nil)

	got := svc.completedTorrentSource(t.Context(), QBitTorrent{
		Hash:        "done123",
		Name:        "Missing.Payload.S01",
		SavePath:    savePath,
		ContentPath: filepath.Join(savePath, "Missing.Payload.S01", "Missing.Payload.S01E01.mkv"),
	})

	if got != "" {
		t.Fatalf("completedTorrentSource fell back to whole save_path %q; want empty", got)
	}
}

func TestDownloadPollBaselinesAlreadyCompletedTorrents(t *testing.T) {
	repos := newOrganizerTestRepo(t)
	svc := NewDownloadService(zap.NewNop(), repos, NewHub(zap.NewNop()), nil)

	svc.processDownloadSnapshot(t.Context(), []QBitTorrent{{
		Hash:     "already-complete",
		Name:     "Already Complete S01E01",
		Progress: 1,
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
		{Hash: "fresh-complete", Name: "Fresh Complete S01E01", Progress: 1, CompletionOn: time.Now().Add(-time.Hour).Unix()},
		{Hash: "stale-complete", Name: "Stale Complete S01E01", Progress: 1, CompletionOn: time.Now().Add(-48 * time.Hour).Unix()},
		{Hash: "no-timestamp", Name: "No Timestamp S01E01", Progress: 1},
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

func TestAutoOrganizeSyncsVisibilityWhenTargetAlreadyExists(t *testing.T) {
	root := t.TempDir()
	src := filepath.Join(root, "downloads", "国产剧", "狂飙.S01E01.2023.1080p.mkv")
	dest := filepath.Join(root, "media")
	writeOrgFile(t, src, "episode")

	repos := newOrganizerTestRepo(t)
	for key, value := range map[string]string{
		"organizer.auto_after_download": "true",
		"organize.target_dir":           dest,
		"organize.transfer_mode":        "copy",
	} {
		if err := repos.Setting.Set(t.Context(), key, value); err != nil {
			t.Fatal(err)
		}
	}
	lib := model.Library{Name: "国产剧", Path: filepath.Join(dest, "电视剧", "国产剧"), Type: "tv", Enabled: true}
	if err := repos.Library.Create(t.Context(), &lib); err != nil {
		t.Fatal(err)
	}
	org := NewOrganizerService(&config.Config{}, zap.NewNop(), repos)
	if _, err := org.OrganizeDirectory(t.Context(), OrganizeOptions{
		SourcePath:   src,
		DestPath:     dest,
		TransferMode: TransferCopy,
	}); err != nil {
		t.Fatalf("seed organized destination: %v", err)
	}
	scanner := NewScannerService(&config.Config{}, zap.NewNop(), repos, NewHub(zap.NewNop()), nil, nil)
	svc := NewDownloadService(zap.NewNop(), repos, NewHub(zap.NewNop()), org)
	svc.SetScanner(scanner)

	svc.onTorrentComplete(t.Context(), QBitTorrent{
		Hash:        "done123",
		Name:        "狂飙.S01E01.2023.1080p",
		Progress:    1,
		SavePath:    filepath.Dir(src),
		ContentPath: src,
	})

	var count int64
	if err := repos.DB.Model(&model.Media{}).Count(&count).Error; err != nil {
		t.Fatal(err)
	}
	if count != 1 {
		t.Fatalf("target already exists should still be scanned into DB, count=%d want 1", count)
	}
}

func TestCompletedTorrentSourceUsesConfiguredMapping(t *testing.T) {
	root := t.TempDir()
	localRoot := filepath.Join(root, "localdl")
	payload := filepath.Join(localRoot, "Show.S01")
	if err := os.MkdirAll(payload, 0o755); err != nil {
		t.Fatal(err)
	}
	repos := newOrganizerTestRepo(t)
	if err := repos.Setting.Set(t.Context(), DownloadPathMappingsSettingKey, "/qb/downloads="+localRoot); err != nil {
		t.Fatal(err)
	}
	svc := NewDownloadService(zap.NewNop(), repos, NewHub(zap.NewNop()), nil)

	got := svc.completedTorrentSource(t.Context(), QBitTorrent{ContentPath: "/qb/downloads/Show.S01"})
	if got != payload {
		t.Fatalf("completedTorrentSource = %q, want %q", got, payload)
	}
}

func TestUserPathMappingsParsing(t *testing.T) {
	repos := newOrganizerTestRepo(t)
	raw := "# comment\n/a=/b\n/c => /d\n/e:/f\nbad-line\n"
	if err := repos.Setting.Set(t.Context(), DownloadPathMappingsSettingKey, raw); err != nil {
		t.Fatal(err)
	}
	svc := NewDownloadService(zap.NewNop(), repos, NewHub(zap.NewNop()), nil)
	got := svc.userPathMappings(t.Context())
	want := map[string]string{"/a": "/b", "/c": "/d", "/e": "/f"}
	if len(got) != len(want) {
		t.Fatalf("userPathMappings = %v, want %v", got, want)
	}
	for k, v := range want {
		if got[k] != v {
			t.Fatalf("mapping %q = %q, want %q", k, got[k], v)
		}
	}
}

func TestPublicDownloadTitleUsesMagnetDisplayName(t *testing.T) {
	got := publicDownloadTitle("magnet:?xt=urn:btih:abc&dn=%E6%B5%8B%E8%AF%95%E5%BD%B1%E7%89%87")
	if got != "测试影片" {
		t.Fatalf("publicDownloadTitle = %q, want %q", got, "测试影片")
	}
}

func configureTestDefaultQB(t *testing.T, repos *repository.Container, baseURL string) {
	t.Helper()
	if err := repos.DownloadClient.Create(t.Context(), &model.DownloadClient{
		Name:      "qB test",
		Type:      "qbittorrent",
		Host:      baseURL,
		Username:  "admin",
		Password:  "admin",
		IsDefault: true,
		Enabled:   true,
	}); err != nil {
		t.Fatalf("create default qB client: %v", err)
	}
	if err := repos.Setting.Set(t.Context(), settingDownloadClientsManaged, "true"); err != nil {
		t.Fatalf("mark download clients managed: %v", err)
	}
}

func TestAddDownloadWithMetaSkipsExistingTaskBeforeQBAdd(t *testing.T) {
	var addCalls int32
	qb := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/api/v2/auth/login":
			_, _ = w.Write([]byte("Ok."))
		case "/api/v2/torrents/info":
			_, _ = w.Write([]byte(`[{"hash":"live123","name":"Some Show S01E01 1080p","state":"downloading","progress":0.5}]`))
		case "/api/v2/torrents/add":
			atomic.AddInt32(&addCalls, 1)
			_, _ = w.Write([]byte("Ok."))
		default:
			http.NotFound(w, r)
		}
	}))
	defer qb.Close()

	db, err := gorm.Open(sqlite.Open(":memory:"), &gorm.Config{})
	if err != nil {
		t.Fatal(err)
	}
	if err := db.AutoMigrate(&model.DownloadTask{}, &model.Setting{}); err != nil {
		t.Fatal(err)
	}
	repos := repository.New(db)
	existing := &model.DownloadTask{
		UserID:   "u1",
		Source:   "qbittorrent",
		URL:      "https://pt.example/download?id=old&passkey=old",
		Title:    "Some Show S01E01 1080p",
		SavePath: "/downloads/tv",
		Status:   "completed",
		Progress: 1,
	}
	if err := repos.Download.Create(t.Context(), existing); err != nil {
		t.Fatal(err)
	}

	svc := NewDownloadService(zap.NewNop(), repos, NewHub(zap.NewNop()), nil)
	svc.qb.Configure(QBitConfig{BaseURL: qb.URL, Username: "admin", Password: "admin"})
	task, err := svc.AddDownloadWithMeta(t.Context(), "u1", "https://pt.example/download?id=new&passkey=new", "/downloads/tv", DownloadTaskMeta{
		Title: "Some Show S01E01 2160p WEB-DL",
	})
	if !errors.Is(err, ErrDownloadAlreadyExists) {
		t.Fatalf("err = %v, want ErrDownloadAlreadyExists", err)
	}
	if task == nil || task.ID != existing.ID {
		t.Fatalf("task = %#v, want existing task %#v", task, existing)
	}
	if got := atomic.LoadInt32(&addCalls); got != 0 {
		t.Fatalf("qb add calls = %d, want 0", got)
	}
}

func TestAddDownloadWithMetaIgnoresDeletedHistoryWhenQBTaskMissing(t *testing.T) {
	var addCalls int32
	var added atomic.Bool
	qb := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/api/v2/auth/login":
			_, _ = w.Write([]byte("Ok."))
		case "/api/v2/torrents/info":
			if added.Load() {
				_, _ = w.Write([]byte(`[{"hash":"new123","name":"User Deleted Show S01E01 1080p WEB-DL","state":"downloading","progress":0}]`))
				return
			}
			_, _ = w.Write([]byte(`[]`))
		case "/api/v2/torrents/add":
			atomic.AddInt32(&addCalls, 1)
			added.Store(true)
			_, _ = w.Write([]byte("Ok."))
		default:
			http.NotFound(w, r)
		}
	}))
	defer qb.Close()

	db, err := gorm.Open(sqlite.Open(":memory:"), &gorm.Config{})
	if err != nil {
		t.Fatal(err)
	}
	if err := db.AutoMigrate(&model.DownloadTask{}, &model.Media{}, &model.Setting{}); err != nil {
		t.Fatal(err)
	}
	repos := repository.New(db)
	if err := repos.Setting.Set(t.Context(), "qbittorrent.url", qb.URL); err != nil {
		t.Fatal(err)
	}
	if err := repos.Setting.Set(t.Context(), "qbittorrent.username", "admin"); err != nil {
		t.Fatal(err)
	}
	if err := repos.Setting.Set(t.Context(), "qbittorrent.password", "admin"); err != nil {
		t.Fatal(err)
	}
	existing := &model.DownloadTask{
		UserID:   "u1",
		Source:   "qbittorrent",
		URL:      "https://pt.example/download?id=old&passkey=old",
		Title:    "User Deleted Show S01E01 1080p",
		SavePath: "/downloads/tv",
		Status:   "deleted",
	}
	if err := repos.Download.Create(t.Context(), existing); err != nil {
		t.Fatal(err)
	}

	svc := NewDownloadService(zap.NewNop(), repos, NewHub(zap.NewNop()), nil)
	task, err := svc.AddDownloadWithMeta(t.Context(), "u1", "https://pt.example/download?id=new&passkey=new", "/downloads/tv", DownloadTaskMeta{
		Title: "User Deleted Show S01E01 1080p WEB-DL",
	})
	if err != nil {
		t.Fatalf("add download: %v", err)
	}
	if task == nil || task.ID == existing.ID {
		t.Fatalf("task = %#v, should be a new task instead of existing %#v", task, existing)
	}
	if got := atomic.LoadInt32(&addCalls); got != 1 {
		t.Fatalf("qb add calls = %d, want 1", got)
	}
}

func TestPrepareDownloadWithMetaReusesLikelyPreparedTorrent(t *testing.T) {
	const hash = "abc123prepared"
	var addCalls int32
	qb := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/api/v2/auth/login":
			_, _ = w.Write([]byte("Ok."))
		case "/api/v2/torrents/info":
			_, _ = w.Write([]byte(`[{"hash":"` + hash + `","name":"Prepared Show S01E01 1080p","state":"pausedDL","progress":0}]`))
		case "/api/v2/torrents/files":
			if r.URL.Query().Get("hash") != hash {
				t.Fatalf("files hash = %q, want %q", r.URL.Query().Get("hash"), hash)
			}
			_, _ = w.Write([]byte(`[{"index":0,"name":"Prepared.Show.S01E01.mkv","size":1234,"priority":1}]`))
		case "/api/v2/torrents/add":
			atomic.AddInt32(&addCalls, 1)
			_, _ = w.Write([]byte("Ok."))
		default:
			http.NotFound(w, r)
		}
	}))
	defer qb.Close()

	db, err := gorm.Open(sqlite.Open(":memory:"), &gorm.Config{})
	if err != nil {
		t.Fatal(err)
	}
	if err := db.AutoMigrate(&model.DownloadTask{}, &model.Media{}, &model.DownloadClient{}, &model.Setting{}); err != nil {
		t.Fatal(err)
	}
	repos := repository.New(db)
	configureTestDefaultQB(t, repos, qb.URL)

	svc := NewDownloadService(zap.NewNop(), repos, NewHub(zap.NewNop()), nil)
	prepared, err := svc.PrepareDownloadWithMeta(t.Context(), "u1", "https://pt.example/download?id=prepared", "/downloads/tv", DownloadTaskMeta{
		Title: "Prepared Show S01E01 1080p",
	})
	if err != nil {
		t.Fatalf("prepare download: %v", err)
	}
	if prepared == nil || prepared.Hash != hash || len(prepared.Files) != 1 {
		t.Fatalf("prepared = %#v, want reused hash and files", prepared)
	}
	if got := atomic.LoadInt32(&addCalls); got != 0 {
		t.Fatalf("qb add calls = %d, want 0", got)
	}
}

func TestDeleteMarksMatchingDownloadTaskDeleted(t *testing.T) {
	const hash = "abc123"
	const title = "Delete Marker Show S01E01 1080p"
	var deleteCalls int32
	qb := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/api/v2/auth/login":
			_, _ = w.Write([]byte("Ok."))
		case "/api/v2/torrents/info":
			_, _ = w.Write([]byte(`[{"hash":"abc123","name":"Delete Marker Show S01E01 1080p","state":"downloading","progress":0.5}]`))
		case "/api/v2/torrents/delete":
			atomic.AddInt32(&deleteCalls, 1)
			_, _ = w.Write([]byte("Ok."))
		default:
			http.NotFound(w, r)
		}
	}))
	defer qb.Close()

	db, err := gorm.Open(sqlite.Open(":memory:"), &gorm.Config{})
	if err != nil {
		t.Fatal(err)
	}
	if err := db.AutoMigrate(&model.DownloadTask{}, &model.DownloadClient{}, &model.Setting{}); err != nil {
		t.Fatal(err)
	}
	repos := repository.New(db)
	configureTestDefaultQB(t, repos, qb.URL)
	task := &model.DownloadTask{
		UserID:   "u1",
		Source:   "qbittorrent",
		URL:      "https://pt.example/download?id=1",
		Title:    title,
		SavePath: "/downloads/tv",
		Status:   "downloading",
		Progress: 0.5,
	}
	if err := repos.Download.Create(t.Context(), task); err != nil {
		t.Fatal(err)
	}

	svc := NewDownloadService(zap.NewNop(), repos, NewHub(zap.NewNop()), nil)
	if err := svc.ReloadConfig(t.Context()); err != nil {
		t.Fatal(err)
	}
	if err := svc.Delete(t.Context(), hash, false); err != nil {
		t.Fatal(err)
	}
	if got := atomic.LoadInt32(&deleteCalls); got != 1 {
		t.Fatalf("delete calls = %d, want 1", got)
	}

	var updated model.DownloadTask
	if err := db.Where("id = ?", task.ID).First(&updated).Error; err != nil {
		t.Fatal(err)
	}
	if updated.Status != "deleted" {
		t.Fatalf("status = %q, want deleted", updated.Status)
	}
}

func TestDeleteMarksMagnetTaskDeletedWhenLiveTorrentNameMissing(t *testing.T) {
	const hash = "0123456789abcdef0123456789abcdef0123c0de"
	var deleteCalls int32
	qb := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/api/v2/auth/login":
			_, _ = w.Write([]byte("Ok."))
		case "/api/v2/torrents/info":
			_, _ = w.Write([]byte(`[]`))
		case "/api/v2/torrents/delete":
			atomic.AddInt32(&deleteCalls, 1)
			_, _ = w.Write([]byte("Ok."))
		default:
			http.NotFound(w, r)
		}
	}))
	defer qb.Close()

	db, err := gorm.Open(sqlite.Open(":memory:"), &gorm.Config{})
	if err != nil {
		t.Fatal(err)
	}
	if err := db.AutoMigrate(&model.DownloadTask{}, &model.DownloadClient{}, &model.Setting{}); err != nil {
		t.Fatal(err)
	}
	repos := repository.New(db)
	configureTestDefaultQB(t, repos, qb.URL)
	task := &model.DownloadTask{
		UserID:   "u1",
		Source:   "qbittorrent",
		URL:      "magnet:?xt=urn:btih:" + hash + "&dn=Codex.Path.Verify.S01E01.2026",
		Title:    "Codex Path Verify S01E01 2026",
		SavePath: "/downloads/tv",
		Status:   "queued",
	}
	if err := repos.Download.Create(t.Context(), task); err != nil {
		t.Fatal(err)
	}

	svc := NewDownloadService(zap.NewNop(), repos, NewHub(zap.NewNop()), nil)
	if err := svc.ReloadConfig(t.Context()); err != nil {
		t.Fatal(err)
	}
	if err := svc.Delete(t.Context(), hash, false); err != nil {
		t.Fatal(err)
	}
	if got := atomic.LoadInt32(&deleteCalls); got != 1 {
		t.Fatalf("delete calls = %d, want 1", got)
	}

	var updated model.DownloadTask
	if err := db.Where("id = ?", task.ID).First(&updated).Error; err != nil {
		t.Fatal(err)
	}
	if updated.Status != "deleted" {
		t.Fatalf("status = %q, want deleted", updated.Status)
	}
}

func TestAddDownloadWithMetaSkipsExistingLocalMovieBeforeQBAdd(t *testing.T) {
	db, err := gorm.Open(sqlite.Open(":memory:"), &gorm.Config{})
	if err != nil {
		t.Fatal(err)
	}
	if err := db.AutoMigrate(&model.Media{}, &model.DownloadTask{}, &model.Setting{}); err != nil {
		t.Fatal(err)
	}
	repos := repository.New(db)
	if err := db.Create(&model.Media{
		Title: "Inception",
		Path:  "/media/movies/Inception (2010)/Inception (2010).mkv",
	}).Error; err != nil {
		t.Fatal(err)
	}

	svc := NewDownloadService(zap.NewNop(), repos, NewHub(zap.NewNop()), nil)
	task, err := svc.AddDownloadWithMeta(t.Context(), "u1", "magnet:?xt=urn:btih:cccccccccccccccccccccccccccccccccccccccc&dn=Inception+2010+1080p", "/downloads", DownloadTaskMeta{
		Title: "Inception 2010 1080p WEB-DL",
	})
	if !errors.Is(err, ErrMediaAlreadyInLibrary) {
		t.Fatalf("err = %v, want ErrMediaAlreadyInLibrary", err)
	}
	if task != nil {
		t.Fatalf("task = %#v, want nil because local media already exists", task)
	}
	rows, err := repos.Download.List(t.Context())
	if err != nil {
		t.Fatal(err)
	}
	if len(rows) != 0 {
		t.Fatalf("download rows = %d, want 0", len(rows))
	}
}

func TestAddDownloadWithMetaSkipsExistingLocalEpisodeBeforeQBAdd(t *testing.T) {
	db, err := gorm.Open(sqlite.Open(":memory:"), &gorm.Config{})
	if err != nil {
		t.Fatal(err)
	}
	if err := db.AutoMigrate(&model.Media{}, &model.DownloadTask{}, &model.Setting{}); err != nil {
		t.Fatal(err)
	}
	repos := repository.New(db)
	if err := db.Create(&model.Media{
		Title:      "Some Show",
		Path:       "/media/tv/Some Show/Season 01/Some Show - S01E01.mkv",
		SeasonNum:  1,
		EpisodeNum: 1,
	}).Error; err != nil {
		t.Fatal(err)
	}

	svc := NewDownloadService(zap.NewNop(), repos, NewHub(zap.NewNop()), nil)
	task, err := svc.AddDownloadWithMeta(t.Context(), "u1", "magnet:?xt=urn:btih:dddddddddddddddddddddddddddddddddddddddd&dn=Some+Show+S01E01", "/downloads", DownloadTaskMeta{
		Title: "Some Show S01E01 2160p WEB-DL",
	})
	if !errors.Is(err, ErrMediaAlreadyInLibrary) {
		t.Fatalf("err = %v, want ErrMediaAlreadyInLibrary", err)
	}
	if task != nil {
		t.Fatalf("task = %#v, want nil because local episode already exists", task)
	}
	rows, err := repos.Download.List(t.Context())
	if err != nil {
		t.Fatal(err)
	}
	if len(rows) != 0 {
		t.Fatalf("download rows = %d, want 0", len(rows))
	}
}

func TestReloadConfigDoesNotFallbackToLegacyAfterClientDeleted(t *testing.T) {
	var addCalls int32
	qb := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/api/v2/auth/login":
			_, _ = w.Write([]byte("Ok."))
		case "/api/v2/torrents/info":
			if atomic.LoadInt32(&addCalls) > 0 {
				_, _ = w.Write([]byte(`[{"hash":"abc123","name":"Movie 2026 1080p","state":"downloading","progress":0.1}]`))
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

	db, err := gorm.Open(sqlite.Open(":memory:"), &gorm.Config{})
	if err != nil {
		t.Fatal(err)
	}
	if err := db.AutoMigrate(&model.DownloadClient{}, &model.DownloadTask{}, &model.Setting{}); err != nil {
		t.Fatal(err)
	}
	repos := repository.New(db)
	if err := repos.Setting.Set(t.Context(), "qbittorrent.url", qb.URL); err != nil {
		t.Fatal(err)
	}
	if err := repos.Setting.Set(t.Context(), "qbittorrent.username", "admin"); err != nil {
		t.Fatal(err)
	}
	if err := repos.Setting.Set(t.Context(), "qbittorrent.password", "admin"); err != nil {
		t.Fatal(err)
	}
	client := &model.DownloadClient{Name: "qB", Type: "qbittorrent", Host: qb.URL, Username: "admin", Password: "admin", IsDefault: true, Enabled: true}
	if err := repos.DownloadClient.Create(t.Context(), client); err != nil {
		t.Fatal(err)
	}
	if err := repos.DownloadClient.Delete(t.Context(), client.ID); err != nil {
		t.Fatal(err)
	}

	svc := NewDownloadService(zap.NewNop(), repos, NewHub(zap.NewNop()), nil)
	if err := svc.ReloadConfig(t.Context()); err != nil {
		t.Fatal(err)
	}
	_, err = svc.AddDownloadWithMeta(t.Context(), "u1", "magnet:?xt=urn:btih:aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa&dn=Movie+2026+1080p", "/downloads", DownloadTaskMeta{
		Title: "Movie 2026 1080p",
	})
	if err == nil {
		t.Fatal("expected add to fail when the configured downloader was deleted")
	}
	if got := atomic.LoadInt32(&addCalls); got != 0 {
		t.Fatalf("qb add calls = %d, want 0", got)
	}
}

func TestAddDownloadWithMetaAutoClassifiesSavePathAndQBitCategory(t *testing.T) {
	var addCalls int32
	var gotSavePath string
	var gotCategory string
	qb := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/api/v2/auth/login":
			_, _ = w.Write([]byte("Ok."))
		case "/api/v2/torrents/info":
			if atomic.LoadInt32(&addCalls) > 0 {
				_, _ = w.Write([]byte(`[{"hash":"auto123","name":"声生不息 S01E01","state":"downloading","progress":0.1}]`))
				return
			}
			_, _ = w.Write([]byte(`[]`))
		case "/api/v2/torrents/add":
			atomic.AddInt32(&addCalls, 1)
			if err := r.ParseMultipartForm(1024 * 1024); err != nil {
				http.Error(w, err.Error(), http.StatusBadRequest)
				return
			}
			gotSavePath = r.FormValue("savepath")
			gotCategory = r.FormValue("category")
			_, _ = w.Write([]byte("Ok."))
		default:
			http.NotFound(w, r)
		}
	}))
	defer qb.Close()

	db, err := gorm.Open(sqlite.Open(":memory:"), &gorm.Config{})
	if err != nil {
		t.Fatal(err)
	}
	if err := db.AutoMigrate(&model.DownloadClient{}, &model.DownloadTask{}, &model.Setting{}); err != nil {
		t.Fatal(err)
	}
	repos := repository.New(db)
	configureTestDefaultQB(t, repos, qb.URL)
	if err := repos.Setting.Set(t.Context(), "qbittorrent.savepath", "/downloads"); err != nil {
		t.Fatal(err)
	}

	svc := NewDownloadService(zap.NewNop(), repos, NewHub(zap.NewNop()), nil)
	task, err := svc.AddDownloadWithMeta(t.Context(), "u1", "magnet:?xt=urn:btih:eeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeee&dn=%E5%A3%B0%E7%94%9F%E4%B8%8D%E6%81%AF+S01E01", "", DownloadTaskMeta{
		Title:          "声生不息 S01E01",
		SourceCategory: "综艺",
	})
	if err != nil {
		t.Fatal(err)
	}
	wantPath := filepath.Join("/downloads", "综艺")
	if task.SavePath != wantPath {
		t.Fatalf("task save path = %q, want %q", task.SavePath, wantPath)
	}
	if gotSavePath != wantPath {
		t.Fatalf("qb savepath = %q, want %q", gotSavePath, wantPath)
	}
	if gotCategory != "综艺" {
		t.Fatalf("qb category = %q, want 综艺", gotCategory)
	}
}

func TestDownloadSavePathCategoryRootKeepsWindowsClientSeparators(t *testing.T) {
	if got := downloadSavePathCategoryRoot(`F:\downloads`, "国产剧"); got != `F:\downloads\国产剧` {
		t.Fatalf("downloadSavePathCategoryRoot() = %q, want Windows qB path", got)
	}
	if got := downloadSavePathCategoryRoot(`F:\downloads\国产剧`, "国产剧"); got != `F:\downloads\国产剧` {
		t.Fatalf("downloadSavePathCategoryRoot() duplicated category: %q", got)
	}
	if got := downloadSavePathCategoryRoot(`/downloads`, "国产剧"); got != filepath.Join(`/downloads`, "国产剧") {
		t.Fatalf("downloadSavePathCategoryRoot() = %q, want local path", got)
	}
}

func TestTranslateClientPathMapsWindowsQBitPathToContainerDownloadPath(t *testing.T) {
	root := t.TempDir()
	containerDownloads := filepath.Join(root, "downloads")
	want := filepath.Join(containerDownloads, "国产剧", "Show.S01E01.mkv")
	if err := os.MkdirAll(filepath.Dir(want), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(want, []byte("episode"), 0o644); err != nil {
		t.Fatal(err)
	}

	got := translateClientPath(`F:\downloads\国产剧\Show.S01E01.mkv`, map[string]string{
		`F:\downloads`: containerDownloads,
	})
	if got != want {
		t.Fatalf("translateClientPath() = %q, want %q", got, want)
	}
}

func TestAddDownloadWithMetaCanDisableAutoClassifiedSavePath(t *testing.T) {
	var addCalls int32
	var gotSavePath string
	var gotCategory string
	qb := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/api/v2/auth/login":
			_, _ = w.Write([]byte("Ok."))
		case "/api/v2/torrents/info":
			if atomic.LoadInt32(&addCalls) > 0 {
				_, _ = w.Write([]byte(`[{"hash":"auto456","name":"声生不息 S01E01","state":"downloading","progress":0.1}]`))
				return
			}
			_, _ = w.Write([]byte(`[]`))
		case "/api/v2/torrents/add":
			atomic.AddInt32(&addCalls, 1)
			if err := r.ParseMultipartForm(1024 * 1024); err != nil {
				http.Error(w, err.Error(), http.StatusBadRequest)
				return
			}
			gotSavePath = r.FormValue("savepath")
			gotCategory = r.FormValue("category")
			_, _ = w.Write([]byte("Ok."))
		default:
			http.NotFound(w, r)
		}
	}))
	defer qb.Close()

	db, err := gorm.Open(sqlite.Open(":memory:"), &gorm.Config{})
	if err != nil {
		t.Fatal(err)
	}
	if err := db.AutoMigrate(&model.DownloadClient{}, &model.DownloadTask{}, &model.Setting{}); err != nil {
		t.Fatal(err)
	}
	repos := repository.New(db)
	configureTestDefaultQB(t, repos, qb.URL)
	if err := repos.Setting.Set(t.Context(), "qbittorrent.savepath", "/downloads"); err != nil {
		t.Fatal(err)
	}
	if err := repos.Setting.Set(t.Context(), DownloadSmartClassifySettingKey, "false"); err != nil {
		t.Fatal(err)
	}

	svc := NewDownloadService(zap.NewNop(), repos, NewHub(zap.NewNop()), nil)
	task, err := svc.AddDownloadWithMeta(t.Context(), "u1", "magnet:?xt=urn:btih:ffffffffffffffffffffffffffffffffffffffff&dn=%E5%A3%B0%E7%94%9F%E4%B8%8D%E6%81%AF+S01E01", "", DownloadTaskMeta{
		Title:          "声生不息 S01E01",
		SourceCategory: "综艺",
	})
	if err != nil {
		t.Fatal(err)
	}
	if task.SavePath != "/downloads" {
		t.Fatalf("task save path = %q, want /downloads", task.SavePath)
	}
	if gotSavePath != "/downloads" {
		t.Fatalf("qb savepath = %q, want /downloads", gotSavePath)
	}
	if gotCategory != "" {
		t.Fatalf("qb category = %q, want empty", gotCategory)
	}
}

func TestReloadConfigDoesNotFallbackToLegacyAfterClientDisabled(t *testing.T) {
	var addCalls int32
	qb := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/api/v2/auth/login":
			_, _ = w.Write([]byte("Ok."))
		case "/api/v2/torrents/info":
			_, _ = w.Write([]byte(`[]`))
		case "/api/v2/torrents/add":
			atomic.AddInt32(&addCalls, 1)
			_, _ = w.Write([]byte("Ok."))
		default:
			http.NotFound(w, r)
		}
	}))
	defer qb.Close()

	db, err := gorm.Open(sqlite.Open(":memory:"), &gorm.Config{})
	if err != nil {
		t.Fatal(err)
	}
	if err := db.AutoMigrate(&model.DownloadClient{}, &model.DownloadTask{}, &model.Setting{}); err != nil {
		t.Fatal(err)
	}
	repos := repository.New(db)
	if err := repos.Setting.Set(t.Context(), "qbittorrent.url", qb.URL); err != nil {
		t.Fatal(err)
	}
	if err := repos.Setting.Set(t.Context(), "qbittorrent.username", "admin"); err != nil {
		t.Fatal(err)
	}
	if err := repos.Setting.Set(t.Context(), "qbittorrent.password", "admin"); err != nil {
		t.Fatal(err)
	}
	client := &model.DownloadClient{Name: "qB", Type: "qbittorrent", Host: qb.URL, Username: "admin", Password: "admin", IsDefault: true, Enabled: true}
	if err := repos.DownloadClient.Create(t.Context(), client); err != nil {
		t.Fatal(err)
	}
	client.Enabled = false
	if err := repos.DownloadClient.Update(t.Context(), client); err != nil {
		t.Fatal(err)
	}

	svc := NewDownloadService(zap.NewNop(), repos, NewHub(zap.NewNop()), nil)
	if err := svc.ReloadConfig(t.Context()); err != nil {
		t.Fatal(err)
	}
	_, err = svc.AddDownloadWithMeta(t.Context(), "u1", "magnet:?xt=urn:btih:bbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbb&dn=Movie+2026+1080p", "/downloads", DownloadTaskMeta{
		Title: "Movie 2026 1080p",
	})
	if err == nil {
		t.Fatal("expected add to fail when the configured downloader was disabled")
	}
	if got := atomic.LoadInt32(&addCalls); got != 0 {
		t.Fatalf("qb add calls = %d, want 0", got)
	}
}

func TestReloadConfigUsesSoleEnabledQBitWhenNoExplicitDefault(t *testing.T) {
	var addCalls int32
	qb := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/api/v2/auth/login":
			_, _ = w.Write([]byte("Ok."))
		case "/api/v2/torrents/info":
			if atomic.LoadInt32(&addCalls) > 0 {
				_, _ = w.Write([]byte(`[{"hash":"sole123","name":"Movie 2026 1080p","state":"downloading","progress":0.1}]`))
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

	db, err := gorm.Open(sqlite.Open(":memory:"), &gorm.Config{})
	if err != nil {
		t.Fatal(err)
	}
	if err := db.AutoMigrate(&model.DownloadClient{}, &model.DownloadTask{}, &model.Setting{}); err != nil {
		t.Fatal(err)
	}
	repos := repository.New(db)
	if err := repos.Setting.Set(t.Context(), settingDownloadClientsManaged, "true"); err != nil {
		t.Fatal(err)
	}
	client := &model.DownloadClient{Name: "qB", Type: "qbittorrent", Host: qb.URL, Username: "admin", Password: "admin", IsDefault: false, Enabled: true}
	if err := repos.DownloadClient.Create(t.Context(), client); err != nil {
		t.Fatal(err)
	}

	svc := NewDownloadService(zap.NewNop(), repos, NewHub(zap.NewNop()), nil)
	task, err := svc.AddDownloadWithMeta(t.Context(), "u1", "magnet:?xt=urn:btih:abababababababababababababababababababab&dn=Movie+2026+1080p", "/downloads", DownloadTaskMeta{
		Title: "Movie 2026 1080p",
	})
	if err != nil {
		t.Fatal(err)
	}
	if task == nil {
		t.Fatal("expected task")
	}
	if got := atomic.LoadInt32(&addCalls); got != 1 {
		t.Fatalf("qb add calls = %d, want 1", got)
	}
}

func TestAddDownloadWithMetaFailsClosedWhenNoDownloaderConfigured(t *testing.T) {
	db, err := gorm.Open(sqlite.Open(":memory:"), &gorm.Config{})
	if err != nil {
		t.Fatal(err)
	}
	if err := db.AutoMigrate(&model.DownloadClient{}, &model.DownloadTask{}, &model.Setting{}); err != nil {
		t.Fatal(err)
	}
	repos := repository.New(db)
	svc := NewDownloadService(zap.NewNop(), repos, NewHub(zap.NewNop()), nil)

	task, err := svc.AddDownloadWithMeta(t.Context(), "u1", "magnet:?xt=urn:btih:cccccccccccccccccccccccccccccccccccccccc&dn=Movie+2026+1080p", "/downloads", DownloadTaskMeta{
		Title: "Movie 2026 1080p",
	})
	if err == nil {
		t.Fatal("expected no downloader configured error")
	}
	if task != nil {
		t.Fatalf("task = %#v, want nil", task)
	}
	rows, err := repos.Download.List(t.Context())
	if err != nil {
		t.Fatal(err)
	}
	if len(rows) != 0 {
		t.Fatalf("download rows = %d, want 0", len(rows))
	}
}

func TestReloadConfigManagedModeDoesNotFallbackToLegacyWithoutRows(t *testing.T) {
	var addCalls int32
	qb := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/api/v2/auth/login":
			_, _ = w.Write([]byte("Ok."))
		case "/api/v2/torrents/info":
			_, _ = w.Write([]byte(`[]`))
		case "/api/v2/torrents/add":
			atomic.AddInt32(&addCalls, 1)
			_, _ = w.Write([]byte("Ok."))
		default:
			http.NotFound(w, r)
		}
	}))
	defer qb.Close()

	db, err := gorm.Open(sqlite.Open(":memory:"), &gorm.Config{})
	if err != nil {
		t.Fatal(err)
	}
	if err := db.AutoMigrate(&model.DownloadClient{}, &model.DownloadTask{}, &model.Setting{}); err != nil {
		t.Fatal(err)
	}
	repos := repository.New(db)
	if err := repos.Setting.Set(t.Context(), "qbittorrent.url", qb.URL); err != nil {
		t.Fatal(err)
	}
	if err := repos.Setting.Set(t.Context(), "qbittorrent.username", "admin"); err != nil {
		t.Fatal(err)
	}
	if err := repos.Setting.Set(t.Context(), "qbittorrent.password", "admin"); err != nil {
		t.Fatal(err)
	}
	if err := repos.Setting.Set(t.Context(), settingDownloadClientsManaged, "true"); err != nil {
		t.Fatal(err)
	}

	svc := NewDownloadService(zap.NewNop(), repos, NewHub(zap.NewNop()), nil)
	_, err = svc.AddDownloadWithMeta(t.Context(), "u1", "magnet:?xt=urn:btih:dddddddddddddddddddddddddddddddddddddddd&dn=Movie+2026+1080p", "/downloads", DownloadTaskMeta{
		Title: "Movie 2026 1080p",
	})
	if err == nil {
		t.Fatal("expected managed mode to reject missing default downloader")
	}
	if got := atomic.LoadInt32(&addCalls); got != 0 {
		t.Fatalf("qb add calls = %d, want 0", got)
	}
}

func TestAddDownloadWithMetaSkipsExistingLocalEpisodeWithReleaseGroup(t *testing.T) {
	db, err := gorm.Open(sqlite.Open(":memory:"), &gorm.Config{})
	if err != nil {
		t.Fatal(err)
	}
	if err := db.AutoMigrate(&model.Media{}, &model.DownloadTask{}, &model.Setting{}, &model.DownloadClient{}); err != nil {
		t.Fatal(err)
	}
	repos := repository.New(db)
	if err := db.Create(&model.Media{
		Title:      "凡人修仙传",
		Path:       "/media/动漫/国漫/凡人修仙传/Season 01/凡人修仙传 - S01E146.mkv",
		SeasonNum:  1,
		EpisodeNum: 146,
	}).Error; err != nil {
		t.Fatal(err)
	}

	svc := NewDownloadService(zap.NewNop(), repos, NewHub(zap.NewNop()), nil)
	task, err := svc.AddDownloadWithMeta(t.Context(), "u1", "magnet:?xt=urn:btih:eeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeee&dn=%5BMagicStar%5D+%E5%87%A1%E4%BA%BA%E4%BF%AE%E4%BB%99%E4%BC%A0+%E5%B9%B4%E7%95%AA+-+146+%5B1080p%5D", "/downloads", DownloadTaskMeta{
		Title: "[MagicStar] 凡人修仙传 年番 - 146 [1080p][WEB-DL]",
	})
	if !errors.Is(err, ErrMediaAlreadyInLibrary) {
		t.Fatalf("err = %v, want ErrMediaAlreadyInLibrary", err)
	}
	if task != nil {
		t.Fatalf("task = %#v, want nil", task)
	}
	rows, err := repos.Download.List(t.Context())
	if err != nil {
		t.Fatal(err)
	}
	if len(rows) != 0 {
		t.Fatalf("download rows = %d, want 0", len(rows))
	}
}
