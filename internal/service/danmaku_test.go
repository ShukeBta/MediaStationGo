package service

import (
	"context"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/glebarez/sqlite"
	"go.uber.org/zap"
	"gorm.io/gorm"

	"github.com/ShukeBta/MediaStationGo/internal/config"
	"github.com/ShukeBta/MediaStationGo/internal/model"
	"github.com/ShukeBta/MediaStationGo/internal/repository"
)

type fakeDanmakuPipeline struct {
	matchResult   DanmakuMatchResult
	matchErr      error
	payload       DanmakuPayload
	payloadErr    error
	searchResult  DanmakuSearchResult
	searchErr     error
	fetchRequests []DanmakuFetchRequest
	matchCalls    int
}

func (f *fakeDanmakuPipeline) MatchDanmaku(context.Context, string) (DanmakuMatchResult, error) {
	f.matchCalls++
	return f.matchResult, f.matchErr
}

func (f *fakeDanmakuPipeline) FetchDanmaku(_ context.Context, request DanmakuFetchRequest) (DanmakuPayload, error) {
	f.fetchRequests = append(f.fetchRequests, request)
	return f.payload, f.payloadErr
}

func (f *fakeDanmakuPipeline) SearchDanmaku(context.Context, string, int) (DanmakuSearchResult, error) {
	return f.searchResult, f.searchErr
}

func newDanmakuTestService(t *testing.T) (*DanmakuService, *gorm.DB) {
	t.Helper()
	dsn := "file:" + strings.ReplaceAll(t.Name(), "/", "_") + "?mode=memory&cache=shared"
	db, err := gorm.Open(sqlite.Open(dsn), &gorm.Config{})
	if err != nil {
		t.Fatal(err)
	}
	if err := db.AutoMigrate(&model.MediaDanmaku{}); err != nil {
		t.Fatal(err)
	}
	return NewDanmakuService(zap.NewNop(), repository.New(db)), db
}

func matchedResult() DanmakuMatchResult {
	return DanmakuMatchResult{
		Matched:      true,
		Provider:     "dandanplay",
		MatchMode:    "tmdb",
		EpisodeID:    "95410010",
		AnimeTitle:   "进击的巨人",
		EpisodeTitle: "第10话",
		Shift:        2,
		Attempts: []DanmakuAttempt{
			{Source: "dandanplay", Mode: "tmdb", Outcome: "matched", CandidateCount: 1},
		},
	}
}

func TestDanmakuUnavailableIsNotSilentlyEmptied(t *testing.T) {
	svc, _ := newDanmakuTestService(t)
	if _, err := svc.Payload(t.Context(), "media-1", DanmakuOptions{}); !errors.Is(err, ErrDanmakuUnavailable) {
		t.Fatalf("Payload error = %v, want ErrDanmakuUnavailable", err)
	}
	if _, err := svc.Match(t.Context(), "media-1"); !errors.Is(err, ErrDanmakuUnavailable) {
		t.Fatalf("Match error = %v, want ErrDanmakuUnavailable", err)
	}
}

func TestDanmakuMatchPersistsAssociationAndShift(t *testing.T) {
	svc, db := newDanmakuTestService(t)
	pipeline := &fakeDanmakuPipeline{matchResult: matchedResult()}
	svc.SetPipelineClient(pipeline)

	result, err := svc.Match(t.Context(), "media-1")
	if err != nil {
		t.Fatal(err)
	}
	if !result.Matched || result.EpisodeID != "95410010" {
		t.Fatalf("unexpected match result: %+v", result)
	}
	var row model.MediaDanmaku
	if err := db.Where("media_id = ?", "media-1").First(&row).Error; err != nil {
		t.Fatal(err)
	}
	if row.Status != DanmakuStatusMatched || row.ProviderShiftSeconds != 2 || row.MatchMode != "tmdb" {
		t.Fatalf("unexpected persisted row: %+v", row)
	}
	if !strings.Contains(row.Attempts, "matched") {
		t.Fatalf("attempts were not persisted: %q", row.Attempts)
	}
}

