package webrtc

import (
	"encoding/json"
	"fmt"

	"github.com/pion/webrtc/v4"
)

// PeerConnectionInterface abstracts webrtc.PeerConnection for testing
type PeerConnectionInterface interface {
	SetRemoteDescription(desc webrtc.SessionDescription) error
	CreateAnswer(options *webrtc.AnswerOptions) (webrtc.SessionDescription, error)
	SetLocalDescription(desc webrtc.SessionDescription) error
	LocalDescription() *webrtc.SessionDescription
	OnDataChannel(f func(*webrtc.DataChannel))
	OnConnectionStateChange(f func(webrtc.PeerConnectionState))
	OnTrack(f func(*webrtc.TrackRemote, *webrtc.RTPReceiver))
	AddTrack(track webrtc.TrackLocal) (*webrtc.RTPSender, error)
	ConnectionState() webrtc.PeerConnectionState
	Close() error

	// Underlying returns the real PeerConnection (nil for mocks)
	Underlying() *webrtc.PeerConnection
}

// RealPeerConnection wraps webrtc.PeerConnection to implement PeerConnectionInterface
type RealPeerConnection struct {
	*webrtc.PeerConnection
}

func (r *RealPeerConnection) Underlying() *webrtc.PeerConnection {
	return r.PeerConnection
}

// PeerConnectionFactory creates PeerConnection instances
type PeerConnectionFactory interface {
	NewPeerConnection(config webrtc.Configuration) (PeerConnectionInterface, error)
}

// DefaultPeerConnectionFactory uses real webrtc implementation
type DefaultPeerConnectionFactory struct{}

func (f *DefaultPeerConnectionFactory) NewPeerConnection(config webrtc.Configuration) (PeerConnectionInterface, error) {
	pc, err := webrtc.NewPeerConnection(config)
	if err != nil {
		return nil, err
	}
	return &RealPeerConnection{PeerConnection: pc}, nil
}

// APIFactory creates WebRTC API instances
type APIFactory interface {
	NewAPI(options ...func(*webrtc.API)) *webrtc.API
}

// DefaultAPIFactory uses real webrtc implementation
type DefaultAPIFactory struct{}

func (f *DefaultAPIFactory) NewAPI(options ...func(*webrtc.API)) *webrtc.API {
	return webrtc.NewAPI(options...)
}

// default factory instances
var (
	defaultPCFactory  PeerConnectionFactory = &DefaultPeerConnectionFactory{}
	defaultAPIFactory APIFactory            = &DefaultAPIFactory{}
)

// SetPeerConnectionFactory allows injection for testing
func SetPeerConnectionFactory(factory PeerConnectionFactory) {
	defaultPCFactory = factory
}

// ResetPeerConnectionFactory resets to default
func ResetPeerConnectionFactory() {
	defaultPCFactory = &DefaultPeerConnectionFactory{}
}

// ActionType defines the type of action
type ActionType string

const (
	ActionTypeControl ActionType = "control"
	ActionTypeCommand ActionType = "command"
)

// BaseMessage is the wrapper for all messages
type BaseMessage struct {
	Action  ActionType      `json:"action"`
	Payload json.RawMessage `json:"payload"`
}

type ControlPayload struct {
	Linear  [3]float64 `json:"linear"`
	Angular [3]float64 `json:"angular"`
}

type CommandPayload struct {
	ID     string          `json:"id"`
	Name   string          `json:"name"`
	Type   string          `json:"type"`
	Params json.RawMessage `json:"params"`
}

func ParseMessage(data []byte) (*BaseMessage, error) {
	var msg BaseMessage
	if err := json.Unmarshal(data, &msg); err != nil {
		return nil, fmt.Errorf("failed to parse message: %w", err)
	}
	return &msg, nil
}

func (m *BaseMessage) ParseControlPayload() (*ControlPayload, error) {
	var payload ControlPayload
	if err := json.Unmarshal(m.Payload, &payload); err != nil {
		return nil, fmt.Errorf("failed to parse control payload: %w", err)
	}
	return &payload, nil
}

func (m *BaseMessage) ParseCommandPayload() (*CommandPayload, error) {
	var payload CommandPayload
	if err := json.Unmarshal(m.Payload, &payload); err != nil {
		return nil, fmt.Errorf("failed to parse command payload: %w", err)
	}

	return &payload, nil
}
