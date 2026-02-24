package config

import (
	"log"
)

type AppConfig struct {
	InfoLog       *log.Logger
	ErrorLog      *log.Logger
	ErrorChan     chan error
	ErrorDoneChan chan bool
}

func NewAppConfig() *AppConfig {

	infoLog := log.New(log.Writer(), "[INFO]\t", log.Ldate|log.Ltime)
	errorLog := log.New(log.Writer(), "[ERROR]\t", log.Ldate|log.Ltime|log.Lshortfile)

	return &AppConfig{
		InfoLog:       infoLog,
		ErrorLog:      errorLog,
		ErrorChan:     make(chan error),
		ErrorDoneChan: make(chan bool),
	}
}

func (app *AppConfig) Shutdown() {

	// perform any cleanup tasks
	app.InfoLog.Println("Would run cleanup tasks...")

	// notify "listenForErrors" channel to close
	app.ErrorDoneChan <- true

	app.InfoLog.Println("closing channels and shutting down application...")
	close(app.ErrorChan)
	close(app.ErrorDoneChan)
}
