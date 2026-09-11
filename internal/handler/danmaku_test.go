package handler

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync/atomic"
	"testing"

	"github.com/gin-gonic/gin"
	"github.com/glebarez/sqlite"
	"go.uber.org/zap"
	"gorm.io/gorm"
	"gorm.io/gorm/clause"

	"github.com/ShukeBta/MediaStationGo/internal/model"
	"github.com/ShukeBta/MediaStationGo/internal/repository"
	"github.com/ShukeBta/MediaStationGo/internal/service"
)

var danmakuContainerSeq atomic.Int64

type stubDanmakuPipeline struct {
	matchResult   service.DanmakuMatchResult
	matchErr      error
	payload       service.DanmakuPayload
	payloadErr    error
	searchResult  service.DanmakuSearchResult
	parseResult   service.DanmakuPayload
	parseErr      error
	parseRequests []service.DanmakuParseRequest
	prewarmTask   service.DanmakuPrewarmTask
	prewarmErr    error
	prewarmGetErr error
}

func (s *stubDanmakuPipeline) MatchDanmaku(context.Context, string) (service.DanmakuMatchResult, error) {
	return s.matchResult, s.matchErr
}

func (s *stubDanmakuPipeline) FetchDanmaku(context.Context, service.DanmakuFetchRequest) (service.DanmakuPayload, error) {
	return s.payload, s.payloadErr
}

func (s *stubDanmakuPipeline) SearchDanmaku(context.Context, string, int) (service.DanmakuSearchResult, error) {
	return s.searchResult, nil
}

func (s *stubDanmakuPipeline) ParseDanmaku(_ context.Context, request service.DanmakuParseRequest) (service.DanmakuPayload, error) {
	s.parseRequests = append(s.parseRequests, request)
	if s.parseErr != nil {
		return service.DanmakuPayload{}, s.parseErr
	}
	payload := s.parseResult
	if payload.Source == "" {
		payload.Source = service.DanmakuProviderLocal
	}
	if payload.Format == "" {
		payload.Format = "bilibili-xml"
	}
	return payload, nil
}

func (s *stubDanmakuPipeline) StartDanmakuPrewarm(context.Context, service.DanmakuPrewarmRequest) (service.DanmakuPrewarmTask, error) {
	if s.prewarmErr != nil {
		return service.DanmakuPrewarmTask{}, s.prewarmErr
	}
	task := s.prewarmTask
	if task.TaskID == "" {
		task.TaskID = "task-1"
		task.Status = "queued"
	}
	return task, nil
}

func (s *stubDanmakuPipeline) GetDanmakuPrewarm(context.Context, string) (service.DanmakuPrewarmTask, error) {
	return s.prewarmTask, s.prewarmGetErr
}

func newDanmakuHandlerContainer(t *testing.T, pipeline *stubDanmakuPipeline) *service.Container {
	t.Helper()
	dsn := "file:" + strings.ReplaceAll(t.Name(), "/", "_") + "?mode=memory&cache=shared"
	db, err := gorm.Open(sqlite.Open(dsn), &gorm.Config{})
	if err != nil {
		t.Fatal(err)
	}
	if err := db.AutoMigrate(&model.MediaDanmaku{}, &model.Media{}); err != nil {
		t.Fatal(err)
	}
	// 弹幕接口按媒体行工作，整季预热还要按剧集/季解析分集，所以这里种一条媒体行。
	media := model.Media{
		Title:      "测试剧集",
		Path:       fmt.Sprintf("/media/%s-%d.mkv", strings.ReplaceAll(t.Name(), "/", "_"), danmakuContainerSeq.Add(1)),
		LibraryID:  "lib-1",
		SeriesID:   "series-1",
		SeasonNum:  1,
		EpisodeNum: 1,
	}
	media.ID = "media-1"
	// 同一个测试可能建多个 router/container，共享内存库里已种过这条媒体行，重复插入忽略即可。
	if err := db.Clauses(clause.OnConflict{DoNothing: true}).Create(&media).Error; err != nil {
		t.Fatal(err)
	}
	danmaku := service.NewDanmakuService(zap.NewNop(), repository.New(db))
	if pipeline != nil {
		danmaku.SetPipelineClient(pipeline)
	}
	return &service.Container{Danmaku: danmaku}
}

