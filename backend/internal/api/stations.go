package api

import (
	"errors"
	"net/http"

	"github.com/gin-gonic/gin"
	"github.com/google/uuid"
	"gorm.io/gorm"

	"ocpp-station-simulator/backend/internal/db"
	"ocpp-station-simulator/backend/internal/simulator"
)

func (app *App) createStation(c *gin.Context) {
	var body createStationRequest
	if err := c.ShouldBindJSON(&body); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}
	if body.HeartbeatInterval < 0 {
		c.JSON(http.StatusBadRequest, gin.H{"error": "heartbeatInterval must not be negative"})
		return
	}
	if body.PingInterval < 0 {
		c.JSON(http.StatusBadRequest, gin.H{"error": "pingInterval must not be negative"})
		return
	}
	actor := actorFrom(c)
	id := uuid.NewString()

	cfg := simulator.StationConfig{
		Identity: body.Identity, CSMSURL: body.CSMSURL, Version: body.Version,
		BasicAuthUser: body.BasicAuthUser, BasicAuthPass: body.BasicAuthPass,
		InsecureSkipTLSVerify: body.InsecureSkipTLSVerify,
		HeartbeatInterval:     body.HeartbeatInterval,
		PingInterval:          body.PingInterval,
	}
	if _, err := app.Registry.Create(id, cfg); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}

	connectorCount := body.ConnectorCount
	if connectorCount <= 0 {
		connectorCount = 1
	}
	row := db.Station{
		ID: id, Identity: body.Identity, CSMSURL: body.CSMSURL, Version: body.Version,
		ConnectorCount: connectorCount, HeartbeatInterval: body.HeartbeatInterval,
		PingInterval:  body.PingInterval,
		BasicAuthUser: body.BasicAuthUser, BasicAuthPass: body.BasicAuthPass,
		InsecureSkipTLSVerify: body.InsecureSkipTLSVerify, CreatedBy: actor,
		LastKnownStatus: "connecting",
	}
	if err := app.DB.Create(&row).Error; err != nil {
		_ = app.Registry.Delete(id)
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	// The creator always gets access to their own station — without this a
	// non-admin user would be immediately locked out of what they just made.
	if err := app.DB.Create(&db.StationAccess{StationID: id, Username: actor, GrantedBy: actor}).Error; err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	app.Writer.enqueue(db.StationEvent{StationID: id, Actor: actor, EventType: "created"})

	c.JSON(http.StatusCreated, toStationResponse(row, "connecting"))
}

func (app *App) listStations(c *gin.Context) {
	// Qualified as stations.created_at: once the non-admin branch below joins
	// in station_accesses (which also has its own created_at column), an
	// unqualified ORDER BY is ambiguous and SQLite rejects the query outright.
	query := app.DB.Order("stations.created_at desc")
	if c.Query("includeDeleted") == "true" {
		// A deleted station's own StationEvent history is never deleted
		// (see plan) — Unscoped is what makes it possible to still find and
		// browse to that history, instead of the row (and any path to its
		// history) just vanishing from the list forever.
		query = query.Unscoped()
	}
	if !isAdminFrom(c) {
		// Non-admins only ever see stations they've been granted access to
		// (see db.StationAccess) — admins see everything unfiltered.
		query = query.Joins("JOIN station_accesses ON station_accesses.station_id = stations.id").
			Where("station_accesses.username = ?", actorFrom(c))
	}
	var rows []db.Station
	if err := query.Find(&rows).Error; err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	result := make([]stationResponse, 0, len(rows))
	for _, row := range rows {
		result = append(result, toStationResponse(row, app.liveState(row.ID)))
	}
	c.JSON(http.StatusOK, result)
}

func (app *App) getStation(c *gin.Context) {
	row, ok := app.mustFindStation(c)
	if !ok {
		return
	}
	c.JSON(http.StatusOK, toStationResponse(row, app.liveState(row.ID)))
}

