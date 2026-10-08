package handler

import (
	"encoding/json"
	"fmt"
	"net/http"
	"testing"

	"github.com/ShukeBta/MediaStationGo/internal/middleware"
	"github.com/ShukeBta/MediaStationGo/internal/model"
	"github.com/ShukeBta/MediaStationGo/internal/service"
	"github.com/gin-gonic/gin"
)

func TestSubscriptionDefaultRulesApplyAcrossCreationEntrypoints(t *testing.T) {
	_, svc, user := currentRoleTestServices(t)
	if err := svc.Repo.DB.AutoMigrate(&model.Subscription{}, &model.Setting{}, &model.Media{}, &model.Library{}, &model.LibraryRoot{}, &model.DownloadTask{}); err != nil {
		t.Fatal(err)
	}
	svc.Subscription = service.NewSubscriptionService(nil, svc.Log, svc.Repo, nil, nil, nil)
	router := gin.New()
	router.Use(func(c *gin.Context) { c.Set(middleware.CtxUserID, user.ID) })
	router.PUT("/defaults", subscriptionDefaultRulesHandler(svc, true))
	router.GET("/defaults", subscriptionDefaultRulesHandler(svc, false))
	router.POST("/subscriptions", createSubscriptionHandler(svc))
	router.POST("/sites/subscribe", siteSubscribeHandler(svc))
	rules := `{"poll_interval_minutes":30,"resolution":"2160p","quality":"web-dl","effects":"hdr","release_groups":"WEB","exclude_words":"sample","min_seeders":5,"max_seeders":0,"min_size_gb":1,"max_size_gb":30,"free_only":true,"wash_enabled":true,"wash_priority":"resolution"}`
	currentRoleRequest(t, router, "", http.MethodPut, "/defaults", rules, http.StatusOK)
	for i, endpoint := range []string{"/subscriptions", "/sites/subscribe"} {
		for _, override := range []bool{false, true} {
			name := fmt.Sprintf("default-test-%d-%t", i, override)
			payload := map[string]any{"name": name, "keyword": name, "feed_url": "site-search://search?keyword=" + name, "enabled": false}
			if override {
				payload["resolution"] = "720p"
				payload["free_only"] = false
				payload["wash_enabled"] = false
				payload["min_seeders"] = 0
				payload["exclude_words"] = ""
			}
			body, _ := json.Marshal(payload)
			currentRoleRequest(t, router, "", http.MethodPost, endpoint, string(body), http.StatusCreated)
			var sub model.Subscription
			if err := svc.Repo.DB.Where("name = ?", name).First(&sub).Error; err != nil {
				t.Fatal(err)
			}
			if sub.PollIntervalMinutes != 30 || sub.Quality != "web-dl" || sub.MinSizeGB != 1 || sub.Name != name || sub.Enabled {
				t.Fatalf("defaults/identity: %+v", sub)
			}
			if override {
				if sub.Resolution != "720p" || sub.FreeOnly || sub.WashEnabled || sub.MinSeeders != 0 || sub.ExcludeWords != "" {
					t.Fatalf("explicit zero/false/empty overridden: %+v", sub)
				}
			} else if sub.Resolution != "2160p" || !sub.FreeOnly || !sub.WashEnabled || sub.MinSeeders != 5 {
				t.Fatalf("template not inherited: %+v", sub)
			}
		}
	}
	// Saving a new global template must not rewrite existing subscriptions.
	currentRoleRequest(t, router, "", http.MethodPut, "/defaults", `{"poll_interval_minutes":90,"resolution":"best","wash_priority":"balanced"}`, http.StatusOK)
	var changed int64
	svc.Repo.DB.Model(&model.Subscription{}).Where("poll_interval_minutes <> 30").Count(&changed)
	if changed != 0 {
		t.Fatal("global template rewrote existing rules")
	}
	currentRoleRequest(t, router, "", http.MethodPut, "/defaults", `{"poll_interval_minutes":1,"resolution":"best","wash_priority":"balanced"}`, http.StatusBadRequest)
	got, err := svc.Subscription.DefaultRules(t.Context())
	if err != nil || got.PollIntervalMinutes != 90 {
		t.Fatalf("invalid save changed defaults: %+v %v", got, err)
	}
}
