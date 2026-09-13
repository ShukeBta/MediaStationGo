package service

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"go.uber.org/zap"

	"github.com/ShukeBta/MediaStationGo/internal/config"
	"github.com/ShukeBta/MediaStationGo/internal/model"
)

func TestOrganizeDirectorySTRMPreservesSourceAndSidecar(t *testing.T) {
	for _, name := range []string{"Dune 2021 1080p.iso", "Some Show S01E02 1080p.mkv"} {
		t.Run(name, func(t *testing.T) {
			root := t.TempDir()
			source := filepath.Join(root, "downloads", name)
			writeOrgFile(t, source, "original media bytes")
			writeOrgFile(t, nfoPath(source), "<movie><title>Local metadata</title></movie>")
			org := NewOrganizerService(&config.Config{}, zap.NewNop(), newOrganizerTestRepo(t))
			opts := OrganizeOptions{SourcePath: source, DestPath: filepath.Join(root, "library"), TransferMode: "strm", DryRun: true}
			preview, err := org.OrganizeDirectory(t.Context(), opts)
			if err != nil || preview.Organized != 1 || len(preview.Items) != 1 {
				t.Fatalf("preview: %+v, %v", preview, err)
			}
			target := preview.Items[0].Target
			if filepath.Ext(target) != ".strm" {
				t.Fatalf("preview target = %q, want STRM", target)
			}
			if _, err := os.Stat(opts.DestPath); !os.IsNotExist(err) {
				t.Fatalf("preview wrote destination: %v", err)
			}
			opts.DryRun = false
			result, err := org.OrganizeDirectory(t.Context(), opts)
			if err != nil || result.Organized != 1 || len(result.Errors) != 0 {
				t.Fatalf("organize: %+v, %v", result, err)
			}
			assertSTRMSourcePreserved(t, source, target, "original media bytes")
			for _, path := range []string{nfoPath(source), nfoPath(target)} {
				data, err := os.ReadFile(path)
				if err != nil || string(data) != "<movie><title>Local metadata</title></movie>" {
					t.Fatalf("sidecar %q = %q, %v", path, data, err)
				}
			}
			again, err := org.OrganizeDirectory(t.Context(), opts)
			if err != nil || again.Organized != 0 || again.Skipped != 1 || len(again.Errors) != 0 {
				t.Fatalf("repeat organize: %+v, %v", again, err)
			}
			assertSTRMSourcePreserved(t, source, target, "original media bytes")
		})
	}
}

func TestOrganizeLibrarySTRMDoesNotMoveSourceDuringReclassification(t *testing.T) {
	root := t.TempDir()
	repos := newOrganizerTestRepo(t)
	sourceLib := model.Library{Name: "Downloads", Path: filepath.Join(root, "downloads"), Type: "movie", Enabled: true}
	if err := repos.Library.Create(t.Context(), &sourceLib); err != nil {
		t.Fatal(err)
	}
	source := filepath.Join(sourceLib.Path, "Dune 2021.iso")
	writeOrgFile(t, source, "original ISO bytes")
	media := model.Media{LibraryID: sourceLib.ID, Path: source, Title: "Dune", Year: 2021, Container: "iso", TMDbID: 438631, Countries: "US", Genres: "科幻", ScrapeStatus: "matched"}
	if err := repos.Media.Upsert(t.Context(), &media); err != nil {
		t.Fatal(err)
	}
	if err := repos.Setting.Set(t.Context(), "organize.transfer_mode", "strm"); err != nil {
		t.Fatal(err)
	}
	cfg := &config.Config{}
	cfg.Organizer.SmartClassify = true
	org := NewOrganizerService(cfg, zap.NewNop(), repos)
	result, err := org.OrganizeLibraryWithOptions(t.Context(), sourceLib.ID, OrganizeOptions{DestPath: filepath.Join(root, "media")})
	if err != nil || result.Organized != 1 || len(result.Errors) != 0 {
		t.Fatalf("organize library: %+v, %v", result, err)
	}
	got, err := repos.Media.FindByID(t.Context(), media.ID)
	if err != nil || got == nil {
		t.Fatalf("organized media: %+v, %v", got, err)
	}
	assertSTRMSourcePreserved(t, source, got.Path, "original ISO bytes")
	if got.Container != "strm" || got.STRMURL != source {
		t.Fatalf("playback fields = container %q, target %q", got.Container, got.STRMURL)
	}
	if id, ok := fileIdentity(got.Path); ok && got.FileID != id {
		t.Fatalf("file identity = %q, want STRM identity %q", got.FileID, id)
	}
	playbackPath, err := localMediaPlaybackPath(got)
	if err != nil || playbackPath != source {
		t.Fatalf("playback path = %q, %v", playbackPath, err)
	}
	if err := repos.DB.Model(&model.Media{}).Where("id = ?", media.ID).Update("title", "Renamed Dune").Error; err != nil {
		t.Fatal(err)
	}
	renamed, err := org.SyncMediaPathWithMetadata(t.Context(), media.ID, OrganizeOptions{DestPath: filepath.Join(root, "media")})
	if err != nil || !strings.Contains(renamed, "Renamed Dune") {
		t.Fatalf("metadata rename = %q, %v", renamed, err)
	}
	assertSTRMSourcePreserved(t, source, renamed, "original ISO bytes")
}

