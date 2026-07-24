package api

import (
	"net/http"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/gorilla/websocket"
)

var upgrader = websocket.Upgrader{
	// Internal tool served same-origin by this same binary; no cross-origin
	// browser client is expected. Kept permissive rather than wiring a
	// CheckOrigin allowlist that would need updating per deployment.
	CheckOrigin: func(*http.Request) bool { return true },
}

const wsPingInterval = 30 * time.Second

// streamEvents upgrades to a WebSocket and relays every Hub broadcast for
// this station until the client disconnects. It never reads application
// messages from the client — this is a one-way push feed — but still runs a
// read pump to notice the client going away and to answer control pings.
func (app *App) streamEvents(c *gin.Context) {
	stationID := c.Param("id")
	conn, err := upgrader.Upgrade(c.Writer, c.Request, nil)
	if err != nil {
		return
	}
	defer conn.Close()

	events := app.Hub.Subscribe(stationID)
	defer app.Hub.Unsubscribe(stationID, events)

	closed := make(chan struct{})
	go func() {
		defer close(closed)
		for {
			if _, _, err := conn.ReadMessage(); err != nil {
				return
			}
		}
	}()

	ticker := time.NewTicker(wsPingInterval)
	defer ticker.Stop()
	for {
		select {
		case payload := <-events:
			if err := conn.WriteMessage(websocket.TextMessage, payload); err != nil {
				return
			}
		case <-ticker.C:
			if err := conn.WriteControl(websocket.PingMessage, nil, time.Now().Add(5*time.Second)); err != nil {
				return
			}
		case <-closed:
			return
		}
	}
}
