// Package simulator wraps github.com/seanlee0923/ocpp's station.Station with
// a version-agnostic interface, so the REST/WS layer never has to know
// whether it's driving an OCPP 1.6, 2.0.1, or 2.1 virtual charge point.
package simulator

import (
	"context"
	"crypto/tls"
	"encoding/json"
	"time"

	"github.com/seanlee0923/ocpp/station"
)

// StationConfig is what the operator supplies when creating a virtual
// charge point, independent of OCPP version.
type StationConfig struct {
	Identity      string
	CSMSURL       string
	Version       string // "1.6", "2.0.1", or "2.1"
	BasicAuthUser string
	BasicAuthPass string
	// InsecureSkipTLSVerify disables server certificate verification on a
	// wss:// connection. Default false (verified TLS) — this exists only
	// for pointing a wss:// test CSMS with a self-signed/internal-CA
	// certificate during local testing; never enable it against a
	// production CSMS.
	InsecureSkipTLSVerify bool
	// HeartbeatInterval is the automatic OCPP Heartbeat period in seconds.
	// Zero disables the loop.
	HeartbeatInterval int
}

// EventType is the kind of thing that just happened to a Simulator. It
// doubles as the value stored in db.StationEvent.EventType.
type EventType string

const (
	EventConnected           EventType = "connected"
	EventDisconnected        EventType = "disconnected"
	EventMessageSent         EventType = "message_sent"
	EventMessageReceived     EventType = "message_received"
	EventRemoteCommandCalled EventType = "remote_command_received"
)

// Event is everything observable a Simulator produces: connection
// transitions, every OCPP frame sent/received, and CSMS-initiated remote
// commands. It is the single feed that backs both the live WebSocket push
// and (via an async writer elsewhere) the persisted message/audit log.
type Event struct {
	Type      EventType
	Action    string
	Direction string // "sent" | "received" | "error" | ""
	Payload   string // always valid JSON, or empty — never a bare error string
	Timestamp time.Time
}

// eventBus is embedded by each version adapter so Connect/Disconnect/Call
// wiring can emit without every adapter reimplementing channel plumbing.
// The channel is generously buffered and emit never blocks: a slow consumer
// must not be able to stall the OCPP read/write loop it's observing.
type eventBus struct {
	events chan Event
}

const eventBufferSize = 256

func newEventBus() eventBus {
	return eventBus{events: make(chan Event, eventBufferSize)}
}

func (b *eventBus) Events() <-chan Event { return b.events }

func (b *eventBus) emit(event Event) {
	if event.Timestamp.IsZero() {
		event.Timestamp = time.Now()
	}
	select {
	case b.events <- event:
	default:
		// Consumer (WS hub / DB writer) is behind; drop rather than block
		// the OCPP connection that produced this event.
	}
}

func (b *eventBus) emitMessage(eventType EventType, action, direction string, payload any) {
	raw, err := json.Marshal(payload)
	if err != nil {
		raw = []byte(`{}`)
	}
	b.emit(Event{Type: eventType, Action: action, Direction: direction, Payload: string(raw)})
}

func (b *eventBus) emitRemoteCommand(action string, payload any) {
	b.emitMessage(EventRemoteCommandCalled, action, "received", payload)
}

// Simulator is the version-agnostic surface the API layer drives. Each
// method mirrors one OCPP action; StartTransaction/StopTransaction hide the
// version-specific detail that OCPP 1.6 has the CSMS allocate the
// transaction ID while 2.0.1/2.1 have the charging station allocate it —
// callers always just get a TransactionID string back from StartTransaction.
type Simulator interface {
	Connect(ctx context.Context) error
	Disconnect()
	State() station.ConnectionState

	SendBootNotification(ctx context.Context, fields BootFields) (BootResult, error)
	SendAuthorize(ctx context.Context, idTag string) (AuthorizeResult, error)
	SendHeartbeat(ctx context.Context) error
	StartTransaction(ctx context.Context, req StartTxRequest) (StartTxResult, error)
	SendMeterValues(ctx context.Context, req MeterValuesRequest) error
	StopTransaction(ctx context.Context, req StopTxRequest) error
	SendStatusNotification(ctx context.Context, req StatusRequest) error
	// SendFirmwareStatusNotification and SendDiagnosticsStatusNotification
	// are the operator-driven follow-up to a CSMS-initiated UpdateFirmware /
	// GetDiagnostics (1.6) or GetLog (2.0.1/2.1) request — see the
	// handleUpdateFirmware/handleGetDiagnostics/handleGetLog handlers in
	// each version adapter, which auto-accept the request itself and just
	// emit an event; the operator then picks whichever status to report
	// next from the UI, same "no automatic progression" philosophy as
	// MeterValues.
	SendFirmwareStatusNotification(ctx context.Context, status string) error
	SendDiagnosticsStatusNotification(ctx context.Context, status string) error

	// RegisterDataTransferResponse/UnregisterDataTransferResponse manage the
	// canned responses the generic inbound DataTransfer handler consults
	// (see dataTransferMatcher). SendDataTransfer is the outbound direction
	// — the simulator initiating a DataTransfer to the CSMS.
	RegisterDataTransferResponse(vendorID, messageID, status, data string)
	UnregisterDataTransferResponse(vendorID, messageID string)
	SendDataTransfer(ctx context.Context, vendorID, messageID, data string) (DataTransferResult, error)

	// GetConfigValues snapshots every key/value the CSMS has set so far via
	// ChangeConfiguration (1.6) or SetVariables (2.0.1/2.1) — see
	// configStore. Purely CSMS-driven; there is no operator-facing setter.
	GetConfigValues() map[string]string

	Events() <-chan Event
}

