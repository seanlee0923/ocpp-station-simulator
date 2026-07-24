package auth

import (
	"crypto/rand"
	"encoding/base64"
	"errors"
	"fmt"
	"log"

	"gorm.io/gorm"

	"ocpp-station-simulator/backend/internal/db"
)

// Bootstrap ensures an admin account exists before the server starts
// accepting requests.
//
//   - If adminUser/adminPass are both set, that account is upserted as
//     admin on every boot — the env vars are the source of truth, so
//     rotating the admin password is just "change the env var and restart".
//   - Otherwise, if no admin account exists yet, one is created with a
//     random password logged once. This keeps the "just run it, no config
//     needed" experience: the operator can log in immediately using the
//     printed credential, then set ADMIN_USER/ADMIN_PASS for anything
//     beyond local, throwaway use.
func Bootstrap(database *gorm.DB, adminUser, adminPass string) error {
	if adminUser != "" && adminPass != "" {
		return upsertAdmin(database, adminUser, adminPass)
	}

	var adminCount int64
	if err := database.Model(&db.User{}).Where("is_admin = ?", true).Count(&adminCount).Error; err != nil {
		return err
	}
	if adminCount > 0 {
		return nil
	}

	password, err := randomPassword()
	if err != nil {
		return fmt.Errorf("generate admin password: %w", err)
	}
	if err := upsertAdmin(database, "admin", password); err != nil {
		return err
	}
	log.Printf("================================================================")
	log.Printf(" no ADMIN_USER/ADMIN_PASS set — generated a one-time admin account:")
	log.Printf("   username: admin")
	log.Printf("   password: %s", password)
	log.Printf(" set ADMIN_USER/ADMIN_PASS env vars to control this instead.")
	log.Printf("================================================================")
	return nil
}

func upsertAdmin(database *gorm.DB, username, password string) error {
	hash, err := HashPassword(password)
	if err != nil {
		return fmt.Errorf("hash admin password: %w", err)
	}
	var existing db.User
	err = database.Where("username = ?", username).First(&existing).Error
	switch {
	case errors.Is(err, gorm.ErrRecordNotFound):
		return database.Create(&db.User{Username: username, PasswordHash: hash, IsAdmin: true, CreatedBy: "bootstrap"}).Error
	case err != nil:
		return err
	default:
		existing.PasswordHash = hash
		existing.IsAdmin = true
		return database.Save(&existing).Error
	}
}

func randomPassword() (string, error) {
	raw := make([]byte, 18)
	if _, err := rand.Read(raw); err != nil {
		return "", err
	}
	return base64.RawURLEncoding.EncodeToString(raw), nil
}
