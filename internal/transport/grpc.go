package transport

import (
	"SyncAI-WebRTC-Worker/internal/pb"
	"context"
	"fmt"
	"sync"

	"google.golang.org/grpc"
	"google.golang.org/grpc/credentials/insecure"
)

// ControlClient defines the interface for robot control operations
type ControlClient interface {
	Connect(ctx context.Context) error
	StartVelocityStream(ctx context.Context) error
	SendVelocityCommand(cmd *pb.VelocityCommand) error
	CloseVelocityStream() (*pb.ControlResponse, error)
	Close() error
}

// GRPCControlClient implements ControlClient using gRPC
type GRPCControlClient struct {
	addr           string
	conn           *grpc.ClientConn
	client         pb.ControlServiceClient
	velocityStream pb.ControlService_StreamVelocityClient
	mu             sync.Mutex
}

// NewGRPCControlClient creates a new gRPC control client
func NewGRPCControlClient(addr string) *GRPCControlClient {
	return &GRPCControlClient{
		addr: addr,
	}
}

func (c *GRPCControlClient) Connect(ctx context.Context) error {
	c.mu.Lock()
	defer c.mu.Unlock()

	if c.conn != nil {
		return nil // Already connected
	}

	fmt.Printf("[gRPC] Connecting to server at: %s\n", c.addr)

	conn, err := grpc.DialContext(ctx, c.addr, grpc.WithTransportCredentials(insecure.NewCredentials()), grpc.WithBlock())
	if err != nil {
		return fmt.Errorf("failed to connect to gRPC server %s: %w", c.addr, err)
	}

	fmt.Println("[gRPC] Connection established")
	c.conn = conn
	c.client = pb.NewControlServiceClient(conn)
	return nil
}

func (c *GRPCControlClient) StartVelocityStream(ctx context.Context) error {
	c.mu.Lock()
	defer c.mu.Unlock()

	if c.client == nil {
		return fmt.Errorf("gRPC client not connected, call Connect first")
	}

	fmt.Println("[gRPC] Creating velocity stream...")
	if err := ctx.Err(); err != nil {
		return fmt.Errorf("context already cancelled before creating stream: %w", err)
	}

	stream, err := c.client.StreamVelocity(ctx)
	if err != nil {
		return fmt.Errorf("failed to create velocity stream: %w", err)
	}

	fmt.Println("[gRPC] Velocity stream created successfully")
	c.velocityStream = stream
	return nil
}

func (c *GRPCControlClient) SendVelocityCommand(cmd *pb.VelocityCommand) error {
	c.mu.Lock()
	defer c.mu.Unlock()

	if c.velocityStream == nil {
		return fmt.Errorf("velocity stream not started, call StartVelocityStream first")
	}

	if err := c.velocityStream.Context().Err(); err != nil {
		return fmt.Errorf("stream context error: %w", err)
	}

	fmt.Printf("[gRPC] Sending: linear=[%.2f, %.2f, %.2f], angular=[%.2f, %.2f, %.2f], ts=%d\n",
		cmd.Linear.X, cmd.Linear.Y, cmd.Linear.Z,
		cmd.Angular.X, cmd.Angular.Y, cmd.Angular.Z,
		cmd.Timestamp)

	if err := c.velocityStream.Send(cmd); err != nil {
		return fmt.Errorf("failed to send velocity command: %w", err)
	}

	fmt.Println("[gRPC] Send() returned successfully")
	return nil
}

func (c *GRPCControlClient) CloseVelocityStream() (*pb.ControlResponse, error) {
	c.mu.Lock()
	defer c.mu.Unlock()

	if c.velocityStream == nil {
		return nil, fmt.Errorf("velocity stream not started")
	}

	resp, err := c.velocityStream.CloseAndRecv()
	if err != nil {
		return nil, fmt.Errorf("failed to close velocity stream: %w", err)
	}

	c.velocityStream = nil
	return resp, nil
}

func (c *GRPCControlClient) Close() error {
	c.mu.Lock()
	defer c.mu.Unlock()

	if c.velocityStream != nil {
		c.velocityStream.CloseAndRecv()
		c.velocityStream = nil
	}

	if c.conn != nil {
		err := c.conn.Close()
		c.conn = nil
		c.client = nil
		return err
	}

	return nil
}
