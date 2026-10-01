package handler

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync/atomic"
	"testing"

	"github.com/gin-gonic/gin"

	"github.com/ShukeBta/MediaStationGo/internal/middleware"
	"github.com/ShukeBta/MediaStationGo/internal/model"
	"github.com/ShukeBta/MediaStationGo/internal/service"
)

func TestSiteSubscribeWithoutAIKeepsRuleAndReportsFirstRunFailure(t *testing.T) {
	for _, enabled := range []bool{true, false} {
		name := "paused"
		if enabled {
			name = "enabled"
		}
		t.Run(name, func(t *testing.T) {
			cfg, svc, user := currentRoleTestServices(t)
			if err := svc.Repo.DB.AutoMigrate(&model.Subscription{}, &model.Media{}, &model.DownloadTask{}, &model.Library{}, &model.LibraryRoot{}); err != nil {
				t.Fatal(err)
			}
			// A missing tracker/downloader must fail the run, not silently look
			// like an empty search or roll back the subscription the user saved.
			svc.Subscription = service.NewSubscriptionService(cfg, svc.Log, svc.Repo, nil, nil, service.NewHub(svc.Log))
			router := gin.New()
			router.POST("/sites/subscribe", func(c *gin.Context) { c.Set(middleware.CtxUserID, user.ID) }, siteSubscribeHandler(svc))
			body, _ := json.Marshal(map[string]any{
				"name": "示例剧集", "keyword": "示例剧集", "media_type": "tv",
				"season_number": 2, "total_episodes": 0, "poll_interval_minutes": 5, "enabled": enabled,
			})
			req := httptest.NewRequest(http.MethodPost, "/sites/subscribe", strings.NewReader(string(body)))
			req.Header.Set("Content-Type", "application/json")
			recorder := httptest.NewRecorder()
			router.ServeHTTP(recorder, req)
			var response struct {
				Subscription model.Subscription `json:"subscription"`
				RunError     string             `json:"run_error"`
				Explanation  []string           `json:"explanation"`
			}
			if recorder.Code != http.StatusCreated {
				t.Fatalf("create status %d: %s", recorder.Code, recorder.Body.String())
			}
			if err := json.Unmarshal(recorder.Body.Bytes(), &response); err != nil {
				t.Fatal(err)
			}
			var stored model.Subscription
			if err := svc.Repo.DB.First(&stored, "id = ?", response.Subscription.ID).Error; err != nil {
				t.Fatal(err)
			}
			if stored.DeliveryMode != "download" || !strings.HasPrefix(stored.FeedURL, "site-search://") || stored.SeasonNumber != 2 || stored.TotalEpisodes != 0 || stored.PollIntervalMinutes != 5 || stored.Enabled != enabled {
				t.Fatalf("PT subscription lost configured fields: %+v", stored)
			}
			if enabled && (response.RunError == "" || !strings.Contains(strings.Join(response.Explanation, " "), "订阅已保存")) {
				t.Fatalf("initial failure must be visible: %s", recorder.Body.String())
			}
			if !enabled && (response.RunError != "" || !strings.Contains(strings.Join(response.Explanation, " "), "暂停")) {
				t.Fatalf("paused subscription must not claim it ran: %s", recorder.Body.String())
			}
			recorder = httptest.NewRecorder()
			req = httptest.NewRequest(http.MethodPost, "/sites/subscribe", strings.NewReader(string(body)))
			req.Header.Set("Content-Type", "application/json")
			router.ServeHTTP(recorder, req)
			if recorder.Code != http.StatusConflict {
				t.Fatalf("duplicate should be explicit 409: %d %s", recorder.Code, recorder.Body.String())
			}
			// A different season of the same title is a separate follow-up rule.
			body = []byte(strings.Replace(string(body), `"season_number":2`, `"season_number":1`, 1))
			recorder = httptest.NewRecorder()
			req = httptest.NewRequest(http.MethodPost, "/sites/subscribe", strings.NewReader(string(body)))
			req.Header.Set("Content-Type", "application/json")
			router.ServeHTTP(recorder, req)
			if recorder.Code != http.StatusCreated {
				t.Fatalf("another season should be independent: %d %s", recorder.Code, recorder.Body.String())
			}
		})
	}
}

