package config

import (
	"os"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestGetEnv_WithExistingEnv(t *testing.T) {
	// Arrange
	key := "TEST_CONFIG_KEY"
	expected := "test_value"
	os.Setenv(key, expected)
	defer os.Unsetenv(key)

	// Act
	result := getEnv(key, "default")

	// Assert
	assert.Equal(t, expected, result)
}

func TestGetEnv_WithMissingEnv(t *testing.T) {
	// Arrange
	key := "NON_EXISTENT_KEY_12345"
	os.Unsetenv(key)

	// Act
	result := getEnv(key, "default")

	// Assert
	assert.Equal(t, "default", result)
}

func TestGetEnv_WithEmptyValue(t *testing.T) {
	// Arrange
	key := "TEST_EMPTY_KEY"
	os.Setenv(key, "")
	defer os.Unsetenv(key)

	// Act
	result := getEnv(key, "default")

	// Assert
	assert.Equal(t, "", result)
}

func TestNewGStreamerConfig_DefaultValues(t *testing.T) {
	// Arrange
	os.Unsetenv("AUDIO_RTP_PORT")
	os.Unsetenv("VIDEO_RTP_PORT")
	os.Unsetenv("VIDEO_AUDIO_RTP_PORT")
	os.Unsetenv("AUDIO_VOLUME")

	// Act
	config := NewGStreamerConfig()

	// Assert
	assert.Equal(t, 5004, config.AudioRTPPort)
	assert.Equal(t, 5006, config.VideoRTPPort)
	assert.Equal(t, 5007, config.VideoAudioRTPPort)
	assert.Equal(t, 1.0, config.AudioConfig.AudioVolume)
}

func TestNewGStreamerConfig_CustomValues(t *testing.T) {
	// Arrange
	os.Setenv("AUDIO_RTP_PORT", "6000")
	os.Setenv("VIDEO_RTP_PORT", "6001")
	os.Setenv("VIDEO_AUDIO_RTP_PORT", "6002")
	os.Setenv("AUDIO_VOLUME", "0.8")
	defer func() {
		os.Unsetenv("AUDIO_RTP_PORT")
		os.Unsetenv("VIDEO_RTP_PORT")
		os.Unsetenv("VIDEO_AUDIO_RTP_PORT")
		os.Unsetenv("AUDIO_VOLUME")
	}()

	// Act
	config := NewGStreamerConfig()

	// Assert
	assert.Equal(t, 6000, config.AudioRTPPort)
	assert.Equal(t, 6001, config.VideoRTPPort)
	assert.Equal(t, 6002, config.VideoAudioRTPPort)
	assert.Equal(t, 0.8, config.AudioConfig.AudioVolume)
}

func TestNewGStreamerConfig_InvalidPortFallsBackToDefault(t *testing.T) {
	// Arrange
	os.Setenv("AUDIO_RTP_PORT", "not_a_number")
	os.Setenv("VIDEO_RTP_PORT", "invalid")
	os.Setenv("VIDEO_AUDIO_RTP_PORT", "abc")
	defer func() {
		os.Unsetenv("AUDIO_RTP_PORT")
		os.Unsetenv("VIDEO_RTP_PORT")
		os.Unsetenv("VIDEO_AUDIO_RTP_PORT")
	}()

	// Act
	config := NewGStreamerConfig()

	// Assert
	assert.Equal(t, 5004, config.AudioRTPPort)
	assert.Equal(t, 5006, config.VideoRTPPort)
	assert.Equal(t, 5007, config.VideoAudioRTPPort)
}

func TestNewGStreamerConfig_InvalidVolumeFallsBackToDefault(t *testing.T) {
	// Arrange
	os.Setenv("AUDIO_VOLUME", "not_a_float")
	defer os.Unsetenv("AUDIO_VOLUME")

	// Act
	config := NewGStreamerConfig()

	// Assert
	assert.Equal(t, 1.0, config.AudioConfig.AudioVolume)
}

func TestNewGStreamerConfig_ZeroVolume(t *testing.T) {
	// Arrange
	os.Setenv("AUDIO_VOLUME", "0")
	defer os.Unsetenv("AUDIO_VOLUME")

	// Act
	config := NewGStreamerConfig()

	// Assert
	assert.Equal(t, 0.0, config.AudioConfig.AudioVolume)
}

func TestNewGStreamerConfig_NegativePort(t *testing.T) {
	// Arrange
	os.Setenv("AUDIO_RTP_PORT", "-100")
	defer os.Unsetenv("AUDIO_RTP_PORT")

	// Act
	config := NewGStreamerConfig()

	// Assert
	assert.Equal(t, -100, config.AudioRTPPort)
}

func TestNewAppConfig_NotNil(t *testing.T) {
	// Act
	config := NewAppConfig()

	// Assert
	require.NotNil(t, config)
	assert.NotNil(t, config.InfoLog)
	assert.NotNil(t, config.ErrorLog)
	assert.NotNil(t, config.ErrorChan)
	assert.NotNil(t, config.ErrorDoneChan)
}

func TestNewAppConfig_ChannelsAreOpen(t *testing.T) {
	// Act
	config := NewAppConfig()

	// Assert - 測試 channel 可以使用
	go func() {
		config.ErrorChan <- nil
	}()

	err := <-config.ErrorChan
	assert.Nil(t, err)
}

func TestAppConfig_Shutdown(t *testing.T) {
	// Arrange
	config := NewAppConfig()

	// Act & Assert
	go func() {
		<-config.ErrorDoneChan
	}()

	assert.NotPanics(t, func() {
		config.Shutdown()
	})
}

func TestNewGStreamerConfig_RTPInPort(t *testing.T) {
	os.Unsetenv("VIDEO_RTP_IN_PORT")
	assert.Equal(t, 5008, NewGStreamerConfig().VideoConfig.RTPInPort,
		"must match the camera feeder's default rtp_port")

	os.Setenv("VIDEO_RTP_IN_PORT", "6008")
	defer os.Unsetenv("VIDEO_RTP_IN_PORT")
	assert.Equal(t, 6008, NewGStreamerConfig().VideoConfig.RTPInPort)

	os.Setenv("VIDEO_RTP_IN_PORT", "0")
	assert.Equal(t, 5008, NewGStreamerConfig().VideoConfig.RTPInPort)
}
