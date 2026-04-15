package transport

import (
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/gorilla/websocket"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/mock"
	"github.com/stretchr/testify/require"
)

// WebSocket server upgrader for testing use
var upgrader = websocket.Upgrader{
	CheckOrigin: func(r *http.Request) bool { return true },
}

func newTestWebSocketServer(t *testing.T, handler func(conn *websocket.Conn)) *httptest.Server {
	return httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		conn, err := upgrader.Upgrade(w, r, nil)
		if err != nil {
			t.Fatalf("Failed to upgrade connection: %v", err)
			return
		}
		defer conn.Close()
		handler(conn)
	}))
}

func toWSURL(url string) string {
	return strings.Replace(url, "http://", "ws://", 1)
}

// WebSocketDialer tests
func TestNewWebSocketDialer(t *testing.T) {
	dialer := NewWebSocketDialer()
	require.NotNil(t, dialer)
}

func TestWebSocketDialer_Dial_Success(t *testing.T) {
	// Arrange
	server := newTestWebSocketServer(t, func(conn *websocket.Conn) {
		for {
			_, _, err := conn.ReadMessage()
			if err != nil {
				return
			}
		}
	})
	defer server.Close()

	dialer := NewWebSocketDialer()

	// Act
	transport, err := dialer.Dial(context.Background(), toWSURL(server.URL))

	// Assert
	require.NoError(t, err)
	require.NotNil(t, transport)
	defer transport.Close()
}

func TestWebSocketDialer_Dial_InvalidURL(t *testing.T) {
	dialer := NewWebSocketDialer()

	// Act
	transport, err := dialer.Dial(context.Background(), "ws://invalid-host-12345.local:9999")

	// Assert
	require.Error(t, err)
	assert.Nil(t, transport)
}

func TestWebSocketDialer_Dial_InvalidScheme(t *testing.T) {
	dialer := NewWebSocketDialer()

	// Act
	transport, err := dialer.Dial(context.Background(), "http://localhost:8080")

	// Assert
	require.Error(t, err)
	assert.Nil(t, transport)
}

func TestWebSocketTransport_SendAndReceive_Success(t *testing.T) {
	// Arrange
	server := newTestWebSocketServer(t, func(conn *websocket.Conn) {
		_, data, err := conn.ReadMessage()
		if err == nil {
			// Echo back the received data as a response
			conn.WriteMessage(websocket.BinaryMessage, data)
		}
	})
	defer server.Close()

	dialer := NewWebSocketDialer()
	transport, err := dialer.Dial(context.Background(), toWSURL(server.URL))
	require.NoError(t, err)
	defer transport.Close()

	// Act
	testData := []byte("hello world")
	resp, err := transport.SendAndReceive(testData, 2*time.Second)

	// Assert
	require.NoError(t, err)
	assert.Equal(t, testData, resp)
}

func TestWebSocketTransport_SendAndReceive_EmptyData(t *testing.T) {
	// Arrange
	server := newTestWebSocketServer(t, func(conn *websocket.Conn) {
		_, data, err := conn.ReadMessage()
		if err == nil {
			conn.WriteMessage(websocket.BinaryMessage, data)
		}
	})
	defer server.Close()

	dialer := NewWebSocketDialer()
	transport, err := dialer.Dial(context.Background(), toWSURL(server.URL))
	require.NoError(t, err)
	defer transport.Close()

	// Act
	resp, err := transport.SendAndReceive([]byte{}, 2*time.Second)

	// Assert
	require.NoError(t, err)
	assert.Empty(t, resp)
}

func TestWebSocketTransport_OnMessage_ReceivesData(t *testing.T) {
	// Arrange
	testData := []byte("server message")
	clientReady := make(chan struct{})

	server := newTestWebSocketServer(t, func(conn *websocket.Conn) {
		// Wait for the client to be ready before sending a message
		<-clientReady
		conn.WriteMessage(websocket.BinaryMessage, testData)
		// Keep the connection open for a short time to allow message processing
		time.Sleep(100 * time.Millisecond)
	})
	defer server.Close()

	dialer := NewWebSocketDialer()
	transport, err := dialer.Dial(context.Background(), toWSURL(server.URL))
	require.NoError(t, err)
	defer transport.Close()

	receivedData := make(chan []byte, 1)

	// Act
	transport.OnMessage(func(data []byte) {
		receivedData <- data
	})

	// Notify server that client is ready to receive messages
	close(clientReady)

	// Assert
	select {
	case received := <-receivedData:
		assert.Equal(t, testData, received)
	case <-time.After(2 * time.Second):
		t.Fatal("Timeout waiting for message")
	}
}

