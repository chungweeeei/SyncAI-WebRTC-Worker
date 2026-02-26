package main

import (
	"SyncAI-WebRTC-Worker/internal/api"
	"SyncAI-WebRTC-Worker/internal/config"
	"log"
	"os"
	"os/signal"
	"syscall"

	"github.com/go-gst/go-gst/gst"
	"github.com/joho/godotenv"
)

func main() {
	// Initialize GStreamer
	gst.Init(nil)

	// Load environment variables
	if err := godotenv.Load(); err != nil {
		log.Println("No .env file found, using default configuration")
	}

	// Initialize API and start server
	app := config.NewAppConfig()
	gCfg := config.NewGStreamerConfig()

	go listenForErrors(app)

	go listenForShutdown(app)

	router := api.NewRouter(app, gCfg)

	app.InfoLog.Println("Starting WebRTC Worker on :18000")
	if err := router.Run(":18000"); err != nil {
		log.Panic(err)
	}
}

func listenForErrors(app *config.AppConfig) {
	for {
		select {
		case err := <-app.ErrorChan:
			app.ErrorLog.Println("Received error:", err)
		case <-app.ErrorDoneChan:
			return
		}
	}
}

func listenForShutdown(app *config.AppConfig) {
	quit := make(chan os.Signal, 1)
	signal.Notify(quit, syscall.SIGINT, syscall.SIGTERM)
	<-quit
	app.Shutdown()
	os.Exit(0)
}
