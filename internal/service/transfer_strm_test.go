package service

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestTransferSTRMResolvesRelativeTargetsAndPreservesURLs(t *testing.T) {
	for _, remote := range []bool{false, true} {
		t.Run(map[bool]string{false: "local", true: "remote"}[remote], func(t *testing.T) {
			root := t.TempDir()
			media := filepath.Join(root, "Movie 100% #1.iso")
			writeOrgFile(t, media, "media")
			content, want := "../Movie 100% #1.iso", media
			if remote {
				content = "https://example.com/Movie.iso?token=abc%2F123"
				want = content
			}
			source := filepath.Join(root, "source", "Movie.strm")
			writeOrgFile(t, source, "# playlist\n"+content+"\n")
			target := filepath.Join(root, "target.strm")
			if err := transferFile(source, target, "strm"); err != nil {
				t.Fatal(err)
			}
			data, err := os.ReadFile(target)
			if err != nil || strings.TrimSpace(string(data)) != want {
				t.Fatalf("target content = %q, want %q; error %v", data, want, err)
			}
			if _, err := os.Stat(source); err != nil {
				t.Fatalf("source STRM removed: %v", err)
			}
		})
	}
}

func TestTransferSTRMRejectsInvalidSourcesAndDestinations(t *testing.T) {
	root := t.TempDir()
	media := writeTemp(t, root, "movie.mkv", "media")
	invalid := writeTemp(t, root, "invalid.strm", "not a video target")
	textFile := writeTemp(t, root, "notes.txt", "notes")
	for _, tc := range []struct{ source, target string }{
		{media, filepath.Join(root, "wrong.mkv")},
		{filepath.Join(root, "missing.mkv"), filepath.Join(root, "missing.strm")},
		{invalid, filepath.Join(root, "invalid-copy.strm")},
		{textFile, filepath.Join(root, "notes.strm")},
		{root, filepath.Join(root, "directory.strm")},
	} {
		if err := transferFile(tc.source, tc.target, "strm"); err == nil {
			t.Fatalf("accepted invalid STRM transfer %q -> %q", tc.source, tc.target)
		}
		if _, err := os.Stat(tc.target); !os.IsNotExist(err) {
			t.Fatalf("invalid transfer created target %q: %v", tc.target, err)
		}
	}
	if err := transferDirectory(root, filepath.Join(t.TempDir(), "dest"), "strm"); err == nil {
		t.Fatal("generic directory transfer must not fall back to moving for STRM mode")
	}
	if data, err := os.ReadFile(media); err != nil || string(data) != "media" {
		t.Fatalf("source was changed: %q, %v", data, err)
	}
	target := writeTemp(t, root, "existing.strm", "existing")
	if err := transferFile(media, target, "strm"); err == nil {
		t.Fatal("STRM transfer overwrote an existing file")
	}
}
