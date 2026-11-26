package webrtc

import (
	"net"
	"strings"
	"testing"

	"github.com/go-gst/go-gst/gst"
	"github.com/pion/webrtc/v4"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/mock"
	"github.com/stretchr/testify/require"

	"SyncAI-WebRTC-Worker/internal/config"
)

// sdpWith builds a minimal offer with one m-line per entry, each "video" or
// "audio" optionally followed by a direction ("video inactive").
func sdpWith(lines ...string) string {
	var b strings.Builder
	b.WriteString("v=0\r\no=- 1 1 IN IP4 127.0.0.1\r\ns=-\r\nt=0 0\r\n")
	for _, l := range lines {
		kind, dir, _ := strings.Cut(l, " ")
		if dir == "" {
			dir = "recvonly"
		}
		pt := "96"
		if kind == "audio" {
			pt = "111"
		}
		b.WriteString("m=" + kind + " 9 UDP/TLS/RTP/SAVPF " + pt + "\r\n")
		b.WriteString("c=IN IP4 0.0.0.0\r\na=" + dir + "\r\n")
	}
	return b.String()
}

func TestCountVideoMLines(t *testing.T) {
	assert.Equal(t, 0, countVideoMLines(sdpWith("audio")))
	assert.Equal(t, 1, countVideoMLines(sdpWith("video", "audio")))
	assert.Equal(t, 2, countVideoMLines(sdpWith("video", "audio", "video")))
	assert.Equal(t, 2, countVideoMLines(sdpWith("video", "video", "audio")))
	assert.Equal(t, 1, countVideoMLines(sdpWith("video", "video inactive", "audio")),
		"an inactive line cannot carry the second camera")
	assert.Equal(t, 1, countVideoMLines("not sdp"),
		"an unparsable offer goes on as one camera; SetRemoteDescription reports it")
}

func rtspConfig(url2 string) config.VideoConfig {
	return config.VideoConfig{
		Format:      "rtsp",
		RTSPURL:     "rtsp://cam/video1",
		RTSPURL2:    url2,
		RTSPCodec:   "h265",
		RTSPLatency: 50,
		Width:       1280,
		Height:      720,
		Framerate:   30,
		Bitrate:     4000000,
	}
}

func TestWantsSecondCamera(t *testing.T) {
	two := sdpWith("video", "audio", "video")

	assert.True(t, wantsSecondCamera(rtspConfig("rtsp://cam/video2"), two))
	assert.False(t, wantsSecondCamera(rtspConfig(""), two), "none configured")
	assert.False(t, wantsSecondCamera(rtspConfig("rtsp://cam/video2"), sdpWith("video", "audio")),
		"today's console offers one video line")

	cfg := rtspConfig("rtsp://cam/video2")
	cfg.Format = "rtpjpeg"
	assert.False(t, wantsSecondCamera(cfg, two), "a second camera is an rtsp source's only")
}

func TestSecondCameraConfig(t *testing.T) {
	cfg := rtspConfig("rtsp://cam/video2")
	cfg.AudioSource = "alsasrc"

	got := secondCameraConfig(cfg)

	assert.Equal(t, "rtsp://cam/video2", got.RTSPURL)
	assert.Equal(t, "", got.AudioSource, "the microphone rides on the first pipeline")
	assert.Equal(t, "rtsp://cam/video1", cfg.RTSPURL, "the caller's config is untouched")
}

// freeUDPPort returns a loopback port nothing is bound to right now.
func freeUDPPort(t *testing.T) int {
	t.Helper()
	conn, err := net.ListenUDP("udp", &net.UDPAddr{IP: net.IPv4(127, 0, 0, 1)})
	require.NoError(t, err)
	port := conn.LocalAddr().(*net.UDPAddr).Port
	require.NoError(t, conn.Close())
	return port
}

