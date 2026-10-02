package service

import (
	"context"
	"errors"
	"sync"
	"testing"
	"time"

	"github.com/ShukeBta/MediaStationGo/internal/model"
	"github.com/ShukeBta/MediaStationGo/internal/repository"
)

type memoryPluginSettings struct {
	mu     sync.Mutex
	values map[string]string
	fail   bool
}

func (s *memoryPluginSettings) Get(ctx context.Context, key string) (string, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.values[key], ctx.Err()
}

func (s *memoryPluginSettings) Set(ctx context.Context, key, value string) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.fail {
		return errors.New("write failed")
	}
	if err := ctx.Err(); err != nil {
		return err
	}
	s.values[key] = value
	return nil
}

type testPlugin struct {
	run func(context.Context, map[string]any) (PluginRunResult, error)
}

func (p *testPlugin) Manifest() PluginManifest {
	return PluginManifest{ID: "test-plugin", Name: "Test", Version: "1", ConfigFields: []PluginConfigField{{Key: "flag", Type: "boolean"}}}
}
func (p *testPlugin) ValidateConfig(map[string]any) error { return nil }
func (p *testPlugin) Run(ctx context.Context, config map[string]any) (PluginRunResult, error) {
	return p.run(ctx, config)
}

func testPluginManager(t *testing.T, p *testPlugin) (*PluginService, *memoryPluginSettings) {
	t.Helper()
	store := &memoryPluginSettings{values: make(map[string]string)}
	s := &PluginService{settings: store, entries: make(map[string]*pluginEntry)}
	if err := s.Register(p); err != nil {
		t.Fatal(err)
	}
	return s, store
}

func TestPluginStateValidationPersistenceAndCopies(t *testing.T) {
	ctx := context.Background()
	p := &testPlugin{run: func(context.Context, map[string]any) (PluginRunResult, error) { return PluginRunResult{}, nil }}
	s, store := testPluginManager(t, p)
	if _, err := s.Run(ctx, "test-plugin"); !errors.Is(err, ErrPluginDisabled) {
		t.Fatalf("disabled run: %v", err)
	}
	if _, err := s.Run(ctx, "missing"); !errors.Is(err, ErrPluginNotFound) {
		t.Fatalf("missing: %v", err)
	}
	for _, config := range []map[string]any{{"flag": "true"}, {"flag": true, "unknown": false}, {}} {
		if _, err := s.Update(ctx, "test-plugin", PluginUpdate{Config: config}); !errors.Is(err, ErrPluginInvalidConfig) {
			t.Fatalf("invalid config %v: %v", config, err)
		}
	}
	if len(store.values) != 0 {
		t.Fatal("invalid update persisted")
	}
	on := true
	config := map[string]any{"flag": true}
	info, err := s.Update(ctx, "test-plugin", PluginUpdate{Enabled: &on, Config: config})
	if err != nil {
		t.Fatal(err)
	}
	config["flag"] = false
	info.Config["flag"] = false
	info.ConfigFields[0].Key = "changed"
	restarted := &PluginService{settings: store, entries: make(map[string]*pluginEntry)}
	if err := restarted.Register(p); err != nil {
		t.Fatal(err)
	}
	items, err := restarted.List(ctx)
	if err != nil || !items[0].Enabled || items[0].Config["flag"] != true || items[0].ConfigFields[0].Key != "flag" {
		t.Fatalf("state lost/aliased: %#v %v", items, err)
	}
	store.fail = true
	off := false
	if _, err := s.Update(ctx, "test-plugin", PluginUpdate{Enabled: &off}); err == nil {
		t.Fatal("write failure accepted")
	}
	items, err = s.List(ctx)
	if err != nil || !items[0].Enabled {
		t.Fatal("failed persistence changed state")
	}
}

