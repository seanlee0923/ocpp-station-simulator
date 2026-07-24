package api

import (
	"encoding/json"
	"log"

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

	payload, err := json.Marshal(wsEvent{
		StationID: stationID, Type: string(event.Type), Action: event.Action,
		Direction: event.Direction, Actor: actor, Payload: rawOrNull(event.Payload), Timestamp: event.Timestamp,
	})
	if err == nil {
		app.Hub.Broadcast(stationID, payload)
	}
}
