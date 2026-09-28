package handler

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/glebarez/sqlite"
	"go.uber.org/zap"
	"gorm.io/gorm"
	"gorm.io/gorm/logger"

	"github.com/ShukeBta/MediaStationGo/internal/config"
	"github.com/ShukeBta/MediaStationGo/internal/middleware"
	"github.com/ShukeBta/MediaStationGo/internal/model"
	"github.com/ShukeBta/MediaStationGo/internal/repository"
	"github.com/ShukeBta/MediaStationGo/internal/service"
)

func currentRoleTestServices(t *testing.T) (*config.Config, *service.Container, *model.User) {
	t.Helper()
	gin.SetMode(gin.TestMode)
	db, err := gorm.Open(sqlite.Open(":memory:"), &gorm.Config{Logger: logger.Default.LogMode(logger.Silent)})
	if err != nil {
		t.Fatal(err)
	}
	sqlDB, err := db.DB()
	if err != nil {
		t.Fatal(err)
	}
	sqlDB.SetMaxOpenConns(1)
	t.Cleanup(func() { _ = sqlDB.Close() })
	if err := db.AutoMigrate(&model.User{}, &model.UserPermission{}, &model.Setting{}); err != nil {
		t.Fatal(err)
	}
	cfg := &config.Config{Secrets: config.SecretsConfig{JWTSecret: "current-role-test-secret"}}
	repos := repository.New(db)
	log := zap.NewNop()
	svc := &service.Container{Repo: repos, Log: log}
	svc.Permissions = service.NewPermissionService(log, repos)
	svc.Auth = service.NewAuthService(cfg, log, repos, nil, svc.Permissions)
	svc.Profile = service.NewProfileService(log, repos)
	root := &model.User{
		Base:     model.Base{ID: "protected-admin", CreatedAt: time.Now().Add(-time.Hour)},
		Username: "root", Role: "admin", Tier: "plus", IsActive: true,
	}
	user := &model.User{
		Base:     model.Base{ID: "current-role-user"},
		Username: "viewer", Role: "admin", Tier: "plus", IsActive: true,
	}
	for _, u := range []*model.User{root, user} {
		if err := repos.User.Create(context.Background(), u); err != nil {
			t.Fatal(err)
		}
	}
	return cfg, svc, user
}

func currentRoleRequest(t *testing.T, router http.Handler, token, method, path, body string, want int) *httptest.ResponseRecorder {
	t.Helper()
	req := httptest.NewRequest(method, path, strings.NewReader(body))
	req.Header.Set("Authorization", "Bearer "+token)
	if body != "" {
		req.Header.Set("Content-Type", "application/json")
	}
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)
	if w.Code != want {
		t.Fatalf("%s %s status=%d want=%d body=%s", method, path, w.Code, want, w.Body.String())
	}
	return w
}

func TestDemotedAdminTokensUseCurrentRoleOnRegisteredRoutes(t *testing.T) {
	for _, tokenKind := range []string{"access-60m", "emby-30d"} {
		t.Run(tokenKind, func(t *testing.T) {
			cfg, svc, user := currentRoleTestServices(t)
			issue := svc.Auth.IssueToken
			if tokenKind == "emby-30d" {
				issue = svc.Auth.IssueEmbyToken
			}
			token, err := issue(user)
			if err != nil {
				t.Fatal(err)
			}
			router := gin.New()
			Register(router, cfg, svc.Log, svc)
			currentRoleRequest(t, router, token, http.MethodGet, "/api/admin/users", "", http.StatusOK)
			if _, err := svc.Profile.AdminUpdateRole(context.Background(), user.ID, "user"); err != nil {
				t.Fatal(err)
			}
			for _, path := range []string{"/api/admin/users", "/api/api-config/providers/list", "/api/license/status", "/api/downloads"} {
				currentRoleRequest(t, router, token, http.MethodGet, path, "", http.StatusForbidden)
			}
			currentRoleRequest(t, router, token, http.MethodPatch, "/api/admin/users/"+user.ID+"/role", `{"role":"admin"}`, http.StatusForbidden)
			current, err := svc.Repo.User.FindByID(context.Background(), user.ID)
			if err != nil || current.Role != "user" {
				t.Fatalf("demoted account regained admin: user=%#v err=%v", current, err)
			}
			currentRoleRequest(t, router, token, http.MethodGet, "/api/me", "", http.StatusOK)

			// A token issued before promotion must also pick up newly granted access.
			userToken, err := issue(current)
			if err != nil {
				t.Fatal(err)
			}
			if _, err := svc.Profile.AdminUpdateRole(context.Background(), user.ID, "admin"); err != nil {
				t.Fatal(err)
			}
			currentRoleRequest(t, router, userToken, http.MethodGet, "/api/admin/users", "", http.StatusOK)
		})
	}
}

