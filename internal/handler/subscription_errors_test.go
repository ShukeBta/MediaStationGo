package handler

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/gin-gonic/gin"

	"github.com/ShukeBta/MediaStationGo/internal/service"
)

func TestWriteSubscriptionConflictReturns409WithExistingID(t *testing.T) {
	gin.SetMode(gin.TestMode)
	w := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(w)
	err := &service.SubscriptionAlreadyExistsError{ExistingID: "existing-sub"}
	if !writeSubscriptionConflict(c, err) {
		t.Fatal("expected conflict to be handled")
	}
	if w.Code != http.StatusConflict {
		t.Fatalf("status = %d body=%s", w.Code, w.Body.String())
	}
	var body map[string]any
	if err := json.Unmarshal(w.Body.Bytes(), &body); err != nil {
		t.Fatal(err)
	}
	if body["existing_id"] != "existing-sub" {
		t.Fatalf("body = %#v", body)
	}
}
