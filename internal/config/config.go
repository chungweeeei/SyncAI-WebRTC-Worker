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

	// Sink is the complete GStreamer sink element the WHIP branch plays the
	// browser's microphone into, properties included.
	//
	// ALSA by name, not by number and not PulseAudio. There is no PulseAudio
	// in the robot container at all -- the old `pulsesink` default could only
	// ever have failed there -- and the speaker is a USB dongle whose card
	// *number* depends on probe order while its name, derived from the USB
	// descriptor, does not. That is the same rule the workspace's udev file
	// states and the backend's TTS gateway follows.
	//
	// Known collision, deliberately not solved here: plughw is exclusive, and
	// the backend's TTS plays through aplay on the same device. A WHIP session
	// holds the speaker for its whole life, so a /speak during one fails with
	// "Device or resource busy". Sharing would mean moving both sides to dmix.
	Sink string
}

// VideoConfig describes the capture half of the WHEP pipeline.
//
// Every field is an environment variable because the capture side is the part
// that differs per robot: the camera symlink is created by the host's udev
// rules, and the sensor only offers a fixed set of resolution/framerate pairs
// (`v4l2-ctl -d <device> --list-formats-ext`). A combination the sensor does
// not advertise fails as a `not-negotiated` error on the GStreamer bus, which
// is why watchBus() in internal/proc exists.
type VideoConfig struct {
	// Device is the V4L2 node. The default is the udev symlink the robot
	// workspace's 99-syncai-devices.rules creates, NOT /dev/videoN: each
	// camera exposes two nodes and probe order decides which minor numbers
	// they get. Unused by the "rtpjpeg" branch, which opens no device.
	Device string

	// Format selects the capture branch:
	//
	//   - "mjpeg" (default): v4l2src -> nvjpegdec. MJPEG rather than raw for
	//     the same reason the host-side publisher chose it -- UYVY at
	//     1920x1200@60 is 2.2 Gbit/s, at the practical USB 3.0 ceiling.
	//   - "uyvy": v4l2src -> videoconvert.
	//   - "rtpjpeg": no device at all. RTP/JPEG on 127.0.0.1:RTPInPort from
	//     the camera feeder (SyncAI-Robot-Camera-Feeder), which is then the
	//     camera's only opener, so ROS can stream the same camera at the
	//     same time. The JPEG bytes are the camera's own, so everything from
	//     nvjpegdec on is identical to "mjpeg".
	//   - "rtsp": no device either. A camera some other computer already
	//     encodes and serves over RTSP -- the Lynx M20's two cameras, which
	//     its perception board streams as H.265 and which no process on this
	//     host can open. Decoded by nvv4l2decoder and re-encoded by the same
	//     H.264 half as every other branch, rather than passed through: the
	//     WHEP track is H.264 (browsers' H.265 WebRTC support is patchy), and
	//     the worker never answers a PLI, so only an encoder we own can
	//     promise a joining browser an IDR within half a second -- the M20's
	//     own GOP is a second, and not ours to set.
	Format string

	// RTPInPort is where the "rtpjpeg" branch listens for the camera feeder.
	// It must match the feeder's rtp_port parameter.
	RTPInPort int

	// RTSPURL is the stream the "rtsp" branch plays, e.g.
	// rtsp://10.21.31.103:8554/video1 on the M20. No default: a URL names one
	// camera on one robot, and a guessed one would be a black view that
	// looks like a working session. Format "rtsp" with this empty is refused
	// before a pipeline is built.
	RTSPURL string

	// RTSPCodec is what the RTSP stream carries: "h265" (the default, the
	// M20's) or "h264". It picks the depayloader and parser only; the
	// decoder (nvv4l2decoder) takes either.
	RTSPCodec string

	// RTSPLatency is rtspsrc's jitter buffer in milliseconds. rtspsrc's own
	// default is 2000, sized for the internet; the M20's stream crosses one
	// switch over TCP, which neither loses nor reorders, so the buffer only
	// adds delay to the operator's view.
	RTSPLatency int

	Width     int
	Height    int
	Framerate int
	Bitrate   int

	// Sensor controls, written through v4l2src's extra-controls at capture
	// start -- by the two v4l2src branches only. Under "rtpjpeg" the camera
	// feeder owns the device and writes the same controls (with the same
	// defaults) itself, so these are ignored.
	//
	// They are not decorative defaults: UVC control state lives in the
	// camera firmware and survives open/close, so a session that sets nothing
	// inherits whatever the last process left behind. The state found on this
	// fleet was auto_exposure=Manual at 4 ms and manual white balance frozen
	// at 4000 K -- an image both very dark and very blue, which reads as a
	// broken encoder rather than as a sensor setting.
	//
	// Polarity trap, and the two controls disagree: auto_exposure is a menu
	// that inherits the UVC numbering (0 = Auto, 1 = Manual), while
	// white_balance_automatic is a plain boolean (1 = automatic). So the
	// defaults below -- AutoExposure 0 means "automatic"; AutoWhiteBalance 0
	// does NOT, it means manual, and that is deliberate (see below).
	//
	// Both manual values are gated by their auto switch: the driver rejects a
	// write to them while the corresponding automatic mode is on, so they are
	// emitted only when that mode is off. Their read-back is stale in auto
	// mode too, so neither can be used to check whether AE/AWB is working --
	// v4l2-ctl showing `flags=inactive` on them is the signal.
	AutoExposure     int // 0 = Auto Mode, 1 = Manual Mode
	ExposureTime     int // 100 us units; only applied when AutoExposure is 1
	AutoWhiteBalance int // 1 = automatic, 0 = manual
	WhiteBalanceTemp int // Kelvin; only applied when AutoWhiteBalance is 0
	Gain             int
	Brightness       int
	PowerLineFreq    int // 0 = disabled, 1 = 50 Hz, 2 = 60 Hz

	// AudioSource is the complete GStreamer source element for the microphone
	// branch, properties included (e.g. `alsasrc device=hw:2,0
	// do-timestamp=true`). Empty -- the default -- drops the branch entirely,
	// and that is the right default on this fleet twice over: the robot has no
	// capture device at all (`arecord -l` lists only Tegra APE dlinks), and
	// the container ships no alsasrc element, which would fail the whole
	// pipeline string at parse time and take the video with it.
	//
	// The WHEP session still offers an Opus track when this is empty; it just
	// carries no packets. Dropping the track instead would mean renegotiating
	// the m-line count with every client.
	AudioSource string
}