func TestPluginConcurrentRunAndUpdate(t *testing.T) {
	ctx := context.Background()
	entered, release := make(chan struct{}), make(chan struct{})
	p := &testPlugin{run: func(ctx context.Context, _ map[string]any) (PluginRunResult, error) {
		close(entered)
		select {
		case <-release:
		case <-ctx.Done():
			return PluginRunResult{}, ctx.Err()
		}
		return PluginRunResult{Summary: "done", Metrics: []PluginMetric{{Label: "count", Value: 7}}}, nil
	}}
	s, _ := testPluginManager(t, p)
	on := true
	if _, err := s.Update(ctx, "test-plugin", PluginUpdate{Enabled: &on}); err != nil {
		t.Fatal(err)
	}
	done := make(chan error, 1)
	go func() { _, err := s.Run(ctx, "test-plugin"); done <- err }()
	select {
	case <-entered:
	case <-time.After(time.Second):
		t.Fatal("run did not begin")
	}
	t.Cleanup(func() {
		select {
		case <-release:
		default:
			close(release)
		}
	})
	if _, err := s.Run(ctx, "test-plugin"); !errors.Is(err, ErrPluginBusy) {
		t.Fatalf("concurrent run: %v", err)
	}
	if _, err := s.Update(ctx, "test-plugin", PluginUpdate{Enabled: &on}); !errors.Is(err, ErrPluginBusy) {
		t.Fatalf("concurrent update: %v", err)
	}
	items, err := s.List(ctx)
	if err != nil || items[0].Status != "running" {
		t.Fatalf("running state: %#v %v", items, err)
	}
	close(release)
	if err := <-done; err != nil {
		t.Fatal(err)
	}
	items, _ = s.List(ctx)
	if items[0].Status != "ready" || items[0].LastRun == nil || items[0].LastRun.CompletedAt.IsZero() {
		t.Fatalf("completion: %#v", items)
	}
	items[0].LastRun.Metrics[0].Value = 99
	items, _ = s.List(ctx)
	if items[0].LastRun.Metrics[0].Value != 7 {
		t.Fatal("result was aliased")
	}
}

func TestPluginPanicAndCancellationReleaseRun(t *testing.T) {
	for _, mode := range []string{"panic", "cancel"} {
		t.Run(mode, func(t *testing.T) {
			p := &testPlugin{run: func(ctx context.Context, _ map[string]any) (PluginRunResult, error) {
				if mode == "panic" {
					panic("secret panic detail")
				}
				<-ctx.Done()
				return PluginRunResult{}, ctx.Err()
			}}
			s, _ := testPluginManager(t, p)
			on := true
			if _, err := s.Update(context.Background(), "test-plugin", PluginUpdate{Enabled: &on}); err != nil {
				t.Fatal(err)
			}
			ctx, cancel := context.WithTimeout(context.Background(), 20*time.Millisecond)
			defer cancel()
			if _, err := s.Run(ctx, "test-plugin"); err == nil {
				t.Fatal("failure accepted")
			}
			items, _ := s.List(context.Background())
			if items[0].Status != "error" || items[0].LastRun != nil || items[0].LastError == "secret panic detail" {
				t.Fatalf("failure state: %#v", items)
			}
			p.run = func(context.Context, map[string]any) (PluginRunResult, error) {
				return PluginRunResult{Summary: "recovered"}, nil
			}
			if _, err := s.Run(context.Background(), "test-plugin"); err != nil {
				t.Fatal(err)
			}
			items, _ = s.List(context.Background())
			if items[0].LastError != "" || items[0].Status != "ready" {
				t.Fatal("error not cleared after recovery")
			}
		})
	}
}

func TestPluginLibrarySummaryFiltersAndRestart(t *testing.T) {
	db := newServiceTestDB(t, &model.Setting{}, &model.Library{}, &model.Media{})
	ctx := context.Background()
	for _, id := range []string{"enabled", "disabled", "deleted"} {
		if err := db.Create(&model.Library{Base: model.Base{ID: id}, Name: id, Enabled: true}).Error; err != nil {
			t.Fatal(err)
		}
		if err := db.Create(&model.Media{Base: model.Base{ID: "media-" + id}, LibraryID: id, Title: id, Path: "/fixtures/" + id + ".mp4"}).Error; err != nil {
			t.Fatal(err)
		}
	}
	if err := db.Model(&model.Library{}).Where("id = ?", "disabled").Update("enabled", false).Error; err != nil {
		t.Fatal(err)
	}
	if err := db.Delete(&model.Library{}, "id = ?", "deleted").Error; err != nil {
		t.Fatal(err)
	}
	if err := db.Create(&model.Media{Base: model.Base{ID: "removed"}, LibraryID: "enabled", Path: "/fixtures/removed.mp4"}).Error; err != nil {
		t.Fatal(err)
	}
	if err := db.Delete(&model.Media{}, "id = ?", "removed").Error; err != nil {
		t.Fatal(err)
	}
	repos := repository.New(db)
	s := NewPluginService(repos)
	on := true
	if _, err := s.Update(ctx, "library-summary", PluginUpdate{Enabled: &on}); err != nil {
		t.Fatal(err)
	}
	result, err := s.Run(ctx, "library-summary")
	if err != nil || result.Metrics[0].Value != 1 || result.Metrics[1].Value != 1 {
		t.Fatalf("enabled counts: %#v %v", result, err)
	}
	if _, err := s.Update(ctx, "library-summary", PluginUpdate{Config: map[string]any{"include_disabled": true}}); err != nil {
		t.Fatal(err)
	}
	s = NewPluginService(repos)
	result, err = s.Run(ctx, "library-summary")
	if err != nil || result.Metrics[0].Value != 2 || result.Metrics[1].Value != 2 {
		t.Fatalf("all counts after restart: %#v %v", result, err)
	}
}