func TestActiveUserMiddlewareRefreshesRoleAndTier(t *testing.T) {
	for _, emby := range []bool{false, true} {
		name := "web"
		if emby {
			name = "emby"
		}
		t.Run(name, func(t *testing.T) {
			cfg, svc, user := currentRoleTestServices(t)
			token, err := svc.Auth.IssueEmbyToken(user)
			if err != nil {
				t.Fatal(err)
			}
			router := gin.New()
			if emby {
				router.Use(middleware.EmbyAuthRequired(cfg.Secrets.JWTSecret), activeEmbyUserRequired(svc))
			} else {
				router.Use(middleware.AuthRequired(cfg.Secrets.JWTSecret), activeUserRequired(svc))
			}
			router.GET("/identity", func(c *gin.Context) {
				c.JSON(http.StatusOK, gin.H{"role": middleware.GetUserRole(c), "tier": middleware.GetUserTier(c)})
			})
			router.GET("/plus", middleware.PlusOrAdminRequired(), func(c *gin.Context) { c.Status(http.StatusOK) })
			for _, state := range []struct{ role, tier string }{{"user", "free"}, {"user", "plus"}, {"admin", "plus"}} {
				if err := svc.Repo.User.UpdateFields(context.Background(), user.ID, map[string]any{"role": state.role, "tier": state.tier}); err != nil {
					t.Fatal(err)
				}
				w := currentRoleRequest(t, router, token, http.MethodGet, "/identity", "", http.StatusOK)
				var identity map[string]string
				if err := json.Unmarshal(w.Body.Bytes(), &identity); err != nil {
					t.Fatal(err)
				}
				if identity["role"] != state.role || identity["tier"] != state.tier {
					t.Fatalf("context has stale claims: got=%v want=%v", identity, state)
				}
				want := http.StatusOK
				if state.tier == "free" {
					want = http.StatusForbidden
				}
				currentRoleRequest(t, router, token, http.MethodGet, "/plus", "", want)
			}
		})
	}
}

func TestActiveUserDatabaseLockDoesNotTrustTokenRole(t *testing.T) {
	for _, emby := range []bool{false, true} {
		name := "web"
		if emby {
			name = "emby"
		}
		t.Run(name, func(t *testing.T) {
			cfg, svc, user := currentRoleTestServices(t)
			token, err := svc.Auth.IssueEmbyToken(user)
			if err != nil {
				t.Fatal(err)
			}
			locked := true
			if err := svc.Repo.DB.Callback().Query().Before("gorm:query").Register("test:authorization-locked", func(tx *gorm.DB) {
				if locked {
					tx.AddError(errors.New("database is locked (SQLITE_BUSY)"))
				}
			}); err != nil {
				t.Fatal(err)
			}
			router := gin.New()
			if emby {
				router.Use(middleware.EmbyAuthRequired(cfg.Secrets.JWTSecret), activeEmbyUserRequired(svc))
			} else {
				router.Use(middleware.AuthRequired(cfg.Secrets.JWTSecret), activeUserRequired(svc))
			}
			router.GET("/admin", middleware.AdminRequired(), func(c *gin.Context) { c.Status(http.StatusOK) })
			w := currentRoleRequest(t, router, token, http.MethodGet, "/admin", "", http.StatusServiceUnavailable)
			if got := w.Header().Get("Retry-After"); got != "1" {
				t.Fatalf("Retry-After=%q want=1", got)
			}
			locked = false
			currentRoleRequest(t, router, token, http.MethodGet, "/admin", "", http.StatusOK)
		})
	}
}
