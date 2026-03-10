package webrtc

import (
	"SyncAI-WebRTC-Worker/internal/gateway"
	"SyncAI-WebRTC-Worker/internal/pb"
	"SyncAI-WebRTC-Worker/internal/transport"
	"context"
	"fmt"
	"sync"
	"time"

	"github.com/google/uuid"
	"github.com/pion/webrtc/v4"
)

type Session struct {
	ID           string
	PC           *webrtc.PeerConnection
	DC           *webrtc.DataChannel
	Transport    transport.Transport
	RobotGateway gateway.RobotGatewayInterface
	ctx          context.Context
	cancel       context.CancelFunc
	mu           sync.Mutex
}

var (
	sessions   = make(map[string]*Session)
	sessionsMu sync.Mutex
)

type SessionOptions struct {
	WebSocketURL string
	Dialer       transport.Dialer
	RobotGateway gateway.RobotGatewayInterface
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

	// create session context
	sessionCtx, cancel := context.WithCancel(ctx)

	// create a new session
	sessionID := generateSessionID()
	session := &Session{
		ID:           sessionID,
		PC:           pc,
		RobotGateway: opts.RobotGateway,
		ctx:          sessionCtx,
		cancel:       cancel,
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

	// Initialize gRPC connection if RobotGateway is provided
	if session.RobotGateway != nil {
		if err := session.RobotGateway.ConnectGRPC(session.ctx); err != nil {
			session.cleanup()
			return "", "", fmt.Errorf("failed to connect gRPC: %w", err)
		}

		if err := session.RobotGateway.StartVelocityStream(context.Background()); err != nil {
			session.cleanup()
			return "", "", fmt.Errorf("failed to start velocity stream: %w", err)
		}
	}

	pc.OnDataChannel(func(dc *webrtc.DataChannel) {
		session.mu.Lock()
		session.DC = dc
		session.mu.Unlock()

		dc.OnOpen(func() {
			fmt.Printf("DataChannel '%s' opened\n", dc.Label())
		})

		dc.OnMessage(func(msg webrtc.DataChannelMessage) {
			session.handleMessage(msg.Data)
		})

		dc.OnClose(func() {
			fmt.Printf("DataChannel '%s' closed\n", dc.Label())
		})

	})
	pc.OnConnectionStateChange(func(state webrtc.PeerConnectionState) {
		fmt.Printf("Connection State: %s\n", state.String())

		switch state {
		case webrtc.PeerConnectionStateDisconnected, webrtc.PeerConnectionStateFailed, webrtc.PeerConnectionStateClosed:
			if session.RobotGateway != nil {
				session.RobotGateway.CloseVelocityStream()
			}
		}
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

func (s *Session) handleMessage(data []byte) {
	msg, err := ParseMessage(data)
	if err != nil {
		fmt.Printf("Failed to parse message: %v\n", err)
		return
	}

	switch msg.Action {
	case ActionTypeControl:
		s.handleControl(msg)
	case ActionTypeCommand:
		s.handleCommand(msg)
	default:
		fmt.Printf("Unknown action type: %s\n", msg.Action)
	}
}

func (s *Session) handleControl(msg *BaseMessage) {
	payload, err := msg.ParseControlPayload()
	if err != nil {
		fmt.Printf("Failed to parse velocity: %v\n", err)
		return
	}

	if s.RobotGateway == nil {
		fmt.Println("RobotGateway not configured")
		return
	}

	cmd := &pb.VelocityCommand{
		Linear: &pb.Vector3{
			X: payload.Linear[0],
			Y: payload.Linear[1],
			Z: payload.Linear[2],
		},
		Angular: &pb.Vector3{
			X: payload.Angular[0],
			Y: payload.Angular[1],
			Z: payload.Angular[2],
		},
		Timestamp: time.Now().UnixMilli(),
	}

	fmt.Println("Ready to send cmd to RobotGateway")
	if err := s.RobotGateway.SendVelocityCommand(cmd); err != nil {
		fmt.Printf("Failed to send velocity command: %v\n", err)
	}
}

func (s *Session) handleCommand(msg *BaseMessage) {
	payload, err := msg.ParseCommandPayload()
	if err != nil {
		fmt.Printf("Failed to parse command: %v\n", err)
		return
	}

	fmt.Printf("Received command: %s, params: %v\n", payload.Name, payload.Params)
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
