package api

import (
	"encoding/json"
	"log"
	"strconv"

	"gorm.io/gorm"

	"ocpp-station-simulator/backend/internal/auth"
	"ocpp-station-simulator/backend/internal/db"
	"ocpp-station-simulator/backend/internal/simulator"
)

// App wires the DB, the runtime station registry, the WebSocket hub, the
// async DB event writer, and session verification together. It is the one
// place that knows about all of them; every other file in this package only
// needs the pieces it uses.
type App struct {
	DB       *gorm.DB
	Registry *simulator.Registry
	Hub      *Hub
	Writer   *eventWriter
	Actors   *actorTracker
	Sessions *auth.Manager
}

func NewApp(database *gorm.DB) *App {
	secret, err := auth.NewSecret()
	if err != nil {
		// crypto/rand failing means the OS entropy source is broken —
		// nothing downstream (sessions, later any token) can be trusted.
		log.Fatalf("generate session signing secret: %v", err)
	}
	app := &App{
		DB: database, Hub: NewHub(), Writer: newEventWriter(database),
		Actors: newActorTracker(), Sessions: auth.NewManager(secret),
	}
	app.Registry = simulator.NewRegistry(app.onSimulatorEvent)
	return app
}

func (app *App) Close() { app.Writer.Close() }

// onSimulatorEvent is the single fan-out point for everything a running
// Simulator produces: persist it (async) and broadcast it to any browser
// currently watching this station. Connection state changes additionally
// update Station.LastKnownStatus so the station list reflects it without
// needing a live registry lookup.
func (app *App) onSimulatorEvent(stationID string, event simulator.Event) {
	actor := "system"
	if event.Type == simulator.EventMessageSent || event.Type == simulator.EventMessageReceived {
		actor = app.Actors.get(stationID)
	}

	app.Writer.enqueue(db.StationEvent{
		StationID: stationID, Actor: actor, EventType: string(event.Type),
		Action: event.Action, Direction: event.Direction, Payload: event.Payload, CreatedAt: event.Timestamp,
	})

	if event.Type == simulator.EventConnected || event.Type == simulator.EventDisconnected {
		app.DB.Model(&db.Station{}).Where("id = ?", stationID).Update("last_known_status", string(event.Type))
	}
	if event.Type == simulator.EventRemoteCommandCalled && event.Action == "ChangeConfiguration" {
		app.persistPingInterval(stationID, event.Payload)
	}

	payload, err := json.Marshal(wsEvent{
		StationID: stationID, Type: string(event.Type), Action: event.Action,
		Direction: event.Direction, Actor: actor, Payload: rawOrNull(event.Payload), Timestamp: event.Timestamp,
	})
	if err == nil {
		app.Hub.Broadcast(stationID, payload)
	}
}

// persistPingInterval records a WebSocketPingInterval the CSMS set via
// ChangeConfiguration so the next connect honours it. It deliberately does
// not touch the running station: station.Config is immutable once built, so
// applying the value now would mean a rebuild-and-reconnect, and the CSMS
// re-sends the key on every fresh connection — an endless reconnect loop.
// Persisting instead makes the CSMS's wish take effect one session later.
func (app *App) persistPingInterval(stationID, payload string) {
	var body struct {
		Key   string `json:"key"`
		Value string `json:"value"`
	}
	if json.Unmarshal([]byte(payload), &body) != nil || body.Key != "WebSocketPingInterval" {
		return
	}
	// Negative values are invalid per OCPP 1.6-J; 0 legitimately means the
	// CSMS is turning client-side pings off, so it is stored like any other.
	seconds, err := strconv.Atoi(body.Value)
	if err != nil || seconds < 0 {
		return
	}
	if err := app.DB.Model(&db.Station{}).Where("id = ?", stationID).Update("ping_interval", seconds).Error; err != nil {
		log.Printf("persist WebSocketPingInterval for station %s: %v", stationID, err)
	}
}
