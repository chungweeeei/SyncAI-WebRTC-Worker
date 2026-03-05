package gateway

import (
	"encoding/json"
	"net"
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestNewRobotGateway_DefaultSocketPath(t *testing.T) {
	// Act
	gw := NewRobotGateway("")

	// Assert
	assert.NotNil(t, gw)
	assert.Equal(t, "/tmp/rtsp_pusher.sock", gw.socketPath)
}

func TestNewRobotGateway_CustomSocketPath(t *testing.T) {
	// Arrange
	customPath := "/custom/path/socket.sock"

	// Act
	gw := NewRobotGateway(customPath)

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

	gw := NewRobotGateway(socketPath)

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

	gw := NewRobotGateway(socketPath)

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
	gw := NewRobotGateway("/nonexistent/path/socket.sock")

	// Act
	err := gw.StartRTSPPusher()

	// Assert
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "failed to connect to socket")
}

func TestStopRTSPPusher_ConnectionError(t *testing.T) {
	// Arrange - use non-existent socket path
	gw := NewRobotGateway("/nonexistent/path/socket.sock")

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

	gw := NewRobotGateway(socketPath)

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

	gw := NewRobotGateway(socketPath)

	// Act
	err = gw.StartRTSPPusher()

	// Assert
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "failed to connect to socket")
}
