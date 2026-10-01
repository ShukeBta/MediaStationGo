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
