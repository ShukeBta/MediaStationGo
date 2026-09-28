package handler

import (
	"bufio"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/ShukeBta/MediaStationGo/internal/middleware"
	"github.com/ShukeBta/MediaStationGo/internal/model"
	"github.com/ShukeBta/MediaStationGo/internal/service"
	"github.com/gin-gonic/gin"
	"github.com/gorilla/websocket"
	"gorm.io/gorm"
)

type realtimeTestEvent struct {
	Topic   string            `json:"topic"`
	Payload map[string]string `json:"payload"`
	err     error
}

func realtimePermissionServer(t *testing.T) (*httptest.Server, *service.Container, *model.User, string) {
	t.Helper()
	cfg, svc, user := currentRoleTestServices(t)
	svc.WSHub = service.NewHub(svc.Log)
	svc.SSEHub = service.NewSSEHub(svc.Log)
	go svc.WSHub.Run()
	go svc.SSEHub.Run()
	router := gin.New()
	authed := router.Group("/api", middleware.AuthRequired(cfg.Secrets.JWTSecret), activeUserRequired(svc))
	authed.GET("/ws", wsHandler(svc))
	authed.GET("/events", sseHandler(svc))
	server := httptest.NewServer(router)
	t.Cleanup(func() {
		// Client cleanups registered below run first. Keep SSE's broker alive
		// until the HTTP handler has unregistered its disconnected client.
		server.Close()
		svc.WSHub.Stop()
		svc.SSEHub.Stop()
	})
	token, err := svc.Auth.IssueToken(user)
	if err != nil {
		t.Fatal(err)
	}
	return server, svc, user, token
}

func realtimeReadEvent(t *testing.T, events <-chan realtimeTestEvent, topic, marker string) {
	t.Helper()
	select {
	case event := <-events:
		if event.err != nil {
			t.Fatalf("realtime stream failed: %v", event.err)
		}
		if event.Topic != topic || event.Payload["marker"] != marker {
			t.Fatalf("received %s %+v; wanted %s %q (an unauthorized event may have leaked)", event.Topic, event.Payload, topic, marker)
		}
	case <-time.After(5 * time.Second):
		t.Fatalf("timed out waiting for %s %q", topic, marker)
	}
}

func realtimeWait(t *testing.T, signal <-chan struct{}, operation string) {
	t.Helper()
	select {
	case <-signal:
	case <-time.After(5 * time.Second):
		t.Fatalf("timed out waiting for %s", operation)
	}
}

func realtimeSetDownloadPermission(t *testing.T, svc *service.Container, userID string, allowed bool) {
	t.Helper()
	permissions := service.DefaultPermissions(userID)
	permissions.CanManageDownloads = allowed
	if err := svc.Permissions.Save(t.Context(), userID, permissions); err != nil {
		t.Fatal(err)
	}
}

// This handshake waits until the existing connection has read the revoked
// permission, then lets the test restore permission and send an allowed marker.
// Thus the next message assertion detects a leak without relying on a short
// "nothing arrived" timeout or opening a replacement connection.
func realtimeObserveRevokedPermission(t *testing.T, svc *service.Container, userID string) <-chan struct{} {
	t.Helper()
	checked := make(chan struct{}, 1)
	const callback = "test-realtime-revoked-permission"
	if err := svc.Repo.DB.Callback().Query().After("gorm:query").Register(callback, func(tx *gorm.DB) {
		permission, ok := tx.Statement.Dest.(*model.UserPermission)
		if ok && tx.Error == nil && permission.UserID == userID && !permission.CanManageDownloads {
			select {
			case checked <- struct{}{}:
			default:
			}
		}
	}); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = svc.Repo.DB.Callback().Query().Remove(callback) })
	return checked
}

func exerciseRealtimePermissionChanges(t *testing.T, svc *service.Container, user *model.User, events <-chan realtimeTestEvent, publish func(string, any)) {
	t.Helper()
	publish("task", map[string]string{"marker": "admin-task"})
	realtimeReadEvent(t, events, "task", "admin-task")

	// Keep the original admin JWT and connection; authorization must use the
	// freshly demoted database role, not the role cached at connection setup.
	realtimeSetDownloadPermission(t, svc, user.ID, true)
	if err := svc.Repo.DB.Model(user).Update("role", "user").Error; err != nil {
		t.Fatal(err)
	}
	for _, topic := range []string{"task", "scan", "scrape", "transcode", "system", "future-private-topic"} {
		publish(topic, map[string]string{"marker": "private-" + topic, "user_id": "another-user", "path": "/private/library/file.mkv"})
	}
	publish("download", map[string]string{"marker": "authorized-download"})
	realtimeReadEvent(t, events, "download", "authorized-download")

	realtimeSetDownloadPermission(t, svc, user.ID, false)
	checked := realtimeObserveRevokedPermission(t, svc, user.ID)
	publish("download", map[string]string{"marker": "revoked-download", "path": "/private/downloads"})
	realtimeWait(t, checked, "the active stream checking revoked download permission")
	realtimeSetDownloadPermission(t, svc, user.ID, true)
	publish("download", map[string]string{"marker": "restored-download"})
	realtimeReadEvent(t, events, "download", "restored-download")
}

