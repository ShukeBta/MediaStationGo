package service

import (
	"context"
	"encoding/json"
	"errors"
	"github.com/ShukeBta/MediaStationGo/internal/model"
	"github.com/ShukeBta/MediaStationGo/internal/repository"
	"go.uber.org/zap"
	"strings"
	"testing"
	"time"
)

func TestTaskPersistenceRestartRevisionAndDailyPagination(t *testing.T) {
	db := newServiceTestDB(t, &model.TaskExecution{}, &model.TaskLogEntry{})
	tracker := NewTaskTrackerService(zap.NewNop(), nil)
	now := time.Date(2026, 9, 27, 23, 59, 59, 0, time.UTC)
	tracker.now = func() time.Time { return now }
	if err := tracker.SetPersistence(db); err != nil {
		t.Fatal(err)
	}
	handle := tracker.Start("scan", "扫描", TaskUpdate{Message: "starting", Details: []string{"token=secret", "https://example.com/private?passkey=secret"}})
	stale := tracker.Snapshot().Active[0]
	now = now.Add(2 * time.Second)
	handle.Update(TaskUpdate{Message: "running", Details: []string{"token=secret", "new file"}})
	var beforeCount int64
	db.Model(&model.TaskLogEntry{}).Count(&beforeCount)
	handle.Update(TaskUpdate{Message: "running", Details: []string{"token=secret", "new file"}})
	var afterCount int64
	db.Model(&model.TaskLogEntry{}).Count(&afterCount)
	if afterCount != beforeCount {
		t.Fatal("identical update duplicated log lines")
	}
	tracker.persist(stale)
	var record model.TaskExecution
	if err := db.First(&record, "id = ?", handle.ID()).Error; err != nil {
		t.Fatal(err)
	}
	if record.Revision != 3 || strings.Contains(record.Snapshot, "secret") || strings.Contains(record.Snapshot, "https://") {
		t.Fatalf("snapshot = %s, revision=%d", record.Snapshot, record.Revision)
	}
	restarted := NewTaskTrackerService(zap.NewNop(), nil)
	restarted.now = func() time.Time { return now }
	if err := restarted.SetPersistence(db); err != nil {
		t.Fatal(err)
	}
	snap := restarted.Snapshot()
	if len(snap.Active) != 0 || len(snap.Recent) != 1 || snap.Recent[0].Status != TaskStatusFailed || snap.Recent[0].FinishedAt == nil {
		t.Fatalf("recovered %#v", snap)
	}
	repo := repository.New(db)
	filter := repository.TaskHistoryFilter{From: time.Date(2026, 9, 27, 0, 0, 0, 0, time.UTC), To: time.Date(2026, 9, 29, 0, 0, 0, 0, time.UTC), Page: 1, PageSize: 1}
	days, err := repo.TaskLogDays(t.Context(), filter)
	if err != nil || len(days) != 2 || days[0].Day != "2026-09-28" || days[1].Day != "2026-09-27" {
		t.Fatalf("days=%#v err=%v", days, err)
	}
	first, total, err := repo.TaskLogs(t.Context(), filter)
	if err != nil || len(first) != 1 || total < 2 {
		t.Fatalf("logs=%#v total=%d err=%v", first, total, err)
	}
	filter.Page = 2
	second, _, err := repo.TaskLogs(t.Context(), filter)
	if err != nil || len(second) != 1 || second[0].ID == first[0].ID {
		t.Fatalf("pagination %#v %v", second, err)
	}
	raw, _ := json.Marshal(first)
	if strings.Contains(string(raw), "secret") {
		t.Fatal("credential persisted")
	}
	filter.Kind = "other"
	filtered, total, err := repo.TaskLogs(t.Context(), filter)
	if err != nil || total != 0 || len(filtered) != 0 {
		t.Fatalf("kind filter %#v %d %v", filtered, total, err)
	}
}

