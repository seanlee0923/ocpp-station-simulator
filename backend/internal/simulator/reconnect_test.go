package simulator

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/gorilla/websocket"
	"github.com/seanlee0923/ocpp/station"
)

func TestAutomaticHeartbeat(t *testing.T) {
	heartbeats := make(chan struct{}, 1)
	upgrader := websocket.Upgrader{Subprotocols: []string{"ocpp1.6"}}
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		conn, err := upgrader.Upgrade(w, r, nil)
		if err != nil {
			return
		}
		defer conn.Close() //nolint:errcheck // test CSMS
		for {
			_, data, err := conn.ReadMessage()
			if err != nil {
				return
			}
			var call []json.RawMessage
			if json.Unmarshal(data, &call) != nil || len(call) != 4 {
				continue
			}
			var id, action string
			if json.Unmarshal(call[1], &id) != nil || json.Unmarshal(call[2], &action) != nil || action != "Heartbeat" {
				continue
			}
			heartbeats <- struct{}{}
			response, _ := json.Marshal([]any{3, id, map[string]string{"currentTime": time.Now().UTC().Format(time.RFC3339)}})
			if conn.WriteMessage(websocket.TextMessage, response) != nil {
				return
			}
		}
	}))
	defer server.Close()

	registry := NewRegistry(nil)
	cfg := StationConfig{
		Identity: "CP-HB", CSMSURL: "ws" + strings.TrimPrefix(server.URL, "http"),
		Version: "1.6", HeartbeatInterval: 1,
	}
	if _, err := registry.Create("heartbeat", cfg); err != nil {
		t.Fatalf("Create: %v", err)
	}
	defer registry.Delete("heartbeat") //nolint:errcheck // test cleanup

	select {
	case <-heartbeats:
	case <-time.After(3 * time.Second):
		t.Fatal("CSMS did not receive an automatic Heartbeat")
	}
}

// TestReconnectAfterDisconnect guards the fix for a reconnect that silently
// never dialed: station.Stop() is permanent, so reusing the same
// station.Station after Disconnect made Run return ErrStopped immediately
// and the CSMS never saw a second upgrade request.
func TestReconnectAfterDisconnect(t *testing.T) {
	sessions := make(chan struct{}, 4)
	upgrader := websocket.Upgrader{Subprotocols: []string{"ocpp1.6"}}
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		conn, err := upgrader.Upgrade(w, r, nil)
		if err != nil {
			return
		}
		defer conn.Close() //nolint:errcheck // test CSMS
		sessions <- struct{}{}
		for {
			if _, _, err := conn.ReadMessage(); err != nil {
				return
			}
		}
	}))
	defer server.Close()

	registry := NewRegistry(nil)
	cfg := StationConfig{Identity: "CP1", CSMSURL: "ws" + strings.TrimPrefix(server.URL, "http"), Version: "1.6"}
	managed, err := registry.Create("s1", cfg)
	if err != nil {
		t.Fatalf("Create: %v", err)
	}
	awaitSession(t, sessions, "initial connect")
	awaitState(t, managed.Sim, station.Connected, "initial connect")

	if err := registry.Disconnect("s1"); err != nil {
		t.Fatalf("Disconnect: %v", err)
	}
	awaitState(t, managed.Sim, station.Disconnected, "disconnect")

	if err := registry.Reconnect("s1"); err != nil {
		t.Fatalf("Reconnect: %v", err)
	}
	awaitSession(t, sessions, "reconnect")
	awaitState(t, managed.Sim, station.Connected, "reconnect")
}

func awaitSession(t *testing.T, sessions <-chan struct{}, stage string) {
	t.Helper()
	select {
	case <-sessions:
	case <-time.After(5 * time.Second):
		t.Fatalf("%s: CSMS never received a WebSocket upgrade", stage)
	}
}

func awaitState(t *testing.T, sim Simulator, want station.ConnectionState, stage string) {
	t.Helper()
	deadline := time.Now().Add(5 * time.Second)
	for time.Now().Before(deadline) {
		if sim.State() == want {
			return
		}
		time.Sleep(10 * time.Millisecond)
	}
	t.Fatalf("%s: state = %v, want %v", stage, sim.State(), want)
}