func newDanmakuRouter(svc *service.Container) *gin.Engine {
	gin.SetMode(gin.TestMode)
	router := gin.New()
	router.GET("/media/:id/danmaku", mediaDanmakuHandler(svc))
	router.GET("/media/:id/danmaku/match", mediaDanmakuStateHandler(svc))
	router.POST("/media/:id/danmaku/match", matchMediaDanmakuHandler(svc))
	router.PATCH("/media/:id/danmaku", updateMediaDanmakuHandler(svc))
	router.DELETE("/media/:id/danmaku", deleteMediaDanmakuHandler(svc))
	router.POST("/media/:id/danmaku/import", importMediaDanmakuHandler(svc))
	router.GET("/danmaku/search", searchDanmakuHandler(svc))
	router.POST("/media/:id/danmaku/prewarm", prewarmMediaDanmakuHandler(svc))
	router.GET("/media/:id/danmaku/prewarm/:task_id", mediaDanmakuPrewarmTaskHandler(svc))
	router.GET("/emby/api/danmu/:id/raw", embyDanmuRawHandler(svc))
	return router
}

func doDanmakuRequest(router *gin.Engine, method, path, body string) *httptest.ResponseRecorder {
	var reader *strings.Reader
	if body == "" {
		reader = strings.NewReader("")
	} else {
		reader = strings.NewReader(body)
	}
	req := httptest.NewRequest(method, path, reader)
	if body != "" {
		req.Header.Set("Content-Type", "application/json")
	}
	recorder := httptest.NewRecorder()
	router.ServeHTTP(recorder, req)
	return recorder
}

func TestDanmakuHandlerUnavailableIsServiceUnavailable(t *testing.T) {
	svc := newDanmakuHandlerContainer(t, nil)
	router := newDanmakuRouter(svc)

	recorder := doDanmakuRequest(router, http.MethodGet, "/media/media-1/danmaku", "")
	if recorder.Code != http.StatusServiceUnavailable {
		t.Fatalf("status = %d, want 503; body=%s", recorder.Code, recorder.Body.String())
	}
	var body map[string]any
	if err := json.Unmarshal(recorder.Body.Bytes(), &body); err != nil {
		t.Fatal(err)
	}
	if body["code"] != "danmaku_unavailable" {
		t.Fatalf("code = %v, want danmaku_unavailable", body["code"])
	}
}

func TestDanmakuHandlerUnmatchedIsNotFoundWithAttempts(t *testing.T) {
	pipeline := &stubDanmakuPipeline{matchResult: service.DanmakuMatchResult{
		Matched:  false,
		Attempts: []service.DanmakuAttempt{{Source: "dandanplay", Mode: "tmdb", Outcome: "no_candidates"}},
	}}
	router := newDanmakuRouter(newDanmakuHandlerContainer(t, pipeline))

	recorder := doDanmakuRequest(router, http.MethodGet, "/media/media-1/danmaku", "")
	if recorder.Code != http.StatusNotFound {
		t.Fatalf("status = %d, want 404; body=%s", recorder.Code, recorder.Body.String())
	}
	if !strings.Contains(recorder.Body.String(), "no_candidates") {
		t.Fatalf("attempts missing from body: %s", recorder.Body.String())
	}
	if !strings.Contains(recorder.Body.String(), "danmaku_unmatched") {
		t.Fatalf("code missing from body: %s", recorder.Body.String())
	}
}

