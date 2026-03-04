package handlers

import (
	"SyncAI-WebRTC-Worker/internal/config"
	"SyncAI-WebRTC-Worker/internal/gateway"
	rtc "SyncAI-WebRTC-Worker/internal/webrtc"
	"context"
)

// SessionManager define WebRTC session operation interface
type SessionManager interface {
	CreateSession(ctx context.Context, offerSDP string, opts rtc.SessionOptions) (string, string, error)
	DeleteSession(sessionID string) error
	CreateAudioSession(offerSDP string, opts rtc.AudioSessionOptions) (string, string, error)
	DeleteAudioSession(sessionID string) error
	CreateVideoSession(offerSDP string, opts rtc.VideoSessionOptions) (string, string, error)
	DeleteVideoSession(sessionID string) error
}

type DefaultSessionManager struct{}

func (d *DefaultSessionManager) CreateSession(ctx context.Context, offerSDP string, opts rtc.SessionOptions) (string, string, error) {
	return rtc.CreateSession(ctx, offerSDP, opts)
}

func (d *DefaultSessionManager) DeleteSession(sessionID string) error {
	return rtc.DeleteSession(sessionID)
}

func (d *DefaultSessionManager) CreateAudioSession(offerSDP string, opts rtc.AudioSessionOptions) (string, string, error) {
	return rtc.CreateAudioSession(offerSDP, opts)
}

func (d *DefaultSessionManager) DeleteAudioSession(sessionID string) error {
	return rtc.DeleteAudioSession(sessionID)
}

func (d *DefaultSessionManager) CreateVideoSession(offerSDP string, opts rtc.VideoSessionOptions) (string, string, error) {
	return rtc.CreateVideoSession(offerSDP, opts)
}

func (d *DefaultSessionManager) DeleteVideoSession(sessionID string) error {
	return rtc.DeleteVideoSession(sessionID)
}

type Handler struct {
	App            *config.AppConfig
	GStreamerCfg   *config.GStreamerConfig
	SessionManager SessionManager
	RobotGateway   gateway.RobotGatewayInterface
}

func NewHandler(app *config.AppConfig, gCfg *config.GStreamerConfig, sm SessionManager, rg gateway.RobotGatewayInterface) *Handler {
	return &Handler{
		App:            app,
		GStreamerCfg:   gCfg,
		SessionManager: sm,
		RobotGateway:   rg,
	}
}
