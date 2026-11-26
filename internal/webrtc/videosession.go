package webrtc

import (
	"fmt"
	"net"
	"strings"
	"sync"

	"github.com/pion/rtp"
	"github.com/pion/sdp/v3"
	"github.com/pion/webrtc/v4"

	"SyncAI-WebRTC-Worker/internal/config"
	"SyncAI-WebRTC-Worker/internal/proc"
)

type VideoSession struct {
	ID               string
	PC               PeerConnectionInterface
	GStreamer        *proc.VideoGStreamerPipeline
	videoTrack       *webrtc.TrackLocalStaticRTP
	audioTrack       *webrtc.TrackLocalStaticRTP
	videoUDPListener *net.UDPConn
	audioUDPListener *net.UDPConn

	// The second camera (VideoConfig.RTSPURL2), nil unless the session sends
	// one. Its own pipeline, not a branch of the first: an ERROR from either
	// rtspsrc stops the whole pipeline it is in, and the rear camera
	// dropping out must not take the front one with it.
	GStreamer2        *proc.VideoGStreamerPipeline
	videoTrack2       *webrtc.TrackLocalStaticRTP
	videoUDPListener2 *net.UDPConn

	mu     sync.Mutex
	closed bool
}

var (
	videoSessions   = make(map[string]*VideoSession)
	videoSessionsMu sync.Mutex
)

type VideoSessionOptions struct {
	VideoRTPPort    int
	AudioRTPPort    int
	VideoRTPPort2   int // the second camera's RTP; unused without RTSPURL2
	VideoConfig     config.VideoConfig
	PCFactory       PeerConnectionFactory
	PipelineFactory proc.PipelineFactory
	ICEServers      []webrtc.ICEServer
}