func TestPTSubscriptionCreateAndClearDestinationWithoutMetadata(t *testing.T) {
	for _, endpoint := range []string{"/subscriptions", "/sites/subscribe"} {
		t.Run(endpoint, func(t *testing.T) {
			cfg, svc, user := currentRoleTestServices(t)
			if err := svc.Repo.DB.AutoMigrate(&model.Subscription{}, &model.Media{}, &model.DownloadTask{}, &model.Library{}, &model.LibraryRoot{}); err != nil {
				t.Fatal(err)
			}
			var metadataCalls atomic.Int32
			metadata := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				metadataCalls.Add(1)
				http.Error(w, "metadata offline", http.StatusServiceUnavailable)
			}))
			t.Cleanup(metadata.Close)
			cfg.Secrets.TMDbAPIKey, cfg.Secrets.TMDbAPIProxy = "test-optional", metadata.URL
			svc.TMDb = service.NewTMDbProvider(cfg, svc.Log, nil)
			svc.Subscription = service.NewSubscriptionService(cfg, svc.Log, svc.Repo, nil, nil, service.NewHub(svc.Log))
			router := gin.New()
			router.Use(func(c *gin.Context) { c.Set(middleware.CtxUserID, user.ID) })
			router.POST("/subscriptions", createSubscriptionHandler(svc))
			router.POST("/sites/subscribe", siteSubscribeHandler(svc))
			router.PATCH("/subscriptions/:id", updateSubscriptionHandler(svc))
			body, _ := json.Marshal(map[string]any{
				"name": "本地目录回归 " + endpoint, "keyword": "本地目录回归", "media_type": "tv",
				"feed_url": "site-search://resources?keyword=LocalDestination", "delivery_mode": "download",
				"season_number": 1, "enabled": false, "save_path": "/qb/downloads/custom",
			})
			currentRoleRequest(t, router, "", http.MethodPost, endpoint, string(body), http.StatusCreated)
			var sub model.Subscription
			if err := svc.Repo.DB.First(&sub).Error; err != nil {
				t.Fatal(err)
			}
			if sub.SavePath != "/qb/downloads/custom" || sub.DeliveryMode != "download" {
				t.Fatalf("created destination = %+v", sub)
			}
			if got := metadataCalls.Load(); got != 0 {
				t.Fatalf("PT create waited on %d optional metadata calls", got)
			}
			currentRoleRequest(t, router, "", http.MethodPatch, "/subscriptions/"+sub.ID, `{"save_path":""}`, http.StatusNoContent)
			if err := svc.Repo.DB.First(&sub, "id = ?", sub.ID).Error; err != nil {
				t.Fatal(err)
			}
			if sub.SavePath != "" {
				t.Fatalf("cleared destination = %q, want downloader default", sub.SavePath)
			}
		})
	}
}

func TestSubscriptionPatchConvertsLegacyCloudRuleToLocalPTWithoutPipeline(t *testing.T) {
	cfg, svc, user := currentRoleTestServices(t)
	if err := svc.Repo.DB.AutoMigrate(&model.Subscription{}); err != nil {
		t.Fatal(err)
	}
	svc.Subscription = service.NewSubscriptionService(cfg, svc.Log, svc.Repo, nil, nil, service.NewHub(svc.Log))
	old := &model.Subscription{
		UserID: user.ID, Name: "本地追更示例", Filter: "本地追更示例", Enabled: true,
		DeliveryMode: "resource_import", FeedURL: "resource-import://default?alias=Local+Show",
		LibraryID: "old-cloud-library", LibraryRootID: "old-cloud-root", ResourceSource: "115",
		MediaType: "tv", SeasonNumber: 1,
	}
	// Existing installations can retain a cloud rule after the optional cloud
	// service has been disabled. Conversion must not need that service or root.
	if err := svc.Repo.DB.Create(old).Error; err != nil {
		t.Fatal(err)
	}
	router := gin.New()
	router.Use(func(c *gin.Context) { c.Set(middleware.CtxUserID, user.ID) })
	router.PATCH("/subscriptions/:id", updateSubscriptionHandler(svc))
	feed := "site-search://resources?keyword=Local+Show&alias=Chinese+Alias&alias=Original+Title"
	body, _ := json.Marshal(map[string]any{
		"delivery_mode": "download", "feed_url": feed, "filter": "",
		"library_id": "", "library_root_id": "", "resource_source": "",
		"save_path": "/qb/local-disk", "season_number": 1,
	})
	currentRoleRequest(t, router, "", http.MethodPatch, "/subscriptions/"+old.ID, string(body), http.StatusNoContent)
	var stored model.Subscription
	if err := svc.Repo.DB.First(&stored, "id = ?", old.ID).Error; err != nil {
		t.Fatal(err)
	}
	if stored.DeliveryMode != "download" || stored.FeedURL != feed || stored.SavePath != "/qb/local-disk" {
		t.Fatalf("converted PT rule lost feed/aliases/destination: %+v", stored)
	}
	if stored.LibraryID != "" || stored.LibraryRootID != "" || stored.ResourceSource != "" || stored.Filter != "" {
		t.Fatalf("conversion retained cloud destination/filter: %+v", stored)
	}
	if !stored.Enabled || stored.SeasonNumber != 1 || stored.TotalEpisodes != 0 || svc.ResourceImport != nil {
		t.Fatalf("conversion changed tracking settings or required cloud service: %+v", stored)
	}
}
