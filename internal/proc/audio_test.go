package proc

import (
	"SyncAI-WebRTC-Worker/internal/config"
	"errors"
	"fmt"
	"testing"

	"github.com/go-gst/go-gst/gst"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/mock"
	"github.com/stretchr/testify/require"
)

func TestBuildAudioPipelineString(t *testing.T) {
	tests := []struct {
		name   string
		port   int
		volume float64
	}{
		{"default", 5004, 1.0},
		{"custom port", 6000, 0.5},
		{"zero volume", 5004, 0.0},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			cfg := config.AudioConfig{
				AudioVolume: tc.volume,
				Sink:        "alsasink device=plughw:CARD=CD002AUDIO",
			}
			result := BuildAudioPipelineString(tc.port, cfg)

			assert.Contains(t, result, fmt.Sprintf("port=%d", tc.port))
			assert.Contains(t, result, "alsasink device=plughw:CARD=CD002AUDIO")
			assert.Contains(t, result, fmt.Sprintf("volume volume=%f", tc.volume))
		})
	}
}

func TestNewAudioGStreamerPipelineWithFactory_Success(t *testing.T) {
	mockPipeline := new(MockPipeline)
	mockFactory := new(MockPipelineFactory)

	mockFactory.On("NewPipelineFromString", mock.Anything).Return(mockPipeline, nil)
	mockPipeline.On("SetState", gst.StatePlaying).Return(nil)

	// Act
	pipeline, err := NewAudioGStreamerPipelineWithFactory(5004, config.AudioConfig{AudioVolume: 1.0}, mockFactory)

	// Assert
	require.NoError(t, err)
	require.NotNil(t, pipeline)
	assert.False(t, pipeline.IsClosed())
	mockFactory.AssertExpectations(t)
	mockPipeline.AssertExpectations(t)
}

func TestNewAudioGStreamerPipelineWithFactory_FactoryError(t *testing.T) {
	// Arrange
	mockFactory := new(MockPipelineFactory)
	mockFactory.On("NewPipelineFromString", mock.Anything).Return(nil, errors.New("invalid pipeline"))

	// Act
	pipeline, err := NewAudioGStreamerPipelineWithFactory(5004, config.AudioConfig{}, mockFactory)

	// Assert
	require.Error(t, err)
	assert.Nil(t, pipeline)
}

func TestNewAudioGStreamerPipelineWithFactory_SetStateError(t *testing.T) {
	// Arrange
	mockPipeline := new(MockPipeline)
	mockFactory := new(MockPipelineFactory)

	mockFactory.On("NewPipelineFromString", mock.Anything).Return(mockPipeline, nil)
	mockPipeline.On("SetState", gst.StatePlaying).Return(errors.New("failed to start"))
	mockPipeline.On("BlockSetState", gst.StateNull).Return(nil)

	// Act
	pipeline, err := NewAudioGStreamerPipelineWithFactory(5004, config.AudioConfig{}, mockFactory)

	// Assert
	require.Error(t, err)
	assert.Nil(t, pipeline)
	// Same reason as the video pipeline's failed start: what go-gst finalizes
	// above NULL keeps its device.
	mockPipeline.AssertCalled(t, "BlockSetState", gst.StateNull)
}

func TestAudioGStreamerPipeline_Close(t *testing.T) {
	// Arrange
	mockPipeline := new(MockPipeline)
	mockFactory := new(MockPipelineFactory)

	mockFactory.On("NewPipelineFromString", mock.Anything).Return(mockPipeline, nil)
	mockPipeline.On("SetState", gst.StatePlaying).Return(nil)
	mockPipeline.On("BlockSetState", gst.StateNull).Return(nil)

	pipeline, _ := NewAudioGStreamerPipelineWithFactory(5004, config.AudioConfig{}, mockFactory)

	// Act
	err := pipeline.Close()

	// Assert
	require.NoError(t, err)
	assert.True(t, pipeline.IsClosed())
}

func TestAudioGStreamerPipeline_Close_Idempotent(t *testing.T) {
	// Arrange
	mockPipeline := new(MockPipeline)
	mockFactory := new(MockPipelineFactory)

	mockFactory.On("NewPipelineFromString", mock.Anything).Return(mockPipeline, nil)
	mockPipeline.On("SetState", gst.StatePlaying).Return(nil)
	mockPipeline.On("BlockSetState", gst.StateNull).Return(nil).Once()

	pipeline, _ := NewAudioGStreamerPipelineWithFactory(5004, config.AudioConfig{}, mockFactory)

	// Act
	pipeline.Close()
	pipeline.Close()

	// Assert
	// One SetState (Playing) on the way up; the way down goes through
	// BlockSetState, which is what waits for the device to be released.
	mockPipeline.AssertNumberOfCalls(t, "SetState", 1)
	mockPipeline.AssertNumberOfCalls(t, "BlockSetState", 1)
}