func CreateVideoSession(offerSDP string, opts VideoSessionOptions) (string, string, error) {
	factory := opts.PCFactory
	if factory == nil {
		factory = defaultPCFactory
	}

	mediaEngine := &webrtc.MediaEngine{}
	if err := mediaEngine.RegisterDefaultCodecs(); err != nil {
		return "", "", fmt.Errorf("failed to register codecs: %w", err)
	}

	api := webrtc.NewAPI(webrtc.WithMediaEngine(mediaEngine))

	rtcPC, err := api.NewPeerConnection(webrtc.Configuration{
		ICEServers: opts.ICEServers,
	})
	if err != nil {
		return "", "", fmt.Errorf("failed to create peer connection: %w", err)
	}

	pc := &RealPeerConnection{PeerConnection: rtcPC}

	sessionID := generateSessionID()
	session := &VideoSession{
		ID: sessionID,
		PC: pc,
	}

	// Video track
	videoCodec := webrtc.RTPCodecCapability{MimeType: webrtc.MimeTypeH264, ClockRate: 90000}
	videoTrack, err := webrtc.NewTrackLocalStaticRTP(videoCodec, "video", "gstreamer-video")
	if err != nil {
		pc.Close()
		return "", "", fmt.Errorf("failed to create video track: %w", err)
	}
	session.videoTrack = videoTrack

	if _, err := pc.AddTrack(videoTrack); err != nil {
		pc.Close()
		return "", "", fmt.Errorf("failed to add video track: %w", err)
	}

	// Audio track
	audioCodec := webrtc.RTPCodecCapability{MimeType: webrtc.MimeTypeOpus, ClockRate: 48000, Channels: 2}
	audioTrack, err := webrtc.NewTrackLocalStaticRTP(audioCodec, "audio", "gstreamer-audio")
	if err != nil {
		pc.Close()
		return "", "", fmt.Errorf("failed to create audio track: %w", err)
	}
	session.audioTrack = audioTrack

	if _, err := pc.AddTrack(audioTrack); err != nil {
		pc.Close()
		return "", "", fmt.Errorf("failed to add audio track: %w", err)
	}

	// The second camera, added last so that the offer's first video m-line
	// is the first camera whatever the client's m-line order: Pion hands
	// each offered m-line the first unused transceiver of its kind, in the
	// order the tracks were added. Only when the offer has room for it --
	// an answerer cannot add an m-line, so a track the offer has no line
	// for is never sent, and its pipeline would decode and encode a camera
	// for nobody.
	secondCamera := wantsSecondCamera(opts.VideoConfig, offerSDP)
	if secondCamera {
		videoTrack2, err := webrtc.NewTrackLocalStaticRTP(videoCodec, "video2", "gstreamer-video2")
		if err != nil {
			pc.Close()
			return "", "", fmt.Errorf("failed to create second video track: %w", err)
		}
		session.videoTrack2 = videoTrack2

		if _, err := pc.AddTrack(videoTrack2); err != nil {
			pc.Close()
			return "", "", fmt.Errorf("failed to add second video track: %w", err)
		}
	}

	if err := session.startRTPListener(opts.VideoRTPPort, session.videoTrack, &session.videoUDPListener, "video"); err != nil {
		pc.Close()
		return "", "", err
	}

	if err := session.startRTPListener(opts.AudioRTPPort, session.audioTrack, &session.audioUDPListener, "audio"); err != nil {
		session.Close()
		return "", "", err
	}

	pipelineFactory := opts.PipelineFactory
	if pipelineFactory == nil {
		pipelineFactory = &proc.DefaultPipelineFactory{}
	}
	gst, err := proc.NewVideoGStreamerPipelineWithFactory(opts.VideoRTPPort, opts.AudioRTPPort, opts.VideoConfig, pipelineFactory)
	if err != nil {
		session.Close()
		return "", "", fmt.Errorf("failed to start GStreamer: %w", err)
	}
	session.GStreamer = gst

	if secondCamera {
		if err := session.startRTPListener(opts.VideoRTPPort2, session.videoTrack2, &session.videoUDPListener2, "video2"); err != nil {
			session.Close()
			return "", "", err
		}

		// No audio port: the microphone branch rides on the first pipeline.
		gst2, err := proc.NewVideoGStreamerPipelineWithFactory(opts.VideoRTPPort2, 0, secondCameraConfig(opts.VideoConfig), pipelineFactory)
		if err != nil {
			session.Close()
			return "", "", fmt.Errorf("failed to start GStreamer for the second camera: %w", err)
		}
		session.GStreamer2 = gst2
	}

	pc.OnConnectionStateChange(func(state webrtc.PeerConnectionState) {
		fmt.Printf("Video session %s: %s\n", sessionID, state.String())

		if state == webrtc.PeerConnectionStateClosed ||
			state == webrtc.PeerConnectionStateFailed {
			go session.Close()
		}
	})

	offer := webrtc.SessionDescription{Type: webrtc.SDPTypeOffer, SDP: offerSDP}
	if err := pc.SetRemoteDescription(offer); err != nil {
		session.Close()
		return "", "", fmt.Errorf("failed to set remote description: %w", err)
	}

	answer, err := pc.CreateAnswer(nil)
	if err != nil {
		session.Close()
		return "", "", fmt.Errorf("failed to create answer: %w", err)
	}

	// Use Underlying() to get the real PeerConnection for GatheringCompletePromise
	if underlying := pc.Underlying(); underlying != nil {
		gatherComplete := webrtc.GatheringCompletePromise(underlying)
		if err := pc.SetLocalDescription(answer); err != nil {
			session.Close()
			return "", "", fmt.Errorf("failed to set local description: %w", err)
		}
		<-gatherComplete
	} else {
		// Mock environment: skip gathering
		if err := pc.SetLocalDescription(answer); err != nil {
			session.Close()
			return "", "", fmt.Errorf("failed to set local description: %w", err)
		}
	}

	videoSessionsMu.Lock()
	videoSessions[sessionID] = session
	videoSessionsMu.Unlock()

	return sessionID, pc.LocalDescription().SDP, nil
}

