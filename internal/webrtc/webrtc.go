package webrtc

import (
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
