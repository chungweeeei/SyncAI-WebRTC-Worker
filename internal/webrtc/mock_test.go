package webrtc

import (
	"SyncAI-WebRTC-Worker/internal/proc"

	"github.com/go-gst/go-gst/gst"
	"github.com/pion/webrtc/v4"
	"github.com/stretchr/testify/mock"
)

// MockPeerConnection mocks PeerConnectionInterface
type MockPeerConnection struct {
	mock.Mock
	localDesc       *webrtc.SessionDescription
	connectionState webrtc.PeerConnectionState
}

func (m *MockPeerConnection) SetRemoteDescription(desc webrtc.SessionDescription) error {
	args := m.Called(desc)
	return args.Error(0)
}

func (m *MockPeerConnection) CreateAnswer(options *webrtc.AnswerOptions) (webrtc.SessionDescription, error) {
	args := m.Called(options)
	return args.Get(0).(webrtc.SessionDescription), args.Error(1)
}

func (m *MockPeerConnection) SetLocalDescription(desc webrtc.SessionDescription) error {
	args := m.Called(desc)
	m.localDesc = &desc
	return args.Error(0)
}

func (m *MockPeerConnection) LocalDescription() *webrtc.SessionDescription {
	return m.localDesc
}

func (m *MockPeerConnection) OnDataChannel(f func(*webrtc.DataChannel)) {
	m.Called(f)
}

func (m *MockPeerConnection) OnConnectionStateChange(f func(webrtc.PeerConnectionState)) {
	m.Called(f)
}

func (m *MockPeerConnection) OnTrack(f func(*webrtc.TrackRemote, *webrtc.RTPReceiver)) {
	m.Called(f)
}

func (m *MockPeerConnection) AddTrack(track webrtc.TrackLocal) (*webrtc.RTPSender, error) {
	args := m.Called(track)
	if args.Get(0) == nil {
		return nil, args.Error(1)
	}
	return args.Get(0).(*webrtc.RTPSender), args.Error(1)
}

func (m *MockPeerConnection) ConnectionState() webrtc.PeerConnectionState {
	return m.connectionState
}

func (m *MockPeerConnection) Close() error {
	args := m.Called()
	m.connectionState = webrtc.PeerConnectionStateClosed
	return args.Error(0)
}

func (m *MockPeerConnection) Underlying() *webrtc.PeerConnection {
	// Mock returns nil to indicate this is a test environment
	return nil
}

// MockPeerConnectionFactory mocks PeerConnectionFactory
type MockPeerConnectionFactory struct {
	mock.Mock
}

func (m *MockPeerConnectionFactory) NewPeerConnection(config webrtc.Configuration) (PeerConnectionInterface, error) {
	args := m.Called(config)
	if args.Get(0) == nil {
		return nil, args.Error(1)
	}
	return args.Get(0).(PeerConnectionInterface), args.Error(1)
}

type MockProcPipelineFactory struct {
	mock.Mock
}

func (m *MockProcPipelineFactory) NewPipelineFromString(pipelineStr string) (proc.Pipeline, error) {
	args := m.Called(pipelineStr)
	if args.Get(0) == nil {
		return nil, args.Error(1)
	}
	return args.Get(0).(proc.Pipeline), args.Error(1)
}

// MockPipeline mocks proc.Pipeline interface
type MockProcPipeline struct {
	mock.Mock
}

func (m *MockProcPipeline) SetState(state gst.State) error {
	args := m.Called(state)
	return args.Error(0)
}

func (m *MockProcPipeline) BlockSetState(state gst.State) error {
	args := m.Called(state)
	return args.Error(0)
}

// MockPipelineFactory mocks proc.PipelineFactory interface
type MockPipelineFactory struct {
	mock.Mock
}

func (m *MockPipelineFactory) NewPipelineFromString(pipelineStr string) (interface {
	SetState(gst.State) error
	BlockSetState(gst.State) error
}, error) {
	args := m.Called(pipelineStr)
	if args.Get(0) == nil {
		return nil, args.Error(1)
	}
	return args.Get(0).(*MockProcPipeline), args.Error(1)
}
