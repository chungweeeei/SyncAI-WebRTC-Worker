package webrtc

import (
	"fmt"
	"net"
	"sync"

	"github.com/pion/rtp"
	"github.com/pion/webrtc/v4"

	"SyncAI-WebRTC-Worker/internal/config"
	"SyncAI-WebRTC-Worker/internal/proc"
)

// DuplexSession carries both directions over ONE peer connection: the robot's
// camera out to the browser, and the browser's microphone in to the robot's
// speaker.
//
// It exists alongside the WHEP and WHIP sessions rather than replacing them,
// and the reason is device contention rather than protocol taste. The camera
// and the speaker are one device each, so a duplex session and a WHEP session
// cannot both run -- the host is what enforces that, because only the host
// knows what it has handed out. What this type buys over running the two
// single-direction sessions together is one ICE/DTLS handshake instead of two,
// one candidate-gathering wait instead of two sequential ones, and a failure
// mode where both directions live or die together rather than one connecting
// while the other does not.
//
// The offer shape it expects, and the frontend produces:
//
//	m=video  recvonly   the browser receives the camera
//	m=audio  sendrecv   the browser sends its microphone AND receives the
//	                    robot's audio track on the same transceiver
//
// The second line is the whole difference from WHEP+WHIP. One bidirectional
// audio m-line is what a browser produces naturally from an addTransceiver
// with direction "sendrecv"; asking for two separate audio m-lines instead
// would work too, but it is not what any WebRTC client does by default.
type DuplexSession struct {
	ID string
	PC PeerConnectionInterface

	// Camera capture and encode, feeding videoTrack/audioTrack over UDP.
	Camera *proc.VideoGStreamerPipeline
	// Opus decode and playback, fed by micConn from the browser's track.
	Speaker *proc.AudioGStreamerPipeline

	videoTrack *webrtc.TrackLocalStaticRTP
	audioTrack *webrtc.TrackLocalStaticRTP

	// Inbound from GStreamer, written to the outgoing tracks.
	videoUDPListener *net.UDPConn
	audioUDPListener *net.UDPConn
	// Outbound to GStreamer, carrying the browser's microphone.
	micConn *net.UDPConn

	mu     sync.Mutex
	closed bool
}

var (
	duplexSessions   = make(map[string]*DuplexSession)
	duplexSessionsMu sync.Mutex
)

type DuplexSessionOptions struct {
	// Camera out.
	VideoRTPPort int
	AudioRTPPort int
	VideoConfig  config.VideoConfig

	// Microphone in.
	MicRTPPort int
	AudioCfg   config.AudioConfig

	PCFactory       PeerConnectionFactory
	PipelineFactory proc.PipelineFactory
	ICEServers      []webrtc.ICEServer
}

