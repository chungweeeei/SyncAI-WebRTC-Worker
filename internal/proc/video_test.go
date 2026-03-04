package proc

import (
	"errors"
	"fmt"
	"testing"

	"github.com/go-gst/go-gst/gst"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/mock"
	"github.com/stretchr/testify/require"
)

func TestBuildVideoPipelineString(t *testing.T) {
	tests := []struct {
		name      string
		videoPort int
		audioPort int
	}{
		{"default ports", 5006, 5007},
		{"custom ports", 6000, 6001},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			result := BuildVideoPipelineString(tc.videoPort, tc.audioPort)

			assert.Contains(t, result, fmt.Sprintf("udpsink host=127.0.0.1 port=%d", tc.videoPort))
			assert.Contains(t, result, fmt.Sprintf("udpsink host=127.0.0.1 port=%d", tc.audioPort))
			assert.Contains(t, result, "v4l2src")
			assert.Contains(t, result, "alsasrc")
		})
	}
}

func TestNewVideoGStreamerPipelineWithFactory_Success(t *testing.T) {
	// Arrange
	mockPipeline := new(MockPipeline)
	mockFactory := new(MockPipelineFactory)

	mockFactory.On("NewPipelineFromString", mock.Anything).Return(mockPipeline, nil)
	mockPipeline.On("SetState", gst.StatePlaying).Return(nil)

	// Act
	pipeline, err := NewVideoGStreamerPipelineWithFactory(5000, 5001, mockFactory)

	// Assert
	require.NoError(t, err)
	require.NotNil(t, pipeline)
	assert.False(t, pipeline.IsClosed())
}

func TestNewVideoGStreamerPipelineWithFactory_FactoryError(t *testing.T) {
	// Arrange
	mockFactory := new(MockPipelineFactory)
	mockFactory.On("NewPipelineFromString", mock.Anything).Return(nil, errors.New("invalid pipeline"))

	// Act
	pipeline, err := NewVideoGStreamerPipelineWithFactory(5000, 5001, mockFactory)

	// Assert
	require.Error(t, err)
	assert.Nil(t, pipeline)
}

func TestNewVideoGStreamerPipelineWithFactory_SetStateError(t *testing.T) {
	// Arrange
	mockPipeline := new(MockPipeline)
	mockFactory := new(MockPipelineFactory)

	mockFactory.On("NewPipelineFromString", mock.Anything).Return(mockPipeline, nil)
	mockPipeline.On("SetState", gst.StatePlaying).Return(errors.New("failed"))

	// Act
	pipeline, err := NewVideoGStreamerPipelineWithFactory(5000, 5001, mockFactory)

	// Assert
	require.Error(t, err)
	assert.Nil(t, pipeline)
}

func TestVideoGStreamerPipeline_Close(t *testing.T) {
	// Arrange
	mockPipeline := new(MockPipeline)
	mockFactory := new(MockPipelineFactory)

	mockFactory.On("NewPipelineFromString", mock.Anything).Return(mockPipeline, nil)
	mockPipeline.On("SetState", gst.StatePlaying).Return(nil)
	mockPipeline.On("SetState", gst.StateNull).Return(nil)
	mockPipeline.On("Unref").Return()

	pipeline, _ := NewVideoGStreamerPipelineWithFactory(5000, 5001, mockFactory)

	// Act
	err := pipeline.Close()

	// Assert
	require.NoError(t, err)
	assert.True(t, pipeline.IsClosed())
}

func TestVideoGStreamerPipeline_Close_Idempotent(t *testing.T) {
	// Arrange
	mockPipeline := new(MockPipeline)
	mockFactory := new(MockPipelineFactory)

	mockFactory.On("NewPipelineFromString", mock.Anything).Return(mockPipeline, nil)
	mockPipeline.On("SetState", gst.StatePlaying).Return(nil)
	mockPipeline.On("SetState", gst.StateNull).Return(nil).Once()
	mockPipeline.On("Unref").Return().Once()

	pipeline, _ := NewVideoGStreamerPipelineWithFactory(5000, 5001, mockFactory)

	// Act
	pipeline.Close()
	pipeline.Close()

	// Assert
	mockPipeline.AssertNumberOfCalls(t, "Unref", 1)
}
