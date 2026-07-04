package handler

import (
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/gin-gonic/gin"

	"github.com/ShukeBta/MediaStationGo/internal/service/cloud"
)

func TestProxyCloudResolvedLinkUsesHEADWithoutSyntheticRange(t *testing.T) {
	gin.SetMode(gin.TestMode)
	var upstreamMethod, upstreamRange string
	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		upstreamMethod = r.Method
		upstreamRange = r.Header.Get("Range")
		w.Header().Set("Content-Type", "video/mp4")
		w.Header().Set("Content-Length", "123456")
		w.Header().Set("Accept-Ranges", "bytes")
		w.WriteHeader(http.StatusOK)
	}))
	defer upstream.Close()

	rec := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(rec)
	c.Request = httptest.NewRequest(http.MethodHead, "/api/cloud/play/openlist?ref=movie", nil)

	proxyCloudResolvedLink(cloudPlaybackRequest{
		c:   c,
		typ: "openlist",
		ref: "movie",
		link: &cloud.DirectLink{
			URL:   upstream.URL + "/movie.mp4",
			Proxy: true,
		},
	})

	if upstreamMethod != http.MethodHead {
		t.Fatalf("upstream method = %q, want HEAD", upstreamMethod)
	}
	if upstreamRange != "" {
		t.Fatalf("upstream Range = %q, want empty", upstreamRange)
	}
	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200", rec.Code)
	}
	if got := rec.Header().Get("Content-Length"); got != "123456" {
		t.Fatalf("Content-Length = %q, want full upstream length", got)
	}
	if rec.Body.Len() != 0 {
		t.Fatalf("HEAD response body length = %d, want 0", rec.Body.Len())
	}
}
