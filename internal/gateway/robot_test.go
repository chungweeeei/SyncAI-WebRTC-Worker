package gateway

import (
	"SyncAI-WebRTC-Worker/internal/pb"
	"SyncAI-WebRTC-Worker/internal/transport"
	"context"
	"encoding/json"
	"errors"
	"net"
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestNewRobotGateway_DefaultSocketPath(t *testing.T) {
	// Act
	gw := NewRobotGateway("", nil)

	// Assert
	assert.NotNil(t, gw)
	assert.Equal(t, "/tmp/rtsp_pusher.sock", gw.socketPath)
}

func TestNewRobotGateway_CustomSocketPath(t *testing.T) {
	// Arrange
	customPath := "/custom/path/socket.sock"

	// Act
	gw := NewRobotGateway(customPath, nil)

	// Assert
	assert.NotNil(t, gw)
	assert.Equal(t, customPath, gw.socketPath)
}

func TestStartRTSPPusher_Success(t *testing.T) {
	// Arrange - create a temporary Unix socket
	tmpDir := t.TempDir()
	socketPath := filepath.Join(tmpDir, "test.sock")

	listener, err := net.Listen("unix", socketPath)
	require.NoError(t, err)
	defer listener.Close()

	var receivedData []byte
	done := make(chan struct{})

	go func() {
		conn, err := listener.Accept()
		if err != nil {
			return
		}
		defer conn.Close()

		buf := make([]byte, 1024)
		n, _ := conn.Read(buf)
		receivedData = buf[:n]
		close(done)
	}()

	gw := NewRobotGateway(socketPath, nil)

	// Act
	err = gw.StartRTSPPusher()

	// Assert
	assert.NoError(t, err)

	<-done

	// Verify JSON command
	var cmd RobotCommand
	err = json.Unmarshal(receivedData[:len(receivedData)-1], &cmd) // remove newline
	assert.NoError(t, err)
	assert.Equal(t, "start", cmd.Action)
}

func TestStopRTSPPusher_Success(t *testing.T) {
	// Arrange - create a temporary Unix socket
	tmpDir := t.TempDir()
	socketPath := filepath.Join(tmpDir, "test.sock")

	listener, err := net.Listen("unix", socketPath)
	require.NoError(t, err)
	defer listener.Close()

	var receivedData []byte
	done := make(chan struct{})

	go func() {
		conn, err := listener.Accept()
		if err != nil {
			return
		}
		defer conn.Close()

		buf := make([]byte, 1024)
		n, _ := conn.Read(buf)
		receivedData = buf[:n]
		close(done)
	}()

	gw := NewRobotGateway(socketPath, nil)

	// Act
	err = gw.StopRTSPPusher()

	// Assert
	assert.NoError(t, err)

	<-done

	// Verify JSON command
	var cmd RobotCommand
	err = json.Unmarshal(receivedData[:len(receivedData)-1], &cmd) // remove newline
	assert.NoError(t, err)
	assert.Equal(t, "stop", cmd.Action)
}

func TestStartRTSPPusher_ConnectionError(t *testing.T) {
	// Arrange - use non-existent socket path
	gw := NewRobotGateway("/nonexistent/path/socket.sock", nil)

	// Act
	err := gw.StartRTSPPusher()

	// Assert
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "failed to connect to socket")
}

func TestStopRTSPPusher_ConnectionError(t *testing.T) {
	// Arrange - use non-existent socket path
	gw := NewRobotGateway("/nonexistent/path/socket.sock", nil)

	// Act
	err := gw.StopRTSPPusher()

	// Assert
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "failed to connect to socket")
}

func TestRobotCommand_JSONFormat(t *testing.T) {
	// Arrange
	cmd := RobotCommand{Action: "start"}

	// Act
	data, err := json.Marshal(cmd)

	// Assert
	assert.NoError(t, err)
	assert.Equal(t, `{"action":"start"}`, string(data))
}

