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

// TestOrganizeLibraryScopesToSourceDir verifies organize reads from the
// source directory (源目录) and writes to the destination directory (目的地目录):
// only media located under SourcePath are organized; media outside it are
// left untouched.
func TestOrganizeLibraryScopesToSourceDir(t *testing.T) {
	root := t.TempDir()
	srcDir := filepath.Join(root, "downloads")
	otherDir := filepath.Join(root, "elsewhere")
	dest := filepath.Join(root, "library")
	for _, d := range []string{srcDir, otherDir} {
		if err := os.MkdirAll(d, 0o755); err != nil {
			t.Fatal(err)
		}
	}
	inSource := filepath.Join(srcDir, "In Source.mkv")
	outside := filepath.Join(otherDir, "Outside.mkv")
	if err := os.WriteFile(inSource, []byte("a"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(outside, []byte("b"), 0o644); err != nil {
		t.Fatal(err)
	}

	repos := newOrganizerTestRepo(t)
	lib := model.Library{Name: "Movies", Path: root, Type: "movie", Enabled: true}
	if err := repos.Library.Create(t.Context(), &lib); err != nil {
		t.Fatal(err)
	}
	mIn := model.Media{LibraryID: lib.ID, Title: "In Source", Path: inSource, Year: 2020, Container: "mkv", ScrapeStatus: "matched"}
	mOut := model.Media{LibraryID: lib.ID, Title: "Outside", Path: outside, Year: 2021, Container: "mkv", ScrapeStatus: "matched"}
	if err := repos.Media.Upsert(t.Context(), &mIn); err != nil {
		t.Fatal(err)
	}
	if err := repos.Media.Upsert(t.Context(), &mOut); err != nil {
		t.Fatal(err)
	}

	org := NewOrganizerService(&config.Config{}, zap.NewNop(), repos)
	res, err := org.OrganizeLibraryWithOptions(t.Context(), lib.ID, OrganizeOptions{
		SourcePath:   srcDir,
		DestPath:     dest,
		TransferMode: TransferCopy,
	})
	if err != nil {
		t.Fatalf("organize: %v", err)
	}
	if res.Organized != 1 {
		t.Fatalf("expected exactly 1 organized (only the in-source media), got %d (skipped %d)", res.Organized, res.Skipped)
	}

	// The in-source media should now live under the destination dir.
	gotIn, err := repos.Media.FindByID(t.Context(), mIn.ID)
	if err != nil || gotIn == nil {
		t.Fatalf("reload in-source media: %v", err)
	}
	if !strings.HasPrefix(gotIn.Path, dest) {
		t.Fatalf("in-source media should be organized under dest %q, got %q", dest, gotIn.Path)
	}

	// The outside media must be left untouched (not moved into dest).
	gotOut, err := repos.Media.FindByID(t.Context(), mOut.ID)
	if err != nil || gotOut == nil {
		t.Fatalf("reload outside media: %v", err)
	}
	if gotOut.Path != outside {
		t.Fatalf("outside media must not be organized; path changed to %q", gotOut.Path)
	}
	if _, err := os.Stat(outside); err != nil {
		t.Fatalf("outside source file must remain in place: %v", err)
	}
}
