package service

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strconv"
	"strings"
	"testing"
)

func TestAria2AdapterRejectsHTTPError(t *testing.T) {
	for _, status := range []int{http.StatusBadRequest, http.StatusUnauthorized, http.StatusServiceUnavailable} {
		t.Run(strconv.Itoa(status), func(t *testing.T) {
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				var request aria2Request
				if err := json.NewDecoder(r.Body).Decode(&request); err != nil {
					t.Error(err)
					w.WriteHeader(http.StatusBadRequest)
					return
				}
				w.Header().Set("Content-Type", "application/json")
				w.WriteHeader(status)
				_ = json.NewEncoder(w).Encode(map[string]any{
					"jsonrpc": "2.0",
					"id":      request.ID,
					"result":  map[string]any{"version": "1.37"},
				})
			}))
			defer server.Close()

			adapter := NewAria2Adapter()
			if err := adapter.Initialize(t.Context(), DownloadClientConfig{Host: server.URL}); err == nil || !strings.Contains(err.Error(), strconv.Itoa(status)) {
				t.Fatalf("Initialize error = %v, want HTTP status %d", err, status)
			}
			if err := adapter.Ping(t.Context()); err == nil || !strings.Contains(err.Error(), strconv.Itoa(status)) {
				t.Fatalf("Ping error = %v, want HTTP status %d", err, status)
			}
		})
	}
}
