package service

import (
	"context"
	"encoding/json"
	"reflect"
	"sync"
	"testing"

	"github.com/ShukeBta/MediaStationGo/internal/model"
	"go.uber.org/zap"
	"go.uber.org/zap/zaptest/observer"
)

func TestEpisodeTraceOptInAndPayloadUnchanged(t *testing.T) {
	e, _ := embyProjectionFixture(t, 1, 20)
	if _, err := e.InitializeBrowseKeys(t.Context()); err != nil {
		t.Fatal(err)
	}
	var row model.Media
	if err := e.repo.DB.First(&row).Error; err != nil {
		t.Fatal(err)
	}
	core, logs := observer.New(zap.InfoLevel)
	e.log = zap.New(core)
	p := ItemsParams{ShowID: row.EmbySeriesKey, ParentID: seasonID(row.EmbySeriesKey, 1), Limit: 48}
	t.Setenv("MEDIASTATION_DIAGNOSTICS_EPISODES", "")
	ctx, finish := e.BeginEpisodeTrace(t.Context())
	if ctx != t.Context() {
		t.Fatal("disabled tracing changed context")
	}
	want, err := e.Items(ctx, p)
	if err != nil {
		t.Fatal(err)
	}
	finish(200, 0)
	if logs.Len() != 0 {
		t.Fatal("disabled tracing emitted logs")
	}
	t.Setenv("MEDIASTATION_DIAGNOSTICS_EPISODES", "1")
	ctx, finish = e.BeginEpisodeTrace(t.Context())
	serviceDone := MeasureEpisodeStage(ctx, "service_total")
	got, err := e.Items(ctx, p)
	serviceDone()
	if err != nil {
		t.Fatal(err)
	}
	encodeDone := MeasureEpisodeStage(ctx, "json_encode_and_write")
	body, err := json.Marshal(got)
	encodeDone()
	if err != nil {
		t.Fatal(err)
	}
	before, _ := json.Marshal(want)
	if !reflect.DeepEqual(before, body) {
		t.Fatal("tracing changed payload")
	}
	finish(200, len(body))
	if logs.Len() != 1 {
		t.Fatalf("logs=%d", logs.Len())
	}
	fields := logs.All()[0].ContextMap()
	stages := fields["stage_ms"].(map[string]float64)
	for _, key := range []string{"projection_check_repair", "season_identity_query", "season_media_query", "episode_filter_sort", "episode_version_collapse", "library_snapshot", "series_titles", "media_version_siblings", "user_favorites_history", "item_payload_assembly", "json_encode_and_write"} {
		if _, ok := stages[key]; !ok {
			t.Errorf("missing stage %s", key)
		}
	}
	t.Logf("local SQLite trace only: %v", fields)
}

func TestEpisodeTraceConcurrentRequestsDoNotShareStages(t *testing.T) {
	t.Setenv("MEDIASTATION_DIAGNOSTICS_EPISODES", "1")
	core, logs := observer.New(zap.InfoLevel)
	e := &EmbyService{log: zap.New(core)}
	var wg sync.WaitGroup
	for i := 0; i < 9; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			ctx, finish := e.BeginEpisodeTrace(context.Background())
			MeasureEpisodeStage(ctx, "only_stage")()
			finish(200, 1)
		}()
	}
	wg.Wait()
	if logs.Len() != 9 {
		t.Fatalf("logs=%d", logs.Len())
	}
	for _, entry := range logs.All() {
		if len(entry.ContextMap()["stage_ms"].(map[string]float64)) != 1 {
			t.Fatal("shared stages")
		}
	}
}
