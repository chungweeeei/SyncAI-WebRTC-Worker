package transport

import (
	"context"
	"sync"

	"github.com/gorilla/websocket"
)

// WebSocketTransport implements the Transport interface using a WebSocket connection.
type WebSocketTransport struct {
	conn      *websocket.Conn
	onMessage MessageHandler
	mu        sync.Mutex
	done      chan struct{}
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
		conn: conn,
		done: make(chan struct{}),
	}

	go t.readLoop()

	return t, nil
}

func (t *WebSocketTransport) Send(data []byte) error {
	t.mu.Lock()
	defer t.mu.Unlock()
	return t.conn.WriteMessage(websocket.BinaryMessage, data)
}

func (t *WebSocketTransport) OnMessage(handler MessageHandler) {
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
			if t.onMessage != nil {
				t.onMessage(data)
			}
		}
	}
}