func TestDanmakuHandlerServesNormalizedPayloadWithStringCID(t *testing.T) {
	pipeline := &stubDanmakuPipeline{
		matchResult: service.DanmakuMatchResult{
			Matched:   true,
			Provider:  "dandanplay",
			MatchMode: "tmdb",
			EpisodeID: "95410010",
			Shift:     2,
		},
		payload: service.DanmakuPayload{
			Source:    "dandanplay",
			EpisodeID: "95410010",
			Count:     1,
			Total:     1,
			Comments: []service.DanmakuComment{
				{CID: "1542278977442529280", P: "847.66,1,16020176,14b30012", M: "前方高能", Time: 847.66, Mode: 1, Color: 16020176},
			},
		},
	}
	router := newDanmakuRouter(newDanmakuHandlerContainer(t, pipeline))

	recorder := doDanmakuRequest(router, http.MethodGet, "/media/media-1/danmaku?ch_convert=1", "")
	if recorder.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200; body=%s", recorder.Code, recorder.Body.String())
	}
	var payload service.DanmakuPayload
	if err := json.Unmarshal(recorder.Body.Bytes(), &payload); err != nil {
		t.Fatal(err)
	}
	if payload.MediaID != "media-1" || len(payload.Comments) != 1 {
		t.Fatalf("unexpected payload: %+v", payload)
	}
	if payload.Comments[0].CID != "1542278977442529280" {
		t.Fatalf("cid = %q, want the string form", payload.Comments[0].CID)
	}
}

func TestDanmakuHandlerRejectsBadQueryParameters(t *testing.T) {
	router := newDanmakuRouter(newDanmakuHandlerContainer(t, &stubDanmakuPipeline{}))

	for _, path := range []string{
		"/media/media-1/danmaku?ch_convert=9",
		"/media/media-1/danmaku?offset_seconds=abc",
		"/media/media-1/danmaku?offset_seconds=999",
	} {
		recorder := doDanmakuRequest(router, http.MethodGet, path, "")
		if recorder.Code != http.StatusBadRequest {
			t.Fatalf("%s status = %d, want 400", path, recorder.Code)
		}
	}
}

func TestDanmakuHandlerManualUpdateValidatesEpisodeID(t *testing.T) {
	router := newDanmakuRouter(newDanmakuHandlerContainer(t, &stubDanmakuPipeline{}))

	recorder := doDanmakuRequest(router, http.MethodPatch, "/media/media-1/danmaku", `{"episode_id":"abc"}`)
	if recorder.Code != http.StatusBadRequest {
		t.Fatalf("status = %d, want 400; body=%s", recorder.Code, recorder.Body.String())
	}

	recorder = doDanmakuRequest(router, http.MethodPatch, "/media/media-1/danmaku", `{"episode_id":"95410010","anime_title":"某番","offset_seconds":-1.5}`)
	if recorder.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200; body=%s", recorder.Code, recorder.Body.String())
	}
	state := doDanmakuRequest(router, http.MethodGet, "/media/media-1/danmaku/match", "")
	if !strings.Contains(state.Body.String(), `"match_mode":"manual"`) {
		t.Fatalf("manual association not reflected in state: %s", state.Body.String())
	}

	recorder = doDanmakuRequest(router, http.MethodDelete, "/media/media-1/danmaku", "")
	if recorder.Code != http.StatusOK {
		t.Fatalf("delete status = %d, want 200", recorder.Code)
	}
	state = doDanmakuRequest(router, http.MethodGet, "/media/media-1/danmaku/match", "")
	if !strings.Contains(state.Body.String(), `"status":"unmatched"`) {
		t.Fatalf("state after delete = %s, want unmatched", state.Body.String())
	}
}

func TestDanmakuSearchRequiresKeyword(t *testing.T) {
	router := newDanmakuRouter(newDanmakuHandlerContainer(t, &stubDanmakuPipeline{}))

	recorder := doDanmakuRequest(router, http.MethodGet, "/danmaku/search", "")
	if recorder.Code != http.StatusBadRequest {
		t.Fatalf("status = %d, want 400", recorder.Code)
	}
	recorder = doDanmakuRequest(router, http.MethodGet, "/danmaku/search?keyword=x&episode=0", "")
	if recorder.Code != http.StatusBadRequest {
		t.Fatalf("status = %d, want 400 for episode=0", recorder.Code)
	}
}

