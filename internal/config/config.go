package config

import (
	"log"
	"os"
	"strconv"
	"strings"

	"github.com/pion/webrtc/v4"
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

type AudioConfig struct {
	AudioVolume float64
}

type GStreamerConfig struct {
	AudioRTPPort      int
	VideoRTPPort      int
	VideoAudioRTPPort int
	AudioConfig       AudioConfig
	ICEServers        []webrtc.ICEServer
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

	audioVolume, err := strconv.ParseFloat(getEnv("AUDIO_VOLUME", "0.2"), 64)
	if err != nil {
		audioVolume = 0.2
	}

	return &GStreamerConfig{
		AudioRTPPort:      audioPort,
		VideoRTPPort:      videoPort,
		VideoAudioRTPPort: videoAudioPort,
		AudioConfig: AudioConfig{
			AudioVolume: audioVolume,
		},
		ICEServers: parseICEServers(),
	}
}

func getEnv(key, defaultValue string) string {
	if value, exists := os.LookupEnv(key); exists {
		return value
	}
	return defaultValue
}

// parseICEServers parses ICE server configuration from environment variables.
//
// STUN_SERVERS: comma-separated STUN URLs (default: stun:stun.l.google.com:19302)
// TURN_SERVERS: comma-separated TURN URLs
// TURN_USERNAME: username for TURN authentication
// TURN_PASSWORD: password for TURN authentication
func parseICEServers() []webrtc.ICEServer {
	var servers []webrtc.ICEServer

	stunURLs := getEnv("STUN_SERVERS", "stun:stun.l.google.com:19302")
	if stunURLs != "" {
		urls := strings.Split(stunURLs, ",")
		for i := range urls {
			urls[i] = strings.TrimSpace(urls[i])
		}
		servers = append(servers, webrtc.ICEServer{URLs: urls})
	}

	turnURLs := getEnv("TURN_SERVERS", "")
	if turnURLs != "" {
		urls := strings.Split(turnURLs, ",")
		for i := range urls {
			urls[i] = strings.TrimSpace(urls[i])
		}
		servers = append(servers, webrtc.ICEServer{
			URLs:       urls,
			Username:   getEnv("TURN_USERNAME", ""),
			Credential: getEnv("TURN_PASSWORD", ""),
		})
	}

	return servers
}
