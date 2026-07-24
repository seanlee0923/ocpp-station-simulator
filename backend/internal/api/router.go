package api

import "github.com/gin-gonic/gin"

// RegisterRoutes mounts every route on router. Kept separate from
// cmd/server/main.go so main.go only has to wire config/db/embed and never
// touches route definitions directly.
func RegisterRoutes(router *gin.Engine, app *App) {
	authGroup := router.Group("/api/auth")
	authGroup.POST("/login", app.login)
	authGroup.POST("/logout", app.logout)
	authGroup.GET("/me", app.requireAuth, app.me)

	admin := router.Group("/api/admin", app.requireAuth, app.requireAdmin)
	admin.GET("/users", app.listUsers)
	admin.POST("/users", app.createUser)
	admin.DELETE("/users/:id", app.deleteUser)

	group := router.Group("/api/stations", app.requireAuth)
	group.POST("", app.createStation)
	group.GET("", app.listStations)
	group.GET("/:id", app.getStation)
	group.DELETE("/:id", app.deleteStation)
	group.POST("/:id/connect", app.connectStation)
	group.POST("/:id/disconnect", app.disconnectStation)
	group.POST("/:id/boot-notification", app.sendBootNotification)
	group.POST("/:id/authorize", app.sendAuthorize)
	group.POST("/:id/meter-values", app.sendMeterValues)
	group.POST("/:id/status-notification", app.sendStatusNotification)
	group.POST("/:id/firmware-status-notification", app.sendFirmwareStatusNotification)
	group.POST("/:id/diagnostics-status-notification", app.sendDiagnosticsStatusNotification)
	group.POST("/:id/data-transfer", app.sendDataTransfer)
	group.GET("/:id/data-transfer-handlers", app.listDataTransferHandlers)
	group.POST("/:id/data-transfer-handlers", app.createDataTransferHandler)
	group.DELETE("/:id/data-transfer-handlers/:handlerId", app.deleteDataTransferHandler)
	group.POST("/:id/transactions/start", app.startTransaction)
	group.POST("/:id/transactions/:txId/stop", app.stopTransaction)
	group.GET("/:id/config", app.getStationConfig)
	group.GET("/:id/events", app.listEvents)
	group.GET("/:id/ws", app.streamEvents)
}
