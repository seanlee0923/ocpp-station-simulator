// Package auth provides password hashing, stateless signed-cookie sessions,
// and the env-var admin bootstrap for ocpp-station-simulator. There is no
// self-signup: every account either comes from the admin bootstrap (see
// bootstrap.go) or was created by an admin via the admin API.
package auth

import "golang.org/x/crypto/bcrypt"

func HashPassword(plain string) (string, error) {
	hash, err := bcrypt.GenerateFromPassword([]byte(plain), bcrypt.DefaultCost)
	return string(hash), err
}

func VerifyPassword(hash, plain string) bool {
	return bcrypt.CompareHashAndPassword([]byte(hash), []byte(plain)) == nil
}