func TestRobotGateway_ConcurrentCalls(t *testing.T) {
	// Arrange - create a temporary Unix socket
	tmpDir := t.TempDir()
	socketPath := filepath.Join(tmpDir, "test.sock")

	listener, err := net.Listen("unix", socketPath)
	require.NoError(t, err)
	defer listener.Close()

	// Handle multiple connections
	go func() {
		for {
			conn, err := listener.Accept()
			if err != nil {
				return
			}
			go func(c net.Conn) {
				defer c.Close()
				buf := make([]byte, 1024)
				c.Read(buf)
			}(conn)
		}
	}()

	gw := NewRobotGateway(socketPath, nil)

	// Act & Assert - concurrent calls should not panic
	done := make(chan struct{})
	for i := 0; i < 10; i++ {
		go func() {
			_ = gw.StartRTSPPusher()
			done <- struct{}{}
		}()
		go func() {
			_ = gw.StopRTSPPusher()
			done <- struct{}{}
		}()
	}

	// Wait for all goroutines
	for i := 0; i < 20; i++ {
		<-done
	}
}

func TestStartRTSPPusher_SocketAlreadyRemoved(t *testing.T) {
	// Arrange - create and immediately remove socket
	tmpDir := t.TempDir()
	socketPath := filepath.Join(tmpDir, "test.sock")

	listener, err := net.Listen("unix", socketPath)
	require.NoError(t, err)
	listener.Close()
	os.Remove(socketPath)

	gw := NewRobotGateway(socketPath, nil)

	// Act
	err = gw.StartRTSPPusher()

	// Assert
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "failed to connect to socket")
}

// ========== gRPC Method Tests with MockControlClient ==========

func TestConnectGRPC_Success(t *testing.T) {
	// Arrange
	mockClient := &transport.MockControlClient{}
	mockClient.On("Connect", context.Background()).Return(nil)

	gw := NewRobotGateway("", mockClient)

	// Act
	err := gw.ConnectGRPC(context.Background())

	// Assert
	assert.NoError(t, err)
	mockClient.AssertExpectations(t)
}

func TestConnectGRPC_Error(t *testing.T) {
	// Arrange
	mockClient := &transport.MockControlClient{}
	expectedErr := errors.New("connection failed")
	mockClient.On("Connect", context.Background()).Return(expectedErr)

	gw := NewRobotGateway("", mockClient)

	// Act
	err := gw.ConnectGRPC(context.Background())

	// Assert
	assert.Error(t, err)
	assert.Equal(t, expectedErr, err)
	mockClient.AssertExpectations(t)
}

func TestConnectGRPC_NilClient(t *testing.T) {
	// Arrange
	gw := NewRobotGateway("", nil)

	// Act
	err := gw.ConnectGRPC(context.Background())

	// Assert
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "control client not configured")
}

func TestStartVelocityStream_Success(t *testing.T) {
	// Arrange
	mockClient := &transport.MockControlClient{}
	mockClient.On("StartVelocityStream", context.Background()).Return(nil)

	gw := NewRobotGateway("", mockClient)

	// Act
	err := gw.StartVelocityStream(context.Background())

	// Assert
	assert.NoError(t, err)
	mockClient.AssertExpectations(t)
}

func TestStartVelocityStream_NilClient(t *testing.T) {
	// Arrange
	gw := NewRobotGateway("", nil)

	// Act
	err := gw.StartVelocityStream(context.Background())

	// Assert
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "control client not configured")
}

func TestSendVelocityCommand_Success(t *testing.T) {
	// Arrange
	mockClient := &transport.MockControlClient{}
	cmd := &pb.VelocityCommand{
		Linear:  &pb.Vector3{X: 1.0, Y: 0.0, Z: 0.0},
		Angular: &pb.Vector3{X: 0.0, Y: 0.0, Z: 0.5},
	}
	mockClient.On("SendVelocityCommand", cmd).Return(nil)

	gw := NewRobotGateway("", mockClient)

	// Act
	err := gw.SendVelocityCommand(cmd)

	// Assert
	assert.NoError(t, err)
	mockClient.AssertExpectations(t)
}

func TestSendVelocityCommand_NilClient(t *testing.T) {
	// Arrange
	gw := NewRobotGateway("", nil)

	// Act
	err := gw.SendVelocityCommand(&pb.VelocityCommand{})

	// Assert
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "control client not configured")
}

func TestCloseVelocityStream_Success(t *testing.T) {
	// Arrange
	mockClient := &transport.MockControlClient{}
	expectedResp := &pb.ControlResponse{Success: true}
	mockClient.On("CloseVelocityStream").Return(expectedResp, nil)

	gw := NewRobotGateway("", mockClient)

	// Act
	resp, err := gw.CloseVelocityStream()

	// Assert
	assert.NoError(t, err)
	assert.Equal(t, expectedResp, resp)
	mockClient.AssertExpectations(t)
}

