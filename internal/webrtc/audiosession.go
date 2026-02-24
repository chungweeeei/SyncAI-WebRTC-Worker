package webrtc

import (
	"SyncAI-WebRTC-Worker/internal/proc"
	"fmt"
	"net"
	"sync"

	"github.com/pion/webrtc/v4"
)

type AudioSession struct {
	ID        string
	PC        *webrtc.PeerConnection
	GStreamer *proc.GStreamerPipeline
	udpConn   *net.UDPConn
	mu        sync.Mutex
}

var (
	audioSessions   = make(map[string]*AudioSession)
	audioSessionsMu sync.Mutex
)

type AudioSessionOptions struct {
	RTPPort int // UDP port for GStreamer
}

func CreateAudioSession(offerSDP string, opts AudioSessionOptions) (string, string, error) {
	pc, err := webrtc.NewPeerConnection(
		webrtc.Configuration{
			ICEServers: []webrtc.ICEServer{
				{
					URLs: []string{"stun:stun.l.google.com:19302"},
				},
			},
		},
	)
	if err != nil {
		return "", "", err
	}

	sessionID := generateSessionID()
	session := &AudioSession{
		ID: sessionID,
		PC: pc,
	}

	port := opts.RTPPort
	if port == 0 {
		port = 5004 // default RTP port
	}

	gst, err := proc.NewGStreamerPipeline(port)
	if err != nil {
		pc.Close()
		return "", "", fmt.Errorf("failed to start GStreamer: %w", err)
	}
	session.GStreamer = gst

	addr, _ := net.ResolveUDPAddr("udp", fmt.Sprintf("127.0.0.1:%d", port))
	udpConn, err := net.DialUDP("udp", nil, addr)
	if err != nil {
		gst.Close()
		pc.Close()
		return "", "", fmt.Errorf("failed to create UDP connection: %w", err)
	}
	session.udpConn = udpConn

	// handle incoming audio track
	pc.OnTrack(func(track *webrtc.TrackRemote, receiver *webrtc.RTPReceiver) {
		if track.Kind() != webrtc.RTPCodecTypeAudio {
			return
		}

		fmt.Printf("Audio track received: %s (codec: %s)\n", track.ID(), track.Codec().MimeType)

		// forward RTP packets to GStreamer
		go func() {
			buf := make([]byte, 1500)
			for {
				n, _, err := track.Read(buf)
				if err != nil {
					fmt.Printf("Track read error: %v\n", err)
					return
				}

				if session.udpConn != nil {
					session.udpConn.Write(buf[:n])
				}
			}
		}()
	})

	pc.OnConnectionStateChange(func(state webrtc.PeerConnectionState) {
		fmt.Printf("Audio session %s: %s\n", sessionID, state.String())

		if state == webrtc.PeerConnectionStateClosed ||
			state == webrtc.PeerConnectionStateFailed {
			session.Close()
		}
	})

	offer := webrtc.SessionDescription{
		Type: webrtc.SDPTypeOffer,
		SDP:  offerSDP,
	}
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

	audioSessionsMu.Lock()
	audioSessions[sessionID] = session
	audioSessionsMu.Unlock()

	return sessionID, pc.LocalDescription().SDP, nil
}

func (s *AudioSession) Close() {
	s.mu.Lock()
	defer s.mu.Unlock()

	if s.udpConn != nil {
		s.udpConn.Close()
	}
	if s.GStreamer != nil {
		s.GStreamer.Close()
	}
	if s.PC != nil {
		s.PC.Close()
	}
}

func DeleteAudioSession(sessionID string) error {
	audioSessionsMu.Lock()
	defer audioSessionsMu.Unlock()

	session, exists := audioSessions[sessionID]
	if !exists {
		return fmt.Errorf("session not found: %s", sessionID)
	}

	session.Close()
	delete(audioSessions, sessionID)
	return nil
}
