package transport

import (
	"SyncAI-WebRTC-Worker/internal/pb"
	"context"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestNewGRPCControlClient(t *testing.T) {
	// Arrange
	addr := "localhost:50051"

	// Act
	client := NewGRPCControlClient(addr)

	// Assert
	assert.NotNil(t, client)
	assert.Equal(t, addr, client.addr)
	assert.Nil(t, client.conn)
	assert.Nil(t, client.client)
	assert.Nil(t, client.velocityStream)
}

func TestGRPCControlClient_Connect_InvalidAddress(t *testing.T) {
	// Arrange
	client := NewGRPCControlClient("invalid-address-without-port")

	// Act
	err := client.Connect(context.Background())

	// Assert - grpc.NewClient doesn't fail on invalid address immediately
	// It fails lazily when trying to actually connect
	assert.NoError(t, err)
	assert.NotNil(t, client.conn)
}

func TestGRPCControlClient_Connect_AlreadyConnected(t *testing.T) {
	// Arrange
	client := NewGRPCControlClient("localhost:50051")
	err := client.Connect(context.Background())
	require.NoError(t, err)

	// Act - connect again
	err = client.Connect(context.Background())

	// Assert - should not error, just return early
	assert.NoError(t, err)

	// Cleanup
	client.Close()
}

func TestGRPCControlClient_StartVelocityStream_NotConnected(t *testing.T) {
	// Arrange
	client := NewGRPCControlClient("localhost:50051")

	// Act
	err := client.StartVelocityStream(context.Background())

	// Assert
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "gRPC client not connected")
}

func TestGRPCControlClient_StartVelocityStream_CancelledContext(t *testing.T) {
	// Arrange
	client := NewGRPCControlClient("localhost:50051")
	err := client.Connect(context.Background())
	require.NoError(t, err)
	defer client.Close()

	ctx, cancel := context.WithCancel(context.Background())
	cancel() // Cancel immediately

	// Act
	err = client.StartVelocityStream(ctx)

	// Assert
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "context already cancelled")
}

func TestGRPCControlClient_SendVelocityCommand_StreamNotStarted(t *testing.T) {
	// Arrange
	client := NewGRPCControlClient("localhost:50051")
	cmd := &pb.VelocityCommand{
		Linear:  &pb.Vector3{X: 1.0, Y: 0.0, Z: 0.0},
		Angular: &pb.Vector3{X: 0.0, Y: 0.0, Z: 0.5},
	}

	// Act
	err := client.SendVelocityCommand(cmd)

	// Assert
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "velocity stream not started")
}

func TestGRPCControlClient_CloseVelocityStream_StreamNotStarted(t *testing.T) {
	// Arrange
	client := NewGRPCControlClient("localhost:50051")

	// Act
	resp, err := client.CloseVelocityStream()

	// Assert
	assert.Error(t, err)
	assert.Nil(t, resp)
	assert.Contains(t, err.Error(), "velocity stream not started")
}

func TestGRPCControlClient_Close_NotConnected(t *testing.T) {
	// Arrange
	client := NewGRPCControlClient("localhost:50051")

	// Act
	err := client.Close()

	// Assert - should not error when not connected
	assert.NoError(t, err)
}

func TestGRPCControlClient_Close_Connected(t *testing.T) {
	// Arrange
	client := NewGRPCControlClient("localhost:50051")
	err := client.Connect(context.Background())
	require.NoError(t, err)

	// Act
	err = client.Close()

	// Assert
	assert.NoError(t, err)
	assert.Nil(t, client.conn)
	assert.Nil(t, client.client)
	assert.Nil(t, client.velocityStream)
}

func TestGRPCControlClient_CloseTwice(t *testing.T) {
	// Arrange
	client := NewGRPCControlClient("localhost:50051")
	err := client.Connect(context.Background())
	require.NoError(t, err)

	// Act
	err1 := client.Close()
	err2 := client.Close()

	// Assert - both should succeed
	assert.NoError(t, err1)
	assert.NoError(t, err2)
}

func TestGRPCControlClient_ConcurrentConnect(t *testing.T) {
	// Arrange
	client := NewGRPCControlClient("localhost:50051")
	defer client.Close()

	// Act - concurrent connects should not panic
	done := make(chan struct{})
	for i := 0; i < 10; i++ {
		go func() {
			_ = client.Connect(context.Background())
			done <- struct{}{}
		}()
	}

	// Wait for all goroutines
	for i := 0; i < 10; i++ {
		<-done
	}

	// Assert - should be connected
	assert.NotNil(t, client.conn)
}

func TestGRPCControlClient_ConcurrentClose(t *testing.T) {
	// Arrange
	client := NewGRPCControlClient("localhost:50051")
	err := client.Connect(context.Background())
	require.NoError(t, err)

	// Act - concurrent closes should not panic
	done := make(chan struct{})
	for i := 0; i < 10; i++ {
		go func() {
			_ = client.Close()
			done <- struct{}{}
		}()
	}

	// Wait for all goroutines
	for i := 0; i < 10; i++ {
		<-done
	}

	// Assert - should be closed
	assert.Nil(t, client.conn)
}

// ========== Interface Compliance Test ==========

func TestGRPCControlClient_ImplementsControlClient(t *testing.T) {
	// Compile-time check that GRPCControlClient implements ControlClient
	var _ ControlClient = (*GRPCControlClient)(nil)
}

// ========== MockControlClient Tests ==========

func TestMockControlClient_ImplementsControlClient(t *testing.T) {
	// Compile-time check that MockControlClient implements ControlClient
	var _ ControlClient = (*MockControlClient)(nil)
}

func TestMockControlClient_Connect(t *testing.T) {
	// Arrange
	mock := NewMockControlClient()
	mock.On("Connect", context.Background()).Return(nil)

	// Act
	err := mock.Connect(context.Background())

	// Assert
	assert.NoError(t, err)
	mock.AssertExpectations(t)
}

func TestMockControlClient_SendVelocityCommand(t *testing.T) {
	// Arrange
	mock := NewMockControlClient()
	cmd := &pb.VelocityCommand{
		Linear:  &pb.Vector3{X: 1.0, Y: 0.5, Z: 0.0},
		Angular: &pb.Vector3{X: 0.0, Y: 0.0, Z: 0.3},
	}
	mock.On("SendVelocityCommand", cmd).Return(nil)

	// Act
	err := mock.SendVelocityCommand(cmd)

	// Assert
	assert.NoError(t, err)
	mock.AssertExpectations(t)
}

func TestMockControlClient_CloseVelocityStream(t *testing.T) {
	// Arrange
	mock := NewMockControlClient()
	expectedResp := &pb.ControlResponse{Success: true, Message: "Stream closed"}
	mock.On("CloseVelocityStream").Return(expectedResp, nil)

	// Act
	resp, err := mock.CloseVelocityStream()

	// Assert
	assert.NoError(t, err)
	assert.Equal(t, expectedResp, resp)
	mock.AssertExpectations(t)
}