func (app *App) deleteStation(c *gin.Context) {
	id := c.Param("id")
	if _, ok := app.mustFindStation(c); !ok {
		return
	}
	if err := app.Registry.Delete(id); err != nil && !errors.Is(err, simulator.ErrNotFound) {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	actor := actorFrom(c)
	app.Writer.enqueue(db.StationEvent{StationID: id, Actor: actor, EventType: "deleted"})
	app.Actors.delete(id)
	app.DB.Delete(&db.Station{}, "id = ?", id)
	c.Status(http.StatusNoContent)
}

// connectStation is the one place that must tolerate the station not being
// in the in-memory Registry at all (every other action handler's
// mustGetManaged 404s for that case, which is correct for them — you
// shouldn't be able to Authorize a station you haven't connected). The
// registry starts empty on every backend restart/redeploy by design (see
// plan), so without this fallback, "연결" would permanently 404 for any
// station that was connected before a restart, with no way to recover it
// short of deleting and recreating it.
func (app *App) connectStation(c *gin.Context) {
	id := c.Param("id")
	actor := actorFrom(c)

	managed, err := app.Registry.Get(id)
	if errors.Is(err, simulator.ErrNotFound) {
		var row db.Station
		if dbErr := app.DB.First(&row, "id = ?", id).Error; dbErr != nil {
			if errors.Is(dbErr, gorm.ErrRecordNotFound) {
				c.JSON(http.StatusNotFound, gin.H{"error": "station not found"})
			} else {
				c.JSON(http.StatusInternalServerError, gin.H{"error": dbErr.Error()})
			}
			return
		}
		cfg := simulator.StationConfig{
			Identity: row.Identity, CSMSURL: row.CSMSURL, Version: row.Version,
			BasicAuthUser: row.BasicAuthUser, BasicAuthPass: row.BasicAuthPass,
			InsecureSkipTLSVerify: row.InsecureSkipTLSVerify,
			HeartbeatInterval:     row.HeartbeatInterval,
			PingInterval:          row.PingInterval,
		}
		// Registry.Create already starts connecting — no separate Reconnect call needed.
		managed, err = app.Registry.Create(id, cfg)
		if err != nil {
			c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
			return
		}
	} else if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	} else {
		// A WebSocketPingInterval the CSMS set during the previous session
		// was persisted rather than applied live (see persistPingInterval);
		// reconnecting is that "next session", so pick it up here. A
		// missing row just leaves the staged value alone.
		var row db.Station
		if dbErr := app.DB.First(&row, "id = ?", id).Error; dbErr == nil {
			_ = app.Registry.SetPingInterval(managed.ID, row.PingInterval)
		}
		if err := app.Registry.Reconnect(managed.ID); err != nil {
			c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
			return
		}
	}

	app.Actors.set(managed.ID, actor)
	app.Writer.enqueue(db.StationEvent{StationID: managed.ID, Actor: actor, EventType: "connect_requested"})
	c.Status(http.StatusNoContent)
}

func (app *App) disconnectStation(c *gin.Context) {
	managed, ok := app.mustGetManaged(c)
	if !ok {
		return
	}
	app.Actors.set(managed.ID, actorFrom(c))
	if err := app.Registry.Disconnect(managed.ID); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	app.Writer.enqueue(db.StationEvent{StationID: managed.ID, Actor: actorFrom(c), EventType: "disconnect_requested"})
	c.Status(http.StatusNoContent)
}

func (app *App) sendBootNotification(c *gin.Context) {
	var body bootRequest
	managed, ok := app.bindAndGetManaged(c, &body)
	if !ok {
		return
	}
	result, err := managed.Sim.SendBootNotification(c.Request.Context(), body)
	if app.fail(c, err) {
		return
	}
	c.JSON(http.StatusOK, result)
}

func (app *App) sendAuthorize(c *gin.Context) {
	var body authorizeRequest
	managed, ok := app.bindAndGetManaged(c, &body)
	if !ok {
		return
	}
	result, err := managed.Sim.SendAuthorize(c.Request.Context(), body.IDTag)
	if app.fail(c, err) {
		return
	}
	c.JSON(http.StatusOK, result)
}

func (app *App) startTransaction(c *gin.Context) {
	var body startTxRequest
	managed, ok := app.bindAndGetManaged(c, &body)
	if !ok {
		return
	}
	result, err := managed.Sim.StartTransaction(c.Request.Context(), body)
	if app.fail(c, err) {
		return
	}
	c.JSON(http.StatusOK, result)
}

func (app *App) stopTransaction(c *gin.Context) {
	var body stopTxRequest
	managed, ok := app.bindAndGetManaged(c, &body)
	if !ok {
		return
	}
	req := simulator.StopTxRequest{TransactionID: c.Param("txId"), MeterStop: body.MeterStop, Reason: body.Reason, Timestamp: body.Timestamp}
	if err := managed.Sim.StopTransaction(c.Request.Context(), req); app.fail(c, err) {
		return
	}
	c.Status(http.StatusNoContent)
}

func (app *App) sendMeterValues(c *gin.Context) {
	var body meterValuesRequest
	managed, ok := app.bindAndGetManaged(c, &body)
	if !ok {
		return
	}
	if err := managed.Sim.SendMeterValues(c.Request.Context(), body); app.fail(c, err) {
		return
	}
	c.Status(http.StatusNoContent)
}

func (app *App) sendStatusNotification(c *gin.Context) {
	var body statusRequest
	managed, ok := app.bindAndGetManaged(c, &body)
	if !ok {
		return
	}
	if err := managed.Sim.SendStatusNotification(c.Request.Context(), body); app.fail(c, err) {
		return
	}
	c.Status(http.StatusNoContent)
}

func (app *App) sendFirmwareStatusNotification(c *gin.Context) {
	var body statusOnlyRequest
	managed, ok := app.bindAndGetManaged(c, &body)
	if !ok {
		return
	}
	if err := managed.Sim.SendFirmwareStatusNotification(c.Request.Context(), body.Status); app.fail(c, err) {
		return
	}
	c.Status(http.StatusNoContent)
}

