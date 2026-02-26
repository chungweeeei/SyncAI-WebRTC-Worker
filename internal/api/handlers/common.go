package handlers

import "SyncAI-WebRTC-Worker/internal/config"

type Handler struct {
	App          *config.AppConfig
	GStreamerCfg *config.GStreamerConfig
}

func NewHandler(app *config.AppConfig, gCfg *config.GStreamerConfig) *Handler {
	return &Handler{
		App:          app,
		GStreamerCfg: gCfg,
	}
}
