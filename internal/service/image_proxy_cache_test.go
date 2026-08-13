package service

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"go.uber.org/zap"

	"github.com/ShukeBta/MediaStationGo/internal/config"
)

func TestPruneImageCacheDirRemovesExpiredAndOldestEntries(t *testing.T) {
	root := t.TempDir()
	now := time.Now().Truncate(time.Second)
	write := func(name string, size int, modTime time.Time) string {
		t.Helper()
		path := filepath.Join(root, name)
		if err := os.MkdirAll(filepath.Dir(path), 0o750); err != nil {
			t.Fatalf("create parent for %s: %v", name, err)
		}
		if err := os.WriteFile(path, []byte(strings.Repeat("x", size)), 0o600); err != nil {
			t.Fatalf("write %s: %v", name, err)
		}
		if err := os.Chtimes(path, modTime, modTime); err != nil {
			t.Fatalf("set mtime for %s: %v", name, err)
		}
		return path
	}

	expired := write(strings.Repeat("a", 64), 3, now.Add(-31*24*time.Hour))
	oldest := write("cloud-"+strings.Repeat("b", 64), 4, now.Add(-2*time.Hour))
	newest := write(strings.Repeat("c", 64), 4, now.Add(-time.Hour))
	negative := write(strings.Repeat("d", 64)+".fail", 1, now.Add(-31*24*time.Hour))
	temporary := write("img-keep.tmp", 1, now.Add(-31*24*time.Hour))
	variant := write(filepath.Join("variants", "source", "entry"), 1, now.Add(-31*24*time.Hour))

	total, err := pruneImageCacheDir(root, now.Add(-30*24*time.Hour), 4)
	if err != nil {
		t.Fatalf("prune image cache: %v", err)
	}
	if total != 4 {
		t.Fatalf("cache size after prune = %d, want 4", total)
	}
	for _, path := range []string{expired, oldest} {
		if _, err := os.Stat(path); !os.IsNotExist(err) {
			t.Fatalf("expected %s to be removed, stat err=%v", path, err)
		}
	}
	for _, path := range []string{newest, negative, temporary, variant} {
		if _, err := os.Stat(path); err != nil {
			t.Fatalf("expected %s to remain: %v", path, err)
		}
	}
}

func TestImageProxyStartupPrunesOriginalCache(t *testing.T) {
	cacheRoot := t.TempDir()
	root := filepath.Join(cacheRoot, "images")
	if err := os.MkdirAll(root, 0o750); err != nil {
		t.Fatal(err)
	}
	old := filepath.Join(root, strings.Repeat("a", 64))
	if err := os.WriteFile(old, []byte("old"), 0o600); err != nil {
		t.Fatal(err)
	}
	expiredAt := time.Now().Add(-25 * time.Hour)
	if err := os.Chtimes(old, expiredAt, expiredAt); err != nil {
		t.Fatal(err)
	}
	fresh := filepath.Join(root, strings.Repeat("b", 64))
	if err := os.WriteFile(fresh, []byte("fresh"), 0o600); err != nil {
		t.Fatal(err)
	}

	proxy := NewImageProxy(&config.Config{Cache: config.CacheConfig{
		CacheDir:                   cacheRoot,
		ImageCacheTTLHours:         24,
		ImageCacheMaxMB:            1,
		ImageCachePruneIntervalMin: 15,
	}}, zap.NewNop())
	if _, err := os.Stat(old); !os.IsNotExist(err) {
		t.Fatalf("expired original cache entry remains, stat err=%v", err)
	}
	if _, err := os.Stat(fresh); err != nil {
		t.Fatalf("fresh original cache entry missing: %v", err)
	}
	if want := int64(len("fresh")); proxy.imageCacheBytes != want {
		t.Fatalf("image cache bytes after startup prune = %d, want %d", proxy.imageCacheBytes, want)
	}
}

func TestImageProxyOriginalCacheWritePrunesOverLimit(t *testing.T) {
	cacheRoot := t.TempDir()
	proxy := NewImageProxy(&config.Config{Cache: config.CacheConfig{
		CacheDir:                   cacheRoot,
		ImageCacheTTLHours:         24,
		ImageCacheMaxMB:            1,
		ImageCachePruneIntervalMin: 60,
	}}, zap.NewNop())
	if err := os.MkdirAll(proxy.cacheDir, 0o750); err != nil {
		t.Fatal(err)
	}
	first := filepath.Join(proxy.cacheDir, strings.Repeat("a", 64))
	second := filepath.Join(proxy.cacheDir, strings.Repeat("b", 64))
	firstData := make([]byte, 768<<10)
	secondData := make([]byte, 768<<10)
	if !proxy.writeOriginalImageCache(first, "", "img-test-*.tmp", firstData) {
		t.Fatal("write first original cache entry failed")
	}
	if !proxy.writeOriginalImageCache(second, "", "img-test-*.tmp", secondData) {
		t.Fatal("write second original cache entry failed")
	}

	deadline := time.Now().Add(2 * time.Second)
	for {
		proxy.imageCacheMu.Lock()
		pruning := proxy.imageCachePruning
		bytes := proxy.imageCacheBytes
		proxy.imageCacheMu.Unlock()
		if !pruning && bytes <= 1<<20 {
			break
		}
		if time.Now().After(deadline) {
			t.Fatalf("original cache prune did not finish within deadline; pruning=%v bytes=%d", pruning, bytes)
		}
		time.Sleep(10 * time.Millisecond)
	}
	if _, err := os.Stat(first); !os.IsNotExist(err) {
		t.Fatalf("oldest over-limit entry remains, stat err=%v", err)
	}
	if _, err := os.Stat(second); err != nil {
		t.Fatalf("newest entry missing after over-limit prune: %v", err)
	}
}
