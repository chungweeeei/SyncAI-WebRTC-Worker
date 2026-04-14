package handlers

import (
	rtc "SyncAI-WebRTC-Worker/internal/webrtc"
	"net/http"

	"github.com/gin-gonic/gin"
)

func (h *Handler) HandleAudioWHIP(c *gin.Context) {
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

	opts := rtc.AudioSessionOptions{
		RTPPort:    h.GStreamerCfg.AudioRTPPort,
		AudioCfg:   h.GStreamerCfg.AudioConfig,
		ICEServers: h.GStreamerCfg.ICEServers,
	}
	sessionID, answerSDP, err := h.SessionManager.CreateAudioSession(string(offerSDP), opts)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to create audio session"})
		return
	}

	c.Header("Content-Type", "application/sdp")
	c.Header("Location", "/audio/whip/"+sessionID)
	c.String(http.StatusCreated, answerSDP)
}

func (h *Handler) DeleteAudioWHIP(c *gin.Context) {
	sessionID := c.Param("id")

	if err := h.SessionManager.DeleteAudioSession(sessionID); err != nil {
		c.JSON(http.StatusNotFound, gin.H{"error": "Session not found"})
		return
	}

	c.Status(http.StatusOK)
}
