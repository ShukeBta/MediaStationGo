package service

import (
	"github.com/ShukeBta/MediaStationGo/internal/model"
	"github.com/ShukeBta/MediaStationGo/internal/repository"
	"go.uber.org/zap"
	"testing"
	"time"
)

func TestPeopleWorkerTracksFailuresAndAvoidsEmptyTaskNoise(t *testing.T) {
	db := newServiceTestDB(t, &model.Person{}, &model.PersonCredit{}, &model.TaskExecution{}, &model.TaskLogEntry{})
	tracker := NewTaskTrackerService(zap.NewNop(), nil)
	if err := tracker.SetPersistence(db); err != nil {
		t.Fatal(err)
	}
	c := &Container{Repo: repository.New(db), Tasks: tracker, Log: zap.NewNop()}
	c.runPeopleEnrichment(t.Context())
	if len(tracker.Snapshot().Recent) != 0 {
		t.Fatal("empty worker created task noise")
	}
	now := time.Now()
	person := model.Person{Name: "Example", NameKey: normalizePersonNameKey("Example"), Source: "tmdb", SourceID: "7", DetailsRefreshedAt: &now}
	if err := db.Create(&person).Error; err != nil {
		t.Fatal(err)
	}
	c.runPeopleEnrichment(t.Context())
	snap := tracker.Snapshot()
	if len(snap.Recent) != 1 || len(snap.Active) != 0 || snap.Recent[0].Status != TaskStatusFailed || snap.Recent[0].Metrics["processed"] != 1 || snap.Recent[0].Metrics["errors"] != 1 {
		t.Fatalf("tasks=%#v", snap)
	}
	var total int64
	if err := db.Model(&model.TaskExecution{}).Where("kind = ?", TaskKindPeople).Count(&total).Error; err != nil || total != 1 {
		t.Fatalf("persisted=%d err=%v", total, err)
	}
	manual := tracker.Start(TaskKindPeople, "manual", TaskUpdate{})
	c.runPeopleEnrichment(t.Context())
	if len(tracker.Snapshot().Recent) != 1 {
		t.Fatal("worker did not defer to active people task")
	}
	manual.Finish(nil, TaskUpdate{})
}