func TestDanmakuPrewarmHandlerValidatesSeasonAndMapsErrors(t *testing.T) {
	router := newDanmakuRouter(newDanmakuHandlerContainer(t, &stubDanmakuPipeline{}))

	// 季号越界属于调用方错误，必须在回源之前拦下。
	recorder := doDanmakuRequest(router, http.MethodPost, "/media/media-1/danmaku/prewarm", `{"season":100}`)
	if recorder.Code != http.StatusBadRequest {
		t.Fatalf("status = %d, want 400; body=%s", recorder.Code, recorder.Body.String())
	}

	// 管线里没有这个任务 -> 404（不是 503，也不是 500）。
	pipeline := &stubDanmakuPipeline{prewarmGetErr: service.ErrDanmakuPrewarmNotFound}
	router = newDanmakuRouter(newDanmakuHandlerContainer(t, pipeline))
	recorder = doDanmakuRequest(router, http.MethodGet, "/media/media-1/danmaku/prewarm/task-x", "")
	if recorder.Code != http.StatusNotFound {
		t.Fatalf("status = %d, want 404; body=%s", recorder.Code, recorder.Body.String())
	}
	if !strings.Contains(recorder.Body.String(), "danmaku_prewarm_not_found") {
		t.Fatalf("code missing: %s", recorder.Body.String())
	}
}

func TestDanmakuPrewarmHandlerAcceptsAndServesTask(t *testing.T) {
	pipeline := &stubDanmakuPipeline{}
	router := newDanmakuRouter(newDanmakuHandlerContainer(t, pipeline))

	recorder := doDanmakuRequest(router, http.MethodPost, "/media/media-1/danmaku/prewarm", `{"season":1}`)
	if recorder.Code != http.StatusAccepted {
		t.Fatalf("status = %d, want 202; body=%s", recorder.Code, recorder.Body.String())
	}

	pipeline.prewarmTask = service.DanmakuPrewarmTask{TaskID: "task-9", Status: "running", Total: 4, Processed: 1}
	recorder = doDanmakuRequest(router, http.MethodGet, "/media/media-1/danmaku/prewarm/task-9", "")
	if recorder.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200; body=%s", recorder.Code, recorder.Body.String())
	}
	if !strings.Contains(recorder.Body.String(), `"task_id":"task-9"`) {
		t.Fatalf("task missing: %s", recorder.Body.String())
	}
}

func TestDanmakuImportHandlerStoresFileAndReportsSummary(t *testing.T) {
	pipeline := &stubDanmakuPipeline{parseResult: service.DanmakuPayload{
		Source:       service.DanmakuProviderLocal,
		Format:       "bilibili-xml",
		Count:        3,
		Total:        5,
		Filtered:     1,
		DroppedModes: 2,
		Skipped:      1,
		Truncated:    true,
		Comments: []service.DanmakuComment{
			{CID: "101", P: "1.5,1,25,16777215,1700000000,0,abc,101", M: "本地弹幕", Time: 1.5, Mode: 1},
		},
	}}
	router := newDanmakuRouter(newDanmakuHandlerContainer(t, pipeline))

	recorder := doDanmakuRequest(router, http.MethodPost, "/media/media-1/danmaku/import",
		`{"content":"<i><d p=\"1.5,1,25,16777215,1700000000,0,abc,101\">本地弹幕</d></i>","format":"xml","title":"某番 第1话"}`)
	if recorder.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200; body=%s", recorder.Code, recorder.Body.String())
	}
	var body struct {
		State    map[string]any `json:"state"`
		Imported struct {
			Source       string `json:"source"`
			Format       string `json:"format"`
			Count        int    `json:"count"`
			Total        int    `json:"total"`
			Filtered     int    `json:"filtered"`
			DroppedModes int    `json:"dropped_modes"`
			Skipped      int    `json:"skipped"`
			Truncated    bool   `json:"truncated"`
		} `json:"imported"`
	}
	if err := json.Unmarshal(recorder.Body.Bytes(), &body); err != nil {
		t.Fatal(err)
	}
	if body.State["provider"] != service.DanmakuProviderLocal || body.State["match_mode"] != service.DanmakuMatchModeImport {
		t.Fatalf("unexpected state: %+v", body.State)
	}
	if _, leaked := body.State["local_content"]; leaked {
		t.Fatalf("the imported file must not be echoed back: %+v", body.State)
	}
	if body.Imported.Format != "bilibili-xml" || body.Imported.Count != 3 || body.Imported.DroppedModes != 2 || !body.Imported.Truncated {
		t.Fatalf("unexpected import summary: %+v", body.Imported)
	}
	if len(pipeline.parseRequests) != 1 {
		t.Fatalf("parse requests = %d, want 1", len(pipeline.parseRequests))
	}
	if pipeline.parseRequests[0].Format != "xml" || pipeline.parseRequests[0].Title != "某番 第1话" {
		t.Fatalf("parse request lost the request parameters: %+v", pipeline.parseRequests[0])
	}

	// 导入之后直接读弹幕：必须走本地文件，而不是去第三方匹配。
	fetched := doDanmakuRequest(router, http.MethodGet, "/media/media-1/danmaku", "")
	if fetched.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200; body=%s", fetched.Code, fetched.Body.String())
	}
	if !strings.Contains(fetched.Body.String(), `"source":"local"`) {
		t.Fatalf("payload source is not local: %s", fetched.Body.String())
	}
}

