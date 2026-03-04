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

type VideoSession struct {
	ID               string
	PC               PeerConnectionInterface
	GStreamer        *proc.VideoGStreamerPipeline
	videoTrack       *webrtc.TrackLocalStaticRTP
	audioTrack       *webrtc.TrackLocalStaticRTP
	videoUDPListener *net.UDPConn
	audioUDPListener *net.UDPConn
	mu               sync.Mutex
	closed           bool
}

var (
	videoSessions   = make(map[string]*VideoSession)
	videoSessionsMu sync.Mutex
)

type VideoSessionOptions struct {
	VideoRTPPort    int
	AudioRTPPort    int
	VideoSource     config.VideoSourceType
	PCFactory       PeerConnectionFactory
	PipelineFactory proc.PipelineFactory
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
		ICEServers: []webrtc.ICEServer{
			{URLs: []string{}},
		},
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

	switch opts.VideoSource {
	case config.VideoSourceCamera:
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
		gst, err := proc.NewVideoGStreamerPipelineWithFactory(opts.VideoRTPPort, opts.AudioRTPPort, pipelineFactory)
		if err != nil {
			session.Close()
			return "", "", fmt.Errorf("failed to start GStreamer: %w", err)
		}
		session.GStreamer = gst
	case config.VideoSourceROS:
		if err := session.startRTPListener(opts.VideoRTPPort, session.videoTrack, &session.videoUDPListener, "video"); err != nil {
			pc.Close()
			return "", "", err
		}
	default:
		pc.Close()
		return "", "", fmt.Errorf("unsupported video source type: %s", opts.VideoSource)
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
		return fmt.Errorf("video session not found: %s", sessionID)
	}

	session.Close()
	return nil
}
