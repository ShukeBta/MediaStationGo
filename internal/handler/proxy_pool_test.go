package handler

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/gin-gonic/gin"
	"go.uber.org/zap"
	"gorm.io/gorm"

	"github.com/ShukeBta/MediaStationGo/internal/model"
	"github.com/ShukeBta/MediaStationGo/internal/repository"
	"github.com/ShukeBta/MediaStationGo/internal/service"
	"github.com/glebarez/sqlite"
)

func TestProxyPoolHandlersNeverReturnCredentials(t *testing.T) {
	gin.SetMode(gin.TestMode)
	db, err := gorm.Open(sqlite.Open(":memory:"), &gorm.Config{})
	if err != nil {
		t.Fatal(err)
	}
	if err := db.AutoMigrate(&model.ProxyPoolEntry{}); err != nil {
		t.Fatal(err)
	}
	proxyPool := service.NewProxyPoolService(
		&repository.Container{DB: db},
		service.NewCryptoService("proxy-pool-handler-secret", zap.NewNop()),
	)
	svc := &service.Container{ProxyPool: proxyPool}
	router := gin.New()
	router.GET("/proxy-pool", listProxyPoolHandler(svc))
	router.PUT("/proxy-pool", replaceProxyPoolHandler(svc))
	router.POST("/proxy-pool/cleanup", cleanupProxyPoolHandler(svc))

	const username = "handler-user"
	const password = "handler-password"
	request := httptest.NewRequest(http.MethodPut, "/proxy-pool", strings.NewReader(
		`{"items":[{"url":"http://`+username+`:`+password+`@proxy.example:8080"}]}`,
	))
	request.Header.Set("Content-Type", "application/json")
	response := httptest.NewRecorder()
	router.ServeHTTP(response, request)
	if response.Code != http.StatusOK || !strings.Contains(response.Body.String(), `"has_auth":true`) {
		t.Fatalf("PUT status=%d body=%s", response.Code, response.Body.String())
	}
	assertProxyPoolResponseSafe(t, response.Body.String(), username, password)

	response = httptest.NewRecorder()
	router.ServeHTTP(response, httptest.NewRequest(http.MethodGet, "/proxy-pool", nil))
	if response.Code != http.StatusOK {
		t.Fatalf("GET status=%d body=%s", response.Code, response.Body.String())
	}
	assertProxyPoolResponseSafe(t, response.Body.String(), username, password)
	var listed struct {
		Items []service.ProxyPoolItem `json:"items"`
	}
	if err := json.Unmarshal(response.Body.Bytes(), &listed); err != nil || len(listed.Items) != 1 {
		t.Fatalf("decode proxy pool: %#v, %v", listed, err)
	}

	request = httptest.NewRequest(http.MethodPost, "/proxy-pool/cleanup", strings.NewReader(`{"token":"invalid"}`))
	request.Header.Set("Content-Type", "application/json")
	response = httptest.NewRecorder()
	router.ServeHTTP(response, request)
	if response.Code != http.StatusBadRequest {
		t.Fatalf("cleanup status=%d body=%s", response.Code, response.Body.String())
	}
	assertProxyPoolResponseSafe(t, response.Body.String(), username, password)

	request = httptest.NewRequest(http.MethodPut, "/proxy-pool", strings.NewReader(
		`{"items":[{"url":"ftp://`+username+`:`+password+`@proxy.example:21"}]}`,
	))
	request.Header.Set("Content-Type", "application/json")
	response = httptest.NewRecorder()
	router.ServeHTTP(response, request)
	if response.Code != http.StatusBadRequest {
		t.Fatalf("invalid PUT status=%d body=%s", response.Code, response.Body.String())
	}
	assertProxyPoolResponseSafe(t, response.Body.String(), username, password)
}

func assertProxyPoolResponseSafe(t *testing.T, response string, forbidden ...string) {
	t.Helper()
	for _, value := range forbidden {
		if strings.Contains(response, value) {
			t.Fatalf("proxy pool response exposed %q: %s", value, response)
		}
	}
}
