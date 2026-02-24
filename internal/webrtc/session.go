package webrtc

import (
	"SyncAI-WebRTC-Worker/internal/transport"
	"context"
	"fmt"
	"sync"

	"github.com/google/uuid"
	"github.com/pion/webrtc/v4"
)

type Session struct {
	ID        string
	PC        *webrtc.PeerConnection
	DC        *webrtc.DataChannel
	Transport transport.Transport
	mu        sync.Mutex
}

var (
	sessions   = make(map[string]*Session)
	sessionsMu sync.Mutex
)

type SessionOptions struct {
	WebSocketURL string
	Dialer       transport.Dialer
}

func CreateSession(ctx context.Context, offerSDP string, opts SessionOptions) (string, string, error) {

	// create a new peer connection
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
		return "", "", fmt.Errorf("failed to create peer connection: %w", err)
	}

	// create a new session
	sessionID := generateSessionID()
	session := &Session{
		ID: sessionID,
		PC: pc,
	}

	// create websocket connection
	if opts.Dialer != nil && opts.WebSocketURL != "" {
		trans, err := opts.Dialer.Dial(ctx, opts.WebSocketURL)
		if err != nil {
			pc.Close()
			return "", "", fmt.Errorf("failed to connect to WebSocket: %w", err)
		}
		session.Transport = trans

		// setting forward websocket messages to data channel
		trans.OnMessage(func(data []byte) {
			fmt.Printf("Received message from WebSocket: %s\n", string(data))
		})
	}

	pc.OnDataChannel(func(dc *webrtc.DataChannel) {
		session.DC = dc

		dc.OnOpen(func() {
			fmt.Printf("DataChannel '%s' opened\n", dc.Label())
		})

		dc.OnMessage(func(msg webrtc.DataChannelMessage) {
			if session.Transport != nil {
				session.Transport.Send(msg.Data)
			}
		})

	})
	pc.OnConnectionStateChange(func(state webrtc.PeerConnectionState) {
		fmt.Printf("Connection State: %s\n", state.String())
	})

	// setting remote Description
	offer := webrtc.SessionDescription{
		Type: webrtc.SDPTypeOffer,
		SDP:  offerSDP,
	}
	if err := pc.SetRemoteDescription(offer); err != nil {
		session.cleanup()
		return "", "", fmt.Errorf("failed to set remote description: %w", err)
	}

	// create answer
	answer, err := pc.CreateAnswer(nil)
	if err != nil {
		session.cleanup()
		return "", "", fmt.Errorf("failed to create answer: %w", err)
	}

	// wait for ICE gathering to complete
	gatherComplete := webrtc.GatheringCompletePromise(pc)
	if err := pc.SetLocalDescription(answer); err != nil {
		session.cleanup()
		return "", "", fmt.Errorf("failed to set local description: %w", err)
	}
	<-gatherComplete

	// generate session ID
	sessionsMu.Lock()
	sessions[sessionID] = session
	sessionsMu.Unlock()

	return sessionID, pc.LocalDescription().SDP, nil
}

func DeleteSession(sessionID string) error {
	sessionsMu.Lock()
	defer sessionsMu.Unlock()

	session, exists := sessions[sessionID]
	if !exists {
		return fmt.Errorf("session not found: %s", sessionID)
	}

	session.cleanup()
	delete(sessions, sessionID)
	return nil
}

func (s *Session) cleanup() {
	if s.Transport != nil {
		s.Transport.Close()
	}
	if s.PC != nil {
		s.PC.Close()
	}
}

func generateSessionID() string {
	return uuid.New().String()
}