func TestWebSocketTransport_OnMessage_MultipleMessages(t *testing.T) {
	// Arrange
	messages := [][]byte{
		[]byte("message 1"),
		[]byte("message 2"),
		[]byte("message 3"),
	}
	clientReady := make(chan struct{})

	server := newTestWebSocketServer(t, func(conn *websocket.Conn) {
		// wait for the client to be ready before sending messages
		<-clientReady
		for _, msg := range messages {
			conn.WriteMessage(websocket.BinaryMessage, msg)
			time.Sleep(10 * time.Millisecond)
		}
		time.Sleep(100 * time.Millisecond)
	})
	defer server.Close()

	dialer := NewWebSocketDialer()
	transport, err := dialer.Dial(context.Background(), toWSURL(server.URL))
	require.NoError(t, err)
	defer transport.Close()

	// Notify server that client is ready to receive messages
	close(clientReady)

	var receivedMessages [][]byte
	var mu sync.Mutex
	done := make(chan bool)

	// Act
	transport.OnMessage(func(data []byte) {
		mu.Lock()
		receivedMessages = append(receivedMessages, data)
		if len(receivedMessages) >= len(messages) {
			done <- true
		}
		mu.Unlock()
	})

	// Assert
	select {
	case <-done:
		assert.Len(t, receivedMessages, len(messages))
	case <-time.After(2 * time.Second):
		t.Fatal("Timeout waiting for all messages")
	}
}

func TestWebSocketTransport_Close_Success(t *testing.T) {
	// Arrange
	server := newTestWebSocketServer(t, func(conn *websocket.Conn) {
		for {
			_, _, err := conn.ReadMessage()
			if err != nil {
				return
			}
		}
	})
	defer server.Close()

	dialer := NewWebSocketDialer()
	transport, err := dialer.Dial(context.Background(), toWSURL(server.URL))
	require.NoError(t, err)

	// Act
	err = transport.Close()

	// Assert
	require.NoError(t, err)
}

func TestWebSocketTransport_Close_SendAfterCloseFails(t *testing.T) {
	// Arrange
	server := newTestWebSocketServer(t, func(conn *websocket.Conn) {
		for {
			_, _, err := conn.ReadMessage()
			if err != nil {
				return
			}
		}
	})
	defer server.Close()

	dialer := NewWebSocketDialer()
	transport, err := dialer.Dial(context.Background(), toWSURL(server.URL))
	require.NoError(t, err)

	// Act
	transport.Close()
	_, err = transport.SendAndReceive([]byte("test"), 1*time.Second)

	// Assert
	require.Error(t, err)
}

func TestTransportInterface_MockImplementation(t *testing.T) {
	// Arrange
	mockTransport := new(MockTransport)
	mockTransport.On("SendAndReceive", []byte("test"), mock.Anything).Return([]byte("response"), nil)
	mockTransport.On("Close").Return(nil)
	mockTransport.On("OnMessage", mock.Anything).Return()

	// Act & Assert
	var transport Transport = mockTransport

	resp, err := transport.SendAndReceive([]byte("test"), 5*time.Second)
	assert.NoError(t, err)
	assert.Equal(t, []byte("response"), resp)

	transport.OnMessage(func(data []byte) {})

	err = transport.Close()
	assert.NoError(t, err)

	mockTransport.AssertExpectations(t)
}

func TestDialerInterface_MockImplementation(t *testing.T) {
	// Arrange
	mockTransport := new(MockTransport)
	mockDialer := new(MockDialer)
	mockDialer.On("Dial", mock.Anything, "ws://test.com").Return(mockTransport, nil)

	// Act
	var dialer Dialer = mockDialer
	transport, err := dialer.Dial(context.Background(), "ws://test.com")

	// Assert
	require.NoError(t, err)
	assert.NotNil(t, transport)
	mockDialer.AssertExpectations(t)
}

func TestWebSocketTransport_ConcurrentSendAndReceive(t *testing.T) {
	// Arrange
	messageCount := 10

	server := newTestWebSocketServer(t, func(conn *websocket.Conn) {
		for {
			_, data, err := conn.ReadMessage()
			if err != nil {
				return
			}
			// Echo back the data
			conn.WriteMessage(websocket.BinaryMessage, data)
		}
	})
	defer server.Close()

	dialer := NewWebSocketDialer()
	transport, err := dialer.Dial(context.Background(), toWSURL(server.URL))
	require.NoError(t, err)
	defer transport.Close()

	// Act - Send multiple messages sequentially (SendAndReceive is request-response)
	for i := 0; i < messageCount; i++ {
		resp, err := transport.SendAndReceive([]byte{byte(i)}, 2*time.Second)
		require.NoError(t, err)
		assert.Equal(t, []byte{byte(i)}, resp)
	}
}
