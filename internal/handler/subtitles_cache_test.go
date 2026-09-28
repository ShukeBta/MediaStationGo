package handler

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/gin-gonic/gin"

	"github.com/ShukeBta/MediaStationGo/internal/service"
)

func TestInvalidateCloudSubtitleCacheHandlerAcceptsEmptyBody(t *testing.T) {
	gin.SetMode(gin.TestMode)
	subtitle := service.NewSubtitleService(nil, nil)
	router := gin.New()
	router.POST("/subtitles/cloud-cache/invalidate", invalidateCloudSubtitleCacheHandler(&service.Container{Subtitle: subtitle}))

	req := httptest.NewRequest(http.MethodPost, "/subtitles/cloud-cache/invalidate", nil)
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)
	if w.Code != http.StatusOK || !strings.Contains(w.Body.String(), `"invalidated":0`) {
		t.Fatalf("status=%d body=%s", w.Code, w.Body.String())
	}
}

func TestInvalidateCloudSubtitleCacheHandlerRejectsInvalidJSON(t *testing.T) {
	gin.SetMode(gin.TestMode)
	subtitle := service.NewSubtitleService(nil, nil)
	router := gin.New()
	router.POST("/subtitles/cloud-cache/invalidate", invalidateCloudSubtitleCacheHandler(&service.Container{Subtitle: subtitle}))

	req := httptest.NewRequest(http.MethodPost, "/subtitles/cloud-cache/invalidate", strings.NewReader(`{"media_id":`))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)
	if w.Code != http.StatusBadRequest {
		t.Fatalf("status=%d body=%s", w.Code, w.Body.String())
	}
}
