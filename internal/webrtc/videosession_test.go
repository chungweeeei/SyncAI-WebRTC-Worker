package webrtc

import (
	"SyncAI-WebRTC-Worker/internal/config"
	"sync"
	"testing"

	"github.com/pion/webrtc/v4"
	"github.com/stretchr/testify/assert"
)

func TestDeleteVideoSession_NotFound(t *testing.T) {
	// Arrange
	videoSessionsMu.Lock()
	videoSessions = make(map[string]*VideoSession)
	videoSessionsMu.Unlock()

	// Act
	err := DeleteVideoSession("non-existent-session-id")

	// Assert
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "video session not found")
}

func TestDeleteVideoSession_Success(t *testing.T) {
	// Arrange
	videoSessionsMu.Lock()
	videoSessions = make(map[string]*VideoSession)
	videoSessionsMu.Unlock()

	mockPC := new(MockPeerConnection)
	mockPC.On("Close").Return(nil)

	session := &VideoSession{
		ID:               "test-video-session",
		PC:               mockPC,
		GStreamer:        nil,
		videoUDPListener: nil,
		audioUDPListener: nil,
	}

	videoSessionsMu.Lock()
	videoSessions["test-video-session"] = session
	videoSessionsMu.Unlock()

	// Act
	err := DeleteVideoSession("test-video-session")

	// Assert
	assert.NoError(t, err)

	videoSessionsMu.Lock()
	_, exists := videoSessions["test-video-session"]
	videoSessionsMu.Unlock()

	assert.False(t, exists, "session should be deleted from videoSessions map")
	mockPC.AssertCalled(t, "Close")
}

func TestVideoSession_Close_WithMock(t *testing.T) {
	// Arrange
	mockPC := new(MockPeerConnection)
	mockPC.On("Close").Return(nil)

	session := &VideoSession{
		ID:               "test-video-session",
		PC:               mockPC,
		GStreamer:        nil,
		videoUDPListener: nil,
		audioUDPListener: nil,
	}

	// Act
	session.Close()

	// Assert
	mockPC.AssertCalled(t, "Close")
}

func TestVideoSession_CloseWithAllNil(t *testing.T) {
	// Arrange
	session := &VideoSession{
		ID:               "test-video-session",
		PC:               nil,
		GStreamer:        nil,
		videoUDPListener: nil,
		audioUDPListener: nil,
	}

	// Act & Assert - should not panic
	assert.NotPanics(t, func() {
		session.Close()
	})
}

func TestVideoSession_CloseTwice(t *testing.T) {
	// Arrange
	mockPC := new(MockPeerConnection)
	mockPC.On("Close").Return(nil)

	session := &VideoSession{
		ID:               "test-video-session",
		PC:               mockPC,
		GStreamer:        nil,
		videoUDPListener: nil,
		audioUDPListener: nil,
	}

	// Act & Assert - closing twice should not panic
	assert.NotPanics(t, func() {
		session.Close()
		session.Close()
	})
}

func TestVideoSession_ConcurrentClose(t *testing.T) {
	// Arrange
	mockPC := new(MockPeerConnection)
	mockPC.On("Close").Return(nil)

	session := &VideoSession{
		ID:               "test-video-session",
		PC:               mockPC,
		GStreamer:        nil,
		videoUDPListener: nil,
		audioUDPListener: nil,
	}

	// Act & Assert - concurrent close should not panic
	var wg sync.WaitGroup
	for i := 0; i < 10; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			session.Close()
		}()
	}

	assert.NotPanics(t, func() {
		wg.Wait()
	})
}

func TestVideoSessionOptions_DefaultValues(t *testing.T) {
	opts := VideoSessionOptions{}

	assert.Equal(t, 0, opts.VideoRTPPort, "expected default VideoRTPPort to be 0")
	assert.Equal(t, 0, opts.AudioRTPPort, "expected default AudioRTPPort to be 0")
	assert.Empty(t, opts.VideoSource, "expected default VideoSource to be empty")
	assert.Nil(t, opts.PCFactory, "expected default PCFactory to be nil")
	assert.Nil(t, opts.PipelineFactory, "expected default PipelineFactory to be nil")
}