func TestDanmakuManualAssociationSurvivesRematch(t *testing.T) {
	svc, _ := newDanmakuTestService(t)
	pipeline := &fakeDanmakuPipeline{matchResult: matchedResult()}
	svc.SetPipelineClient(pipeline)

	if _, err := svc.SetManual(t.Context(), "media-1", "dandanplay", "111", "手动番剧", "手动第1话", -1.5); err != nil {
		t.Fatal(err)
	}
	result, err := svc.Match(t.Context(), "media-1")
	if err != nil {
		t.Fatal(err)
	}
	if pipeline.matchCalls != 0 {
		t.Fatalf("manual association must not trigger an upstream match, got %d calls", pipeline.matchCalls)
	}
	if result.EpisodeID != "111" || result.MatchMode != DanmakuMatchModeManual {
		t.Fatalf("manual association was overwritten: %+v", result)
	}
}

func TestDanmakuMatchFailureIsPersistedAndReported(t *testing.T) {
	svc, db := newDanmakuTestService(t)
	svc.SetPipelineClient(&fakeDanmakuPipeline{matchErr: errors.New("upstream down")})

	if _, err := svc.Match(t.Context(), "media-1"); !errors.Is(err, ErrDanmakuUnavailable) {
		t.Fatalf("Match error = %v, want wrapped ErrDanmakuUnavailable", err)
	}
	var row model.MediaDanmaku
	if err := db.Where("media_id = ?", "media-1").First(&row).Error; err != nil {
		t.Fatal(err)
	}
	if row.Status != DanmakuStatusFailed {
		t.Fatalf("status = %q, want failed", row.Status)
	}
}

func TestDanmakuUnmatchedIsReportedWithAttempts(t *testing.T) {
	svc, _ := newDanmakuTestService(t)
	svc.SetPipelineClient(&fakeDanmakuPipeline{matchResult: DanmakuMatchResult{
		Matched: false,
		Attempts: []DanmakuAttempt{
			{Source: "dandanplay", Mode: "tmdb", Outcome: "no_candidates"},
			{Source: "dandanplay", Mode: "filename", Outcome: "no_candidates"},
		},
	}})

	_, err := svc.Payload(t.Context(), "media-1", DanmakuOptions{})
	if !errors.Is(err, ErrDanmakuUnmatched) {
		t.Fatalf("Payload error = %v, want ErrDanmakuUnmatched", err)
	}
	if !strings.Contains(err.Error(), "no_candidates") {
		t.Fatalf("error must carry the recorded attempts, got %q", err.Error())
	}
}

func TestDanmakuPayloadAppliesProviderShiftAndUserOffset(t *testing.T) {
	svc, _ := newDanmakuTestService(t)
	pipeline := &fakeDanmakuPipeline{
		matchResult: matchedResult(),
		payload: DanmakuPayload{
			Source:    "dandanplay",
			EpisodeID: "95410010",
			Count:     1,
			Comments: []DanmakuComment{
				{CID: "1542278977442529280", P: "847.66,1,16020176,14b30012", M: "前方高能", Time: 847.66, Mode: 1},
			},
		},
	}
	svc.SetPipelineClient(pipeline)

	payload, err := svc.Payload(t.Context(), "media-1", DanmakuOptions{ChConvert: 1})
	if err != nil {
		t.Fatal(err)
	}
	if payload.MediaID != "media-1" || payload.Count != 1 {
		t.Fatalf("unexpected payload: %+v", payload)
	}
	if len(pipeline.fetchRequests) != 1 {
		t.Fatalf("fetch requests = %d, want 1", len(pipeline.fetchRequests))
	}
	request := pipeline.fetchRequests[0]
	if request.EpisodeID != "95410010" || request.ProviderShiftSeconds != 2 || request.ChConvert != 1 {
		t.Fatalf("fetch request lost match context: %+v", request)
	}
	if payload.Comments[0].CID != "1542278977442529280" {
		t.Fatalf("cid must stay a string, got %q", payload.Comments[0].CID)
	}
}

