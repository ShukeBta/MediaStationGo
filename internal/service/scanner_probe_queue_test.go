package service

import (
	"testing"
	"time"

	"go.uber.org/zap"
)

func newProbeQueueTestScanner(capacity int) *ScannerService {
	return &ScannerService{
		log:                    zap.NewNop(),
		storage:                &StorageConfigService{},
		probe:                  &FFprobeService{},
		cloudMediaProbeQueue:   make(chan cloudMediaProbeTask, capacity),
		cloudMediaProbing:      make(map[string]struct{}),
		cloudMediaProbeBackoff: make(map[string]time.Time),
	}
}

func TestQueueCloudMediaProbeTrimsTaskAndRejectsDuplicate(t *testing.T) {
	scanner := newProbeQueueTestScanner(1)
	if !scanner.queueCloudMediaProbe(" openlist ", " /Movies/a.mkv ", " cloud://openlist/Movies/a.mkv ") {
		t.Fatal("first cloud probe should enqueue")
	}
	if scanner.queueCloudMediaProbe("openlist", "/Movies/a.mkv", "cloud://openlist/Movies/a.mkv") {
		t.Fatal("duplicate cloud probe should be rejected while in flight")
	}

	task := <-scanner.cloudMediaProbeQueue
	if task.typ != "openlist" || task.ref != "/Movies/a.mkv" || task.path != "cloud://openlist/Movies/a.mkv" {
		t.Fatalf("task was not normalized: %#v", task)
	}
}

func TestQueueCloudMediaProbeFullQueueBacksOffAndReleases(t *testing.T) {
	scanner := newProbeQueueTestScanner(0)
	if scanner.queueCloudMediaProbe("openlist", "/Movies/a.mkv", "cloud://openlist/Movies/a.mkv") {
		t.Fatal("unbuffered queue without receiver should reject enqueue")
	}

	scanner.cloudMediaProbeMu.Lock()
	_, probing := scanner.cloudMediaProbing["cloud://openlist/Movies/a.mkv"]
	until, backedOff := scanner.cloudMediaProbeBackoff["cloud://openlist/Movies/a.mkv"]
	scanner.cloudMediaProbeMu.Unlock()
	if probing {
		t.Fatal("queue-full path should release in-flight marker")
	}
	if !backedOff || !until.After(time.Now()) {
		t.Fatalf("queue-full path should receive future backoff, got %v", until)
	}
	if scanner.queueCloudMediaProbe("openlist", "/Movies/a.mkv", "cloud://openlist/Movies/a.mkv") {
		t.Fatal("backed-off path should not be retried immediately")
	}
}

func TestQueueCloudMediaProbeBudgetConsumesAttempts(t *testing.T) {
	scanner := newProbeQueueTestScanner(0)
	budget := 1
	if scanner.queueCloudMediaProbeWithBudget("openlist", "/Movies/a.mkv", "cloud://openlist/Movies/a.mkv", &budget) {
		t.Fatal("unbuffered queue without receiver should reject enqueue")
	}
	if budget != 0 {
		t.Fatalf("budget = %d, want 0 after attempted enqueue", budget)
	}
	if scanner.queueCloudMediaProbeWithBudget("openlist", "/Movies/b.mkv", "cloud://openlist/Movies/b.mkv", &budget) {
		t.Fatal("zero budget should prevent enqueue")
	}
}
