package main

import (
	"SyncAI-WebRTC-Worker/internal/api"
	"SyncAI-WebRTC-Worker/internal/config"
	"log"
	"os"
	"os/signal"
	"syscall"
)

func main() {

	app := config.NewAppConfig()

	go listenForErrors(app)

	go listenForShutdown(app)

	router := api.NewRouter(app)

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
