// Command server runs the ocpp-station-simulator backend: REST + WebSocket
// API, the OCPP station registry, and (once built) the embedded frontend,
// all in one process — see the project plan for why this stays a single
// binary instead of splitting into separate services.
package main

import (
	"context"
	"log"
	"net/http"
	"os/signal"
	"syscall"
	"time"

	"github.com/gin-gonic/gin"

	"ocpp-station-simulator/backend/internal/api"
	"ocpp-station-simulator/backend/internal/auth"
	"ocpp-station-simulator/backend/internal/config"
	"ocpp-station-simulator/backend/internal/db"
	"ocpp-station-simulator/backend/internal/webui"
)

func main() {
	cfg := config.Load()

	database, err := db.Open(cfg.DBDriver, cfg.DBDSN)
	if err != nil {
		log.Fatalf("open database: %v", err)
	}
	if err := auth.Bootstrap(database, cfg.AdminUser, cfg.AdminPass); err != nil {
		log.Fatalf("bootstrap admin account: %v", err)
	}

	app := api.NewApp(database)
	defer app.Close()

	router := gin.Default()
	api.RegisterRoutes(router, app)

	if frontend, err := webui.FS(); err != nil {
		log.Printf("embedded frontend unavailable, API-only mode: %v", err)
	} else {
		router.NoRoute(gin.WrapH(http.FileServer(http.FS(frontend))))
	}

	server := &http.Server{Addr: ":" + cfg.Port, Handler: router}

	ctx, stop := signal.NotifyContext(context.Background(), syscall.SIGINT, syscall.SIGTERM)
	defer stop()

	go func() {
		log.Printf("ocpp-station-simulator listening on :%s (db=%s)", cfg.Port, cfg.DBDriver)
		if err := server.ListenAndServe(); err != nil && err != http.ErrServerClosed {
			log.Fatalf("server error: %v", err)
		}
	}()

	<-ctx.Done()
	log.Print("shutting down")
	shutdownCtx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	if err := server.Shutdown(shutdownCtx); err != nil {
		log.Printf("graceful shutdown failed: %v", err)
	}
}
