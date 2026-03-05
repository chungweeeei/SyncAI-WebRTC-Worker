package handlers

import (
	"net/http"

	rtc "SyncAI-WebRTC-Worker/internal/webrtc"

	"github.com/gin-gonic/gin"
)

func (h *Handler) HandleVideoWHEP(c *gin.Context) {
	contentType := c.GetHeader("Content-Type")
	if contentType != "application/sdp" {
		c.JSON(http.StatusUnsupportedMediaType, gin.H{"error": "Invalid Content-Type"})
		return
	}

	offerSDP, err := c.GetRawData()
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to read request body"})
		return
	}

	opts := rtc.VideoSessionOptions{
		VideoRTPPort: h.GStreamerCfg.VideoRTPPort,
		AudioRTPPort: h.GStreamerCfg.VideoAudioRTPPort,
		VideoSource:  h.GStreamerCfg.VideoSource,
	}

	sessionID, answerSDP, err := h.SessionManager.CreateVideoSession(string(offerSDP), opts)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to create WebRTC session"})
		return
	}

	if err := h.RobotGateway.StartRTSPPusher(); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to open ROS Node"})
		return
	}

	c.Header("Content-Type", "application/sdp")
	c.Header("Location", "/video/whep/"+sessionID)
	c.String(http.StatusCreated, answerSDP)
}

func (h *Handler) DeleteVideoWHEP(c *gin.Context) {
	sessionID := c.Param("id")

	if err := h.RobotGateway.StopRTSPPusher(); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to closed ROS Node"})
		return
	}

	if err := h.SessionManager.DeleteVideoSession(sessionID); err != nil {
		c.JSON(http.StatusNotFound, gin.H{"error": "Session not found"})
		return
	}

	c.Status(http.StatusOK)
}
