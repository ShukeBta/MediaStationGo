package service

import (
	"context"
	"errors"
	"testing"

	"github.com/ShukeBta/MediaStationGo/internal/model"
)

func TestBeginCloudScanAllowsQueuedEntryToStart(t *testing.T) {
	scanner := &ScannerService{
		cloudScans: map[string]*cloudScanEntry{
			"lib-1": {status: CloudScanStatus{LibraryID: "lib-1", Provider: "openlist", State: "queued", Stage: "queued"}},
		},
	}
	lib := &model.Library{Base: model.Base{ID: "lib-1"}, Name: "Movies"}
	mount := CloudMountInfo{Provider: "openlist"}

	_, finish, err := scanner.beginCloudScan(context.Background(), lib, mount)
	if err != nil {
		t.Fatalf("queued scan should be allowed to start, got %v", err)
	}
	if finish == nil {
		t.Fatal("finish callback should not be nil")
	}
	statuses := scanner.CloudScanStatuses()
	if len(statuses) != 1 || statuses[0].State != "running" || statuses[0].Stage != "listing" {
		t.Fatalf("status after begin = %#v, want running/listing", statuses)
	}

	finish(&ScanResult{Visited: 5, Added: 2, Updated: 1, ErrorCount: 1, Errors: []string{"bad file"}}, nil)
	statuses = scanner.CloudScanStatuses()
	if len(statuses) != 1 || statuses[0].State != "finished" || statuses[0].Visited != 5 || statuses[0].Added != 2 || statuses[0].ErrorCount != 1 {
		t.Fatalf("status after finish = %#v", statuses)
	}
	if statuses[0].Error == "" {
		t.Fatal("finished scan with error_count should keep summary error text")
	}
}

func TestBeginCloudScanRejectsRunningEntry(t *testing.T) {
	scanner := &ScannerService{
		cloudScans: map[string]*cloudScanEntry{
			"lib-1": {status: CloudScanStatus{LibraryID: "lib-1", Provider: "openlist", State: "running", Stage: "listing"}},
		},
	}
	lib := &model.Library{Base: model.Base{ID: "lib-1"}, Name: "Movies"}

	_, finish, err := scanner.beginCloudScan(context.Background(), lib, CloudMountInfo{Provider: "openlist"})
	if !errors.Is(err, ErrCloudScanAlreadyRunning) {
		t.Fatalf("err = %v, want ErrCloudScanAlreadyRunning", err)
	}
	if finish != nil {
		t.Fatal("finish callback should be nil when begin is rejected")
	}
}