func TestCloseVelocityStream_NilClient(t *testing.T) {
	// Arrange
	gw := NewRobotGateway("", nil)

	// Act
	resp, err := gw.CloseVelocityStream()

	// Assert
	assert.Error(t, err)
	assert.Nil(t, resp)
	assert.Contains(t, err.Error(), "control client not configured")
}

func TestClose_Success(t *testing.T) {
	// Arrange
	mockClient := &transport.MockControlClient{}
	mockClient.On("Close").Return(nil)

	gw := NewRobotGateway("", mockClient)

	// Act
	err := gw.Close()

	// Assert
	assert.NoError(t, err)
	mockClient.AssertExpectations(t)
}

func TestClose_NilClient(t *testing.T) {
	// Arrange
	gw := NewRobotGateway("", nil)

	// Act
	err := gw.Close()

	// Assert
	assert.NoError(t, err) // Should not error when client is nil
}

// ========== MockRobotGateway Usage Examples ==========

func TestMockRobotGateway_DefaultBehavior(t *testing.T) {
	// Arrange
	mock := NewMockRobotGateway()

	// Act & Assert - default implementations should succeed
	assert.NoError(t, mock.StartRTSPPusher())
	assert.NoError(t, mock.StopRTSPPusher())
	assert.NoError(t, mock.ConnectGRPC(context.Background()))
	assert.NoError(t, mock.StartVelocityStream(context.Background()))
	assert.NoError(t, mock.SendVelocityCommand(&pb.VelocityCommand{}))

	resp, err := mock.CloseVelocityStream()
	assert.NoError(t, err)
	assert.True(t, resp.Success)

	assert.NoError(t, mock.Close())

	// Verify call tracking
	assert.True(t, mock.StartRTSPPusherCalled)
	assert.True(t, mock.StopRTSPPusherCalled)
	assert.True(t, mock.ConnectGRPCCalled)
	assert.True(t, mock.StartVelocityStreamCalled)
	assert.True(t, mock.SendVelocityCommandCalled)
	assert.True(t, mock.CloseVelocityStreamCalled)
	assert.True(t, mock.CloseCalled)
}

func TestMockRobotGateway_CustomBehavior(t *testing.T) {
	// Arrange
	mock := NewMockRobotGateway()
	expectedErr := errors.New("custom error")
	mock.StartRTSPPusherFunc = func() error {
		return expectedErr
	}

	// Act
	err := mock.StartRTSPPusher()

	// Assert
	assert.Equal(t, expectedErr, err)
	assert.True(t, mock.StartRTSPPusherCalled)
}

func TestMockRobotGateway_CapturedArguments(t *testing.T) {
	// Arrange
	mock := NewMockRobotGateway()
	cmd := &pb.VelocityCommand{
		Linear:  &pb.Vector3{X: 2.0, Y: 1.0, Z: 0.0},
		Angular: &pb.Vector3{X: 0.0, Y: 0.0, Z: 0.75},
	}

	// Act
	_ = mock.SendVelocityCommand(cmd)

	// Assert
	assert.Equal(t, cmd, mock.LastVelocityCommand)
}

func TestMockRobotGateway_Reset(t *testing.T) {
	// Arrange
	mock := NewMockRobotGateway()
	_ = mock.StartRTSPPusher()
	_ = mock.SendVelocityCommand(&pb.VelocityCommand{Linear: &pb.Vector3{X: 1.0}})

	// Act
	mock.Reset()

	// Assert
	assert.False(t, mock.StartRTSPPusherCalled)
	assert.False(t, mock.SendVelocityCommandCalled)
	assert.Nil(t, mock.LastVelocityCommand)
}

func TestMockRobotGateway_Reset_Multiple(t *testing.T) {
	// Arrange
	mock := NewMockRobotGateway()
	_ = mock.StartRTSPPusher()
	_ = mock.SendVelocityCommand(&pb.VelocityCommand{Linear: &pb.Vector3{X: 1.0}})

	// Act
	mock.Reset()

	// Assert
	assert.False(t, mock.StartRTSPPusherCalled)
	assert.False(t, mock.SendVelocityCommandCalled)
	assert.Nil(t, mock.LastVelocityCommand)
}
