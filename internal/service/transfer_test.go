package service

import (
	"os"
	"path/filepath"
	"testing"
)

func TestParseTransferMode(t *testing.T) {
	cases := map[string]TransferMode{
		"":         TransferMove,
		"move":     TransferMove,
		"移动":       TransferMove,
		"copy":     TransferCopy,
		"复制":       TransferCopy,
		"hardlink": TransferHardlink,
		"硬链接":      TransferHardlink,
		"symlink":  TransferSymlink,
		"软链接":      TransferSymlink,
		"garbage":  TransferMove,
	}
	for in, want := range cases {
		if got := parseTransferMode(in); got != want {
			t.Errorf("parseTransferMode(%q) = %q, want %q", in, got, want)
		}
	}
}

func writeTemp(t *testing.T, dir, name, content string) string {
	t.Helper()
	p := filepath.Join(dir, name)
	if err := os.WriteFile(p, []byte(content), 0o644); err != nil {
		t.Fatalf("write %s: %v", p, err)
	}
	return p
}

func TestTransferFileCopyKeepsSource(t *testing.T) {
	dir := t.TempDir()
	src := writeTemp(t, dir, "src.mkv", "payload")
	dst := filepath.Join(dir, "out", "dst.mkv")
	if err := os.MkdirAll(filepath.Dir(dst), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := transferFile(src, dst, TransferCopy); err != nil {
		t.Fatalf("copy: %v", err)
	}
	if _, err := os.Stat(src); err != nil {
		t.Fatalf("copy must keep source: %v", err)
	}
	b, _ := os.ReadFile(dst)
	if string(b) != "payload" {
		t.Fatalf("copied content = %q", b)
	}
}

func TestTransferFileHardlinkSharesInodeAndKeepsSource(t *testing.T) {
	dir := t.TempDir()
	src := writeTemp(t, dir, "src.mkv", "payload")
	dst := filepath.Join(dir, "dst.mkv")
	if err := transferFile(src, dst, TransferHardlink); err != nil {
		t.Fatalf("hardlink: %v", err)
	}
	si, err := os.Stat(src)
	if err != nil {
		t.Fatalf("hardlink must keep source: %v", err)
	}
	di, err := os.Stat(dst)
	if err != nil {
		t.Fatalf("stat dst: %v", err)
	}
	if !os.SameFile(si, di) {
		t.Fatal("hardlink dst should share inode with source")
	}
}

func TestTransferFileSymlinkKeepsSource(t *testing.T) {
	dir := t.TempDir()
	src := writeTemp(t, dir, "src.mkv", "payload")
	dst := filepath.Join(dir, "dst.mkv")
	if err := transferFile(src, dst, TransferSymlink); err != nil {
		t.Fatalf("symlink: %v", err)
	}
	fi, err := os.Lstat(dst)
	if err != nil {
		t.Fatalf("lstat dst: %v", err)
	}
	if fi.Mode()&os.ModeSymlink == 0 {
		t.Fatal("dst should be a symlink")
	}
	if _, err := os.Stat(src); err != nil {
		t.Fatalf("symlink must keep source: %v", err)
	}
}

func TestTransferFileMoveRemovesSource(t *testing.T) {
	dir := t.TempDir()
	src := writeTemp(t, dir, "src.mkv", "payload")
	dst := filepath.Join(dir, "dst.mkv")
	if err := transferFile(src, dst, TransferMove); err != nil {
		t.Fatalf("move: %v", err)
	}
	if _, err := os.Stat(src); !os.IsNotExist(err) {
		t.Fatalf("move should remove source, stat err = %v", err)
	}
	if _, err := os.Stat(dst); err != nil {
		t.Fatalf("move should create dst: %v", err)
	}
}

func TestTransferFileNeverOverwrites(t *testing.T) {
	dir := t.TempDir()
	src := writeTemp(t, dir, "src.mkv", "new")
	dst := writeTemp(t, dir, "dst.mkv", "existing")
	for _, mode := range []TransferMode{TransferMove, TransferCopy, TransferHardlink, TransferSymlink} {
		if err := transferFile(src, dst, mode); err == nil {
			t.Fatalf("mode %q should refuse to overwrite existing dst", mode)
		}
		if b, _ := os.ReadFile(dst); string(b) != "existing" {
			t.Fatalf("mode %q clobbered existing dst", mode)
		}
	}
}
