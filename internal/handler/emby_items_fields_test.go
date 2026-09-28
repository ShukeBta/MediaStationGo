package handler

import (
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/gin-gonic/gin"
)

func TestParseEmbyItemsParamsOmitsMediaSourcesOnlyWhenExplicitlyExcluded(t *testing.T) {
	gin.SetMode(gin.TestMode)
	tests := []struct {
		name string
		path string
		want bool
	}{
		{name: "fields omitted", path: "/emby/Items", want: false},
		{name: "fields exclude sources", path: "/emby/Items?Fields=BasicSyncInfo,ChildCount,RunTimeTicks", want: true},
		{name: "fields include sources", path: "/emby/Items?Fields=BasicSyncInfo,MediaSources", want: false},
		{name: "fields are case insensitive", path: "/emby/Items?fields=mediasources", want: false},
		{name: "empty fields are explicit", path: "/emby/Items?Fields=", want: true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			w := httptest.NewRecorder()
			c, _ := gin.CreateTestContext(w)
			c.Request = httptest.NewRequest(http.MethodGet, tt.path, nil)
			if got := parseEmbyItemsParams(c).OmitMediaSources; got != tt.want {
				t.Fatalf("OmitMediaSources = %v, want %v", got, tt.want)
			}
		})
	}
}
