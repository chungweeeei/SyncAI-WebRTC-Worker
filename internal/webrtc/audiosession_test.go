package webrtc

import (
	"SyncAI-WebRTC-Worker/internal/config"
	"errors"
	"testing"

	"github.com/pion/webrtc/v4"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/mock"
)

func TestCreateAudioSession_FactoryError(t *testing.T) {
	// Arrange
	audioSessionsMu.Lock()
	audioSessions = make(map[string]*AudioSession)
	audioSessionsMu.Unlock()

	mockFactory := new(MockPeerConnectionFactory)
	mockFactory.On("NewPeerConnection", mock.Anything).Return(nil, errors.New("connection failed"))

	opts := AudioSessionOptions{
		RTPPort:   5004,
		AudioCfg:  config.AudioConfig{AudioVolume: 0.5},
		PCFactory: mockFactory,
	}

	// Act
	_, _, err := CreateAudioSession("v=0\r\n", opts)

	// Assert
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "connection failed")
	mockFactory.AssertExpectations(t)
}

func TestCreateAudioSession_GStreamerError(t *testing.T) {
	// Arrange
	audioSessionsMu.Lock()
	audioSessions = make(map[string]*AudioSession)
	audioSessionsMu.Unlock()

	mockPC := new(MockPeerConnection)
	mockPCFactory := new(MockPeerConnectionFactory)
	mockPipelineFactory := new(MockProcPipelineFactory)

	mockPCFactory.On("NewPeerConnection", mock.Anything).Return(mockPC, nil)
	mockPipelineFactory.On("NewPipelineFromString", mock.Anything).Return(nil, errors.New("gstreamer failed"))
	mockPC.On("Close").Return(nil)

	opts := AudioSessionOptions{
		RTPPort:         5004,
		AudioCfg:        config.AudioConfig{AudioVolume: 0.5},
		PCFactory:       mockPCFactory,
		PipelineFactory: mockPipelineFactory,
	}

	// Act
	_, _, err := CreateAudioSession("v=0\r\n", opts)

	// Assert
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "failed to start GStreamer")
	mockPCFactory.AssertExpectations(t)
	mockPC.AssertCalled(t, "Close")
}

func TestCreateAudioSession_SetRemoteDescriptionError(t *testing.T) {
	// Arrange
	audioSessionsMu.Lock()
	audioSessions = make(map[string]*AudioSession)
	audioSessionsMu.Unlock()

	mockPC := new(MockPeerConnection)
	mockPCFactory := new(MockPeerConnectionFactory)
	mockPipelineFactory := new(MockProcPipelineFactory)
	mockPipeline := new(MockProcPipeline)

	mockPCFactory.On("NewPeerConnection", mock.Anything).Return(mockPC, nil)
	mockPipelineFactory.On("NewPipelineFromString", mock.Anything).Return(mockPipeline, nil)
	mockPipeline.On("SetState", mock.Anything).Return(nil)
	mockPipeline.On("BlockSetState", mock.Anything).Return(nil)
	mockPC.On("OnTrack", mock.Anything).Return()
	mockPC.On("OnConnectionStateChange", mock.Anything).Return()
	mockPC.On("SetRemoteDescription", mock.Anything).Return(errors.New("invalid SDP"))
	mockPC.On("Close").Return(nil)

	opts := AudioSessionOptions{
		RTPPort:         5004,
		AudioCfg:        config.AudioConfig{AudioVolume: 0.5},
		PCFactory:       mockPCFactory,
		PipelineFactory: mockPipelineFactory,
	}

	// Act
	_, _, err := CreateAudioSession("invalid sdp", opts)

	// Assert
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "failed to set remote description")
	mockPCFactory.AssertExpectations(t)
	mockPC.AssertExpectations(t)
}

func TestCreateAudioSession_CreateAnswerError(t *testing.T) {
	// Arrange
	audioSessionsMu.Lock()
	audioSessions = make(map[string]*AudioSession)
	audioSessionsMu.Unlock()

	mockPC := new(MockPeerConnection)
	mockPCFactory := new(MockPeerConnectionFactory)
	mockPipelineFactory := new(MockProcPipelineFactory)
	mockPipeline := new(MockProcPipeline)

	mockPCFactory.On("NewPeerConnection", mock.Anything).Return(mockPC, nil)
	mockPipelineFactory.On("NewPipelineFromString", mock.Anything).Return(mockPipeline, nil)
	mockPipeline.On("SetState", mock.Anything).Return(nil)
	mockPipeline.On("BlockSetState", mock.Anything).Return(nil)
	mockPC.On("OnTrack", mock.Anything).Return()
	mockPC.On("OnConnectionStateChange", mock.Anything).Return()
	mockPC.On("SetRemoteDescription", mock.Anything).Return(nil)
	mockPC.On("CreateAnswer", mock.Anything).Return(webrtc.SessionDescription{}, errors.New("answer failed"))
	mockPC.On("Close").Return(nil)

	opts := AudioSessionOptions{
		RTPPort:         5004,
		AudioCfg:        config.AudioConfig{AudioVolume: 0.5},
		PCFactory:       mockPCFactory,
		PipelineFactory: mockPipelineFactory,
	}

	// Act
	_, _, err := CreateAudioSession("v=0\r\n", opts)

	// Assert
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "failed to create answer")
	mockPCFactory.AssertExpectations(t)
	mockPC.AssertExpectations(t)
}

