package transport

import (
	"context"
	"fmt"
	"sync"
	"time"

	"github.com/gorilla/websocket"
)

// WebSocketTransport implements the Transport interface using a WebSocket connection.
type WebSocketTransport struct {
	conn       *websocket.Conn
	onMessage  MessageHandler
	mu         sync.Mutex
	handleMu   sync.RWMutex
	done       chan struct{}
	responseCh chan []byte
}

// WebSocketDialer implements the Dialer interface for WebSocket connections.
type WebSocketDialer struct{}

func NewWebSocketDialer() *WebSocketDialer {
	return &WebSocketDialer{}
}

func (d *WebSocketDialer) Dial(ctx context.Context, url string) (Transport, error) {
	conn, _, err := websocket.DefaultDialer.Dial(url, nil)
	if err != nil {
		return nil, err
	}

	t := &WebSocketTransport{
		conn:       conn,
		done:       make(chan struct{}),
		responseCh: make(chan []byte, 1),
	}

	go t.readLoop()

	return t, nil
}

func (t *WebSocketTransport) SendAndReceive(data []byte, timeout time.Duration) ([]byte, error) {
	// clear previous data in channel
	select {
	case <-t.responseCh:
	default:
	}

	// send request
	t.mu.Lock()
	err := t.conn.WriteMessage(websocket.BinaryMessage, data)
	t.mu.Unlock()
	if err != nil {
		return nil, fmt.Errorf("failed to send: %w", err)
	}

	// waiting for response
	select {
	case resp := <-t.responseCh:
		return resp, nil
	case <-time.After(timeout):
		return nil, fmt.Errorf("timeout waiting for response")
	case <-t.done:
		return nil, fmt.Errorf("connection closed")
	}
}

func (t *WebSocketTransport) OnMessage(handler MessageHandler) {
	t.handleMu.Lock()
	defer t.handleMu.Unlock()
	t.onMessage = handler
}

func (t *WebSocketTransport) Close() error {
	close(t.done)
	return t.conn.Close()
}

func (t *WebSocketTransport) readLoop() {
	for {
		select {
		case <-t.done:
			return
		default:
			_, data, err := t.conn.ReadMessage()
			if err != nil {
				return
			}

			// send result to response channel if waiting
			select {
			case t.responseCh <- data:
			default:
			}

			t.handleMu.RLock()
			handler := t.onMessage
			t.handleMu.RUnlock()
			if handler != nil {
				handler(data)
			}
		}
	}
}
