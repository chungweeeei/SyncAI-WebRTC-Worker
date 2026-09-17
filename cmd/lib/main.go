// Package main builds the c-shared entry point for embedding the WebRTC
// session core into a host process (e.g. Python/FastAPI via ctypes):
//
//	go build -buildmode=c-shared -o dist/libsyncai_worker.so ./cmd/lib
//
// Every exported function returns a malloc'd JSON string (the result
// envelope in result.go); the host must release it with FreeString.
package main

/*
#include <stdlib.h>
*/
import "C"

import (
	"log"
	"sync"
	"unsafe"

	"SyncAI-WebRTC-Worker/internal/config"
	rtc "SyncAI-WebRTC-Worker/internal/webrtc"

	"github.com/go-gst/go-gst/gst"
	"github.com/joho/godotenv"
)

var (
	initOnce sync.Once
	gCfgMu   sync.RWMutex
	gCfg     *config.GStreamerConfig
)

// loadedConfig returns the config set by InitWorker, or nil before init.
func loadedConfig() *config.GStreamerConfig {
	gCfgMu.RLock()
	defer gCfgMu.RUnlock()
	return gCfg
}

// InitWorker initializes GStreamer and loads configuration from environment
// variables (or a .env file in the working directory). It must be called once
// before any session function; further calls are no-ops that return success.
//
//export InitWorker
func InitWorker() *C.char {
	initOnce.Do(func() {
		gst.Init(nil)

		if err := godotenv.Load(); err != nil {
			log.Println("No .env file found, using default configuration")
		}

		gCfgMu.Lock()
		gCfg = config.NewGStreamerConfig()
		gCfgMu.Unlock()
	})

	return C.CString(successResult())
}

// CreateAudioSession creates a WHIP-style audio input session from an SDP
// offer. Blocks until ICE gathering completes.
//
//export CreateAudioSession
func CreateAudioSession(offerSDP *C.char) *C.char {
	cfg := loadedConfig()
	if cfg == nil {
		return C.CString(errorResult("worker not initialized"))
	}

	opts := rtc.AudioSessionOptions{
		RTPPort:    cfg.AudioRTPPort,
		AudioCfg:   cfg.AudioConfig,
		ICEServers: cfg.ICEServers,
	}

	sessionID, answerSDP, err := rtc.CreateAudioSession(C.GoString(offerSDP), opts)
	if err != nil {
		return C.CString(errorResult(err.Error()))
	}
	return C.CString(sessionResult(sessionID, answerSDP))
}

// DeleteAudioSession closes an audio session by ID.
//
//export DeleteAudioSession
func DeleteAudioSession(sessionID *C.char) *C.char {
	if loadedConfig() == nil {
		return C.CString(errorResult("worker not initialized"))
	}

	if err := rtc.DeleteAudioSession(C.GoString(sessionID)); err != nil {
		return C.CString(errorResult(err.Error()))
	}
	return C.CString(successResult())
}

// CreateVideoSession creates a WHEP-style video output session from an SDP
// offer, spawning the GStreamer camera capture pipeline. Blocks until ICE
// gathering completes.
//
//export CreateVideoSession
func CreateVideoSession(offerSDP *C.char) *C.char {
	cfg := loadedConfig()
	if cfg == nil {
		return C.CString(errorResult("worker not initialized"))
	}

	opts := rtc.VideoSessionOptions{
		VideoRTPPort: cfg.VideoRTPPort,
		AudioRTPPort: cfg.VideoAudioRTPPort,
		ICEServers:   cfg.ICEServers,
	}

	sessionID, answerSDP, err := rtc.CreateVideoSession(C.GoString(offerSDP), opts)
	if err != nil {
		return C.CString(errorResult(err.Error()))
	}
	return C.CString(sessionResult(sessionID, answerSDP))
}

// DeleteVideoSession closes a video session by ID.
//
//export DeleteVideoSession
func DeleteVideoSession(sessionID *C.char) *C.char {
	if loadedConfig() == nil {
		return C.CString(errorResult("worker not initialized"))
	}

	if err := rtc.DeleteVideoSession(C.GoString(sessionID)); err != nil {
		return C.CString(errorResult(err.Error()))
	}
	return C.CString(successResult())
}

// FreeString releases a string returned by any exported function.
//
//export FreeString
func FreeString(s *C.char) {
	C.free(unsafe.Pointer(s))
}

func main() {}