func TestDeleteAudioSession_NotFound(t *testing.T) {
	// Arrange
	audioSessionsMu.Lock()
	audioSessions = make(map[string]*AudioSession)
	audioSessionsMu.Unlock()

	// Act
	err := DeleteAudioSession("non-existent-session-id")

	// Assert
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "session not found")
}

func TestDeleteAudioSession_Success(t *testing.T) {
	// Arrange
	audioSessionsMu.Lock()
	audioSessions = make(map[string]*AudioSession)
	audioSessionsMu.Unlock()

	mockPC := new(MockPeerConnection)
	mockPC.On("Close").Return(nil)

	session := &AudioSession{
		ID:        "test-session",
		PC:        mockPC,
		GStreamer: nil,
		udpConn:   nil,
	}

	audioSessionsMu.Lock()
	audioSessions["test-session"] = session
	audioSessionsMu.Unlock()

	// Act
	err := DeleteAudioSession("test-session")

	// Assert
	assert.NoError(t, err)

	audioSessionsMu.Lock()
	_, exists := audioSessions["test-session"]
	audioSessionsMu.Unlock()

	assert.False(t, exists, "session should be deleted from audioSessions map")
	mockPC.AssertCalled(t, "Close")
}

func TestAudioSession_Close_WithMock(t *testing.T) {
	// Arrange
	mockPC := new(MockPeerConnection)
	mockPC.On("Close").Return(nil)

	session := &AudioSession{
		ID:        "test-session",
		PC:        mockPC,
		GStreamer: nil,
		udpConn:   nil,
	}

	// Act
	session.Close()

	// Assert
	mockPC.AssertCalled(t, "Close")
}

func TestAudioSession_CloseWithAllNil(t *testing.T) {
	// Arrange
	session := &AudioSession{
		ID:        "test-audio-session",
		PC:        nil,
		GStreamer: nil,
		udpConn:   nil,
	}

	// Act & Assert - should not panic
	assert.NotPanics(t, func() {
		session.Close()
	})
}

func TestAudioSession_CloseTwice(t *testing.T) {
	// Arrange
	mockPC := new(MockPeerConnection)
	mockPC.On("Close").Return(nil)

	session := &AudioSession{
		ID:        "test-audio-session",
		PC:        mockPC,
		GStreamer: nil,
		udpConn:   nil,
	}

	// Act & Assert - closing twice should not panic
	assert.NotPanics(t, func() {
		session.Close()
		session.Close()
	})
}

func TestAudioSessionOptions_DefaultValues(t *testing.T) {
	opts := AudioSessionOptions{}

	assert.Equal(t, 0, opts.RTPPort, "expected default RTPPort to be 0")
	assert.Equal(t, float64(0), opts.AudioCfg.AudioVolume, "expected default AudioVolume to be 0")
	assert.Nil(t, opts.PCFactory, "expected default PCFactory to be nil")
	assert.Nil(t, opts.PipelineFactory, "expected default PipelineFactory to be nil")
}

func TestMultipleAudioSessions_Isolation(t *testing.T) {
	// Arrange
	audioSessionsMu.Lock()
	audioSessions = make(map[string]*AudioSession)
	audioSessionsMu.Unlock()

	mockPC1 := new(MockPeerConnection)
	mockPC1.On("Close").Return(nil)

	mockPC2 := new(MockPeerConnection)
	mockPC2.On("Close").Return(nil)

	session1 := &AudioSession{
		ID: "session-1",
		PC: mockPC1,
	}

	session2 := &AudioSession{
		ID: "session-2",
		PC: mockPC2,
	}

	// Add sessions to map
	audioSessionsMu.Lock()
	audioSessions["session-1"] = session1
	audioSessions["session-2"] = session2
	audioSessionsMu.Unlock()

	// Verify both sessions exist
	audioSessionsMu.Lock()
	assert.Len(t, audioSessions, 2, "expected 2 sessions")
	audioSessionsMu.Unlock()

	// Delete first session
	err := DeleteAudioSession("session-1")
	assert.NoError(t, err, "DeleteAudioSession failed")

	// Verify only second session remains
	audioSessionsMu.Lock()
	assert.Len(t, audioSessions, 1, "expected 1 session after deletion")
	_, exists := audioSessions["session-2"]
	audioSessionsMu.Unlock()

	assert.True(t, exists, "session-2 should still exist")

	// Cleanup
	_ = DeleteAudioSession("session-2")
}

func TestGenerateSessionID_Uniqueness(t *testing.T) {
	ids := make(map[string]bool)
	const numIDs = 100

	for i := 0; i < numIDs; i++ {
		id := generateSessionID()
		assert.False(t, ids[id], "Session ID should be unique, got duplicate: %s", id)
		ids[id] = true
	}

	assert.Len(t, ids, numIDs, "All session IDs should be unique")
}

func TestGenerateSessionID_NotEmpty(t *testing.T) {
	id := generateSessionID()
	assert.NotEmpty(t, id, "Session ID should not be empty")
}
