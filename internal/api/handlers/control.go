package handlers

import (
	"context"
	"net/http"

	"SyncAI-WebRTC-Worker/internal/transport"
	rtc "SyncAI-WebRTC-Worker/internal/webrtc"

	"github.com/gin-gonic/gin"
)

func (h *Handler) HandleControlWHIP(c *gin.Context) {

	contentType := c.GetHeader("Content-Type")
	if contentType != "application/sdp" {
		c.JSON(http.StatusUnsupportedMediaType, gin.H{"error": "Invalid Content-Type"})
		return
	}

	// read the offer SDP from the request body
	offerSDP, err := c.GetRawData()
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to read request body"})
		return
	}

	opts := rtc.SessionOptions{
		WebSocketURL: "ws://host.docker.internal:9090",
		Dialer:       transport.NewWebSocketDialer(),
	}

	sessionID, answerSDP, err := rtc.CreateSession(context.Background(), string(offerSDP), opts)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to create WebRTC session"})
		return
	}

	c.Header("Content-Type", "application/sdp")
	c.Header("Location", "/control/whip/"+sessionID)
	c.String(http.StatusCreated, answerSDP)
}

func (h *Handler) DeleteControlWHIP(c *gin.Context) {
	sessionID := c.Param("id")

	if err := rtc.DeleteSession(sessionID); err != nil {
		c.JSON(http.StatusNotFound, gin.H{"error": "Session not found"})
		return
	}

	c.Status(http.StatusOK)
}
