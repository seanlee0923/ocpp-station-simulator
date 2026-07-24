package auth

import (
	"crypto/hmac"
	"crypto/rand"
	"crypto/sha256"
	"crypto/subtle"
	"encoding/base64"
	"encoding/json"
	"errors"
	"strings"
	"time"
)

const SessionCookieName = "ocpp_session"
const sessionTTL = 12 * time.Hour

var ErrInvalidSession = errors.New("invalid or expired session")

// Claims is what a session cookie asserts about the request. Sessions are
// stateless — there is no server-side session table — so everything the
// rest of the app needs to know about the caller lives in this signed blob.
type Claims struct {
	Username string `json:"username"`
	IsAdmin  bool   `json:"isAdmin"`
	Exp      int64  `json:"exp"`
}

// Manager signs and verifies session cookies with a key generated fresh at
// process startup (see NewSecret). Restarting the server therefore
// invalidates every existing session — an intentional simplification: no
// session table to store, clean up, or migrate.
type Manager struct {
	secret []byte
}

func NewSecret() ([]byte, error) {
	secret := make([]byte, 32)
	if _, err := rand.Read(secret); err != nil {
		return nil, err
	}
	return secret, nil
}

func NewManager(secret []byte) *Manager { return &Manager{secret: secret} }

// Issue returns a signed cookie value good for sessionTTL and its expiry.
func (m *Manager) Issue(username string, isAdmin bool) (string, time.Time) {
	expires := time.Now().Add(sessionTTL)
	claims := Claims{Username: username, IsAdmin: isAdmin, Exp: expires.Unix()}
	payload, _ := json.Marshal(claims) // Claims is always marshalable; err is unreachable
	encodedPayload := base64.RawURLEncoding.EncodeToString(payload)
	return encodedPayload + "." + m.sign(encodedPayload), expires
}

func (m *Manager) Verify(cookieValue string) (Claims, error) {
	dot := strings.IndexByte(cookieValue, '.')
	if dot < 0 {
		return Claims{}, ErrInvalidSession
	}
	encodedPayload, signature := cookieValue[:dot], cookieValue[dot+1:]
	if subtle.ConstantTimeCompare([]byte(signature), []byte(m.sign(encodedPayload))) != 1 {
		return Claims{}, ErrInvalidSession
	}
	payload, err := base64.RawURLEncoding.DecodeString(encodedPayload)
	if err != nil {
		return Claims{}, ErrInvalidSession
	}
	var claims Claims
	if err := json.Unmarshal(payload, &claims); err != nil {
		return Claims{}, ErrInvalidSession
	}
	if time.Now().Unix() > claims.Exp {
		return Claims{}, ErrInvalidSession
	}
	return claims, nil
}

func (m *Manager) sign(encodedPayload string) string {
	mac := hmac.New(sha256.New, m.secret)
	mac.Write([]byte(encodedPayload))
	return base64.RawURLEncoding.EncodeToString(mac.Sum(nil))
}
