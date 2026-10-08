package proc

import (
	"errors"
	"fmt"
	"strings"
	"testing"

	"SyncAI-WebRTC-Worker/internal/config"

	"github.com/go-gst/go-gst/gst"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/mock"
	"github.com/stretchr/testify/require"
)

// testVideoConfig mirrors the shipped defaults; tests that care about one
// field override it rather than restating the whole struct.
func testVideoConfig() config.VideoConfig {
	return config.VideoConfig{
		Device:    "/dev/syncai/camera0",
		Format:    "mjpeg",
		Width:     1280,
		Height:    720,
		Framerate: 60,
		Bitrate:   4000000,
	}
}

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
			cfg := testVideoConfig()
			cfg.AudioSource = "alsasrc device=hw:2,0 do-timestamp=true"

			result := BuildVideoPipelineString(tc.videoPort, tc.audioPort, cfg)

			assert.Contains(t, result, fmt.Sprintf("udpsink host=127.0.0.1 port=%d", tc.videoPort))
			assert.Contains(t, result, fmt.Sprintf("udpsink host=127.0.0.1 port=%d", tc.audioPort))
			assert.Contains(t, result, "v4l2src device=/dev/syncai/camera0")
			assert.Contains(t, result, "alsasrc device=hw:2,0")
		})
	}
}

// The default is a video-only pipeline: the robot has no capture device and
// the container has no alsasrc element, so an audio branch that is merely
// unused would still fail the whole pipeline string at parse time.
func TestBuildVideoPipelineString_OmitsAudioBranchByDefault(t *testing.T) {
	result := BuildVideoPipelineString(5006, 5007, testVideoConfig())

	assert.NotContains(t, result, "opusenc")
	assert.NotContains(t, result, "port=5007")
	assert.Contains(t, result, "image/jpeg,width=1280,height=720,framerate=60/1")
	assert.Contains(t, result, "nvjpegdec")
}

func TestBuildVideoPipelineString_UYVYUsesRawCaptureBranch(t *testing.T) {
	cfg := testVideoConfig()
	cfg.Format = "uyvy"
	cfg.Width, cfg.Height, cfg.Framerate = 640, 480, 60

	result := BuildVideoPipelineString(5006, 5007, cfg)

	assert.Contains(t, result, "video/x-raw,format=UYVY,width=640,height=480,framerate=60/1")
	assert.Contains(t, result, "videoconvert")
	assert.NotContains(t, result, "nvjpegdec")
}

// The camera feeder owns the device, so the rtpjpeg branch must not touch it:
// no v4l2src, no extra-controls (the feeder wrote them), and no jitter buffer
// adding its latency on loopback.
func TestBuildVideoPipelineString_RTPJPEGReadsFromTheFeeder(t *testing.T) {
	cfg := testVideoConfig()
	cfg.Format = "rtpjpeg"
	cfg.RTPInPort = 5008

	result := BuildVideoPipelineString(5006, 5007, cfg)

	assert.True(t, strings.HasPrefix(result, "udpsrc address=127.0.0.1 port=5008 "), result)
	assert.Contains(t, result, `caps="application/x-rtp,media=video,encoding-name=JPEG,payload=26,clock-rate=90000"`)
	assert.Contains(t, result, "timeout=3000000000")
	assert.Contains(t, result, "! rtpjpegdepay ! queue")
	assert.Contains(t, result, "leaky=downstream ! nvjpegdec ! video/x-raw(memory:NVMM) ! nvvidconv")
	assert.Contains(t, result, "udpsink host=127.0.0.1 port=5006")
	assert.NotContains(t, result, "v4l2src")
	assert.NotContains(t, result, "extra-controls")
	assert.NotContains(t, result, "rtpjitterbuffer")
}

func TestBuildVideoPipelineString_RTPJPEGFormatIsCaseInsensitive(t *testing.T) {
	cfg := testVideoConfig()
	cfg.Format = "RTPJPEG"
	cfg.RTPInPort = 6008

	assert.Contains(t, BuildVideoPipelineString(5006, 5007, cfg), "udpsrc address=127.0.0.1 port=6008")
}

