package service

import (
	"os"
	"path/filepath"
	"testing"
)

func TestTransferSidecarArtworkMovesPosterAndBackdrop(t *testing.T) {
	srcDir := t.TempDir()
	dstDir := t.TempDir()

	srcMedia := filepath.Join(srcDir, "ADN-188.mkv")
	// Write a fake media + its scraped sidecar artwork.
	if err := os.WriteFile(srcMedia, []byte("media"), 0o644); err != nil {
		t.Fatal(err)
	}
	poster := filepath.Join(srcDir, "ADN-188-poster.jpg")
	backdrop := filepath.Join(srcDir, "ADN-188-backdrop.jpg")
	if err := os.WriteFile(poster, []byte("poster"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(backdrop, []byte("backdrop"), 0o644); err != nil {
		t.Fatal(err)
	}

	dstMedia := filepath.Join(dstDir, "出口, of a title (2018).mkv")
	if err := transferSidecarArtwork(srcMedia, dstMedia, TransferCopy); err != nil {
		t.Fatalf("transferSidecarArtwork: %v", err)
	}

	// Poster + backdrop relocated with the same base file name.
	dstPoster := filepath.Join(dstDir, "ADN-188-poster.jpg")
	dstBackdrop := filepath.Join(dstDir, "ADN-188-backdrop.jpg")
	for _, p := range []string{dstPoster, dstBackdrop} {
		if _, err := os.Stat(p); err != nil {
			t.Fatalf("expected %s to exist after transfer: %v", p, err)
		}
	}
}

func TestTransferSidecarArtworkNoopWhenNoArtwork(t *testing.T) {
	srcDir := t.TempDir()
	dstDir := t.TempDir()
	srcMedia := filepath.Join(srcDir, "A.mkv")
	if err := os.WriteFile(srcMedia, []byte("x"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := transferSidecarArtwork(srcMedia, filepath.Join(dstDir, "B.mkv"), TransferCopy); err != nil {
		t.Fatalf("expected no error with no artwork, got %v", err)
	}
}

func TestTransferSidecarArtworkNeverOverwritesExisting(t *testing.T) {
	srcDir := t.TempDir()
	dstDir := t.TempDir()
	srcMedia := filepath.Join(srcDir, "A.mkv")
	poster := filepath.Join(srcDir, "A-poster.jpg")
	if err := os.WriteFile(srcMedia, []byte("x"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(poster, []byte("new"), 0o644); err != nil {
		t.Fatal(err)
	}
	// Pre-existing poster at destination must be preserved.
	dstMedia := filepath.Join(dstDir, "A.mkv")
	dstPoster := filepath.Join(dstDir, "A-poster.jpg")
	if err := os.WriteFile(dstPoster, []byte("keep"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := transferSidecarArtwork(srcMedia, dstMedia, TransferCopy); err != nil {
		t.Fatalf("transferSidecarArtwork: %v", err)
	}
	data, _ := os.ReadFile(dstPoster)
	if string(data) != "keep" {
		t.Fatalf("destination poster overwritten, got %q", data)
	}
}