func (app *App) sendDiagnosticsStatusNotification(c *gin.Context) {
	var body statusOnlyRequest
	managed, ok := app.bindAndGetManaged(c, &body)
	if !ok {
		return
	}
	if err := managed.Sim.SendDiagnosticsStatusNotification(c.Request.Context(), body.Status); app.fail(c, err) {
		return
	}
	c.Status(http.StatusNoContent)
}

func (app *App) setHeartbeatInterval(c *gin.Context) {
	var body heartbeatSettingsRequest
	if err := c.ShouldBindJSON(&body); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}
	if body.Interval < 0 {
		c.JSON(http.StatusBadRequest, gin.H{"error": "interval must not be negative"})
		return
	}
	managed, ok := app.mustGetManaged(c)
	if !ok {
		return
	}
	if err := app.DB.Model(&db.Station{}).Where("id = ?", managed.ID).Update("heartbeat_interval", body.Interval).Error; err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	if err := app.Registry.SetHeartbeatInterval(managed.ID, body.Interval); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	c.Status(http.StatusNoContent)
}

// setPingInterval stores the period and stages it on the running station.
// Unlike the heartbeat it cannot apply to the connection already open — the
// ping loop lives inside station.Station, built once at connect time — so
// the operator has to reconnect for a change to take effect.
func (app *App) setPingInterval(c *gin.Context) {
	var body pingSettingsRequest
	if err := c.ShouldBindJSON(&body); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}
	if body.Interval < 0 {
		c.JSON(http.StatusBadRequest, gin.H{"error": "interval must not be negative"})
		return
	}
	managed, ok := app.mustGetManaged(c)
	if !ok {
		return
	}
	if err := app.DB.Model(&db.Station{}).Where("id = ?", managed.ID).Update("ping_interval", body.Interval).Error; err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	if err := app.Registry.SetPingInterval(managed.ID, body.Interval); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	c.JSON(http.StatusOK, gin.H{"appliesOnReconnect": true})
}

const defaultEventLimit = 200

func (app *App) listEvents(c *gin.Context) {
	id := c.Param("id")
	var rows []db.StationEvent
	query := app.DB.Where("station_id = ?", id).Order("created_at desc").Limit(defaultEventLimit)
	if err := query.Find(&rows).Error; err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	result := make([]eventResponse, 0, len(rows))
	for _, row := range rows {
		result = append(result, toEventResponse(row))
	}
	c.JSON(http.StatusOK, result)
}

// getStationConfig returns whatever the CSMS has set so far via
// ChangeConfiguration (1.6) or SetVariables (2.0.1/2.1) — see
// simulator.configStore. Only meaningful while the station is connected in
// this process; there's nothing to return for a station that isn't
// currently running (see mustGetManaged's 404 for why).
func (app *App) getStationConfig(c *gin.Context) {
	managed, ok := app.mustGetManaged(c)
	if !ok {
		return
	}
	c.JSON(http.StatusOK, managed.Sim.GetConfigValues())
}

// --- shared helpers ---

// mustFindStation always looks up Unscoped (soft-deleted stations included):
// if a caller already has the exact ID — from the "deleted" list, a bookmark,
// a shared link — its details and history must stay reachable, only the
// active-station list itself hides deleted rows.
func (app *App) mustFindStation(c *gin.Context) (db.Station, bool) {
	var row db.Station
	err := app.DB.Unscoped().First(&row, "id = ?", c.Param("id")).Error
	if errors.Is(err, gorm.ErrRecordNotFound) {
		c.JSON(http.StatusNotFound, gin.H{"error": "station not found"})
		return db.Station{}, false
	}
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return db.Station{}, false
	}
	return row, true
}

func (app *App) mustGetManaged(c *gin.Context) (*simulator.ManagedStation, bool) {
	managed, err := app.Registry.Get(c.Param("id"))
	if errors.Is(err, simulator.ErrNotFound) {
		c.JSON(http.StatusNotFound, gin.H{"error": "station is not currently running; connect it first"})
		return nil, false
	}
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return nil, false
	}
	return managed, true
}

func (app *App) bindAndGetManaged(c *gin.Context, body any) (*simulator.ManagedStation, bool) {
	if err := c.ShouldBindJSON(body); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return nil, false
	}
	managed, ok := app.mustGetManaged(c)
	if !ok {
		return nil, false
	}
	app.Actors.set(managed.ID, actorFrom(c))
	return managed, true
}

func (app *App) fail(c *gin.Context, err error) bool {
	if err == nil {
		return false
	}
	c.JSON(http.StatusBadGateway, gin.H{"error": err.Error()})
	return true
}

func (app *App) liveState(stationID string) string {
	managed, err := app.Registry.Get(stationID)
	if err != nil {
		return "not_running"
	}
	return managed.Sim.State().String()
}