// An IDR every half second, and never an interval of zero -- nvv4l2h264enc
// takes the property in frames.
func TestBuildVideoPipelineString_IDRIntervalTracksFramerate(t *testing.T) {
	cfg := testVideoConfig()
	cfg.Framerate = 30
	assert.Contains(t, BuildVideoPipelineString(5006, 5007, cfg), "idrinterval=15")

	cfg.Framerate = 1
	assert.Contains(t, BuildVideoPipelineString(5006, 5007, cfg), "idrinterval=1")
}

func TestNewVideoGStreamerPipelineWithFactory_Success(t *testing.T) {
	// Arrange
	mockPipeline := new(MockPipeline)
	mockFactory := new(MockPipelineFactory)

	mockFactory.On("NewPipelineFromString", mock.Anything).Return(mockPipeline, nil)
	mockPipeline.On("SetState", gst.StatePlaying).Return(nil)

	// Act
	pipeline, err := NewVideoGStreamerPipelineWithFactory(5000, 5001, testVideoConfig(), mockFactory)

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
	pipeline, err := NewVideoGStreamerPipelineWithFactory(5000, 5001, testVideoConfig(), mockFactory)

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
	mockPipeline.On("BlockSetState", gst.StateNull).Return(nil)

	// Act
	pipeline, err := NewVideoGStreamerPipelineWithFactory(5000, 5001, testVideoConfig(), mockFactory)

	// Assert
	require.Error(t, err)
	assert.Nil(t, pipeline)
	// A pipeline nobody holds is still a pipeline go-gst will finalize, and
	// one finalized above NULL keeps the camera.
	mockPipeline.AssertCalled(t, "BlockSetState", gst.StateNull)
}

func TestVideoGStreamerPipeline_Close(t *testing.T) {
	// Arrange
	mockPipeline := new(MockPipeline)
	mockFactory := new(MockPipelineFactory)

	mockFactory.On("NewPipelineFromString", mock.Anything).Return(mockPipeline, nil)
	mockPipeline.On("SetState", gst.StatePlaying).Return(nil)
	mockPipeline.On("BlockSetState", gst.StateNull).Return(nil)

	pipeline, _ := NewVideoGStreamerPipelineWithFactory(5000, 5001, testVideoConfig(), mockFactory)

	// Act
	err := pipeline.Close()

	// Assert
	require.NoError(t, err)
	assert.True(t, pipeline.IsClosed())
}

// The camera fd is released on the way down to NULL, and SetState only asks
// for that transition. A teardown that does not wait for it leaves the device
// open for the life of the host process -- which cannot unload this library,
// so every later session negotiates nothing.
func TestVideoGStreamerPipeline_Close_WaitsForNullState(t *testing.T) {
	// Arrange
	mockPipeline := new(MockPipeline)
	mockFactory := new(MockPipelineFactory)

	mockFactory.On("NewPipelineFromString", mock.Anything).Return(mockPipeline, nil)
	mockPipeline.On("SetState", gst.StatePlaying).Return(nil)
	mockPipeline.On("BlockSetState", gst.StateNull).Return(nil)

	pipeline, _ := NewVideoGStreamerPipelineWithFactory(5000, 5001, testVideoConfig(), mockFactory)

	// Act
	require.NoError(t, pipeline.Close())

	// Assert
	mockPipeline.AssertCalled(t, "BlockSetState", gst.StateNull)
	mockPipeline.AssertNotCalled(t, "SetState", gst.StateNull)
}

func TestVideoGStreamerPipeline_Close_Idempotent(t *testing.T) {
	// Arrange
	mockPipeline := new(MockPipeline)
	mockFactory := new(MockPipelineFactory)

	mockFactory.On("NewPipelineFromString", mock.Anything).Return(mockPipeline, nil)
	mockPipeline.On("SetState", gst.StatePlaying).Return(nil)
	mockPipeline.On("BlockSetState", gst.StateNull).Return(nil).Once()

	pipeline, _ := NewVideoGStreamerPipelineWithFactory(5000, 5001, testVideoConfig(), mockFactory)

	// Act
	pipeline.Close()
	pipeline.Close()

	// Assert
	mockPipeline.AssertNumberOfCalls(t, "BlockSetState", 1)
}
