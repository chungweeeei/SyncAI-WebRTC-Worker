package webrtc

import (
	"fmt"
	"net"
	"sync"

	"github.com/pion/rtp"
	"github.com/pion/webrtc/v4"

	"SyncAI-WebRTC-Worker/internal/proc"
)

type VideoSession struct {
	ID          string
	PC          *webrtc.PeerConnection
	GStreamer   *proc.VideoGStreamerPipeline
	videoTrack  *webrtc.TrackLocalStaticRTP
	udpListener *net.UDPConn
	mu          sync.Mutex
	closed      bool
}

var (
	videoSessions   = make(map[string]*VideoSession)
	videoSessionsMu sync.Mutex
)

type VideoSessionOptions struct {
	RTPPort int
}

// CreateVideoSession 建立 WHEP video session
func CreateVideoSession(offerSDP string, opts VideoSessionOptions) (string, string, error) {
	if opts.RTPPort == 0 {
		opts.RTPPort = 5006 // default RTP port for video
	}

	mediaEngine := &webrtc.MediaEngine{}
	if err := mediaEngine.RegisterDefaultCodecs(); err != nil {
		return "", "", fmt.Errorf("failed to register codecs: %w", err)
	}

	api := webrtc.NewAPI(webrtc.WithMediaEngine(mediaEngine))

	pc, err := api.NewPeerConnection(webrtc.Configuration{
		ICEServers: []webrtc.ICEServer{
			{URLs: []string{"stun:stun.l.google.com:19302"}},
		},
	})
	if err != nil {
		return "", "", fmt.Errorf("failed to create peer connection: %w", err)
	}

	sessionID := generateSessionID()
	session := &VideoSession{
		ID: sessionID,
		PC: pc,
	}

	// Video track
	codecCapability := webrtc.RTPCodecCapability{MimeType: webrtc.MimeTypeH264, ClockRate: 90000}
	videoTrack, err := webrtc.NewTrackLocalStaticRTP(codecCapability, "video", "gstreamer")
	if err != nil {
		pc.Close()
		return "", "", fmt.Errorf("failed to create video track: %w", err)
	}
	session.videoTrack = videoTrack

	if _, err := pc.AddTrack(videoTrack); err != nil {
		pc.Close()
		return "", "", fmt.Errorf("failed to add video track: %w", err)
	}

	// 啟動 UDP 監聽接收 GStreamer RTP
	if err := session.startRTPListener(opts.RTPPort); err != nil {
		pc.Close()
		return "", "", err
	}

	// 啟動 GStreamer pipeline
	gst, err := proc.NewVideoGStreamerPipeline(opts.RTPPort)
	if err != nil {
		session.Close()
		return "", "", fmt.Errorf("failed to start GStreamer: %w", err)
	}
	session.GStreamer = gst
	fmt.Printf("Video session %s: GStreamer pipeline started\n", sessionID)

	pc.OnConnectionStateChange(func(state webrtc.PeerConnectionState) {
		fmt.Printf("Video session %s: %s\n", sessionID, state.String())

		if state == webrtc.PeerConnectionStateClosed ||
			state == webrtc.PeerConnectionStateFailed {
			go session.Close()
		}
	})

	// SDP 交換
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

	gatherComplete := webrtc.GatheringCompletePromise(pc)
	if err := pc.SetLocalDescription(answer); err != nil {
		session.Close()
		return "", "", fmt.Errorf("failed to set local description: %w", err)
	}
	<-gatherComplete

	// 儲存 session
	videoSessionsMu.Lock()
	videoSessions[sessionID] = session
	videoSessionsMu.Unlock()

	return sessionID, pc.LocalDescription().SDP, nil
}

func (s *VideoSession) startRTPListener(port int) error {
	addr, err := net.ResolveUDPAddr("udp", fmt.Sprintf("127.0.0.1:%d", port))
	if err != nil {
		return err
	}

	conn, err := net.ListenUDP("udp", addr)
	if err != nil {
		return fmt.Errorf("failed to listen on UDP port %d: %w", port, err)
	}
	s.udpListener = conn

	if err := conn.SetReadBuffer(1024 * 1024); err != nil {
		return fmt.Errorf("failed to set UDP read buffer: %w", err)
	}

	fmt.Printf("Video session %s: listening for RTP on port %d\n", s.ID, port)

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

			n, _, err := conn.ReadFromUDP(buf)
			if err != nil {
				s.mu.Lock()
				closed = s.closed
				s.mu.Unlock()
				if !closed {
					fmt.Printf("Video session %s: UDP read error: %v\n", s.ID, err)
				}
				return
			}
			if err := rtpPacket.Unmarshal(buf[:n]); err != nil {
				fmt.Printf("Video session %s: RTP unmarshal error: %v\n", s.ID, err)
				continue
			}

			// fmt.Printf("Video session %s: received RTP packet, size=%d\n", s.ID, n)
			if err := s.videoTrack.WriteRTP(rtpPacket); err != nil {
				fmt.Printf("Video session %s: WriteRTP error: %v\n", s.ID, err)
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

	if s.udpListener != nil {
		s.udpListener.Close()
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
