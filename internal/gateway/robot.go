package gateway

import (
	"SyncAI-WebRTC-Worker/internal/pb"
	"SyncAI-WebRTC-Worker/internal/transport"
	"context"
	"encoding/json"
	"fmt"
	"net"
	"sync"
)

type RobotGatewayInterface interface {
	// Send Command to ROS Node via tcp socket
	// TODO: maybe change to gRPC
	StartRTSPPusher() error
	StopRTSPPusher() error

	// gRPC methods
	ConnectGRPC(ctx context.Context) error
	StartVelocityStream(ctx context.Context) error
	SendVelocityCommand(cmd *pb.VelocityCommand) error
	CloseVelocityStream() (*pb.ControlResponse, error)
	Close() error
}

type RobotGateway struct {
	socketPath    string
	controlClient transport.ControlClient
	mu            sync.Mutex
}

type RobotCommand struct {
	Action string `json:"action"`
}

func NewRobotGateway(socketPath string, controlClient transport.ControlClient) *RobotGateway {
	if socketPath == "" {
		socketPath = "/tmp/rtsp_pusher.sock"
	}

	return &RobotGateway{
		socketPath:    socketPath,
		controlClient: controlClient,
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

func (r *RobotGateway) ConnectGRPC(ctx context.Context) error {
	if r.controlClient == nil {
		return fmt.Errorf("control client not configured")
	}

	return r.controlClient.Connect(ctx)
}

func (r *RobotGateway) StartVelocityStream(ctx context.Context) error {
	if r.controlClient == nil {
		return fmt.Errorf("control client not configured")
	}
	return r.controlClient.StartVelocityStream(ctx)
}

func (r *RobotGateway) SendVelocityCommand(cmd *pb.VelocityCommand) error {
	if r.controlClient == nil {
		return fmt.Errorf("control client not configured")
	}
	return r.controlClient.SendVelocityCommand(cmd)
}

func (r *RobotGateway) CloseVelocityStream() (*pb.ControlResponse, error) {
	if r.controlClient == nil {
		return nil, fmt.Errorf("control client not configured")
	}
	return r.controlClient.CloseVelocityStream()
}

func (r *RobotGateway) Close() error {
	if r.controlClient != nil {
		return r.controlClient.Close()
	}
	return nil
}
