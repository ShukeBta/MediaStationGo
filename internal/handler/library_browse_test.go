package handler

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/ShukeBta/MediaStationGo/internal/middleware"
	"github.com/ShukeBta/MediaStationGo/internal/service"
)

func TestLibraryBrowseHTTPContract(t *testing.T) {
	router, svc, secret := newPlaybackScopeTestRouter(t)
	authed := router.Group("/api", middleware.AuthRequired(secret))
	registerAuthedLibraryRoutes(authed, svc)
	token := signedTestToken(t, secret)
	for _, tc := range []struct {
		url           string
		authenticated bool
		status        int
	}{
		{"/api/libraries/lib-1/browse?page=1&facets=1", false, http.StatusUnauthorized},
		{"/api/libraries/lib-1/browse?page=1&facets=1", true, http.StatusOK},
		{"/api/libraries/lib-1/browse?page=0", true, http.StatusBadRequest},
		{"/api/libraries/lib-1/browse?page=1.5", true, http.StatusBadRequest},
		{"/api/libraries/lib-1/browse?page=10000001", true, http.StatusBadRequest},
		{"/api/libraries/lib-1/browse?actor=Someone", true, http.StatusBadRequest},
		{"/api/libraries/lib-1/browse?adult_type=FC2", true, http.StatusBadRequest},
		{"/api/libraries/missing/browse", true, http.StatusNotFound},
	} {
		t.Run(tc.url, func(t *testing.T) {
			req := httptest.NewRequest(http.MethodGet, tc.url, nil)
			if tc.authenticated {
				req.Header.Set("Authorization", "Bearer "+token)
			}
			w := httptest.NewRecorder()
			router.ServeHTTP(w, req)
			if w.Code != tc.status {
				t.Fatalf("status=%d body=%s", w.Code, w.Body.String())
			}
			if tc.status == http.StatusOK {
				var page service.LibraryBrowsePage
				if err := json.Unmarshal(w.Body.Bytes(), &page); err != nil {
					t.Fatal(err)
				}
				if page.PageSize != 48 || page.Page != 1 || page.Total != 2 || len(page.Items) != 2 || page.SeriesCards == nil || page.Facets == nil {
					t.Fatalf("unexpected browse JSON: %s", w.Body.String())
				}
			}
		})
	}
}
