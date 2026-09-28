// Package handler — WebSocket endpoint.
//
// Clients connect to /api/ws?token=... (the token is the same JWT used for
// REST calls). The first message they send is a JSON {"action":"subscribe",
// "topics":["scan","scrape","transcode"]}. Subsequent server-pushed events
// arrive as {"topic":"...","payload":{...}}.
package handler

import (
	"encoding/json"
	"net/http"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/google/uuid"
	"github.com/gorilla/websocket"

	"github.com/ShukeBta/MediaStationGo/internal/service"
)

var wsUpgrader = websocket.Upgrader{
	ReadBufferSize:  1024,
	WriteBufferSize: 1024,
	// Allow any origin: the AuthRequired middleware already validated the
	// JWT before we got here, and we never serve sensitive cross-domain
	// state through the socket.
	CheckOrigin: func(_ *http.Request) bool { return true },
}

func wsHandler(svc *service.Container) gin.HandlerFunc {
	return func(c *gin.Context) {
		conn, err := wsUpgrader.Upgrade(c.Writer, c.Request, nil)
		if err != nil {
			return
		}
		defer conn.Close()

		id := uuid.NewString()
		sub := svc.WSHub.Subscribe(id, nil)
		defer svc.WSHub.Unsubscribe(id)

		// Reader: accept ping + subscription updates.
		go func() {
			for {
				if _, _, err := conn.NextReader(); err != nil {
					_ = conn.Close()
					return
				}
			}
		}()

		// Writer: drain hub events into the socket with a periodic ping.
		ticker := time.NewTicker(30 * time.Second)
		defer ticker.Stop()
		for {
			select {
			case ev, ok := <-sub.Out:
				if !ok {
					return
				}
				if !realtimeEventAllowed(c, svc, ev.Topic) {
					continue
				}
				data, _ := json.Marshal(ev)
				_ = conn.SetWriteDeadline(time.Now().Add(10 * time.Second))
				if err := conn.WriteMessage(websocket.TextMessage, data); err != nil {
					return
				}
			case <-ticker.C:
				_ = conn.SetWriteDeadline(time.Now().Add(10 * time.Second))
				if err := conn.WriteMessage(websocket.PingMessage, nil); err != nil {
					return
				}
			}
		}
	}
}
