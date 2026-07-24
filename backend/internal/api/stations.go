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
	actor := actorFrom(c)
	id := uuid.NewString()

	cfg := simulator.StationConfig{
		Identity: body.Identity, CSMSURL: body.CSMSURL, Version: body.Version,
		BasicAuthUser: body.BasicAuthUser, BasicAuthPass: body.BasicAuthPass,
		InsecureSkipTLSVerify: body.InsecureSkipTLSVerify,
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
		ConnectorCount: connectorCount, BasicAuthUser: body.BasicAuthUser,
		InsecureSkipTLSVerify: body.InsecureSkipTLSVerify, CreatedBy: actor,
		LastKnownStatus: "connecting",
	}
	if err := app.DB.Create(&row).Error; err != nil {
		_ = app.Registry.Delete(id)
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	app.Writer.enqueue(db.StationEvent{StationID: id, Actor: actor, EventType: "created"})

	c.JSON(http.StatusCreated, toStationResponse(row, "connecting"))
}

func (app *App) listStations(c *gin.Context) {
	query := app.DB.Order("created_at desc")
	if c.Query("includeDeleted") == "true" {
		// A deleted station's own StationEvent history is never deleted
		// (see plan) — Unscoped is what makes it possible to still find and
		// browse to that history, instead of the row (and any path to its
		// history) just vanishing from the list forever.
		query = query.Unscoped()
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

func (app *App) connectStation(c *gin.Context) {
	managed, ok := app.mustGetManaged(c)
	if !ok {
		return
	}
	app.Actors.set(managed.ID, actorFrom(c))
	if err := app.Registry.Reconnect(managed.ID); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	app.Writer.enqueue(db.StationEvent{StationID: managed.ID, Actor: actorFrom(c), EventType: "connect_requested"})
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
	req := simulator.StopTxRequest{TransactionID: c.Param("txId"), MeterStop: body.MeterStop, Reason: body.Reason}
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
