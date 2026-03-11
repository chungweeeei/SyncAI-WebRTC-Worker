package transport

import (
	"SyncAI-WebRTC-Worker/internal/pb"
	"context"

	"github.com/stretchr/testify/mock"
)

// Ensure MockControlClient implements ControlClient interface
var _ ControlClient = (*MockControlClient)(nil)

// MockControlClient is a mock implementation of ControlClient for testing
type MockControlClient struct {
	mock.Mock
}

// NewMockControlClient creates a new MockControlClient instance
func NewMockControlClient() *MockControlClient {
	return &MockControlClient{}
}

func (m *MockControlClient) Connect(ctx context.Context) error {
	args := m.Called(ctx)
	return args.Error(0)
}

func (m *MockControlClient) StartVelocityStream(ctx context.Context) error {
	args := m.Called(ctx)
	return args.Error(0)
}

func (m *MockControlClient) SendVelocityCommand(cmd *pb.VelocityCommand) error {
	args := m.Called(cmd)
	return args.Error(0)
}

func (m *MockControlClient) CloseVelocityStream() (*pb.ControlResponse, error) {
	args := m.Called()
	if args.Get(0) == nil {
		return nil, args.Error(1)
	}
	return args.Get(0).(*pb.ControlResponse), args.Error(1)
}

func (m *MockControlClient) Close() error {
	args := m.Called()
	return args.Error(0)
}