type GStreamerConfig struct {
	AudioRTPPort      int
	VideoRTPPort      int
	VideoAudioRTPPort int
	AudioConfig       AudioConfig
	VideoConfig       VideoConfig
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

	// Unity gain by default: the `volume` element attenuates, it does not
	// amplify below 1.0, and the old 0.2 made a working WHIP session sound
	// like a broken one. Loudness belongs to the speaker's own mixer, not to
	// a constant compiled in here. Values above 1.0 do amplify, and clip.
	audioVolume, err := strconv.ParseFloat(getEnv("AUDIO_VOLUME", "1.0"), 64)
	if err != nil {
		audioVolume = 1.0
	}

	return &GStreamerConfig{
		AudioRTPPort:      audioPort,
		VideoRTPPort:      videoPort,
		VideoAudioRTPPort: videoAudioPort,
		AudioConfig: AudioConfig{
			AudioVolume: audioVolume,
			Sink:        getEnv("AUDIO_SINK", "alsasink device=plughw:CARD=CD002AUDIO"),
		},
		VideoConfig: VideoConfig{
			Device:    getEnv("VIDEO_DEVICE", "/dev/syncai/camera0"),
			Format:    getEnv("VIDEO_FORMAT", "mjpeg"),
			RTPInPort: getEnvInt("VIDEO_RTP_IN_PORT", 5008),
			RTSPURL:   getEnv("VIDEO_RTSP_URL", ""),
			RTSPCodec: getEnv("VIDEO_RTSP_CODEC", "h265"),
			// AllowZero: 0 is a valid jitter buffer (none) on a link that
			// never reorders.
			RTSPLatency: getEnvIntAllowZero("VIDEO_RTSP_LATENCY_MS", 50),
			Width:       getEnvInt("VIDEO_WIDTH", 1280),
			Height:      getEnvInt("VIDEO_HEIGHT", 720),
			Framerate:   getEnvInt("VIDEO_FRAMERATE", 60),
			Bitrate:     getEnvInt("VIDEO_BITRATE", 4000000),

			// getEnvIntAllowZero, not getEnvInt: 0 is a meaningful value for
			// three of these (Auto Mode, manual white balance, flicker
			// correction disabled) and the "<= 0 falls back" rule would make
			// them unsettable.
			AutoExposure: getEnvIntAllowZero("VIDEO_AUTO_EXPOSURE", 0),
			ExposureTime: getEnvIntAllowZero("VIDEO_EXPOSURE", 330),
			// AWB off, and a fixed 5200 K, measured on the robot rather
			// than guessed. The camera's AWB settles distinctly blue under
			// the lab's fluorescent lighting: sampling a white wall and a
			// grey carpet across a 2800-6500 K sweep, auto sat at R/G-B/G
			// = -0.27 (wall) and -0.41 (carpet), where 0 is neutral on the
			// red-blue axis. The zero crossing is 5040 K against the wall
			// and 5410 K against the carpet; 5200 splits them at +0.02 and
			// -0.04, which is closer to neutral than auto ever gets.
			//
			// Fixed rather than automatic for a second reason: AWB retunes
			// as the robot drives and the frame fills with a coloured wall
			// or a lit screen, so the operator's view shifts colour while
			// nothing in the scene changed. A constant is worth more here
			// than per-frame correctness.
			//
			// What this does NOT fix is the green cast -- R/G never reaches
			// 1.0 at any temperature, because the sensor exposes no tint
			// control and the temperature axis only trades red against
			// blue. Re-measure with the sweep above if the lighting
			// changes; do not expect a temperature to remove it.
			AutoWhiteBalance: getEnvIntAllowZero("VIDEO_AUTO_WB", 0),
			WhiteBalanceTemp: getEnvIntAllowZero("VIDEO_WB_TEMP", 5200),
			Gain:             getEnvIntAllowZero("VIDEO_GAIN", 1),
			Brightness:       getEnvIntAllowZero("VIDEO_BRIGHTNESS", 16),
			// 50 Hz by default: the camera ships with flicker correction
			// disabled, and mains lighting then bands the image.
			PowerLineFreq: getEnvIntAllowZero("VIDEO_POWER_LINE_FREQ", 1),

			AudioSource: getEnv("VIDEO_AUDIO_SRC", ""),
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

// getEnvInt reads an integer environment variable.
//
// A value that does not parse, and a value <= 0, both fall back to the
// default: every caller is a pixel count, a framerate or a bitrate, and zero
// would build a pipeline string that cannot negotiate. Same shape as the port
// parsing above, which also prefers its default over a broken value.
func getEnvInt(key string, defaultValue int) int {
	value, err := strconv.Atoi(getEnv(key, ""))
	if err != nil || value <= 0 {
		return defaultValue
	}
	return value
}

// getEnvIntAllowZero is getEnvInt for values where zero is a setting rather
// than a mistake. Only a value that does not parse falls back.
func getEnvIntAllowZero(key string, defaultValue int) int {
	value, err := strconv.Atoi(getEnv(key, ""))
	if err != nil {
		return defaultValue
	}
	return value
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