func TestMultipleVideoSessions_Isolation(t *testing.T) {
	// Arrange
	videoSessionsMu.Lock()
	videoSessions = make(map[string]*VideoSession)
	videoSessionsMu.Unlock()

	mockPC1 := new(MockPeerConnection)
	mockPC1.On("Close").Return(nil)

	mockPC2 := new(MockPeerConnection)
	mockPC2.On("Close").Return(nil)

	session1 := &VideoSession{
		ID: "video-session-1",
		PC: mockPC1,
	}

	session2 := &VideoSession{
		ID: "video-session-2",
		PC: mockPC2,
	}

	// Add sessions to map
	videoSessionsMu.Lock()
	videoSessions["video-session-1"] = session1
	videoSessions["video-session-2"] = session2
	videoSessionsMu.Unlock()

	// Verify both sessions exist
	videoSessionsMu.Lock()
	assert.Len(t, videoSessions, 2, "expected 2 sessions")
	videoSessionsMu.Unlock()

	// Delete first session
	err := DeleteVideoSession("video-session-1")
	assert.NoError(t, err, "DeleteVideoSession failed")

	// Verify only second session remains
	videoSessionsMu.Lock()
	assert.Len(t, videoSessions, 1, "expected 1 session after deletion")
	_, exists := videoSessions["video-session-2"]
	videoSessionsMu.Unlock()

	assert.True(t, exists, "video-session-2 should still exist")

	// Cleanup
	_ = DeleteVideoSession("video-session-2")
}

func TestVideoSession_ConnectionState(t *testing.T) {
	// Arrange
	mockPC := new(MockPeerConnection)
	mockPC.connectionState = webrtc.PeerConnectionStateNew

	session := &VideoSession{
		ID: "test-video-session",
		PC: mockPC,
	}

	// Assert initial state
	assert.Equal(t, webrtc.PeerConnectionStateNew, session.PC.ConnectionState())

	// Simulate state change
	mockPC.connectionState = webrtc.PeerConnectionStateConnected
	assert.Equal(t, webrtc.PeerConnectionStateConnected, session.PC.ConnectionState())
}

func TestVideoSession_ClosedFlag(t *testing.T) {
	// Arrange
	mockPC := new(MockPeerConnection)
	mockPC.On("Close").Return(nil)

	session := &VideoSession{
		ID:               "test-video-session",
		PC:               mockPC,
		GStreamer:        nil,
		videoUDPListener: nil,
		audioUDPListener: nil,
	}

	// Verify closed flag is false initially
	session.mu.Lock()
	assert.False(t, session.closed, "closed flag should be false initially")
	session.mu.Unlock()

	// Close session
	session.Close()

	// Verify closed flag is true after close
	session.mu.Lock()
	assert.True(t, session.closed, "closed flag should be true after close")
	session.mu.Unlock()
}

func TestCreateVideoSession_UnsupportedVideoSource(t *testing.T) {
	// Arrange
	videoSessionsMu.Lock()
	videoSessions = make(map[string]*VideoSession)
	videoSessionsMu.Unlock()

	// Act
	_, _, err := CreateVideoSession("v=0\r\n", VideoSessionOptions{
		VideoRTPPort: 5006,
		AudioRTPPort: 5008,
		VideoSource:  config.VideoSourceType("unsupported"),
	})

	// Assert
	assert.Error(t, err, "CreateVideoSession should fail with unsupported video source")
	assert.Contains(t, err.Error(), "unsupported video source type")
}

func TestVideoSession_IDNotEmpty(t *testing.T) {
	// Arrange
	mockPC := new(MockPeerConnection)
	mockPC.On("Close").Return(nil)

	session := &VideoSession{
		ID: "test-video-session-id",
		PC: mockPC,
	}

	// Assert
	assert.NotEmpty(t, session.ID, "Session ID should not be empty")
	assert.Equal(t, "test-video-session-id", session.ID)
}
