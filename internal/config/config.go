package config

import (
	"log"
	"os"
	"strconv"
)

type AppConfig struct {
	InfoLog       *log.Logger
	ErrorLog      *log.Logger
	ErrorChan     chan error
	ErrorDoneChan chan bool
}

func NewAppConfig() *AppConfig {

	infoLog := log.New(log.Writer(), "[INFO]\t", log.Ldate|log.Ltime)
	errorLog := log.New(log.Writer(), "[ERROR]\t", log.Ldate|log.Ltime|log.Lshortfile)

	return &AppConfig{
		InfoLog:       infoLog,
		ErrorLog:      errorLog,
		ErrorChan:     make(chan error),
		ErrorDoneChan: make(chan bool),
	}
}

func (app *AppConfig) Shutdown() {

	// perform any cleanup tasks
	app.InfoLog.Println("Would run cleanup tasks...")

	// notify "listenForErrors" channel to close
	app.ErrorDoneChan <- true

	app.InfoLog.Println("closing channels and shutting down application...")
	close(app.ErrorChan)
	close(app.ErrorDoneChan)
}

type VideoSourceType string

const (
	VideoSourceCamera VideoSourceType = "camera" // received image data directly from /dev/video0
	VideoSourceROS    VideoSourceType = "ros"    // received image data from udp src which is published by ROS node
)

type AudioConfig struct {
	AudioCard   int
	AudioDevice int
	AudioVolume float64
}

type GStreamerConfig struct {
	AudioRTPPort      int
	VideoRTPPort      int
	VideoAudioRTPPort int
	VideoSource       VideoSourceType
	AudioConfig       AudioConfig
}

func NewGStreamerConfig() *GStreamerConfig {
	audioPort, err := strconv.Atoi(getEnv("AUDIO_RTP_PORT", "5004"))
	if err != nil {
		// audio port use default value if conversion fails
		audioPort = 5004
	}

	videoPort, err := strconv.Atoi(getEnv("VIDEO_RTP_PORT", "5006"))
	if err != nil {
		// video port use default value if conversion fails
		videoPort = 5006
	}

	videoAudioPort, err := strconv.Atoi(getEnv("VIDEO_AUDIO_RTP_PORT", "5007"))
	if err != nil {
		// video audio port use default value if conversion fails
		videoAudioPort = 5007
	}

	audioCard, err := strconv.Atoi(getEnv("AUDIO_CARD", "0"))
	if err != nil {
		audioCard = 0
	}

	audioDevice, err := strconv.Atoi(getEnv("AUDIO_DEVICE", "0"))
	if err != nil {
		audioDevice = 0
	}

	audioVolume, err := strconv.ParseFloat(getEnv("AUDIO_VOLUME", "0.2"), 64)
	if err != nil {
		audioVolume = 0.2
	}

	return &GStreamerConfig{
		AudioRTPPort:      audioPort,
		VideoRTPPort:      videoPort,
		VideoAudioRTPPort: videoAudioPort,
		VideoSource:       VideoSourceCamera,
		AudioConfig: AudioConfig{
			AudioCard:   audioCard,
			AudioDevice: audioDevice,
			AudioVolume: audioVolume,
		},
	}
}

func getEnv(key, defaultValue string) string {
	if value, exists := os.LookupEnv(key); exists {
		return value
	}
	return defaultValue
}

func detectVideoSource() VideoSourceType {
	if _, err := os.Stat("/dev/syncai_camera"); err == nil {
		return VideoSourceCamera
	}
	return VideoSourceROS
}
