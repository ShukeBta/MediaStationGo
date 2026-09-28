package handler

import (
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/ShukeBta/MediaStationGo/internal/middleware"
	"github.com/ShukeBta/MediaStationGo/internal/model"
	"github.com/ShukeBta/MediaStationGo/internal/service"
	"go.uber.org/zap"
)

func TestStopTranscodeRequiresVisibleMedia(t *testing.T) {
	router, svc, secret := newPlaybackScopeTestRouter(t)
	svc.Transcoder = service.NewTranscoderService(svc.Cfg, zap.NewNop(), svc.Repo, nil)
	router.DELETE("/api/hls/:id", middleware.AuthRequired(secret), stopTranscodeHandler(svc))
	if err := svc.Repo.Setting.Set(t.Context(), "adult.enabled", "false"); err != nil {
		t.Fatal(err)
	}
	if err := svc.Repo.DB.Model(&model.Media{}).Where("id = ?", "media-2").Update("nsfw", true).Error; err != nil {
		t.Fatal(err)
	}
	for _, tc := range []struct {
		id     string
		status int
	}{{"media-1", http.StatusNoContent}, {"media-2", http.StatusNotFound}, {"missing", http.StatusNotFound}} {
		req := httptest.NewRequest(http.MethodDelete, "/api/hls/"+tc.id, nil)
		req.Header.Set("Authorization", "Bearer "+signedTestToken(t, secret))
		response := httptest.NewRecorder()
		router.ServeHTTP(response, req)
		if response.Code != tc.status {
			t.Fatalf("%s: status=%d body=%s", tc.id, response.Code, response.Body.String())
		}
	}
}