func TestTaskPersistenceFinishAndCancellationSurviveRestart(t *testing.T) {
	db := newServiceTestDB(t, &model.TaskExecution{}, &model.TaskLogEntry{})
	tracker := NewTaskTrackerService(zap.NewNop(), nil)
	if err := tracker.SetPersistence(db); err != nil {
		t.Fatal(err)
	}
	completed := tracker.Start("scrape", "complete", TaskUpdate{})
	completed.Finish(nil, TaskUpdate{Message: "done"})
	canceled := tracker.Start("scrape", "cancel", TaskUpdate{})
	canceled.Cancel(TaskUpdate{Message: "canceled"})
	failed := tracker.Start("scrape", "fail", TaskUpdate{})
	failed.Finish(errors.New("request token=private failed"), TaskUpdate{})
	restarted := NewTaskTrackerService(zap.NewNop(), nil)
	if err := restarted.SetPersistence(db); err != nil {
		t.Fatal(err)
	}
	statuses := map[string]bool{}
	for _, task := range restarted.Snapshot().Recent {
		statuses[task.Status] = true
		if strings.Contains(task.Error, "private") {
			t.Fatal("unredacted error")
		}
	}
	if !statuses[TaskStatusCanceled] || !statuses[TaskStatusCompleted] || !statuses[TaskStatusFailed] {
		t.Fatal(statuses)
	}
}

func TestTaskLogRedactionCoversStructuredCredentials(t *testing.T) {
	for _, value := range []string{`{"token":"secret","next":"value"}`, "Cookie: session=secret; other=secret", "Authorization: Bearer secret", "GET https://example.com/path/secret?foo=value", "passkey='secret'"} {
		if got := redactTaskText(value); strings.Contains(got, "secret") {
			t.Fatalf("credential remained: %s", got)
		}
	}
}

func TestSchedulerTrackedRunRecoversPanicAndCanRunAgain(t *testing.T) {
	tracker := NewTaskTrackerService(zap.NewNop(), nil)
	job := &scheduledJob{name: "test", run: func(context.Context) error { panic("provider failure") }}
	scheduler := &SchedulerService{log: zap.NewNop(), tasks: tracker, jobs: []*scheduledJob{job}, now: time.Now}
	if err := scheduler.RunNow(t.Context(), "test"); err == nil {
		t.Fatal("panic was not recorded as failure")
	}
	if job.running || len(tracker.Snapshot().Active) != 0 || tracker.Snapshot().Recent[0].Status != TaskStatusFailed {
		t.Fatal("panic left task running")
	}
	job.run = func(context.Context) error { return nil }
	if err := scheduler.RunNow(t.Context(), "test"); err != nil {
		t.Fatal(err)
	}
	if tracker.Snapshot().Recent[0].Status != TaskStatusCompleted {
		t.Fatal("retry did not finish")
	}
}

func TestTaskPendingScrapeFiltersAndOmitsSourceURL(t *testing.T) {
	db := newServiceTestDB(t, &model.Media{})
	rows := []model.Media{
		{Base: model.Base{ID: "a"}, LibraryID: "one", Title: "pending", Path: "a.strm", STRMURL: "https://example.com/token=secret", ScrapeStatus: "pending"},
		{Base: model.Base{ID: "b"}, LibraryID: "two", Title: "failed", Path: "b.mp4", ScrapeStatus: "failed"},
		{Base: model.Base{ID: "c"}, LibraryID: "one", Title: "matched", Path: "c.mp4", ScrapeStatus: "matched"},
		{Base: model.Base{ID: "d"}, LibraryID: "one", Title: "deleted", Path: "d.mp4", ScrapeStatus: "pending"},
	}
	if err := db.Create(&rows).Error; err != nil {
		t.Fatal(err)
	}
	db.Delete(&rows[3])
	items, total, err := repository.New(db).PendingScrape(t.Context(), "one", 1, 25)
	if err != nil || total != 1 || len(items) != 1 || !items[0].IsSTRM {
		t.Fatalf("pending=%#v total=%d err=%v", items, total, err)
	}
	raw, _ := json.Marshal(items)
	if strings.Contains(string(raw), "secret") || strings.Contains(string(raw), "https://") {
		t.Fatal("pending API exposes signed URL")
	}
}
