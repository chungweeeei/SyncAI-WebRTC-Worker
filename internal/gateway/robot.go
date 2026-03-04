package gateway

import (
	"encoding/json"
	"fmt"
	"net"
	"sync"
)

type RobotGatewayInterface interface {
	StartRTSPPusher() error
	StopRTSPPusher() error
}

type RobotGateway struct {
	socketPath string
	mu         sync.Mutex
}

type RobotCommand struct {
	Action string `json:"action"`
}

func NewRobotGateway(socketPath string) *RobotGateway {
	if socketPath == "" {
		socketPath = "/tmp/rtsp_pusher.sock"
	}
	return &RobotGateway{
		socketPath: socketPath,
	}
}

func (r *RobotGateway) StartRTSPPusher() error {
	r.mu.Lock()
	defer r.mu.Unlock()

	conn, err := net.Dial("unix", r.socketPath)
	if err != nil {
		return fmt.Errorf("failed to connect to socket %s: %w", r.socketPath, err)
	}
	defer conn.Close()

	// Create command as JSON
	command := RobotCommand{
		Action: "start",
	}

	commandJSON, err := json.Marshal(command)
	if err != nil {
		return fmt.Errorf("failed to marshal command: %w", err)
	}

	// Send JSON command with newline
	_, err = conn.Write(append(commandJSON, '\n'))
	if err != nil {
		return fmt.Errorf("failed to write to socket: %w", err)
	}

	return nil
}

func (r *RobotGateway) StopRTSPPusher() error {
	r.mu.Lock()
	defer r.mu.Unlock()

	conn, err := net.Dial("unix", r.socketPath)
	if err != nil {
		return fmt.Errorf("failed to connect to socket %s: %w", r.socketPath, err)
	}
	defer conn.Close()

	// Create command as JSON
	command := RobotCommand{
		Action: "stop",
	}

	commandJSON, err := json.Marshal(command)
	if err != nil {
		return fmt.Errorf("failed to marshal command: %w", err)
	}

	// Send JSON command with newline
	_, err = conn.Write(append(commandJSON, '\n'))
	if err != nil {
		return fmt.Errorf("failed to write to socket: %w", err)
	}

	return nil
}