func TestDanmakuImportHandlerMapsClientAndServiceErrors(t *testing.T) {
	pipeline := &stubDanmakuPipeline{parseErr: errors.New("media-pipeline request failed with status 502")}
	router := newDanmakuRouter(newDanmakuHandlerContainer(t, pipeline))

	// 上游解析失败：不能假装成功，也不能说成 400。
	recorder := doDanmakuRequest(router, http.MethodPost, "/media/media-1/danmaku/import", `{"content":"<i/>"}`)
	if recorder.Code != http.StatusServiceUnavailable {
		t.Fatalf("status = %d, want 503; body=%s", recorder.Code, recorder.Body.String())
	}

	// 空正文属于调用方错误。
	pipeline.parseErr = nil
	recorder = doDanmakuRequest(router, http.MethodPost, "/media/media-1/danmaku/import", `{"content":"  "}`)
	if recorder.Code != http.StatusBadRequest {
		t.Fatalf("status = %d, want 400; body=%s", recorder.Code, recorder.Body.String())
	}

	// 超过体积上限：413 + code，而不是 400 或 500。
	oversized := fmt.Sprintf(`{"content":%q}`, strings.Repeat("a", service.DanmakuMaxImportBytes+1))
	recorder = doDanmakuRequest(router, http.MethodPost, "/media/media-1/danmaku/import", oversized)
	if recorder.Code != http.StatusRequestEntityTooLarge {
		t.Fatalf("status = %d, want 413; body=%s", recorder.Code, recorder.Body.String())
	}
	if !strings.Contains(recorder.Body.String(), "danmaku_file_too_large") {
		t.Fatalf("code missing: %s", recorder.Body.String())
	}

	// 服务不可用仍是 503。
	recorder = doDanmakuRequest(newDanmakuRouter(newDanmakuHandlerContainer(t, nil)), http.MethodPost, "/media/media-1/danmaku/import", `{"content":"x"}`)
	if recorder.Code != http.StatusServiceUnavailable {
		t.Fatalf("status = %d, want 503", recorder.Code)
	}
}

// Emby 兼容端点必须保持 200（它的存在就是为了不让客户端因 404 中断播放），
// 但要用 X-Danmaku-Status 说明真实状态，不能假装有数据。
func TestEmbyDanmuRawAlwaysAnswersWithStatusHeader(t *testing.T) {
	router := newDanmakuRouter(newDanmakuHandlerContainer(t, nil))
	recorder := doDanmakuRequest(router, http.MethodGet, "/emby/api/danmu/media-1/raw", "")
	if recorder.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200", recorder.Code)
	}
	if got := recorder.Header().Get("X-Danmaku-Status"); got != "unavailable" {
		t.Fatalf("X-Danmaku-Status = %q, want unavailable", got)
	}
	if !strings.Contains(recorder.Body.String(), "<i>") {
		t.Fatalf("body is not an empty danmaku document: %s", recorder.Body.String())
	}
}
