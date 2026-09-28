package service

import (
	"errors"
	"github.com/ShukeBta/MediaStationGo/internal/model"
	"os"
	"path/filepath"
	"testing"
)

func TestSTRMTargetDeletionPreservesReferenceAndRejectsChangedPreview(t *testing.T) {
	root := t.TempDir()
	s, repo := newFileManagerTestServiceWithRepo(t, root)
	parent := filepath.Join(root, "movie")
	if err := os.Mkdir(parent, 0700); err != nil {
		t.Fatal(err)
	}
	target := filepath.Join(parent, "movie.mkv")
	if err := os.WriteFile(target, []byte("video"), 0600); err != nil {
		t.Fatal(err)
	}
	sidecar := filepath.Join(root, "movie.strm")
	if err := os.WriteFile(sidecar, []byte("movie/movie.mkv\n"), 0600); err != nil {
		t.Fatal(err)
	}
	m := model.Media{Title: "strm", Path: sidecar, STRMURL: "/stale/old.mkv"}
	if err := repo.DB.Create(&m).Error; err != nil {
		t.Fatal(err)
	}
	preview, err := s.ResolveSTRMDeleteTarget(t.Context(), m.ID)
	if err != nil {
		t.Fatal(err)
	}
	if preview.TargetPath != target || preview.ParentPath != parent {
		t.Fatalf("preview=%+v", preview)
	}
	if err := os.WriteFile(target, []byte("new video"), 0600); err != nil {
		t.Fatal(err)
	}
	if _, err := s.DeleteSTRMTarget(t.Context(), m.ID, false, preview.Confirmation); !errors.Is(err, ErrSTRMTargetChanged) {
		t.Fatalf("changed preview accepted: %v", err)
	}
	preview, err = s.ResolveSTRMDeleteTarget(t.Context(), m.ID)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := s.DeleteSTRMTarget(t.Context(), m.ID, false, preview.Confirmation); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(target); !errors.Is(err, os.ErrNotExist) {
		t.Fatal("target remains")
	}
	if _, err := os.Stat(sidecar); err != nil {
		t.Fatal("sidecar removed", err)
	}
	if found, err := repo.Media.FindByID(t.Context(), m.ID); err != nil || found == nil {
		t.Fatal("database row removed")
	}
}

func TestSTRMDeleteRootAndReferenceDirectoryAreProtected(t *testing.T) {
	root := t.TempDir()
	s, repo := newFileManagerTestServiceWithRepo(t, root)
	target := filepath.Join(root, "movie.mkv")
	if err := os.WriteFile(target, []byte("video"), 0600); err != nil {
		t.Fatal(err)
	}
	sidecar := filepath.Join(root, "movie.strm")
	if err := os.WriteFile(sidecar, []byte(target), 0600); err != nil {
		t.Fatal(err)
	}
	m := model.Media{Path: sidecar}
	if err := repo.DB.Create(&m).Error; err != nil {
		t.Fatal(err)
	}
	preview, err := s.ResolveSTRMDeleteTarget(t.Context(), m.ID)
	if err != nil {
		t.Fatal(err)
	}
	if preview.ParentPath != "" {
		t.Fatal("root offered for deletion")
	}
	if _, err := s.DeleteSTRMTarget(t.Context(), m.ID, true, preview.Confirmation); !errors.Is(err, ErrRootMutation) {
		t.Fatalf("root deletion accepted: %v", err)
	}
	if err := os.WriteFile(sidecar, []byte(filepath.Join(t.TempDir(), "outside.mkv")), 0600); err != nil {
		t.Fatal(err)
	}
	if _, err := s.ResolveSTRMDeleteTarget(t.Context(), m.ID); err == nil {
		t.Fatal("outside target accepted")
	}
}

func TestSTRMDeleteMappedParentAndRejectSymlinkEscape(t *testing.T) {
	root := t.TempDir()
	s, repo := newFileManagerTestServiceWithRepo(t, root)
	mappedRoot := t.TempDir()
	parent := filepath.Join(mappedRoot, "movie")
	if err := os.Mkdir(parent, 0700); err != nil {
		t.Fatal(err)
	}
	target := filepath.Join(parent, "movie.mkv")
	if err := os.WriteFile(target, []byte("video"), 0600); err != nil {
		t.Fatal(err)
	}
	sidecar := filepath.Join(root, "movie.strm")
	if err := os.WriteFile(sidecar, []byte("https://example.com/d/movie/movie.mkv"), 0600); err != nil {
		t.Fatal(err)
	}
	m := model.Media{Path: sidecar}
	if err := repo.DB.Create(&m).Error; err != nil {
		t.Fatal(err)
	}
	if err := repo.Setting.Set(t.Context(), FFprobePathMappingsSettingKey, "https://example.com/d => "+mappedRoot); err != nil {
		t.Fatal(err)
	}
	preview, err := s.ResolveSTRMDeleteTarget(t.Context(), m.ID)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := s.DeleteSTRMTarget(t.Context(), m.ID, true, preview.Confirmation); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(parent); !errors.Is(err, os.ErrNotExist) {
		t.Fatal("parent remains")
	}
	if _, err := os.Stat(mappedRoot); err != nil {
		t.Fatal("mapping root removed")
	}
	outside := t.TempDir()
	if err := os.WriteFile(filepath.Join(outside, "movie.mkv"), []byte("keep"), 0600); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink(outside, parent); err != nil {
		t.Skipf("symlinks unavailable: %v", err)
	}
	if _, err := s.ResolveSTRMDeleteTarget(t.Context(), m.ID); !errors.Is(err, ErrPathOutOfBounds) {
		t.Fatalf("symlink escape accepted: %v", err)
	}
}
