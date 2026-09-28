package service

import "testing"

func TestOrganizeTaskMetricsIncludesScrapeProcessed(t *testing.T) {
	metrics := OrganizeTaskMetrics(&OrganizeResult{
		Scrapes: []OrganizeScrapeSummary{
			{Name: "A", Processed: 3, Matched: 2},
			{Name: "B", Processed: 4, Matched: 1, Error: "failed"},
			{Name: "C", Skipped: true},
		},
	})

	if metrics["scrapes"] != 3 {
		t.Fatalf("scrapes = %d, want 3", metrics["scrapes"])
	}
	if metrics["scrape_processed"] != 7 {
		t.Fatalf("scrape_processed = %d, want 7", metrics["scrape_processed"])
	}
	if metrics["scrape_matched"] != 3 {
		t.Fatalf("scrape_matched = %d, want 3", metrics["scrape_matched"])
	}
	if metrics["scrape_errors"] != 1 {
		t.Fatalf("scrape_errors = %d, want 1", metrics["scrape_errors"])
	}
	if metrics["scrape_skipped"] != 1 {
		t.Fatalf("scrape_skipped = %d, want 1", metrics["scrape_skipped"])
	}
}

func TestTaskTrackerCancelMovesTaskToRecent(t *testing.T) {
	tracker := NewTaskTrackerService(nil, nil)
	handle := tracker.Start(TaskKindArtwork, "artwork", TaskUpdate{Stage: "running"})
	handle.Cancel(TaskUpdate{Stage: "canceled", Message: "已取消"})

	snapshot := tracker.Snapshot()
	if len(snapshot.Active) != 0 || len(snapshot.Recent) != 1 {
		t.Fatalf("snapshot = %#v", snapshot)
	}
	if snapshot.Recent[0].Status != TaskStatusCanceled || snapshot.Recent[0].Stage != "canceled" {
		t.Fatalf("canceled task = %#v", snapshot.Recent[0])
	}
}

func TestTaskTrackerStartUniqueRejectsDuplicateKind(t *testing.T) {
	tracker := NewTaskTrackerService(nil, nil)
	first, started := tracker.StartUnique(TaskKindProbe, "first", TaskUpdate{Stage: "queued"})
	if !started || first == nil || first.ID() == "" {
		t.Fatal("first unique task was not started")
	}
	if second, started := tracker.StartUnique(TaskKindProbe, "second", TaskUpdate{Stage: "queued"}); started || second != nil {
		t.Fatal("duplicate unique task should not start")
	}
	first.Finish(nil, TaskUpdate{Stage: "completed"})
	if next, started := tracker.StartUnique(TaskKindProbe, "next", TaskUpdate{Stage: "queued"}); !started || next == nil {
		t.Fatal("completed unique task should allow a new task")
	}
}
