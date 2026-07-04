package service

import (
	"os"
	"path/filepath"
	"testing"
	"time"

	"go.uber.org/zap"

	"github.com/ShukeBta/MediaStationGo/internal/config"
	"github.com/ShukeBta/MediaStationGo/internal/model"
)

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

func TestDownloadCompleteRecordsUnsupportedVideoAsHandled(t *testing.T) {
	root := t.TempDir()
	src := filepath.Join(root, "downloads", "Toy.Story.4.2019.iso")
	dest := filepath.Join(root, "media")
	writeOrgFile(t, src, "iso")

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
	torrent := QBitTorrent{
		Hash:         "unsupported-iso",
		Name:         "Toy.Story.4.2019",
		Progress:     1,
		SavePath:     filepath.Dir(src),
		ContentPath:  src,
		CompletionOn: time.Now().Add(-time.Hour).Unix(),
	}

	org := NewOrganizerService(&config.Config{}, zap.NewNop(), repos)
	svc := NewDownloadService(zap.NewNop(), repos, NewHub(zap.NewNop()), org)
	svc.onTorrentComplete(t.Context(), torrent)

	if !svc.completedTorrentCatchupRecorded(t.Context(), torrent) {
		t.Fatalf("unsupported completed torrent should be marked handled to avoid repeated auto-organize retries")
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
