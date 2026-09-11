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
	parsePayload  DanmakuPayload
	parseErr      error
	parseRequests []DanmakuParseRequest
	fetchRequests []DanmakuFetchRequest
	matchCalls    int
	prewarmTask   DanmakuPrewarmTask
	prewarmErr    error
	prewarmCalls  []DanmakuPrewarmRequest
	prewarmGet    DanmakuPrewarmTask
	prewarmGetErr error
	prewarmGets   []string
}

func (f *fakeDanmakuPipeline) StartDanmakuPrewarm(_ context.Context, request DanmakuPrewarmRequest) (DanmakuPrewarmTask, error) {
	f.prewarmCalls = append(f.prewarmCalls, request)
	if f.prewarmErr != nil {
		return DanmakuPrewarmTask{}, f.prewarmErr
	}
	task := f.prewarmTask
	if task.TaskID == "" {
		task.TaskID = "task-1"
	}
	return task, nil
}

func (f *fakeDanmakuPipeline) GetDanmakuPrewarm(_ context.Context, taskID string) (DanmakuPrewarmTask, error) {
	f.prewarmGets = append(f.prewarmGets, taskID)
	return f.prewarmGet, f.prewarmGetErr
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

func (f *fakeDanmakuPipeline) ParseDanmaku(_ context.Context, request DanmakuParseRequest) (DanmakuPayload, error) {
	f.parseRequests = append(f.parseRequests, request)
	if f.parseErr != nil {
		return DanmakuPayload{}, f.parseErr
	}
	payload := f.parsePayload
	if payload.Source == "" {
		payload.Source = DanmakuProviderLocal
	}
	if payload.Format == "" {
		payload.Format = "bilibili-xml"
	}
	return payload, nil
}

func newDanmakuTestService(t *testing.T) (*DanmakuService, *gorm.DB) {
	t.Helper()
	dsn := "file:" + strings.ReplaceAll(t.Name(), "/", "_") + "?mode=memory&cache=shared"
	db, err := gorm.Open(sqlite.Open(dsn), &gorm.Config{})
	if err != nil {
		t.Fatal(err)
	}
	if err := db.AutoMigrate(&model.MediaDanmaku{}, &model.Media{}); err != nil {
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

func TestDanmakuPrewarmResolvesSeasonEpisodes(t *testing.T) {
	svc, db := newDanmakuTestService(t)
	pipeline := &fakeDanmakuPipeline{}
	svc.SetPipelineClient(pipeline)

	seed := []model.Media{
		{Title: "作品", Path: "/media/1.mkv", LibraryID: "lib-1", SeriesID: "series-1", SeasonNum: 1, EpisodeNum: 1},
		{Title: "作品", Path: "/media/2.mkv", LibraryID: "lib-1", SeriesID: "series-1", SeasonNum: 1, EpisodeNum: 2},
		{Title: "作品", Path: "/media/3.mkv", LibraryID: "lib-1", SeriesID: "series-1", SeasonNum: 1, EpisodeNum: 3},
		{Title: "作品", Path: "/media/4.mkv", LibraryID: "lib-1", SeriesID: "series-1", SeasonNum: 2, EpisodeNum: 1},
		{Title: "作品", Path: "/media/special.mkv", LibraryID: "lib-1", SeriesID: "series-1", SeasonNum: 1, EpisodeNum: 0},
	}
	for index := range seed {
		if err := db.Create(&seed[index]).Error; err != nil {
			t.Fatal(err)
		}
	}

	task, err := svc.PrewarmSeason(t.Context(), seed[0].ID, 1)
	if err != nil {
		t.Fatal(err)
	}
	if task.TaskID != "task-1" {
		t.Fatalf("unexpected task: %+v", task)
	}
	if len(pipeline.prewarmCalls) != 1 {
		t.Fatalf("prewarm calls = %d, want 1", len(pipeline.prewarmCalls))
	}
	request := pipeline.prewarmCalls[0]
	if request.Season != 1 || request.MediaID != seed[0].ID {
		t.Fatalf("unexpected request: %+v", request)
	}
	// 只包含 S01 的 3 集：第 4 条是 S02，第 5 条 episode_num=0（特典）不算。
	if len(request.Episodes) != 3 {
		t.Fatalf("episodes = %+v, want the three S01 episodes", request.Episodes)
	}
	keys := []string{request.Episodes[0].EpisodeKey, request.Episodes[1].EpisodeKey, request.Episodes[2].EpisodeKey}
	if keys[0] != "S01E01" || keys[2] != "S01E03" {
		t.Fatalf("unexpected episode keys: %v", keys)
	}
	if request.Episodes[0].MediaID != seed[0].ID || request.Episodes[2].MediaID != seed[2].ID {
		t.Fatalf("episodes are not in episode order: %+v", request.Episodes)
	}
}

func TestDanmakuPrewarmReportsInvalidInputInsteadOfGuessing(t *testing.T) {
	svc, db := newDanmakuTestService(t)
	svc.SetPipelineClient(&fakeDanmakuPipeline{})

	media := model.Media{Title: "电影", Path: "/media/movie.mkv", LibraryID: "lib-1"}
	if err := db.Create(&media).Error; err != nil {
		t.Fatal(err)
	}
	// 电影没有季号：预热必须报错，而不是悄悄拿第 0 季去回源。
	if _, err := svc.PrewarmSeason(t.Context(), media.ID, 0); !errors.Is(err, ErrDanmakuInvalidInput) {
		t.Fatalf("error = %v, want ErrDanmakuInvalidInput", err)
	}
	if _, err := svc.PrewarmSeason(t.Context(), "missing-media", 1); !errors.Is(err, ErrDanmakuInvalidInput) {
		t.Fatalf("error = %v, want ErrDanmakuInvalidInput for unknown media", err)
	}
}

func TestDanmakuPrewarmUnavailableWithoutPipeline(t *testing.T) {
	svc, _ := newDanmakuTestService(t)
	if _, err := svc.PrewarmSeason(t.Context(), "media-1", 1); !errors.Is(err, ErrDanmakuUnavailable) {
		t.Fatalf("error = %v, want ErrDanmakuUnavailable", err)
	}
	if _, err := svc.PrewarmTask(t.Context(), "task-1"); !errors.Is(err, ErrDanmakuUnavailable) {
		t.Fatalf("error = %v, want ErrDanmakuUnavailable", err)
	}
}

func TestDanmakuPrewarmTaskForwardsPipeline404AsNotFound(t *testing.T) {
	svc, _ := newDanmakuTestService(t)
	svc.SetPipelineClient(&fakeDanmakuPipeline{
		prewarmGetErr: &resourcePipelineError{StatusCode: 404, Code: "danmaku_prewarm_not_found", Message: "not found"},
	})
	if _, err := svc.PrewarmTask(t.Context(), "task-404"); !errors.Is(err, ErrDanmakuPrewarmNotFound) {
		t.Fatalf("error = %v, want ErrDanmakuPrewarmNotFound", err)
	}

	svc.SetPipelineClient(&fakeDanmakuPipeline{
		prewarmGet: DanmakuPrewarmTask{TaskID: "task-2", Status: "running", Processed: 2, Total: 5},
	})
	task, err := svc.PrewarmTask(t.Context(), "task-2")
	if err != nil {
		t.Fatal(err)
	}
	if task.Status != "running" || task.Total != 5 {
		t.Fatalf("unexpected task: %+v", task)
	}
}

func TestDanmakuOffsetIsValidatedBeforeItReachesThePipeline(t *testing.T) {
	svc, _ := newDanmakuTestService(t)
	pipeline := &fakeDanmakuPipeline{matchResult: matchedResult(), payload: DanmakuPayload{EpisodeID: "95410010"}}
	svc.SetPipelineClient(pipeline)

	// 手动指定、单独调偏移、单次覆盖三条路径都要在本地拦下越界值，
	// 否则管线会用 400 拒绝，而调用方只会看到一个笼统的 503。
	if _, err := svc.SetManual(t.Context(), "media-1", "dandanplay", "95410010", "", "", 9999); !errors.Is(err, ErrDanmakuInvalidInput) {
		t.Fatalf("SetManual error = %v, want ErrDanmakuInvalidInput", err)
	}
	if _, err := svc.SetOffset(t.Context(), "media-1", -9999); !errors.Is(err, ErrDanmakuInvalidInput) {
		t.Fatalf("SetOffset error = %v, want ErrDanmakuInvalidInput", err)
	}
	if _, err := svc.Payload(t.Context(), "media-1", DanmakuOptions{OffsetSeconds: 601}); !errors.Is(err, ErrDanmakuInvalidInput) {
		t.Fatalf("Payload error = %v, want ErrDanmakuInvalidInput", err)
	}
	if len(pipeline.fetchRequests) != 0 {
		t.Fatalf("越界偏移不应触发任何回源，实际 %d 次", len(pipeline.fetchRequests))
	}

	// 边界值本身是合法的。
	if _, err := svc.SetManual(t.Context(), "media-1", "dandanplay", "95410010", "", "", -600); err != nil {
		t.Fatalf("SetManual(-600) error = %v, want nil", err)
	}
	if _, err := svc.Payload(t.Context(), "media-1", DanmakuOptions{OffsetSeconds: 600}); err != nil {
		t.Fatalf("Payload(600) error = %v, want nil", err)
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

const localXML = `<i><d p="1.5,1,25,16777215,1700000000,0,abc,101">本地弹幕</d></i>`

// media-pipeline /v1/danmaku/parse 的真实响应（字段名对不上时 JSON 解码会静默丢字段，
// 所以这里直接用它，而不是手写一份"看起来差不多"的 JSON）。
const pipelineParseResponse = `{"source": "local", "episode_id": "", "anime_title": "t", "episode_title": "", "match_mode": "import", "confidence": null, "provider_shift_seconds": 0.0, "offset_seconds": 1.5, "ch_convert": 0, "count": 1, "total": 1, "filtered": 0, "dropped_modes": 0, "skipped": 0, "truncated": false, "cached": false, "comments": [{"cid": "101", "p": "1.5,1,25,16777215,1700000000,0,abc,101", "m": "hello", "time": 3.0, "mode": 1, "mode_name": "scroll", "color": 16777215, "size": 25, "user": "abc"}], "format": "bilibili-xml", "size_bytes": 62}`

func TestDanmakuParsePipelineHTTPContract(t *testing.T) {
	var body map[string]any
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/v1/danmaku/parse" {
			t.Errorf("unexpected path %s", r.URL.Path)
		}
		raw, _ := io.ReadAll(r.Body)
		_ = json.Unmarshal(raw, &body)
		_, _ = w.Write([]byte(pipelineParseResponse))
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
	payload, err := client.ParseDanmaku(t.Context(), DanmakuParseRequest{
		Content:       localXML,
		Format:        "xml",
		OffsetSeconds: 1.5,
		Title:         "t",
	})
	if err != nil {
		t.Fatal(err)
	}
	if payload.Source != DanmakuProviderLocal || payload.Format != "bilibili-xml" || payload.MatchMode != DanmakuMatchModeImport {
		t.Fatalf("parse response lost fields: %+v", payload)
	}
	if payload.Count != 1 || payload.Total != 1 || payload.DroppedModes != 0 || payload.Truncated {
		t.Fatalf("parse response counts are wrong: %+v", payload)
	}
	if payload.OffsetSeconds != 1.5 {
		t.Fatalf("offset was not decoded: %+v", payload)
	}
	if len(payload.Comments) != 1 || payload.Comments[0].CID != "101" || payload.Comments[0].Time != 3.0 {
		t.Fatalf("comments were not decoded: %+v", payload.Comments)
	}
	if body["content"] != localXML || body["format"] != "xml" || body["title"] != "t" {
		t.Fatalf("parse request lost parameters: %+v", body)
	}
	if body["offset_seconds"] != 1.5 {
		t.Fatalf("parse request lost the offset: %+v", body)
	}
}

func TestDanmakuImportLocalStoresFileAndServesItThroughThePipeline(t *testing.T) {
	svc, db := newDanmakuTestService(t)
	pipeline := &fakeDanmakuPipeline{parsePayload: DanmakuPayload{
		Source:   DanmakuProviderLocal,
		Format:   "bilibili-xml",
		Count:    1,
		Total:    1,
		Comments: []DanmakuComment{{CID: "101", P: "1.5,1,25,16777215,1700000000,0,abc,101", M: "本地弹幕", Time: 1.5, Mode: 1}},
	}}
	svc.SetPipelineClient(pipeline)

	row, payload, err := svc.ImportLocal(t.Context(), "media-1", localXML, "", "某番 第1话")
	if err != nil {
		t.Fatal(err)
	}
	if row == nil || row.Provider != DanmakuProviderLocal || row.MatchMode != DanmakuMatchModeImport {
		t.Fatalf("unexpected association: %+v", row)
	}
	if row.EpisodeID != "" || row.LocalContent != localXML || row.LocalFormat != "bilibili-xml" {
		t.Fatalf("imported file was not stored as-is: %+v", row)
	}
	if payload.Count != 1 || payload.Format != "bilibili-xml" || payload.MediaID != "media-1" {
		t.Fatalf("unexpected import payload: %+v", payload)
	}
	if len(pipeline.parseRequests) != 1 || pipeline.parseRequests[0].Content != localXML {
		t.Fatalf("import must validate through the pipeline first: %+v", pipeline.parseRequests)
	}

	// 读取时重新解析（偏移/简繁都是请求级参数），并且不再走自动匹配。
	got, err := svc.Payload(t.Context(), "media-1", DanmakuOptions{})
	if err != nil {
		t.Fatal(err)
	}
	if got.Source != DanmakuProviderLocal || got.MatchMode != DanmakuMatchModeImport || got.Count != 1 {
		t.Fatalf("unexpected payload: %+v", got)
	}
	if pipeline.matchCalls != 0 {
		t.Fatalf("a local import must not trigger upstream matching, got %d calls", pipeline.matchCalls)
	}
	if len(pipeline.fetchRequests) != 0 {
		t.Fatalf("a local import must not be fetched from a provider: %+v", pipeline.fetchRequests)
	}
	if len(pipeline.parseRequests) != 2 {
		t.Fatalf("parse requests = %d, want 2 (import + serve)", len(pipeline.parseRequests))
	}

	// 偏移保存在关联上，读取时按当前偏移重新解析。
	if _, err := svc.SetOffset(t.Context(), "media-1", 2.5); err != nil {
		t.Fatal(err)
	}
	if _, err := svc.Payload(t.Context(), "media-1", DanmakuOptions{}); err != nil {
		t.Fatal(err)
	}
	if got := pipeline.parseRequests[len(pipeline.parseRequests)-1].OffsetSeconds; got != 2.5 {
		t.Fatalf("stored offset not applied to the imported file: %v", got)
	}

	var stored model.MediaDanmaku
	if err := db.Where("media_id = ?", "media-1").First(&stored).Error; err != nil {
		t.Fatal(err)
	}
	if stored.Status != DanmakuStatusMatched || stored.ProviderShiftSeconds != 0 {
		t.Fatalf("unexpected stored row: %+v", stored)
	}
	if state, err := svc.State(t.Context(), "media-1"); err != nil {
		t.Fatal(err)
	} else if !state.Matched || state.LocalFormat != "bilibili-xml" {
		t.Fatalf("state must report the local import as matched: %+v", state)
	}
}

func TestDanmakuImportLocalRejectsBadFilesWithoutStoringThem(t *testing.T) {
	cases := []struct {
		name   string
		client *fakeDanmakuPipeline
		body   string
		want   error
	}{
		{
			name:   "pipeline rejects the format",
			client: &fakeDanmakuPipeline{parseErr: &resourcePipelineError{StatusCode: 400, Code: "invalid_danmaku_file", Message: "danmaku json is not valid JSON"}},
			body:   localXML,
			want:   ErrDanmakuInvalidInput,
		},
		{
			name:   "pipeline rejects the size",
			client: &fakeDanmakuPipeline{parseErr: &resourcePipelineError{StatusCode: 413, Code: "request_too_large", Message: "JSON request body is too large"}},
			body:   localXML,
			want:   ErrDanmakuFileTooLarge,
		},
		{
			name:   "pipeline is unreachable",
			client: &fakeDanmakuPipeline{parseErr: errors.New("connection refused")},
			body:   localXML,
			want:   ErrDanmakuUnavailable,
		},
		{
			name:   "content is empty",
			client: &fakeDanmakuPipeline{},
			body:   "   ",
			want:   ErrDanmakuInvalidInput,
		},
	}
	for _, testCase := range cases {
		t.Run(testCase.name, func(t *testing.T) {
			svc, db := newDanmakuTestService(t)
			svc.SetPipelineClient(testCase.client)
			_, _, err := svc.ImportLocal(t.Context(), "media-1", testCase.body, "", "")
			if !errors.Is(err, testCase.want) {
				t.Fatalf("ImportLocal error = %v, want %v", err, testCase.want)
			}
			var count int64
			if err := db.Model(&model.MediaDanmaku{}).Where("media_id = ?", "media-1").Count(&count).Error; err != nil {
				t.Fatal(err)
			}
			if count != 0 {
				t.Fatalf("a rejected import must not be persisted (rows=%d)", count)
			}
		})
	}
}

func TestDanmakuImportLocalEnforcesSizeLimitItself(t *testing.T) {
	svc, _ := newDanmakuTestService(t)
	pipeline := &fakeDanmakuPipeline{}
	svc.SetPipelineClient(pipeline)

	oversized := strings.Repeat("a", DanmakuMaxImportBytes+1)
	if _, _, err := svc.ImportLocal(t.Context(), "media-1", oversized, "", ""); !errors.Is(err, ErrDanmakuFileTooLarge) {
		t.Fatalf("ImportLocal error = %v, want ErrDanmakuFileTooLarge", err)
	}
	if len(pipeline.parseRequests) != 0 {
		t.Fatalf("an oversized file must be rejected before it reaches the pipeline")
	}
}

func TestDanmakuLocalImportSurvivesMatchAndIsReplacedByManualAssociation(t *testing.T) {
	svc, _ := newDanmakuTestService(t)
	pipeline := &fakeDanmakuPipeline{
		matchResult:  matchedResult(),
		parsePayload: DanmakuPayload{Source: DanmakuProviderLocal, Format: "dandanplay-json", Count: 2},
	}
	svc.SetPipelineClient(pipeline)
	if _, _, err := svc.ImportLocal(t.Context(), "media-1", `{"comments":[]}`, "", ""); err != nil {
		t.Fatal(err)
	}

	// 自动匹配不得覆盖用户导入的文件。
	result, err := svc.Match(t.Context(), "media-1")
	if err != nil {
		t.Fatal(err)
	}
	if pipeline.matchCalls != 0 {
		t.Fatalf("a local import must not be auto-matched over, got %d calls", pipeline.matchCalls)
	}
	if result.Provider != DanmakuProviderLocal || !result.Matched {
		t.Fatalf("local import was overwritten: %+v", result)
	}

	// 手动指向第三方弹幕库时，导入的原文必须一起清掉，避免留下两份来源。
	row, err := svc.SetManual(t.Context(), "media-1", "dandanplay", "111", "", "", 0)
	if err != nil {
		t.Fatal(err)
	}
	if row.LocalContent != "" || row.LocalFormat != "" {
		t.Fatalf("manual association kept the imported file: %+v", row)
	}
	if row.MatchMode != DanmakuMatchModeManual || row.EpisodeID != "111" {
		t.Fatalf("unexpected manual association: %+v", row)
	}
}
