package service

import (
	"os"
	"path/filepath"
	"testing"

	"go.uber.org/zap"

	"github.com/ShukeBta/MediaStationGo/internal/config"
	"github.com/ShukeBta/MediaStationGo/internal/model"
)

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
