package handler

import (
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/gin-gonic/gin"
)

func TestWriteInternalOrCanceledUses499ForCanceledRequest(t *testing.T) {
	gin.SetMode(gin.TestMode)
	w := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(w)
	ctx, cancel := context.WithCancel(t.Context())
	req := httptest.NewRequest(http.MethodGet, "/api/libraries/lib/media", nil).WithContext(ctx)
	c.Request = req
	cancel()

	writeInternalOrCanceled(c, errors.New("sql: statement is closed"))

	if w.Code != statusClientClosedRequest {
		t.Fatalf("status = %d, want %d", w.Code, statusClientClosedRequest)
	}
}

func TestWriteInternalOrCanceledKeepsRealErrorsAs500(t *testing.T) {
	gin.SetMode(gin.TestMode)
	w := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(w)
	c.Request = httptest.NewRequest(http.MethodGet, "/api/libraries/lib/media", nil)

	writeInternalOrCanceled(c, errors.New("database is unavailable"))

	if w.Code != http.StatusInternalServerError {
		t.Fatalf("status = %d, want %d", w.Code, http.StatusInternalServerError)
	}
}