func TestDanmakuOffsetOverrideAndClear(t *testing.T) {
	svc, _ := newDanmakuTestService(t)
	pipeline := &fakeDanmakuPipeline{matchResult: matchedResult(), payload: DanmakuPayload{EpisodeID: "95410010"}}
	svc.SetPipelineClient(pipeline)

	if _, err := svc.Payload(t.Context(), "media-1", DanmakuOptions{OffsetSeconds: -3.5}); err != nil {
		t.Fatal(err)
	}
	if got := pipeline.fetchRequests[len(pipeline.fetchRequests)-1].OffsetSeconds; got != -3.5 {
		t.Fatalf("offset override = %v, want -3.5", got)
	}
	row, err := svc.SetOffset(t.Context(), "media-1", 1.25)
	if err != nil {
		t.Fatal(err)
	}
	if row.OffsetSeconds != 1.25 {
		t.Fatalf("stored offset = %v, want 1.25", row.OffsetSeconds)
	}
	if _, err := svc.Payload(t.Context(), "media-1", DanmakuOptions{}); err != nil {
		t.Fatal(err)
	}
	if got := pipeline.fetchRequests[len(pipeline.fetchRequests)-1].OffsetSeconds; got != 1.25 {
		t.Fatalf("stored offset not applied: %v", got)
	}
	if err := svc.Clear(t.Context(), "media-1"); err != nil {
		t.Fatal(err)
	}
	row, err = svc.Association(t.Context(), "media-1")
	if err != nil {
		t.Fatal(err)
	}
	if row != nil {
		t.Fatalf("association still present after Clear: %+v", row)
	}
}

func TestDanmakuXMLIsEscapedAndUsesBilibiliShape(t *testing.T) {
	xml := string(DanmakuXML(DanmakuPayload{
		EpisodeID: "95410010",
		Count:     1,
		Comments: []DanmakuComment{
			{CID: "1", M: `a<b>&"c"`, Time: 12.5, Mode: 5, Color: 16777215},
		},
	}))
	if !strings.Contains(xml, "<chatid>95410010</chatid>") {
		t.Fatalf("chatid missing: %s", xml)
	}
	if !strings.Contains(xml, "a&lt;b&gt;&amp;&quot;c&quot;") {
		t.Fatalf("text was not escaped: %s", xml)
	}
	if !strings.Contains(xml, `<d p="12.50,5,25,16777215,`) {
		t.Fatalf("p attribute is not in bilibili shape: %s", xml)
	}
}

func TestDanmakuPipelineHTTPContract(t *testing.T) {
	var bodies []map[string]any
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		raw, _ := io.ReadAll(r.Body)
		var decoded map[string]any
		_ = json.Unmarshal(raw, &decoded)
		decoded["__path"] = r.URL.Path
		decoded["__auth"] = r.Header.Get("Authorization")
		bodies = append(bodies, decoded)
		switch r.URL.Path {
		case "/v1/danmaku/match":
			_, _ = w.Write([]byte(`{"media_id":"media-1","target":{"title":"x"},"match":{"matched":true,"source":"dandanplay","match_mode":"tmdb","episode_id":"7","shift":2}}`))
		default:
			_, _ = w.Write([]byte(`{"media_id":"media-1","count":0,"comments":[]}`))
		}
	}))
	defer server.Close()

	client, err := newResourcePipelineHTTPClient(config.ResourceImportConfig{
		PipelineURL:          server.URL,
		PipelineToken:        "token",
		SearchTimeoutSeconds: 5,
	})
	if err != nil {
		t.Fatal(err)
	}
	result, err := client.MatchDanmaku(t.Context(), "media-1")
	if err != nil {
		t.Fatal(err)
	}
	if !result.Matched || result.EpisodeID != "7" || result.Shift != 2 {
		t.Fatalf("unexpected match result: %+v", result)
	}
	if result.Status != DanmakuStatusMatched {
		t.Fatalf("status = %q, want matched", result.Status)
	}
	if _, err := client.FetchDanmaku(t.Context(), DanmakuFetchRequest{
		MediaID: "media-1", EpisodeID: "7", ChConvert: 2, OffsetSeconds: 1.5, ProviderShiftSeconds: 2,
	}); err != nil {
		t.Fatal(err)
	}
	if len(bodies) != 2 {
		t.Fatalf("requests = %d, want 2", len(bodies))
	}
	if bodies[0]["__path"] != "/v1/danmaku/match" || bodies[0]["__auth"] != "Bearer token" {
		t.Fatalf("unexpected first request: %+v", bodies[0])
	}
	if bodies[1]["ch_convert"] != float64(2) || bodies[1]["episode_id"] != "7" {
		t.Fatalf("fetch request did not carry ch_convert/episode_id: %+v", bodies[1])
	}
	if bodies[1]["provider_shift_seconds"] != float64(2) {
		t.Fatalf("fetch request did not carry provider shift: %+v", bodies[1])
	}
}
