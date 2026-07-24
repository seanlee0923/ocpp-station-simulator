// Package db holds the GORM models and connection setup shared by both the
// SQLite (default, zero-config) and MySQL (Config.DBDriver=mysql) backends.
package db

import "time"

// Station is a virtual charging station's configuration. It is the source of
// truth for what exists; internal/simulator's registry is the runtime
// reflection of whichever of these are currently connecting/connected.
type Station struct {
	ID                    string `gorm:"type:varchar(64);primaryKey"`
	Identity              string `gorm:"type:varchar(255);index"`
	CSMSURL               string `gorm:"type:varchar(1024)"`
	Version               string `gorm:"type:varchar(16)"`
	BasicAuthUser         string `gorm:"type:varchar(255)"`
	ConnectorCount        int    `gorm:"default:1"`     // physical charge points commonly expose 2+ connectors sharing one identity/session
	InsecureSkipTLSVerify bool   `gorm:"default:false"` // wss:// only; test CSMS with a self-signed/internal CA cert
	CreatedBy             string `gorm:"type:varchar(255)"`
	LastKnownStatus       string `gorm:"type:varchar(32)"`
	CreatedAt             time.Time
	UpdatedAt             time.Time
	DeletedAt             *time.Time `gorm:"index"`
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
