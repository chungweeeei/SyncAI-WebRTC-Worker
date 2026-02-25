package webrtc

import (
	"fmt"
	"net"
	"sync"

	"github.com/pion/rtp"
	"github.com/pion/webrtc/v4"
)

type VideoSession struct {
	ID          string
	PC          *webrtc.PeerConnection
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
	RTPListenPort int
	VideoCodec    string // "vp8", "vp9", "h264"
}

// CreateVideoSession 建立 WHEP video session
// WHEP: 客戶端發送 offer，伺服器回應 answer
func CreateVideoSession(offerSDP string, opts VideoSessionOptions) (string, string, error) {
	if opts.RTPListenPort == 0 {
		opts.RTPListenPort = 5006 // default RTP port for video
	}
	if opts.VideoCodec == "" {
		opts.VideoCodec = "h264" // default video codec
	}

	// setting MediaEngine 支援指定的 codec
	mediaEngine := &webrtc.MediaEngine{}
	if err := registerVideoCodec(mediaEngine, opts.VideoCodec); err != nil {
		return "", "", fmt.Errorf("failed to register video codec: %w", err)
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
	codecCapbility := getCodecCapability(opts.VideoCodec)
	videoTrack, err := webrtc.NewTrackLocalStaticRTP(codecCapbility, "video", "gstreamer")
	if err != nil {
		pc.Close()
		return "", "", fmt.Errorf("failed to create video track: %w", err)
	}
	session.videoTrack = videoTrack

	// 加入 Track 到 PeerConnection
	if _, err := pc.AddTrack(videoTrack); err != nil {
		pc.Close()
		return "", "", fmt.Errorf("failed to add video track: %w", err)
	}

	// 啟動 UDP 監聽接收 GStreamer RTP
	if err := session.startRTPListener(opts.RTPListenPort); err != nil {
		pc.Close()
		return "", "", err
	}

	pc.OnConnectionStateChange(func(state webrtc.PeerConnectionState) {
		fmt.Printf("Video session %s: %s\n", sessionID, state.String())

		if state == webrtc.PeerConnectionStateClosed ||
			state == webrtc.PeerConnectionStateFailed ||
			state == webrtc.PeerConnectionStateDisconnected {
			session.Close()
		}
	})

	// SDP 交換
	offer := webrtc.SessionDescription{Type: webrtc.SDPTypeOffer, SDP: offerSDP}
	if err := pc.SetRemoteDescription(offer); err != nil {
		session.Close()
		return "", "", err
	}

	answer, err := pc.CreateAnswer(nil)
	if err != nil {
		session.Close()
		return "", "", err
	}

	gatherComplete := webrtc.GatheringCompletePromise(pc)
	if err := pc.SetLocalDescription(answer); err != nil {
		session.Close()
		return "", "", err
	}
	<-gatherComplete

	// 儲存 session
	videoSessionsMu.Lock()
	videoSessions[sessionID] = session
	videoSessionsMu.Unlock()

	return sessionID, pc.LocalDescription().SDP, nil
}

func registerVideoCodec(m *webrtc.MediaEngine, codec string) error {
	switch codec {
	case "vp8":
		return m.RegisterCodec(webrtc.RTPCodecParameters{
			RTPCodecCapability: webrtc.RTPCodecCapability{
				MimeType:    webrtc.MimeTypeVP8,
				ClockRate:   90000,
				SDPFmtpLine: "",
			},
			PayloadType: 96,
		}, webrtc.RTPCodecTypeVideo)
	case "vp9":
		return m.RegisterCodec(webrtc.RTPCodecParameters{
			RTPCodecCapability: webrtc.RTPCodecCapability{
				MimeType:    webrtc.MimeTypeVP9,
				ClockRate:   90000,
				SDPFmtpLine: "profile-id=0",
			},
			PayloadType: 96,
		}, webrtc.RTPCodecTypeVideo)
	case "h264":
		return m.RegisterCodec(webrtc.RTPCodecParameters{
			RTPCodecCapability: webrtc.RTPCodecCapability{
				MimeType:    webrtc.MimeTypeH264,
				ClockRate:   90000,
				SDPFmtpLine: "level-asymmetry-allowed=1;packetization-mode=1;profile-level-id=42001f",
			},
			PayloadType: 102,
		}, webrtc.RTPCodecTypeVideo)
	default:
		return m.RegisterDefaultCodecs()
	}
}

func getCodecCapability(codec string) webrtc.RTPCodecCapability {
	switch codec {
	case "vp8":
		return webrtc.RTPCodecCapability{MimeType: webrtc.MimeTypeVP8, ClockRate: 90000}
	case "vp9":
		return webrtc.RTPCodecCapability{MimeType: webrtc.MimeTypeVP9, ClockRate: 90000}
	case "h264":
		return webrtc.RTPCodecCapability{MimeType: webrtc.MimeTypeH264, ClockRate: 90000}
	default:
		return webrtc.RTPCodecCapability{MimeType: webrtc.MimeTypeVP8, ClockRate: 90000}
	}
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

			if err := s.videoTrack.WriteRTP(rtpPacket); err != nil {
				fmt.Printf("Video session %s: WriteRTP error: %v\n", s.ID, err)
			}
		}
	}()

	return nil
}

func (s *VideoSession) Close() {
	s.mu.Lock()
	defer s.mu.Unlock()

	if s.closed {
		return
	}
	s.closed = true

	fmt.Printf("Video session %s: closing\n", s.ID)

	if s.udpListener != nil {
		s.udpListener.Close()
	}
	if s.PC != nil {
		s.PC.Close()
	}

	// 從 map 中移除
	videoSessionsMu.Lock()
	delete(videoSessions, s.ID)
	videoSessionsMu.Unlock()
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
