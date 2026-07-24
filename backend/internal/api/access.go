package api

import (
	"encoding/json"
	"errors"
	"net/http"
	"time"

	"github.com/gin-gonic/gin"
	"gorm.io/gorm"

	"ocpp-station-simulator/backend/internal/db"
)

// accessEventPayload encodes the affected username as valid JSON for a
// StationEvent row — see dto.go's rawOrNull, which requires every
// Payload to already be well-formed JSON.
func accessEventPayload(username string) string {
	encoded, _ := json.Marshal(map[string]string{"username": username})
	return string(encoded)
}

type accessResponse struct {
	Username  string    `json:"username"`
	GrantedBy string    `json:"grantedBy"`
	CreatedAt time.Time `json:"createdAt"`
}

func toAccessResponse(row db.StationAccess) accessResponse {
	return accessResponse{Username: row.Username, GrantedBy: row.GrantedBy, CreatedAt: row.CreatedAt}
}

// listStationAccess is admin-only (see router.go) and nested under the
// requireStationAccess-guarded :id group, so a bad station ID already 404s
// before this ever runs.
func (app *App) listStationAccess(c *gin.Context) {
	var rows []db.StationAccess
	err := app.DB.Where("station_id = ?", c.Param("id")).Order("created_at asc").Find(&rows).Error
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	result := make([]accessResponse, 0, len(rows))
	for _, row := range rows {
		result = append(result, toAccessResponse(row))
	}
	c.JSON(http.StatusOK, result)
}

type grantAccessRequest struct {
	Username string `json:"username" binding:"required"`
}

// grantStationAccess requires the target username to already be a known
// user (see plan) — granting access to a typo'd or nonexistent username
// would silently do nothing useful, so it's rejected up front instead of
// creating a StationAccess row that can never match anyone's session.
func (app *App) grantStationAccess(c *gin.Context) {
	var body grantAccessRequest
	if err := c.ShouldBindJSON(&body); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}
	var user db.User
	if err := app.DB.First(&user, "username = ?", body.Username).Error; err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			c.JSON(http.StatusBadRequest, gin.H{"error": "unknown username"})
			return
		}
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}

	id := c.Param("id")
	row := db.StationAccess{StationID: id, Username: body.Username, GrantedBy: actorFrom(c)}
	if err := app.DB.Create(&row).Error; err != nil {
		c.JSON(http.StatusConflict, gin.H{"error": "access already granted"})
		return
	}
	app.Writer.enqueue(db.StationEvent{StationID: id, Actor: actorFrom(c), EventType: "access_granted", Payload: accessEventPayload(body.Username)})
	c.JSON(http.StatusCreated, toAccessResponse(row))
}

// revokeStationAccess deliberately does not stop an admin from revoking a
// user's access to a station that user themself created (see plan: admin is
// the sole authority over grants, no special-casing the original creator).
func (app *App) revokeStationAccess(c *gin.Context) {
	id := c.Param("id")
	username := c.Param("username")
	result := app.DB.Where("station_id = ? AND username = ?", id, username).Delete(&db.StationAccess{})
	if result.Error != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": result.Error.Error()})
		return
	}
	if result.RowsAffected == 0 {
		c.JSON(http.StatusNotFound, gin.H{"error": "access grant not found"})
		return
	}
	app.Writer.enqueue(db.StationEvent{StationID: id, Actor: actorFrom(c), EventType: "access_revoked", Payload: accessEventPayload(username)})
	c.Status(http.StatusNoContent)
}