func assertSTRMSourcePreserved(t *testing.T, source, target, payload string) {
	t.Helper()
	if filepath.Ext(target) != ".strm" {
		t.Fatalf("target = %q, want STRM", target)
	}
	data, err := os.ReadFile(source)
	if err != nil || string(data) != payload {
		t.Fatalf("source content = %q, %v", data, err)
	}
	data, err = os.ReadFile(target)
	if err != nil || strings.TrimSpace(string(data)) != source {
		t.Fatalf("STRM content = %q, want %q; error %v", data, source, err)
	}
}

func TestOrganizeSTRMDoesNotRenameExistingVideoToSTRM(t *testing.T) {
	root := t.TempDir()
	source := filepath.Join(root, "downloads", "Dune 2021 1080p.iso")
	dest := filepath.Join(root, "media")
	existing := filepath.Join(dest, "电影", "其他", "Dune (2021)", "Dune (2021).iso")
	writeOrgFile(t, source, "source ISO")
	writeOrgFile(t, existing, "existing ISO")
	repos := newOrganizerTestRepo(t)
	lib := model.Library{Name: "Movies", Path: dest, Type: "movie", Enabled: true}
	if err := repos.Library.Create(t.Context(), &lib); err != nil {
		t.Fatal(err)
	}
	if err := repos.Media.Upsert(t.Context(), &model.Media{LibraryID: lib.ID, Path: existing, Title: "Dune", Year: 2021}); err != nil {
		t.Fatal(err)
	}
	org := NewOrganizerService(&config.Config{}, zap.NewNop(), repos)
	result, err := org.OrganizeDirectory(t.Context(), OrganizeOptions{
		SourcePath: source, DestPath: dest, TransferMode: TransferSTRM, MediaType: "movie", MediaCategory: "欧美电影",
	})
	if err != nil || result.Skipped != 1 || result.Reclassified != 0 || result.Organized != 0 || len(result.Errors) != 0 {
		t.Fatalf("organize existing video: %+v, %v", result, err)
	}
	for path, want := range map[string]string{source: "source ISO", existing: "existing ISO"} {
		data, err := os.ReadFile(path)
		if err != nil || string(data) != want {
			t.Fatalf("existing video %q changed: %q, %v", path, data, err)
		}
	}
}

func TestMetadataRenamePreservesVideoFormatWithSTRMIngestDefault(t *testing.T) {
	repos := newOrganizerTestRepo(t)
	lib := model.Library{Name: "Movies", Path: t.TempDir(), Type: "movie", Enabled: true}
	if err := repos.Library.Create(t.Context(), &lib); err != nil {
		t.Fatal(err)
	}
	source := filepath.Join(lib.Path, "Old Movie Name.iso")
	writeOrgFile(t, source, "original ISO bytes")
	media := model.Media{LibraryID: lib.ID, Path: source, Title: "New Movie Name", Container: "iso", ScrapeStatus: "matched"}
	if err := repos.Media.Upsert(t.Context(), &media); err != nil {
		t.Fatal(err)
	}
	if err := repos.Setting.Set(t.Context(), "organize.transfer_mode", "strm"); err != nil {
		t.Fatal(err)
	}
	org := NewOrganizerService(&config.Config{}, zap.NewNop(), repos)
	target, err := org.SyncMediaPathWithMetadata(t.Context(), media.ID, OrganizeOptions{})
	if err != nil || filepath.Ext(target) != ".iso" {
		t.Fatalf("metadata rename changed format: %q, %v", target, err)
	}
	if data, err := os.ReadFile(target); err != nil || string(data) != "original ISO bytes" {
		t.Fatalf("renamed media = %q, %v", data, err)
	}
}
