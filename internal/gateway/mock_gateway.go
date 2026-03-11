package gateway

import (
	"SyncAI-WebRTC-Worker/internal/pb"
	"context"
)

// MockRobotGateway is a mock implementation of RobotGatewayInterface for testing
type MockRobotGateway struct {
	// Function fields for customizing behavior in tests
	StartRTSPPusherFunc     func() error
	StopRTSPPusherFunc      func() error
	ConnectGRPCFunc         func(ctx context.Context) error
	StartVelocityStreamFunc func(ctx context.Context) error
	SendVelocityCommandFunc func(cmd *pb.VelocityCommand) error
	CloseVelocityStreamFunc func() (*pb.ControlResponse, error)
	CloseFunc               func() error

	// Call tracking
	StartRTSPPusherCalled     bool
	StopRTSPPusherCalled      bool
	ConnectGRPCCalled         bool
	StartVelocityStreamCalled bool
	SendVelocityCommandCalled bool
	CloseVelocityStreamCalled bool
	CloseCalled               bool

	// Captured arguments
	LastVelocityCommand *pb.VelocityCommand
}

// NewMockRobotGateway creates a new MockRobotGateway with default no-op implementations
func NewMockRobotGateway() *MockRobotGateway {
	return &MockRobotGateway{
		StartRTSPPusherFunc:     func() error { return nil },
		StopRTSPPusherFunc:      func() error { return nil },
		ConnectGRPCFunc:         func(ctx context.Context) error { return nil },
		StartVelocityStreamFunc: func(ctx context.Context) error { return nil },
		SendVelocityCommandFunc: func(cmd *pb.VelocityCommand) error { return nil },
		CloseVelocityStreamFunc: func() (*pb.ControlResponse, error) { return &pb.ControlResponse{Success: true}, nil },
		CloseFunc:               func() error { return nil },
	}
}

func (m *MockRobotGateway) StartRTSPPusher() error {
	m.StartRTSPPusherCalled = true
	if m.StartRTSPPusherFunc != nil {
		return m.StartRTSPPusherFunc()
	}
	return nil
}

func (m *MockRobotGateway) StopRTSPPusher() error {
	m.StopRTSPPusherCalled = true
	if m.StopRTSPPusherFunc != nil {
		return m.StopRTSPPusherFunc()
	}
	return nil
}

func (m *MockRobotGateway) ConnectGRPC(ctx context.Context) error {
	m.ConnectGRPCCalled = true
	if m.ConnectGRPCFunc != nil {
		return m.ConnectGRPCFunc(ctx)
	}
	return nil
}

func (m *MockRobotGateway) StartVelocityStream(ctx context.Context) error {
	m.StartVelocityStreamCalled = true
	if m.StartVelocityStreamFunc != nil {
		return m.StartVelocityStreamFunc(ctx)
	}
	return nil
}

func (m *MockRobotGateway) SendVelocityCommand(cmd *pb.VelocityCommand) error {
	m.SendVelocityCommandCalled = true
	m.LastVelocityCommand = cmd
	if m.SendVelocityCommandFunc != nil {
		return m.SendVelocityCommandFunc(cmd)
	}
	return nil
}

func (m *MockRobotGateway) CloseVelocityStream() (*pb.ControlResponse, error) {
	m.CloseVelocityStreamCalled = true
	if m.CloseVelocityStreamFunc != nil {
		return m.CloseVelocityStreamFunc()
	}
	return &pb.ControlResponse{Success: true}, nil
}

func (m *MockRobotGateway) Close() error {
	m.CloseCalled = true
	if m.CloseFunc != nil {
		return m.CloseFunc()
	}
	return nil
}

// Reset clears all call tracking state
func (m *MockRobotGateway) Reset() {
	m.StartRTSPPusherCalled = false
	m.StopRTSPPusherCalled = false
	m.ConnectGRPCCalled = false
	m.StartVelocityStreamCalled = false
	m.SendVelocityCommandCalled = false
	m.CloseVelocityStreamCalled = false
	m.CloseCalled = false
	m.LastVelocityCommand = nil
}