type BootFields struct {
	VendorName      string `json:"vendorName"`
	Model           string `json:"model"`
	SerialNumber    string `json:"serialNumber"`
	FirmwareVersion string `json:"firmwareVersion"`
}

type BootResult struct {
	Status      string `json:"status"`
	CurrentTime string `json:"currentTime"`
	Interval    int    `json:"interval"`
}

type AuthorizeResult struct {
	Status string `json:"status"`
}

type StartTxRequest struct {
	ConnectorID int    `json:"connectorId"` // OCPP 1.6
	EVSEID      int    `json:"evseId"`      // OCPP 2.0.1 / 2.1
	IDTag       string `json:"idTag"`
	MeterStart  int    `json:"meterStart"` // Wh; OCPP 1.6 only
	Timestamp   string `json:"timestamp"`
}

type StartTxResult struct {
	TransactionID string `json:"transactionId"`
}

type StopTxRequest struct {
	TransactionID string `json:"transactionId"`
	MeterStop     int    `json:"meterStop"` // Wh; OCPP 1.6 only
	Reason        string `json:"reason"`
	Timestamp     string `json:"timestamp"`
}

type MeterValuesRequest struct {
	TransactionID string        `json:"transactionId"` // optional; OCPP 1.6 only
	ConnectorID   int           `json:"connectorId"`   // OCPP 1.6
	EVSEID        int           `json:"evseId"`        // OCPP 2.0.1 / 2.1
	Samples       []MeterSample `json:"samples"`
	Timestamp     string        `json:"timestamp"`
}

type MeterSample struct {
	Measurand string `json:"measurand"` // e.g. "Energy.Active.Import.Register", "Power.Active.Import", "SoC"
	Value     string `json:"value"`     // decimal value as text; adapters parse to float64 where the version needs it
	Unit      string `json:"unit,omitempty"`
}

type StatusRequest struct {
	ConnectorID int    `json:"connectorId"` // OCPP 1.6 and 2.0.1/2.1 connector
	EVSEID      int    `json:"evseId"`      // OCPP 2.0.1 / 2.1 only
	Status      string `json:"status"`
	ErrorCode   string `json:"errorCode,omitempty"` // OCPP 1.6 only
	Info        string `json:"info,omitempty"`      // OCPP 1.6 only
	Timestamp   string `json:"timestamp"`
}

// requestTimestamp uses the caller-supplied time when present and otherwise
// defaults to the send time. OCPP timestamps are always emitted in UTC.
func requestTimestamp(timestamp string) string {
	if timestamp == "" {
		return time.Now().UTC().Format(time.RFC3339)
	}
	return timestamp
}

// New constructs the Simulator implementation matching cfg.Version.
func New(cfg StationConfig) (Simulator, error) {
	switch cfg.Version {
	case "1.6":
		return newV16Simulator(cfg)
	case "2.0.1":
		return newV201Simulator(cfg)
	case "2.1":
		return newV21Simulator(cfg)
	default:
		return nil, unsupportedVersionError(cfg.Version)
	}
}

func basicAuth(cfg StationConfig) *station.BasicCredentials {
	if cfg.BasicAuthUser == "" {
		return nil
	}
	return &station.BasicCredentials{Username: cfg.BasicAuthUser, Password: cfg.BasicAuthPass}
}

// tlsConfig returns nil (standard verified TLS, or plain ws:// if the URL
// isn't wss://) unless the operator explicitly opted out of verification
// for a test CSMS.
func tlsConfig(cfg StationConfig) *tls.Config {
	if !cfg.InsecureSkipTLSVerify {
		return nil
	}
	return &tls.Config{InsecureSkipVerify: true} //nolint:gosec // explicit opt-in, test CSMS only — see StationConfig doc comment
}
