package service

import (
	"context"
	"errors"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"go.uber.org/zap"

	"github.com/ShukeBta/MediaStationGo/internal/helper"
)

func TestSchedulerRunNowAsyncSurvivesCallerCancellation(t *testing.T) {
	scheduler := NewSchedulerService(zap.NewNop(), nil, nil, nil, nil, nil, nil, "")
	started := make(chan struct{})
	release := make(chan struct{})
	finished := make(chan struct{})
	scheduler.jobs = []*scheduledJob{{
		name:     "organize_source",
		interval: time.Minute,
		run: func(ctx context.Context) error {
			close(started)
			defer close(finished)
			select {
			case <-ctx.Done():
				return ctx.Err()
			case <-release:
				return nil
			}
		},
	}}

	ctx, cancel := context.WithCancel(t.Context())
	if err := scheduler.RunNowAsync(ctx, "organize_source"); err != nil {
		t.Fatalf("run now async: %v", err)
	}
	<-started
	cancel()
	select {
	case <-finished:
		t.Fatal("manual scheduled job was canceled with the HTTP caller context")
	case <-time.After(50 * time.Millisecond):
	}
	close(release)
	select {
	case <-finished:
	case <-time.After(time.Second):
		t.Fatal("manual scheduled job did not finish after release")
	}
	var status []JobStatus
	deadline := time.Now().Add(time.Second)
	for {
		status = scheduler.Status()
		if len(status) == 1 && !status[0].Running {
			break
		}
		if time.Now().After(deadline) {
			break
		}
		time.Sleep(10 * time.Millisecond)
	}
	if len(status) != 1 || status[0].Running || status[0].LastErr != "" {
		t.Fatalf("unexpected status after async run: %+v", status)
	}
}

func TestSchedulerRunNowAsyncRejectsDuplicateRun(t *testing.T) {
	scheduler := NewSchedulerService(zap.NewNop(), nil, nil, nil, nil, nil, nil, "")
	started := make(chan struct{})
	release := make(chan struct{})
	scheduler.jobs = []*scheduledJob{{
		name:     "organize_source",
		interval: time.Minute,
		run: func(ctx context.Context) error {
			close(started)
			<-release
			return nil
		},
	}}

	if err := scheduler.RunNowAsync(t.Context(), "organize_source"); err != nil {
		t.Fatalf("first run now async: %v", err)
	}
	<-started
	if err := scheduler.RunNowAsync(t.Context(), "organize_source"); !errors.Is(err, ErrSchedulerJobAlreadyRunning) {
		t.Fatalf("duplicate run error = %v, want %v", err, ErrSchedulerJobAlreadyRunning)
	}
	close(release)
}

func TestSchedulerStartDoesNotRegisterSubscriptionPullJob(t *testing.T) {
	scheduler := NewSchedulerService(zap.NewNop(), nil, nil, nil, nil, nil, nil, "")
	ctx, cancel := context.WithCancel(t.Context())
	cancel()
	scheduler.Start(ctx)
	defer scheduler.Stop()

	for _, status := range scheduler.Status() {
		if strings.Contains(status.Name, "subscription") {
			t.Fatalf("scheduler registered subscription job %q; subscriptions must be owned by SubscriptionService only", status.Name)
		}
	}
}

func TestSchedulerStartRegistersFD2PPVSessionCheck(t *testing.T) {
	scheduler := NewSchedulerService(zap.NewNop(), nil, nil, nil, nil, nil, nil, "")
	ctx, cancel := context.WithCancel(t.Context())
	cancel()
	scheduler.Start(ctx)
	defer scheduler.Stop()

	for _, status := range scheduler.Status() {
		if status.Name == "fd2ppv_session_check" {
			if status.Interval != fd2PPVSessionCheckInterval.String() {
				t.Fatalf("interval = %q, want %q", status.Interval, fd2PPVSessionCheckInterval)
			}
			return
		}
	}
	t.Fatal("fd2ppv_session_check was not registered")
}

func TestFD2PPVSessionTimeoutCoversConfiguredFlareSolverrAttempts(t *testing.T) {
	if got, want := fd2PPVSessionTimeout(60), 200*time.Second; got != want {
		t.Fatalf("timeout = %s, want %s", got, want)
	}
	if got, want := fd2PPVSessionTimeout(0), 200*time.Second; got != want {
		t.Fatalf("default timeout = %s, want %s", got, want)
	}
	if got, want := fd2PPVSessionTimeout(5), 90*time.Second; got != want {
		t.Fatalf("custom timeout = %s, want %s", got, want)
	}
}

func TestSchedulerFD2PPVSessionCheckRecordsFailure(t *testing.T) {
	provider := newConfiguredFD2PPVTestProvider(t, "fd2-user", "fd2-password")
	provider.SetFlareSolverr("http://flaresolverr.invalid", 5)
	credentials, err := provider.resolveFD2PPVCredentials(t.Context())
	if err != nil {
		t.Fatal(err)
	}
	provider.fd2ppv.cookies = []helper.FlareSolverrCookie{{Name: "member", Value: "member-token"}}
	provider.fd2ppv.credentialSignature = credentials.signature
	provider.fd2ppv.direct = fd2PPVDirectFetcherFunc(func(
		context.Context,
		string,
		string,
		[]helper.FlareSolverrCookie,
	) (fd2PPVDirectFetchResult, error) {
		return fd2PPVDirectFetchResult{}, errors.New("health probe failed")
	})

	scheduler := NewSchedulerService(zap.NewNop(), nil, nil, nil, nil, nil, nil, "")
	scheduler.SetAdultProvider(provider)
	scheduler.jobs = []*scheduledJob{{
		name:     "fd2ppv_session_check",
		interval: fd2PPVSessionCheckInterval,
		run:      scheduler.jobCheckFD2PPVSession,
	}}
	err = scheduler.RunNow(t.Context(), "fd2ppv_session_check")
	if err == nil || !strings.Contains(err.Error(), "health probe failed") {
		t.Fatalf("run error = %v", err)
	}
	status := scheduler.Status()
	if len(status) != 1 || !strings.Contains(status[0].LastErr, "health probe failed") {
		t.Fatalf("status = %+v", status)
	}
}

func TestSchedulerLoopWaitsIntervalAfterSlowRun(t *testing.T) {
	scheduler := NewSchedulerService(zap.NewNop(), nil, nil, nil, nil, nil, nil, "")
	ctx, cancel := context.WithCancel(t.Context())
	defer cancel()

	var runs atomic.Int32
	job := &scheduledJob{
		name:     "slow",
		interval: 25 * time.Millisecond,
		run: func(ctx context.Context) error {
			runs.Add(1)
			time.Sleep(50 * time.Millisecond)
			return nil
		},
	}

	done := make(chan struct{})
	go func() {
		scheduler.loopWithInitialDelay(ctx, job, time.Millisecond)
		close(done)
	}()
	time.Sleep(120 * time.Millisecond)
	cancel()
	select {
	case <-done:
	case <-time.After(250 * time.Millisecond):
		t.Fatal("scheduler loop did not stop")
	}
	if got := runs.Load(); got > 2 {
		t.Fatalf("slow job ran %d times; scheduler should not catch up missed ticks", got)
	}
}
