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
	// listStations filters by access itself (admin sees everything, everyone
	// else sees only what they've been granted) rather than rejecting —
	// there's no single station ID here for requireStationAccess to check.
	group.GET("", app.listStations)

	// Every route below operates on one specific station, so
	// requireStationAccess (admin bypass, else must have a db.StationAccess
	// grant, else 404) applies to all of them in one place instead of each
	// handler checking it individually.
	scoped := group.Group("/:id", app.requireStationAccess)
	scoped.GET("", app.getStation)
	scoped.DELETE("", app.deleteStation)
	scoped.POST("/connect", app.connectStation)
	scoped.POST("/disconnect", app.disconnectStation)
	scoped.POST("/boot-notification", app.sendBootNotification)
	scoped.POST("/authorize", app.sendAuthorize)
	scoped.POST("/meter-values", app.sendMeterValues)
	scoped.POST("/status-notification", app.sendStatusNotification)
	scoped.POST("/firmware-status-notification", app.sendFirmwareStatusNotification)
	scoped.POST("/diagnostics-status-notification", app.sendDiagnosticsStatusNotification)
	scoped.POST("/data-transfer", app.sendDataTransfer)
	scoped.GET("/data-transfer-handlers", app.listDataTransferHandlers)
	scoped.POST("/data-transfer-handlers", app.createDataTransferHandler)
	scoped.DELETE("/data-transfer-handlers/:handlerId", app.deleteDataTransferHandler)
	scoped.POST("/transactions/start", app.startTransaction)
	scoped.POST("/transactions/:txId/stop", app.stopTransaction)
	scoped.GET("/config", app.getStationConfig)
	scoped.GET("/events", app.listEvents)
	scoped.GET("/ws", app.streamEvents)

	// Access-grant management is additionally admin-only (see plan: no
	// delegated sharing) — still nested under scoped so a non-admin with no
	// grant gets the same 404 as everywhere else, not a 403 that would leak
	// whether the station exists.
	accessScoped := scoped.Group("/access", app.requireAdmin)
	accessScoped.GET("", app.listStationAccess)
	accessScoped.POST("", app.grantStationAccess)
	accessScoped.DELETE("/:username", app.revokeStationAccess)
}
