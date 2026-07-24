package api

import (
	"net/http"
	"sync"

	"github.com/gin-gonic/gin"

	"ocpp-station-simulator/backend/internal/auth"
)

const actorContextKey = "actor"
const isAdminContextKey = "isAdmin"

// requireAuth verifies the session cookie (see internal/auth/session.go)
// and rejects the request with 401 if it's missing or invalid. On success,
// it stores the authenticated username under actorContextKey — every place
// downstream that used to read a client-asserted X-Actor header now reads
// this instead, so it can no longer be spoofed by just setting a header.
func (app *App) requireAuth(c *gin.Context) {
	cookie, err := c.Cookie(auth.SessionCookieName)
	if err != nil {
		c.AbortWithStatusJSON(http.StatusUnauthorized, gin.H{"error": "not authenticated"})
		return
	}
	claims, err := app.Sessions.Verify(cookie)
	if err != nil {
		c.AbortWithStatusJSON(http.StatusUnauthorized, gin.H{"error": "not authenticated"})
		return
	}
	c.Set(actorContextKey, claims.Username)
	c.Set(isAdminContextKey, claims.IsAdmin)
	c.Next()
}

// requireAdmin must run after requireAuth (see router.go route groups).
func (app *App) requireAdmin(c *gin.Context) {
	isAdmin, _ := c.Get(isAdminContextKey)
	if admin, ok := isAdmin.(bool); !ok || !admin {
		c.AbortWithStatusJSON(http.StatusForbidden, gin.H{"error": "admin access required"})
		return
	}
	c.Next()
}

func actorFrom(c *gin.Context) string {
	value, _ := c.Get(actorContextKey)
	actor, _ := value.(string)
	if actor == "" {
		return "unknown"
	}
	return actor
}

func isAdminFrom(c *gin.Context) bool {
	value, _ := c.Get(isAdminContextKey)
	isAdmin, _ := value.(bool)
	return isAdmin
}

// actorTracker lets the async simulator-event writer (which only ever sees
// a stationID, not an HTTP request) attribute a message_sent/message_received
// row to whoever most recently issued a mutating call against that station.
// This is a best-effort correlation, not a strict guarantee: two operators
// racing to act on the same virtual station at the same instant could see a
// message briefly attributed to the other. Acceptable for an internal test
// tool with light concurrent use per station; a stronger guarantee would
// require threading actor through every simulator.Simulator method.
type actorTracker struct {
	mu   sync.RWMutex
	byID map[string]string
}

func newActorTracker() *actorTracker { return &actorTracker{byID: make(map[string]string)} }

func (t *actorTracker) set(stationID, actor string) {
	t.mu.Lock()
	t.byID[stationID] = actor
	t.mu.Unlock()
}

func (t *actorTracker) get(stationID string) string {
	t.mu.RLock()
	defer t.mu.RUnlock()
	if actor, ok := t.byID[stationID]; ok {
		return actor
	}
	return "system"
}

func (t *actorTracker) delete(stationID string) {
	t.mu.Lock()
	delete(t.byID, stationID)
	t.mu.Unlock()
}