// CreateDuplexSession negotiates a bidirectional session from an SDP offer.
// Blocks until ICE gathering completes.
func CreateDuplexSession(offerSDP string, opts DuplexSessionOptions) (string, string, error) {
	factory := opts.PCFactory
	if factory == nil {
		factory = defaultPCFactory
	}

	mediaEngine := &webrtc.MediaEngine{}
	if err := mediaEngine.RegisterDefaultCodecs(); err != nil {
		return "", "", fmt.Errorf("failed to register codecs: %w", err)
	}

	api := webrtc.NewAPI(webrtc.WithMediaEngine(mediaEngine))

	rtcPC, err := api.NewPeerConnection(webrtc.Configuration{ICEServers: opts.ICEServers})
	if err != nil {
		return "", "", fmt.Errorf("failed to create peer connection: %w", err)
	}

	pc := &RealPeerConnection{PeerConnection: rtcPC}

	sessionID := generateSessionID()
	session := &DuplexSession{ID: sessionID, PC: pc}

	// --- Outbound: the camera ------------------------------------------------
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

	if err := session.startRTPListener(opts.VideoRTPPort, videoTrack, &session.videoUDPListener, "video"); err != nil {
		session.Close()
		return "", "", err
	}
	if err := session.startRTPListener(opts.AudioRTPPort, audioTrack, &session.audioUDPListener, "audio"); err != nil {
		session.Close()
		return "", "", err
	}

	pipelineFactory := opts.PipelineFactory
	if pipelineFactory == nil {
		pipelineFactory = &proc.DefaultPipelineFactory{}
	}

	camera, err := proc.NewVideoGStreamerPipelineWithFactory(
		opts.VideoRTPPort, opts.AudioRTPPort, opts.VideoConfig, pipelineFactory)
	if err != nil {
		session.Close()
		return "", "", fmt.Errorf("failed to start camera GStreamer: %w", err)
	}
	session.Camera = camera

	// --- Inbound: the browser's microphone -----------------------------------
	//
	// The playback pipeline comes up before the socket that feeds it, so the
	// udpsrc is already bound when the first packet arrives. The reverse order
	// would drop the opening milliseconds of speech into a closed port.
	speaker, err := proc.NewAudioGStreamerPipelineWithFactory(
		opts.MicRTPPort, opts.AudioCfg, pipelineFactory)
	if err != nil {
		session.Close()
		return "", "", fmt.Errorf("failed to start speaker GStreamer: %w", err)
	}
	session.Speaker = speaker

	micAddr, err := net.ResolveUDPAddr("udp", fmt.Sprintf("127.0.0.1:%d", opts.MicRTPPort))
	if err != nil {
		session.Close()
		return "", "", err
	}
	micConn, err := net.DialUDP("udp", nil, micAddr)
	if err != nil {
		session.Close()
		return "", "", fmt.Errorf("failed to create microphone UDP connection: %w", err)
	}
	session.micConn = micConn

	pc.OnTrack(func(track *webrtc.TrackRemote, receiver *webrtc.RTPReceiver) {
		if track.Kind() != webrtc.RTPCodecTypeAudio {
			// Nothing consumes inbound video: the robot has no screen. A
			// browser that offers one gets it ignored rather than refused.
			fmt.Printf("Duplex session %s: ignoring inbound %s track\n", sessionID, track.Kind())
			return
		}

		fmt.Printf("Duplex session %s: microphone track received (codec: %s)\n",
			sessionID, track.Codec().MimeType)

		// Raw RTP straight through to GStreamer, payload type included: the
		// udpsrc caps name payload 111, which is what both Chrome and pion's
		// default media engine negotiate for Opus.
		go func() {
			buf := make([]byte, 1500)
			for {
				n, _, err := track.Read(buf)
				if err != nil {
					session.mu.Lock()
					closed := session.closed
					session.mu.Unlock()
					if !closed {
						fmt.Printf("Duplex session %s: microphone read error: %v\n", sessionID, err)
					}
					return
				}

				session.mu.Lock()
				conn := session.micConn
				closed := session.closed
				session.mu.Unlock()
				if closed || conn == nil {
					return
				}
				if _, err := conn.Write(buf[:n]); err != nil {
					fmt.Printf("Duplex session %s: microphone write error: %v\n", sessionID, err)
				}
			}
		}()
	})

	pc.OnConnectionStateChange(func(state webrtc.PeerConnectionState) {
		fmt.Printf("Duplex session %s: %s\n", sessionID, state.String())

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

	duplexSessionsMu.Lock()
	duplexSessions[sessionID] = session
	duplexSessionsMu.Unlock()

	return sessionID, pc.LocalDescription().SDP, nil
}

// startRTPListener pumps one GStreamer UDP stream into one outgoing track.
// Same shape as the WHEP session's, kept separate rather than shared because
// the two log with their own session kind and that is the first thing anyone
// greps for.
func (s *DuplexSession) startRTPListener(port int, track *webrtc.TrackLocalStaticRTP, conn **net.UDPConn, trackType string) error {
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

	fmt.Printf("Duplex session %s: listening for %s RTP on port %d\n", s.ID, trackType, port)

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
					fmt.Printf("Duplex session %s: %s UDP read error: %v\n", s.ID, trackType, err)
				}
				return
			}

			if err := rtpPacket.Unmarshal(buf[:n]); err != nil {
				fmt.Printf("Duplex session %s: %s RTP unmarshal error: %v\n", s.ID, trackType, err)
				continue
			}

			if err := track.WriteRTP(rtpPacket); err != nil {
				fmt.Printf("Duplex session %s: %s WriteRTP error: %v\n", s.ID, trackType, err)
			}
		}
	}()

	return nil
}

// Close releases both devices. Idempotent, and safe to call from the
// connection-state callback as well as from DeleteDuplexSession.
func (s *DuplexSession) Close() {
	s.mu.Lock()
	if s.closed {
		s.mu.Unlock()
		return
	}
	s.closed = true
	s.mu.Unlock()

	fmt.Printf("Duplex session %s: closing\n", s.ID)

	// Pipelines first, and both of them: each blocks until its elements reach
	// NULL, which is what actually hands the camera and the ALSA device back.
	// Leaking either wedges the next session, and the host cannot unload this
	// library to recover.
	if s.Camera != nil {
		s.Camera.Close()
	}
	if s.Speaker != nil {
		s.Speaker.Close()
	}
	if s.videoUDPListener != nil {
		s.videoUDPListener.Close()
	}
	if s.audioUDPListener != nil {
		s.audioUDPListener.Close()
	}
	if s.micConn != nil {
		s.micConn.Close()
	}
	if s.PC != nil {
		s.PC.Close()
	}

	duplexSessionsMu.Lock()
	delete(duplexSessions, s.ID)
	duplexSessionsMu.Unlock()

	fmt.Printf("Duplex session %s: closed\n", s.ID)
}

// DeleteDuplexSession closes a session by ID.
//
// Idempotent, matching the WHEP export rather than the WHIP one: a duplex
// session self-closes on connection failure, so an id that is already gone is
// the normal case for a browser that dropped off the network, not an error the
// caller can act on.
func DeleteDuplexSession(sessionID string) error {
	duplexSessionsMu.Lock()
	session, exists := duplexSessions[sessionID]
	duplexSessionsMu.Unlock()

	if !exists {
		return nil
	}

	session.Close()
	return nil
}
