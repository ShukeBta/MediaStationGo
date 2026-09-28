package service

import (
	"context"
	"errors"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"go.uber.org/zap"
)

func TestLocalScanAdmissionSerializesAndRejectsStaleOwnership(t *testing.T) {
	s := &ScannerService{}
	ctx, release, ok := s.TryReserveLocalScan(t.Context())
	if !ok {
		t.Fatal("first rejected")
	}
	if _, _, ok := s.TryReserveLocalScan(t.Context()); ok {
		t.Fatal("concurrent manual scan accepted")
	}
	_, nested, err := s.WaitReserveLocalScan(ctx)
	if err != nil {
		t.Fatal(err)
	}
	nested()
	if _, _, ok := s.TryReserveLocalScan(t.Context()); ok {
		t.Fatal("nested release freed parent slot")
	}
	canceled, cancel := context.WithCancel(t.Context())
	cancel()
	if _, _, err := s.WaitReserveLocalScan(canceled); !errors.Is(err, context.Canceled) {
		t.Fatalf("cancel: %v", err)
	}
	release()
	release()
	_, second, ok := s.TryReserveLocalScan(t.Context())
	if !ok {
		t.Fatal("slot not released")
	}
	if _, _, ok := s.TryReserveLocalScan(ctx); ok {
		t.Fatal("stale context bypassed reservation")
	}
	second()
	var wg sync.WaitGroup
	var active atomic.Int32
	for range 20 {
		wg.Add(1)
		go func() {
			defer wg.Done()
			_, done, err := s.WaitReserveLocalScan(t.Context())
			if err != nil {
				t.Error(err)
				return
			}
			defer done()
			if active.Add(1) != 1 {
				t.Error("parallel local scans")
			}
			time.Sleep(time.Millisecond)
			active.Add(-1)
		}()
	}
	wg.Wait()
}

func TestSchedulerScanAdmissionAndShutdown(t *testing.T) {
	scan := &ScannerService{}
	s := NewSchedulerService(zap.NewNop(), nil, scan, nil, nil, nil, nil, "")
	entered := make(chan struct{})
	s.jobs = []*scheduledJob{{name: "library_scan", run: func(ctx context.Context) error { close(entered); <-ctx.Done(); return ctx.Err() }}}
	_, release, ok := scan.TryReserveLocalScan(t.Context())
	if !ok {
		t.Fatal("reserve")
	}
	if err := s.RunNowAsync(t.Context(), "library_scan"); !errors.Is(err, ErrLocalScanAlreadyRunning) {
		t.Fatalf("expected scan conflict: %v", err)
	}
	if s.Status()[0].Running {
		t.Fatal("rejected scheduler scan marked running")
	}
	release()
	if err := s.RunNowAsync(t.Context(), "library_scan"); err != nil {
		t.Fatal(err)
	}
	<-entered
	done := make(chan struct{})
	go func() { s.Stop(); close(done) }()
	select {
	case <-done:
	case <-time.After(time.Second):
		t.Fatal("stop did not cancel running scan")
	}
	if err := s.RunNowAsync(t.Context(), "library_scan"); !errors.Is(err, context.Canceled) {
		t.Fatalf("scan started after shutdown: %v", err)
	}
}