func (s *VideoSession) startRTPListener(port int, track *webrtc.TrackLocalStaticRTP, conn **net.UDPConn, trackType string) error {
	addr, err := net.ResolveUDPAddr("udp", fmt.Sprintf("127.0.0.1:%d", port))
	if err != nil {
		return err
	}

	udpConn, err := net.ListenUDP("udp", addr)
	if err != nil {
		return fmt.Errorf("failed to listen on UDP port %d: %w", port, err)
	}
	*conn = udpConn

	if err := udpConn.SetReadBuffer(1024 * 1024); err != nil {
		return fmt.Errorf("failed to set UDP read buffer: %w", err)
	}

	fmt.Printf("Video session %s: listening for %s RTP on port %d\n", s.ID, trackType, port)

	go func() {
		buf := make([]byte, 1500)
		rtpPacket := &rtp.Packet{}

		for {
			s.mu.Lock()
			closed := s.closed
			s.mu.Unlock()
			if closed {
				return
			}

			n, _, err := udpConn.ReadFromUDP(buf)
			if err != nil {
				s.mu.Lock()
				closed = s.closed
				s.mu.Unlock()
				if !closed {
					fmt.Printf("Video session %s: %s UDP read error: %v\n", s.ID, trackType, err)
				}
				return
			}

			if err := rtpPacket.Unmarshal(buf[:n]); err != nil {
				fmt.Printf("Video session %s: %s RTP unmarshal error: %v\n", s.ID, trackType, err)
				continue
			}

			if err := track.WriteRTP(rtpPacket); err != nil {
				fmt.Printf("Video session %s: %s WriteRTP error: %v\n", s.ID, trackType, err)
			}
		}
	}()

	return nil
}

func (s *VideoSession) Close() {
	s.mu.Lock()
	if s.closed {
		s.mu.Unlock()
		return
	}
	s.closed = true
	s.mu.Unlock()

	fmt.Printf("Video session %s: closing\n", s.ID)

	if s.GStreamer != nil {
		s.GStreamer.Close()
	}
	if s.GStreamer2 != nil {
		s.GStreamer2.Close()
	}
	if s.videoUDPListener2 != nil {
		s.videoUDPListener2.Close()
	}
	if s.videoUDPListener != nil {
		s.videoUDPListener.Close()
	}
	if s.audioUDPListener != nil {
		s.audioUDPListener.Close()
	}
	if s.PC != nil {
		s.PC.Close()
	}

	videoSessionsMu.Lock()
	delete(videoSessions, s.ID)
	videoSessionsMu.Unlock()

	fmt.Printf("Video session %s: closed\n", s.ID)
}

func DeleteVideoSession(sessionID string) error {
	videoSessionsMu.Lock()
	session, exists := videoSessions[sessionID]
	videoSessionsMu.Unlock()

	if !exists {
		fmt.Printf("Video session %s not found, might be closed.\n", sessionID)
		return nil
	}

	session.Close()
	return nil
}

// wantsSecondCamera reports whether a session should send the second camera:
// one is configured (an "rtsp" source with RTSPURL2) and the client's offer
// has a second video m-line to receive it on.
func wantsSecondCamera(cfg config.VideoConfig, offerSDP string) bool {
	if !strings.EqualFold(cfg.Format, "rtsp") || cfg.RTSPURL2 == "" {
		return false
	}
	return countVideoMLines(offerSDP) >= 2
}

// countVideoMLines counts the offer's video m-lines that can carry media.
// An inactive one (a stopped transceiver, or a rejected line kept to hold
// its place) cannot. An offer that does not parse counts as one: the
// session then goes on as it always did, and SetRemoteDescription reports
// the real problem.
func countVideoMLines(offerSDP string) int {
	var desc sdp.SessionDescription
	if err := desc.Unmarshal([]byte(offerSDP)); err != nil {
		return 1
	}

	n := 0
	for _, m := range desc.MediaDescriptions {
		if m.MediaName.Media != "video" {
			continue
		}
		if _, inactive := m.Attribute("inactive"); inactive {
			continue
		}
		n++
	}
	return n
}

// secondCameraConfig is the first camera's configuration pointed at the
// second stream, with no microphone branch.
func secondCameraConfig(cfg config.VideoConfig) config.VideoConfig {
	cfg.RTSPURL = cfg.RTSPURL2
	cfg.AudioSource = ""
	return cfg
}