func TestWebSocketRechecksRoleAndPermissionsOnExistingConnection(t *testing.T) {
	server, svc, user, token := realtimePermissionServer(t)
	header := http.Header{"Authorization": {"Bearer " + token}}
	conn, response, err := websocket.DefaultDialer.Dial("ws"+strings.TrimPrefix(server.URL, "http")+"/api/ws", header)
	if err != nil {
		t.Fatalf("websocket connect: %v (response=%v)", err, response)
	}
	t.Cleanup(func() { _ = conn.Close() })
	pong := make(chan struct{}, 1)
	conn.SetPongHandler(func(string) error {
		select {
		case pong <- struct{}{}:
		default:
		}
		return nil
	})
	events := make(chan realtimeTestEvent, 32)
	go func() {
		for {
			var event realtimeTestEvent
			if err := conn.ReadJSON(&event); err != nil {
				events <- realtimeTestEvent{err: err}
				return
			}
			events <- event
		}
	}()
	// The HTTP upgrade precedes hub.Subscribe. A pong proves the server's
	// reader has started after subscription, eliminating the first-event race.
	if err := conn.WriteControl(websocket.PingMessage, []byte("subscription-ready"), time.Now().Add(time.Second)); err != nil {
		t.Fatal(err)
	}
	realtimeWait(t, pong, "websocket subscription")
	exerciseRealtimePermissionChanges(t, svc, user, events, svc.WSHub.Publish)
}

func TestSSERechecksRoleAndPermissionsOnExistingConnection(t *testing.T) {
	server, svc, user, token := realtimePermissionServer(t)
	req, err := http.NewRequestWithContext(t.Context(), http.MethodGet, server.URL+"/api/events", nil)
	if err != nil {
		t.Fatal(err)
	}
	req.Header.Set("Authorization", "Bearer "+token)
	response, err := server.Client().Do(req)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = response.Body.Close() })
	if response.StatusCode != http.StatusOK || !strings.HasPrefix(response.Header.Get("Content-Type"), "text/event-stream") {
		t.Fatalf("SSE connect: status=%d content-type=%q", response.StatusCode, response.Header.Get("Content-Type"))
	}
	events := make(chan realtimeTestEvent, 32)
	go func() {
		scanner := bufio.NewScanner(response.Body)
		var event realtimeTestEvent
		for scanner.Scan() {
			line := scanner.Text()
			switch {
			case strings.HasPrefix(line, "event:"):
				event.Topic = strings.TrimSpace(strings.TrimPrefix(line, "event:"))
			case strings.HasPrefix(line, "data:"):
				if err := json.Unmarshal([]byte(strings.TrimSpace(strings.TrimPrefix(line, "data:"))), &event.Payload); err != nil {
					events <- realtimeTestEvent{err: err}
					return
				}
			case line == "" && event.Topic != "":
				events <- event
				event = realtimeTestEvent{}
			}
		}
		events <- realtimeTestEvent{err: fmt.Errorf("SSE stream ended: %v", scanner.Err())}
	}()
	realtimeReadEvent(t, events, "connected", "")
	exerciseRealtimePermissionChanges(t, svc, user, events, svc.SSEHub.Broadcast)
}

func TestRealtimeEventsDenyInactiveExpiredMissingAndDatabaseFailure(t *testing.T) {
	for _, state := range []string{"inactive", "expired", "missing", "database-error"} {
		t.Run(state, func(t *testing.T) {
			_, svc, user := currentRoleTestServices(t)
			switch state {
			case "inactive":
				if err := svc.Repo.DB.Model(user).Update("is_active", false).Error; err != nil {
					t.Fatal(err)
				}
			case "expired":
				if err := svc.Repo.DB.Model(user).Update("expired_at", time.Now().Add(-time.Minute)).Error; err != nil {
					t.Fatal(err)
				}
			case "missing":
				if err := svc.Repo.DB.Delete(user).Error; err != nil {
					t.Fatal(err)
				}
			case "database-error":
				sqlDB, err := svc.Repo.DB.DB()
				if err != nil {
					t.Fatal(err)
				}
				if err := sqlDB.Close(); err != nil {
					t.Fatal(err)
				}
			}
			c, _ := gin.CreateTestContext(httptest.NewRecorder())
			c.Request = httptest.NewRequest(http.MethodGet, "/api/ws", nil)
			c.Set(middleware.CtxUserID, user.ID)
			c.Set(middleware.CtxUserRole, "admin")
			for _, topic := range []string{"task", "scan", "download"} {
				if realtimeEventAllowed(c, svc, topic) {
					t.Errorf("%s user retained stale admin access to %s", state, topic)
				}
			}
		})
	}
}
