package service

import (
	"os"
	"path/filepath"
	"testing"

	"go.uber.org/zap"
)

func TestGenerateSTRMFromTreeOverwriteAndTraversal(t *testing.T) {
	outDir := filepath.Join(t.TempDir(), "strm")
	existing := filepath.Join(outDir, "Movies", "Movie.strm")
	if err := os.MkdirAll(filepath.Dir(existing), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(existing, []byte("old\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	svc := NewSTRMService(zap.NewNop(), nil, nil)

	res, err := svc.GenerateFromTree(t.Context(), GenerateSTRMTreeOptions{
		Provider:  "openlist",
		Paths:     []string{"Movies/Movie.mkv", "../escape.mkv"},
		OutputDir: outDir,
	})
	if err != nil {
		t.Fatal(err)
	}
	if res.Generated != 0 || res.Skipped != 1 || len(res.Errors) != 1 {
		t.Fatalf("result = %#v, want existing skipped and traversal rejected", res)
	}
	if got := readSTRM(t, existing); got != "old" {
		t.Fatalf("existing strm = %q, want unchanged", got)
	}

	res, err = svc.GenerateFromTree(t.Context(), GenerateSTRMTreeOptions{
		Provider:  "openlist",
		Paths:     []string{"Movies/Movie.mkv"},
		OutputDir: outDir,
		Overwrite: true,
	})
	if err != nil {
		t.Fatal(err)
	}
	if res.Updated != 1 {
		t.Fatalf("updated = %d, want 1", res.Updated)
	}
	if got := readSTRM(t, existing); got == "old" {
		t.Fatalf("existing strm should be overwritten, got %q", got)
	}
}

func TestGenerateSTRMFromTreeCleanupStaleFiles(t *testing.T) {
	outDir := filepath.Join(t.TempDir(), "strm")
	current := filepath.Join(outDir, "Shows", "Show.S01E01.strm")
	stale := filepath.Join(outDir, "Shows", "Show.S01E02.strm")
	for _, file := range []string{current, stale} {
		if err := os.MkdirAll(filepath.Dir(file), 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(file, []byte("old\n"), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	svc := NewSTRMService(zap.NewNop(), nil, nil)

	res, err := svc.GenerateFromTree(t.Context(), GenerateSTRMTreeOptions{
		Provider:  "openlist",
		Paths:     []string{"Shows/Show.S01E01.mkv"},
		OutputDir: outDir,
		Cleanup:   true,
	})
	if err != nil {
		t.Fatal(err)
	}
	if res.Skipped != 1 || res.Cleaned != 1 || len(res.Errors) != 0 {
		t.Fatalf("result = %#v, want current skipped and one stale file cleaned", res)
	}
	if _, err := os.Stat(current); err != nil {
		t.Fatalf("current strm should remain: %v", err)
	}
	if _, err := os.Stat(stale); !os.IsNotExist(err) {
		t.Fatalf("stale strm should be removed, stat err=%v", err)
	}
}

func TestGenerateSTRMFromTreeCleanupWithOutputPrefixStaysInPrefix(t *testing.T) {
	outDir := filepath.Join(t.TempDir(), "strm")
	current := filepath.Join(outDir, "电影", "欧美电影", "Movie.strm")
	staleInPrefix := filepath.Join(outDir, "电影", "欧美电影", "Old.strm")
	otherCategory := filepath.Join(outDir, "电视剧", "国产剧", "Show.strm")
	for _, file := range []string{current, staleInPrefix, otherCategory} {
		if err := os.MkdirAll(filepath.Dir(file), 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(file, []byte("old\n"), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	svc := NewSTRMService(zap.NewNop(), nil, nil)

	res, err := svc.GenerateFromTree(t.Context(), GenerateSTRMTreeOptions{
		Provider:     "openlist",
		Paths:        []string{"Movie.mkv"},
		OutputPrefix: "电影/欧美电影",
		OutputDir:    outDir,
		Cleanup:      true,
	})
	if err != nil {
		t.Fatal(err)
	}
	if res.Skipped != 1 || res.Cleaned != 1 || len(res.Errors) != 0 {
		t.Fatalf("result = %#v, want one stale file cleaned inside prefix only", res)
	}
	if _, err := os.Stat(staleInPrefix); !os.IsNotExist(err) {
		t.Fatalf("stale prefixed strm should be removed, stat err=%v", err)
	}
	if _, err := os.Stat(otherCategory); err != nil {
		t.Fatalf("other category strm should remain: %v", err)
	}
}

func TestGenerateSTRMFromTreeCleanupSkipsWhenNoValidSources(t *testing.T) {
	outDir := filepath.Join(t.TempDir(), "strm")
	existing := filepath.Join(outDir, "Movies", "Movie.strm")
	if err := os.MkdirAll(filepath.Dir(existing), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(existing, []byte("keep\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	svc := NewSTRMService(zap.NewNop(), nil, nil)

	res, err := svc.GenerateFromTree(t.Context(), GenerateSTRMTreeOptions{
		Provider:  "openlist",
		Paths:     []string{"Movies/poster.jpg"},
		OutputDir: outDir,
		Cleanup:   true,
	})
	if err != nil {
		t.Fatal(err)
	}
	if res.Cleaned != 0 || len(res.Errors) != 0 {
		t.Fatalf("result = %#v, want cleanup skipped without valid media sources", res)
	}
	if got := readSTRM(t, existing); got != "keep" {
		t.Fatalf("existing strm = %q, want kept", got)
	}
}
