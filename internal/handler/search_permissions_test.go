package handler

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/gin-gonic/gin"

	"github.com/ShukeBta/MediaStationGo/internal/middleware"
	"github.com/ShukeBta/MediaStationGo/internal/model"
	"github.com/ShukeBta/MediaStationGo/internal/service"
)

func TestOrdinarySearchRoutesWorkWithoutAI(t *testing.T) {
	cfg, svc, user := currentRoleTestServices(t)
	user.Role = "user"
	if err := svc.Repo.User.UpdateFields(t.Context(), user.ID, map[string]any{"role": user.Role}); err != nil {
		t.Fatal(err)
	}
	if err := svc.Repo.DB.AutoMigrate(&model.Library{}, &model.Media{}, &model.PlayProfile{}, &model.Site{}); err != nil {
		t.Fatal(err)
	}
	svc.Media = service.NewMediaService(cfg, svc.Log, svc.Repo)
	svc.Site = service.NewSiteService(svc.Log, svc.Repo, "")
	svc.AI = service.NewAIService(cfg, svc.Log, nil)
	if svc.AI.Enabled() {
		t.Fatal("test requires AI to be completely disabled")
	}
	lib := model.Library{Base: model.Base{ID: "search-library"}, Name: "电影", Path: "/media/search", Type: "movie", Enabled: true}
	if err := svc.Repo.DB.Create(&lib).Error; err != nil {
		t.Fatal(err)
	}
	media := model.Media{Base: model.Base{ID: "search-movie"}, LibraryID: lib.ID, Title: "Example movie", Path: "/media/search/example.mkv"}
	if err := svc.Repo.DB.Create(&media).Error; err != nil {
		t.Fatal(err)
	}
	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "application/rss+xml")
		_, _ = w.Write([]byte(`<rss version="2.0"><channel><item><title>Example movie</title><link>https://tracker.example/1</link><enclosure url="https://tracker.example/1.torrent" length="1024" type="application/x-bittorrent"/></item></channel></rss>`))
	}))
	defer upstream.Close()
	if err := svc.Repo.DB.Create(&model.Site{Name: "Fixture site", Type: "custom_rss", URL: upstream.URL, Enabled: true, Timeout: 5}).Error; err != nil {
		t.Fatal(err)
	}
	router := gin.New()
	authed := router.Group("/api", func(c *gin.Context) {
		c.Set(middleware.CtxUserID, user.ID)
		c.Set(middleware.CtxUserRole, user.Role)
		c.Next()
	})
	registerAuthedSearchRoutes(authed, svc)
	// The genuinely AI-specific route must retain its own permission.
	authed.POST("/ai/search", requirePermission(svc, "can_use_ai"), smartSearchHandler(svc))

	for _, tt := range []struct {
		name       string
		path       string
		permission model.UserPermission
		wantTitle  bool
	}{
		{name: "library", path: "/api/search?q=Example", permission: model.UserPermission{CanPlayMedia: true}, wantTitle: true},
		{name: "advanced library", path: "/api/search/advanced?q=Example", permission: model.UserPermission{CanPlayMedia: true}, wantTitle: true},
		{name: "catalog", path: "/api/search/tmdb?query=Example", permission: model.UserPermission{CanViewDiscover: true}},
		{name: "tracker", path: "/api/search/sites?keyword=Example", permission: model.UserPermission{CanManageSites: true}, wantTitle: true},
	} {
		t.Run(tt.name, func(t *testing.T) {
			if err := svc.Permissions.Save(t.Context(), user.ID, &tt.permission); err != nil {
				t.Fatal(err)
			}
			response := httptest.NewRecorder()
			router.ServeHTTP(response, httptest.NewRequest(http.MethodGet, tt.path, nil))
			if response.Code != http.StatusOK {
				t.Fatalf("ordinary search without AI: status=%d body=%s", response.Code, response.Body)
			}
			var payload struct {
				Items []struct {
					Title string `json:"title"`
				} `json:"items"`
			}
			if err := json.Unmarshal(response.Body.Bytes(), &payload); err != nil {
				t.Fatal(err)
			}
			if tt.wantTitle && (len(payload.Items) != 1 || payload.Items[0].Title != media.Title) {
				t.Fatalf("missing ordinary search result: %s", response.Body)
			}
			response = httptest.NewRecorder()
			router.ServeHTTP(response, httptest.NewRequest(http.MethodPost, "/api/ai/search", strings.NewReader(`{"query":"Example"}`)))
			if response.Code != http.StatusForbidden {
				t.Fatalf("AI search permission was bypassed: status=%d", response.Code)
			}
			if err := svc.Permissions.Save(t.Context(), user.ID, &model.UserPermission{CanUseAI: true}); err != nil {
				t.Fatal(err)
			}
			response = httptest.NewRecorder()
			router.ServeHTTP(response, httptest.NewRequest(http.MethodGet, tt.path, nil))
			if response.Code != http.StatusForbidden {
				t.Fatalf("AI permission bypassed normal search scope: status=%d body=%s", response.Code, response.Body)
			}
		})
	}
}
