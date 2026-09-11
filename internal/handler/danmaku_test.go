package handler

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/gin-gonic/gin"
	"github.com/glebarez/sqlite"
	"go.uber.org/zap"
	"gorm.io/gorm"

	"github.com/ShukeBta/MediaStationGo/internal/model"
	"github.com/ShukeBta/MediaStationGo/internal/repository"
	"github.com/ShukeBta/MediaStationGo/internal/service"
)

type stubDanmakuPipeline struct {
	matchResult  service.DanmakuMatchResult
	matchErr     error
	payload      service.DanmakuPayload
	payloadErr   error
	searchResult service.DanmakuSearchResult
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

func newDanmakuHandlerContainer(t *testing.T, pipeline *stubDanmakuPipeline) *service.Container {
	t.Helper()
	dsn := "file:" + strings.ReplaceAll(t.Name(), "/", "_") + "?mode=memory&cache=shared"
	db, err := gorm.Open(sqlite.Open(dsn), &gorm.Config{})
	if err != nil {
		t.Fatal(err)
	}
	if err := db.AutoMigrate(&model.MediaDanmaku{}); err != nil {
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
	router.GET("/danmaku/search", searchDanmakuHandler(svc))
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
