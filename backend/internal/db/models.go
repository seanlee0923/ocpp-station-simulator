// Package db holds the GORM models and connection setup shared by both the
// SQLite (default, zero-config) and MySQL (Config.DBDriver=mysql) backends.
package db

import (
	"time"

	"gorm.io/gorm"
)

// Station is a virtual charging station's configuration. It is the source of
// truth for what exists; internal/simulator's registry is the runtime
// reflection of whichever of these are currently connecting/connected.
type Station struct {
	ID            string `gorm:"type:varchar(64);primaryKey"`
	Identity      string `gorm:"type:varchar(255);index"`
	CSMSURL       string `gorm:"type:varchar(1024)"`
	Version       string `gorm:"type:varchar(16)"`
	BasicAuthUser string `gorm:"type:varchar(255)"`
	// BasicAuthPass is stored in plaintext — an accepted tradeoff for this
	// internal test tool (see plan): without it, the registry can't rebuild
	// a Basic-Auth-secured station's connection after a backend restart
	// (the in-memory registry is empty on every boot; only the DB row
	// survives), which is exactly the "connect button stops working after
	// a redeploy" bug this field fixes. Never returned by the station API —
	// see toStationResponse.
	BasicAuthPass         string `gorm:"type:varchar(255)"`
	ConnectorCount        int    `gorm:"default:1"`     // physical charge points commonly expose 2+ connectors sharing one identity/session
	HeartbeatInterval     int    `gorm:"default:0"`     // seconds; 0 disables automatic OCPP Heartbeat calls
	// PingInterval is the WebSocket-level ping period in seconds (0 = off),
	// a different layer from HeartbeatInterval. A CSMS may dictate it via
	// the OCPP 1.6-J WebSocketPingInterval configuration key, in which case
	// the value is persisted here and applied on the next connect rather
	// than to the live connection: rebuilding the station mid-session would
	// reconnect, prompting the CSMS to send the key again, looping forever.
	PingInterval          int    `gorm:"default:0"`
	InsecureSkipTLSVerify bool   `gorm:"default:false"` // wss:// only; test CSMS with a self-signed/internal CA cert
	CreatedBy             string `gorm:"type:varchar(255)"`
	LastKnownStatus       string `gorm:"type:varchar(32)"`
	CreatedAt             time.Time
	UpdatedAt             time.Time
	// DeletedAt must be exactly gorm.DeletedAt (not *time.Time) — that's the
	// specific type GORM recognizes for soft delete. With a plain *time.Time
	// here, Delete() silently performed a real DELETE instead, which is
	// exactly the bug this fixes: a deleted station's row (and the ability
	// to look up its still-intact StationEvent history) was gone for good.
	DeletedAt gorm.DeletedAt `gorm:"index"`
}

// User is an account an admin created. There is no self-signup: accounts
// only ever come from the env-var-bootstrapped admin or from an admin using
// the admin page. PasswordHash is a bcrypt hash, never the plaintext.
type User struct {
	ID           uint64 `gorm:"primaryKey;autoIncrement"`
	Username     string `gorm:"type:varchar(255);uniqueIndex"`
	PasswordHash string `gorm:"type:varchar(255)"`
	IsAdmin      bool   `gorm:"default:false"`
	CreatedBy    string `gorm:"type:varchar(255)"`
	CreatedAt    time.Time
}

// StationAccess grants a non-admin user access to one station. Keyed by
// Username (a plain string), not User.ID — matching how CreatedBy/Actor are
// already handled everywhere else in this codebase (Station.CreatedBy,
// StationEvent.Actor), so this doesn't need a join to the users table.
// Admins bypass this table entirely (see requireStationAccess).
type StationAccess struct {
	ID        uint64 `gorm:"primaryKey;autoIncrement"`
	StationID string `gorm:"type:varchar(64);uniqueIndex:idx_station_user"`
	Username  string `gorm:"type:varchar(255);uniqueIndex:idx_station_user"`
	GrantedBy string `gorm:"type:varchar(255)"`
	CreatedAt time.Time
}

// DataTransferHandler is an operator-registered canned response for an
// inbound DataTransfer.req matching VendorID (and, if set, MessageID — an
// empty MessageID matches any messageId for that vendorId). It exists
// because DataTransfer's payload shape is entirely vendor-defined, so there
// is no schema-driven way to know in advance what a CSMS will send or what
// a real device would answer with — the operator has to tell the simulator.
type DataTransferHandler struct {
	ID             uint64 `gorm:"primaryKey;autoIncrement"`
	StationID      string `gorm:"type:varchar(64);index"`
	VendorID       string `gorm:"type:varchar(255)"`
	MessageID      string `gorm:"type:varchar(255)"`
	ResponseStatus string `gorm:"type:varchar(32)"` // Accepted | Rejected | UnknownMessageId | UnknownVendorId
	ResponseData   string `gorm:"type:text"`
	CreatedBy      string `gorm:"type:varchar(255)"`
	CreatedAt      time.Time
}

// StationEvent is both the audit trail ("who created/connected/disconnected
// this station") and the OCPP message log ("what frame went out/came in") —
// every observable thing that happens to a Station is one of these rows, so
// the frontend's history panel and message log panels share one data source.
type StationEvent struct {
	ID        uint64    `gorm:"primaryKey;autoIncrement"`
	StationID string    `gorm:"type:varchar(64);index"`
	Actor     string    `gorm:"type:varchar(255)"`
	EventType string    `gorm:"type:varchar(32);index"`
	Action    string    `gorm:"type:varchar(64)"`
	Direction string    `gorm:"type:varchar(16)"`
	Payload   string    `gorm:"type:text"`
	CreatedAt time.Time `gorm:"index"`
}
