package handler

import (
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/gin-gonic/gin"
	"github.com/glebarez/sqlite"
	"go.uber.org/zap"
	"gorm.io/gorm"

	"github.com/ShukeBta/MediaStationGo/internal/config"
	"github.com/ShukeBta/MediaStationGo/internal/model"
	"github.com/ShukeBta/MediaStationGo/internal/repository"
	"github.com/ShukeBta/MediaStationGo/internal/service"
)

func TestEmbyItemsAlwaysReturnsDynamicPayload(t *testing.T) {
	gin.SetMode(gin.TestMode)
	db, err := gorm.Open(sqlite.Open(":memory:"), &gorm.Config{})
	if err != nil {
		t.Fatalf("open db: %v", err)
	}
	if err := db.AutoMigrate(model.AllModels()...); err != nil {
		t.Fatalf("migrate: %v", err)
	}
	repos := repository.New(db)
	if err := repos.User.Create(t.Context(), &model.User{
		Base:         model.Base{ID: "user-1"},
		Username:     "tester",
		PasswordHash: "x",
		Role:         "admin",
		Tier:         "plus",
		IsActive:     true,
	}); err != nil {
		t.Fatalf("create user: %v", err)
	}
	library := model.Library{Name: "电影", Path: "D:\\media\\movies", Type: "movie", Enabled: true}
	if err := repos.Library.Create(t.Context(), &library); err != nil {
		t.Fatalf("create library: %v", err)
	}
	if err := db.Create(&model.Media{
		Base:      model.Base{ID: "movie-1"},
		LibraryID: library.ID,
		Title:     "条件请求测试",
		Path:      "D:\\media\\movies\\conditional.mkv",
		Container: "mkv",
	}).Error; err != nil {
		t.Fatalf("create media: %v", err)
	}

	const secret = "test-secret"
	router := gin.New()
	registerEmbyRoutes(router, secret, &service.Container{
		Repo: repos,
		Emby: service.NewEmbyService(&config.Config{}, zap.NewNop(), repos),
	})
	token := signedTestToken(t, secret)
	path := "/emby/Users/user-1/Items?ParentId=" + library.ID + "&StartIndex=0&Limit=12"

	var firstBody string
	for _, ifNoneMatch := range []string{"", `"matching-client-validator"`, `W/"weak-client-validator"`, `*`} {
		response := performAuthenticatedItemsRequest(router, path, token, ifNoneMatch)
		if response.Code != http.StatusOK {
			t.Fatalf("If-None-Match %q status = %d body=%s", ifNoneMatch, response.Code, response.Body.String())
		}
		if response.Body.Len() == 0 {
			t.Fatalf("If-None-Match %q must receive the current JSON payload", ifNoneMatch)
		}
		if got := response.Header().Get("ETag"); got != "" {
			t.Fatalf("If-None-Match %q ETag = %q, want empty", ifNoneMatch, got)
		}
		if got := response.Header().Get("Cache-Control"); got != "no-store" {
			t.Fatalf("If-None-Match %q Cache-Control = %q, want no-store", ifNoneMatch, got)
		}
		if firstBody == "" {
			firstBody = response.Body.String()
		} else if response.Body.String() != firstBody {
			t.Fatalf("If-None-Match %q changed dynamic Items payload", ifNoneMatch)
		}
	}
}

func performAuthenticatedItemsRequest(
	router http.Handler,
	path string,
	token string,
	ifNoneMatch string,
) *httptest.ResponseRecorder {
	req := httptest.NewRequest(http.MethodGet, path, nil)
	req.Header.Set("X-Emby-Token", token)
	if ifNoneMatch != "" {
		req.Header.Set("If-None-Match", ifNoneMatch)
	}
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)
	return w
}
