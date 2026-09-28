package service

import (
	"errors"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"testing"
)

func fileManagerTestDirectoryLink(t *testing.T, target, link string) {
	t.Helper()
	if err := os.Symlink(target, link); err == nil {
		return
	} else if runtime.GOOS != "windows" {
		t.Skipf("directory symlinks unavailable: %v", err)
	}
	// Windows junctions do not require Developer Mode or symlink privileges.
	if output, err := exec.Command("cmd", "/c", "mklink", "/J", link, target).CombinedOutput(); err != nil {
		t.Skipf("directory links unavailable: %v (%s)", err, output)
	}
}

func TestFileManagerRejectsEscapingDirectoryLinks(t *testing.T) {
	root, outside := t.TempDir(), t.TempDir()
	victim := filepath.Join(outside, "keep")
	if err := os.MkdirAll(victim, 0o755); err != nil {
		t.Fatal(err)
	}
	marker := filepath.Join(victim, "marker.txt")
	if err := os.WriteFile(marker, []byte("keep"), 0o600); err != nil {
		t.Fatal(err)
	}
	link := filepath.Join(root, "link")
	fileManagerTestDirectoryLink(t, outside, link)
	svc := newFileManagerTestService(t, root)
	if _, err := svc.List(filepath.Join(link, "keep"), 100); !errors.Is(err, ErrPathOutOfBounds) {
		t.Fatalf("List error = %v", err)
	}
	if _, err := svc.CreateFolder(filepath.Join(link, "new", "nested"), "child"); !errors.Is(err, ErrPathOutOfBounds) {
		t.Fatalf("CreateFolder error = %v", err)
	}
	if err := svc.Delete(filepath.Join(link, "keep")); !errors.Is(err, ErrPathOutOfBounds) {
		t.Fatalf("Delete error = %v", err)
	}
	if content, err := os.ReadFile(marker); err != nil || string(content) != "keep" {
		t.Fatalf("outside file changed: %q, %v", content, err)
	}
}

func TestFileManagerProtectsRootAliases(t *testing.T) {
	root := t.TempDir()
	alias := filepath.Join(root, "root-alias")
	fileManagerTestDirectoryLink(t, root, alias)
	svc := newFileManagerTestService(t, root)
	if _, _, err := svc.requireAllowedPath(alias, true); !errors.Is(err, ErrRootMutation) {
		t.Fatalf("root alias error = %v", err)
	}
}

func TestFileManagerRejectsEscapingLinksInsideTransferSource(t *testing.T) {
	root, outside := t.TempDir(), t.TempDir()
	src, dst := filepath.Join(root, "source"), filepath.Join(root, "destination")
	for _, path := range []string{src, dst} {
		if err := os.Mkdir(path, 0o755); err != nil {
			t.Fatal(err)
		}
	}
	fileManagerTestDirectoryLink(t, outside, filepath.Join(src, "outside"))
	svc := newFileManagerTestService(t, root)
	if _, err := svc.Transfer(src, dst, TransferCopy); !errors.Is(err, ErrPathOutOfBounds) {
		t.Fatalf("transfer error = %v", err)
	}
	if entries, err := os.ReadDir(dst); err != nil || len(entries) != 0 {
		t.Fatalf("output created before source validation: %v, %v", entries, err)
	}
}

func TestFileManagerAllowsConfiguredDirectoryLink(t *testing.T) {
	parent, target := t.TempDir(), t.TempDir()
	link := filepath.Join(parent, "configured-root")
	fileManagerTestDirectoryLink(t, target, link)
	svc := newFileManagerTestService(t, link)
	created, err := svc.CreateFolder(link, "new")
	if err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(filepath.Join(target, "new")); err != nil {
		t.Fatal(err)
	}
	if err := svc.Delete(created.Path); err != nil {
		t.Fatal(err)
	}
}

func TestTransferDirectoryRejectsDescendantsBeforeWriting(t *testing.T) {
	for _, mode := range []TransferMode{TransferCopy, TransferMove, TransferHardlink, TransferSymlink} {
		t.Run(string(mode), func(t *testing.T) {
			src := t.TempDir()
			// Validate topology before creating even the first output directory.
			dst := filepath.Join(src, "not-created", "copy")
			if err := transferDirectory(src, dst, mode); !errors.Is(err, ErrTransferIntoSource) {
				t.Fatalf("error = %v", err)
			}
			if _, err := os.Stat(filepath.Dir(dst)); !errors.Is(err, os.ErrNotExist) {
				t.Fatalf("destination was created: %v", err)
			}
		})
	}
}

func TestFileManagerRejectsTransferDescendantViaAlias(t *testing.T) {
	root := t.TempDir()
	src := filepath.Join(root, "source")
	if err := os.MkdirAll(src, 0o755); err != nil {
		t.Fatal(err)
	}
	alias := filepath.Join(root, "source-alias")
	fileManagerTestDirectoryLink(t, src, alias)
	svc := newFileManagerTestService(t, root)
	if _, err := svc.Transfer(src, alias, TransferCopy); !errors.Is(err, ErrTransferIntoSource) {
		t.Fatalf("error = %v", err)
	}
	if _, err := os.Stat(filepath.Join(src, "source")); !errors.Is(err, os.ErrNotExist) {
		t.Fatalf("destination was created: %v", err)
	}
}