// browserOffer is what a browser sends for `videoLines` recvonly video
// transceivers plus one recvonly audio, made by a real Pion offerer so that
// the answerer's m-line matching is the real one.
func browserOffer(t *testing.T, videoLines int) string {
	t.Helper()
	pc, err := webrtc.NewPeerConnection(webrtc.Configuration{})
	require.NoError(t, err)
	t.Cleanup(func() { pc.Close() })

	recv := webrtc.RTPTransceiverInit{Direction: webrtc.RTPTransceiverDirectionRecvonly}
	for i := 0; i < videoLines; i++ {
		_, err := pc.AddTransceiverFromKind(webrtc.RTPCodecTypeVideo, recv)
		require.NoError(t, err)
	}
	_, err = pc.AddTransceiverFromKind(webrtc.RTPCodecTypeAudio, recv)
	require.NoError(t, err)

	offer, err := pc.CreateOffer(nil)
	require.NoError(t, err)
	return offer.SDP
}

// createWithCameras runs CreateVideoSession against a mock GStreamer and
// returns the pipeline strings it built, in order, and the answer.
func createWithCameras(t *testing.T, cfg config.VideoConfig, videoLines int) ([]string, string) {
	t.Helper()
	pipeline := new(MockProcPipeline)
	pipeline.On("SetState", gst.StatePlaying).Return(nil)
	pipeline.On("BlockSetState", gst.StateNull).Return(nil)

	var built []string
	factory := new(MockProcPipelineFactory)
	factory.On("NewPipelineFromString", mock.Anything).
		Run(func(args mock.Arguments) { built = append(built, args.String(0)) }).
		Return(pipeline, nil)

	id, answer, err := CreateVideoSession(browserOffer(t, videoLines), VideoSessionOptions{
		VideoRTPPort:    freeUDPPort(t),
		AudioRTPPort:    freeUDPPort(t),
		VideoRTPPort2:   freeUDPPort(t),
		VideoConfig:     cfg,
		PipelineFactory: factory,
	})
	require.NoError(t, err)
	t.Cleanup(func() { _ = DeleteVideoSession(id) })
	return built, answer
}

// sendingVideoLines counts the answer's video m-lines that carry a camera:
// the ones with a track (a=msid). Not the direction -- Pion answers an
// offered line it has no track for as sendonly too, and sends nothing on it.
func sendingVideoLines(answer string) int {
	n := 0
	for _, section := range strings.Split(answer, "m=")[1:] {
		if strings.HasPrefix(section, "video") && strings.Contains(section, "a=msid:") {
			n++
		}
	}
	return n
}

func TestCreateVideoSession_SecondCameraOnASecondVideoLine(t *testing.T) {
	built, answer := createWithCameras(t, rtspConfig("rtsp://cam/video2"), 2)

	require.Len(t, built, 2, "one pipeline per camera")
	assert.Contains(t, built[0], `location="rtsp://cam/video1"`)
	assert.Contains(t, built[1], `location="rtsp://cam/video2"`)
	assert.NotContains(t, built[1], "opusenc", "no microphone branch on the second camera")
	assert.Equal(t, 2, sendingVideoLines(answer))
}

// Today's console offers one video line: it gets the first camera alone and
// the second pipeline never runs, exactly as before.
func TestCreateVideoSession_OneVideoLineGetsTheFirstCameraOnly(t *testing.T) {
	built, answer := createWithCameras(t, rtspConfig("rtsp://cam/video2"), 1)

	require.Len(t, built, 1)
	assert.Contains(t, built[0], `location="rtsp://cam/video1"`)
	assert.Equal(t, 1, sendingVideoLines(answer))
}

func TestCreateVideoSession_NoSecondCameraConfigured(t *testing.T) {
	built, answer := createWithCameras(t, rtspConfig(""), 2)

	require.Len(t, built, 1)
	assert.Equal(t, 1, sendingVideoLines(answer),
		"the offer's second video line is answered without a track")
}
