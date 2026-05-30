package service

import (
	"os"
	"path/filepath"
	"testing"
)

func TestResolveAccessibleLibraryPathMapsConfiguredHostMediaDir(t *testing.T) {
	root := t.TempDir()
	hostRoot := filepath.Join(root, "nas", "host", "media")
	containerRoot := filepath.Join(root, "container", "media")
	containerLibrary := filepath.Join(containerRoot, "电视剧", "国产剧")
	if err := os.MkdirAll(containerLibrary, 0o755); err != nil {
		t.Fatal(err)
	}
	t.Setenv("MEDIASTATION_MEDIA_DIR", hostRoot)
	t.Setenv("MEDIASTATION_MEDIA_CONTAINER_DIR", containerRoot)

	got, err := resolveAccessibleLibraryPath(filepath.Join(hostRoot, "电视剧", "国产剧"))
	if err != nil {
		t.Fatalf("resolveAccessibleLibraryPath() error = %v", err)
	}
	if got != filepath.Clean(containerLibrary) {
		t.Fatalf("resolveAccessibleLibraryPath() = %q, want %q", got, filepath.Clean(containerLibrary))
	}
}

func TestResolveAccessibleLibraryPathKeepsAccessibleContainerPath(t *testing.T) {
	containerLibrary := filepath.Join(t.TempDir(), "media", "电影")
	if err := os.MkdirAll(containerLibrary, 0o755); err != nil {
		t.Fatal(err)
	}

	got, err := resolveAccessibleLibraryPath(containerLibrary)
	if err != nil {
		t.Fatalf("resolveAccessibleLibraryPath() error = %v", err)
	}
	if got != filepath.Clean(containerLibrary) {
		t.Fatalf("resolveAccessibleLibraryPath() = %q, want %q", got, filepath.Clean(containerLibrary))
	}
}
