package api

import (
	"errors"
	"net/http"
	"time"

	"github.com/gin-gonic/gin"
	"gorm.io/gorm"

	"ocpp-station-simulator/backend/internal/db"
)

type dataTransferHandlerRequest struct {
	VendorID  string `json:"vendorId" binding:"required"`
	MessageID string `json:"messageId"` // empty matches any messageId for this vendorId
	Status    string `json:"status" binding:"required"`
	Data      string `json:"data"`
}

type dataTransferHandlerResponse struct {
	ID        uint64    `json:"id"`
	VendorID  string    `json:"vendorId"`
	MessageID string    `json:"messageId,omitempty"`
	Status    string    `json:"status"`
	Data      string    `json:"data,omitempty"`
	CreatedBy string    `json:"createdBy"`
	CreatedAt time.Time `json:"createdAt"`
}

func toDataTransferHandlerResponse(row db.DataTransferHandler) dataTransferHandlerResponse {
	return dataTransferHandlerResponse{
		ID: row.ID, VendorID: row.VendorID, MessageID: row.MessageID, Status: row.ResponseStatus,
		Data: row.ResponseData, CreatedBy: row.CreatedBy, CreatedAt: row.CreatedAt,
	}
}

func (app *App) listDataTransferHandlers(c *gin.Context) {
	var rows []db.DataTransferHandler
	if err := app.DB.Where("station_id = ?", c.Param("id")).Order("created_at desc").Find(&rows).Error; err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	result := make([]dataTransferHandlerResponse, 0, len(rows))
	for _, row := range rows {
		result = append(result, toDataTransferHandlerResponse(row))
	}
	c.JSON(http.StatusOK, result)
}

// createDataTransferHandler persists the registration and, if the station
// is currently running, pushes it into that Simulator's in-memory matcher
// immediately — see dataTransferMatcher's doc comment for why matching has
// to happen in memory rather than via a per-vendorId station.Handle
// registration.
func (app *App) createDataTransferHandler(c *gin.Context) {
	stationID := c.Param("id")
	var body dataTransferHandlerRequest
	if err := c.ShouldBindJSON(&body); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}
	row := db.DataTransferHandler{
		StationID: stationID, VendorID: body.VendorID, MessageID: body.MessageID,
		ResponseStatus: body.Status, ResponseData: body.Data, CreatedBy: actorFrom(c),
	}
	if err := app.DB.Create(&row).Error; err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	if managed, err := app.Registry.Get(stationID); err == nil {
		managed.Sim.RegisterDataTransferResponse(body.VendorID, body.MessageID, body.Status, body.Data)
	}
	c.JSON(http.StatusCreated, toDataTransferHandlerResponse(row))
}

func (app *App) deleteDataTransferHandler(c *gin.Context) {
	stationID := c.Param("id")
	var row db.DataTransferHandler
	err := app.DB.First(&row, "id = ? AND station_id = ?", c.Param("handlerId"), stationID).Error
	if errors.Is(err, gorm.ErrRecordNotFound) {
		c.JSON(http.StatusNotFound, gin.H{"error": "handler not found"})
		return
	}
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	app.DB.Delete(&row)
	if managed, err := app.Registry.Get(stationID); err == nil {
		managed.Sim.UnregisterDataTransferResponse(row.VendorID, row.MessageID)
	}
	c.Status(http.StatusNoContent)
}

type sendDataTransferRequest struct {
	VendorID  string `json:"vendorId" binding:"required"`
	MessageID string `json:"messageId"`
	Data      string `json:"data"`
}

func (app *App) sendDataTransfer(c *gin.Context) {
	var body sendDataTransferRequest
	managed, ok := app.bindAndGetManaged(c, &body)
	if !ok {
		return
	}
	result, err := managed.Sim.SendDataTransfer(c.Request.Context(), body.VendorID, body.MessageID, body.Data)
	if app.fail(c, err) {
		return
	}
	c.JSON(http.StatusOK, result)
}
