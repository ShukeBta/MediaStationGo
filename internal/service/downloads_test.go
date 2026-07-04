package service

import (
	"testing"
	"time"

	"go.uber.org/zap"
)

func TestLiveTorrentSnapshotUsesPollingSnapshot(t *testing.T) {
	now := time.Date(2026, 6, 23, 0, 0, 0, 0, time.UTC)
	svc := NewDownloadService(zap.NewNop(), nil, NewHub(zap.NewNop()), nil)
	svc.now = func() time.Time { return now }

	live := []QBitTorrent{{
		Hash:     "hash-1",
		Name:     "Release.Name.S01E01",
		State:    "downloading",
		Progress: 0.5,
	}}
	svc.processDownloadSnapshot(t.Context(), live, nil)
	live[0].Name = "mutated"

	got := svc.LiveTorrentSnapshot(30 * time.Second)
	if len(got) != 1 || got[0].Name != "Release.Name.S01E01" {
		t.Fatalf("snapshot = %#v, want cloned live torrent", got)
	}
	got[0].Name = "changed"
	again := svc.LiveTorrentSnapshot(30 * time.Second)
	if len(again) != 1 || again[0].Name != "Release.Name.S01E01" {
		t.Fatalf("snapshot was mutable through caller: %#v", again)
	}

	now = now.Add(31 * time.Second)
	if stale := svc.LiveTorrentSnapshot(30 * time.Second); len(stale) != 0 {
		t.Fatalf("stale snapshot = %#v, want empty", stale)
	}
}
