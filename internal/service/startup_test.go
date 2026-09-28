package service

import (
	"context"
	"errors"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/ShukeBta/MediaStationGo/internal/model"
	"github.com/ShukeBta/MediaStationGo/internal/repository"
	"go.uber.org/zap"
)

func TestStartupProgressRemainsReadableDuringBlockedStep(t *testing.T) {
	c := &Container{Startup: NewStartupState(), stopCtx: t.Context()}
	c.Startup.started = time.Now().Add(-10 * time.Second)
	entered, release, done := make(chan struct{}), make(chan struct{}), make(chan struct{})
	go func() {
		defer close(done)
		_ = c.startupStep("建立媒体库目录监听", func() error {
			c.Startup.updateDirectories(200, 128)
			close(entered)
			<-release
			return errors.New("private filesystem path")
		})
	}()
	<-entered
	status := c.StartupStatus()
	close(release)
	<-done
	if status.State != "starting" || status.ElapsedSeconds < 10 || status.DirectoriesFound != 200 || status.DirectoriesWatched != 128 {
		t.Fatalf("progress: %#v", status)
	}
	c.Startup.finish("ready")
	status = c.StartupStatus()
	if status.State != "ready" || len(status.Warnings) != 1 || strings.Contains(status.Warnings[0], "private") {
		t.Fatalf("status leaks error: %#v", status)
	}
	status.Warnings[0] = "changed"
	if c.StartupStatus().Warnings[0] == "changed" {
		t.Fatal("mutable snapshot alias")
	}
}

func TestStartupBootCloseCoordinatesWatcherAndScheduler(t *testing.T) {
	for _, shutdown := range []bool{false, true} {
		t.Run(map[bool]string{false: "ready", true: "shutdown"}[shutdown], func(t *testing.T) {
			db := newServiceTestDB(t, &model.Library{}, &model.Setting{})
			repo := repository.New(db)
			repo.Media = nil
			ctx, cancel := context.WithCancel(t.Context())
			c := &Container{Repo: repo, Log: zap.NewNop(), Startup: NewStartupState(), stopCtx: ctx, stopCancel: cancel}
			c.Watcher = NewWatcherService(c.Log, repo, nil)
			c.Scheduler = NewSchedulerService(c.Log, repo, nil, nil, nil, nil, nil, "")
			entered, release, done := make(chan struct{}), make(chan struct{}), make(chan struct{})
			var once sync.Once
			c.Watcher.progress = func(found, watched int) {
				c.Startup.updateDirectories(found, watched)
				once.Do(func() { close(entered); <-release })
			}
			go func() { c.Boot(); close(done) }()
			select {
			case <-entered:
			case <-time.After(3 * time.Second):
				t.Fatal("watcher not reached")
			}
			if len(c.Scheduler.Status()) != 0 || c.StartupStatus().State != "starting" {
				t.Fatal("scheduler started before watcher completed")
			}
			closed := make(chan struct{})
			if shutdown {
				go func() { c.Close(); close(closed) }()
				<-ctx.Done()
				select {
				case <-closed:
					t.Fatal("close overtook boot")
				default:
				}
			}
			close(release)
			<-done
			if shutdown {
				select {
				case <-closed:
				case <-time.After(3 * time.Second):
					t.Fatal("close stuck")
				}
				if c.StartupStatus().State != "failed" || len(c.Scheduler.Status()) != 0 {
					t.Fatal("canceled startup marked ready")
				}
			} else {
				if c.StartupStatus().State != "ready" || len(c.Scheduler.Status()) == 0 {
					t.Fatal("startup never ready")
				}
				c.Boot()
				c.Close()
				c.Close()
			}
		})
	}
}

func TestStartupCanceledBootRegistersNoJobs(t *testing.T) {
	ctx, cancel := context.WithCancel(t.Context())
	cancel()
	c := &Container{Startup: NewStartupState(), stopCtx: ctx, Scheduler: NewSchedulerService(zap.NewNop(), nil, nil, nil, nil, nil, nil, "")}
	c.Boot()
	defer c.Scheduler.Stop()
	if c.StartupStatus().State != "failed" || len(c.Scheduler.Status()) != 0 {
		t.Fatal("canceled boot ran services")
	}
}
