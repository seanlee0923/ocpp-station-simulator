package api

import (
	"encoding/json"
	"time"

	"ocpp-station-simulator/backend/internal/db"
	"ocpp-station-simulator/backend/internal/simulator"
)

type createStationRequest struct {
	Identity              string `json:"identity" binding:"required"`
	CSMSURL               string `json:"csmsUrl" binding:"required"`
	Version               string `json:"version" binding:"required"` // "1.6" | "2.0.1" | "2.1"
	ConnectorCount        int    `json:"connectorCount"`
	BasicAuthUser         string `json:"basicAuthUser"`
	BasicAuthPass         string `json:"basicAuthPass"`
	InsecureSkipTLSVerify bool   `json:"insecureSkipTlsVerify"`
}

type stationResponse struct {
	ID                    string    `json:"id"`
	Identity              string    `json:"identity"`
	CSMSURL               string    `json:"csmsUrl"`
	Version               string    `json:"version"`
	ConnectorCount        int       `json:"connectorCount"`
	BasicAuthUser         string    `json:"basicAuthUser,omitempty"`
	InsecureSkipTLSVerify bool      `json:"insecureSkipTlsVerify"`
	CreatedBy             string    `json:"createdBy"`
	CreatedAt             time.Time `json:"createdAt"`
	LastKnownStatus       string    `json:"lastKnownStatus"`
	State                 string    `json:"state"` // live station.ConnectionState, "unknown" if not currently running
}

func toStationResponse(row db.Station, state string) stationResponse {
	return stationResponse{
		ID: row.ID, Identity: row.Identity, CSMSURL: row.CSMSURL, Version: row.Version,
		ConnectorCount: row.ConnectorCount, BasicAuthUser: row.BasicAuthUser,
		InsecureSkipTLSVerify: row.InsecureSkipTLSVerify, CreatedBy: row.CreatedBy,
		CreatedAt: row.CreatedAt, LastKnownStatus: row.LastKnownStatus, State: state,
	}
}

type wsEvent struct {
	StationID string          `json:"stationId"`
	Type      string          `json:"type"`
	Action    string          `json:"action,omitempty"`
	Direction string          `json:"direction,omitempty"`
	Actor     string          `json:"actor,omitempty"`
	Payload   json.RawMessage `json:"payload,omitempty"`
	Timestamp time.Time       `json:"timestamp"`
}

// rawOrNull passes payload through as embedded JSON. It validates first:
// simulator.Event.Payload is always produced as valid JSON, but this also
// reads back rows written to the DB before that guarantee was enforced —
// treating an invalid string as raw JSON here would corrupt the entire
// response's encoding, not just this one field.
func rawOrNull(payload string) json.RawMessage {
	if payload == "" {
		return json.RawMessage("null")
	}
	if !json.Valid([]byte(payload)) {
		encoded, err := json.Marshal(payload)
		if err != nil {
			return json.RawMessage("null")
		}
		return json.RawMessage(encoded)
	}
	return json.RawMessage(payload)
}

type eventResponse struct {
	ID        uint64          `json:"id"`
	Actor     string          `json:"actor"`
	EventType string          `json:"eventType"`
	Action    string          `json:"action,omitempty"`
	Direction string          `json:"direction,omitempty"`
	Payload   json.RawMessage `json:"payload,omitempty"`
	CreatedAt time.Time       `json:"createdAt"`
}

func toEventResponse(row db.StationEvent) eventResponse {
	return eventResponse{
		ID: row.ID, Actor: row.Actor, EventType: row.EventType, Action: row.Action,
		Direction: row.Direction, Payload: rawOrNull(row.Payload), CreatedAt: row.CreatedAt,
	}
}

type bootRequest = simulator.BootFields
type authorizeRequest struct {
	IDTag string `json:"idTag" binding:"required"`
}
type startTxRequest = simulator.StartTxRequest
type stopTxRequest struct {
	MeterStop int    `json:"meterStop"`
	Reason    string `json:"reason"`
}
type meterValuesRequest = simulator.MeterValuesRequest
type statusRequest = simulator.StatusRequest
