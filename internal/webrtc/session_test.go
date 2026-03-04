package webrtc

import (
	"SyncAI-WebRTC-Worker/internal/transport"
	"context"
	"errors"
	"sync"
	"testing"

	"github.com/pion/webrtc/v4"
)

// Mock Transport
type mockTransport struct {
	sendCalled    bool
	closeCalled   bool
	sentData      []byte
	onMessageFunc transport.MessageHandler
	sendErr       error
	mu            sync.Mutex
}

// make sure mockTransport implements transport.Transport interface
var _ transport.Transport = (*mockTransport)(nil)

func (m *mockTransport) Send(data []byte) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.sendCalled = true
	m.sentData = data
	return m.sendErr
}

func (m *mockTransport) Close() error {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.closeCalled = true
	return nil
}

func (m *mockTransport) OnMessage(handler transport.MessageHandler) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.onMessageFunc = handler
}

// Mock Dialer
type mockDialer struct {
	transport transport.Transport
	shouldErr bool
	dialURL   string
}

// make sure mockDialer implements transport.Dialer interface
var _ transport.Dialer = (*mockDialer)(nil)

func (d *mockDialer) Dial(ctx context.Context, url string) (transport.Transport, error) {
	d.dialURL = url
	if d.shouldErr {
		return nil, errors.New("dial error")
	}
	return d.transport, nil
}

func createValidOffer(t *testing.T) string {
	t.Helper()
	pc, err := webrtc.NewPeerConnection(webrtc.Configuration{})
	if err != nil {
		t.Fatalf("failed to create peer connection: %v", err)
	}
	defer pc.Close()

	// Create data channel to generate a valid offer
	_, err = pc.CreateDataChannel("test", nil)
	if err != nil {
		t.Fatalf("failed to create data channel: %v", err)
	}

	offer, err := pc.CreateOffer(nil)
	if err != nil {
		t.Fatalf("failed to create offer: %v", err)
	}

	return offer.SDP
}

func TestCreateSession_Success(t *testing.T) {
	// Clear previous sessions
	sessionsMu.Lock()
	sessions = make(map[string]*Session)
	sessionsMu.Unlock()

	ctx := context.Background()
	offerSDP := createValidOffer(t)

	sessionID, answerSDP, err := CreateSession(ctx, offerSDP, SessionOptions{})

	if err != nil {
		t.Fatalf("CreateSession failed: %v", err)
	}

	if sessionID == "" {
		t.Error("sessionID should not be empty")
	}

	if answerSDP == "" {
		t.Error("answerSDP should not be empty")
	}

	sessionsMu.Lock()
	_, exists := sessions[sessionID]
	sessionsMu.Unlock()

	if !exists {
		t.Error("session should be stored in sessions map")
	}

	DeleteSession(sessionID)
}

func TestCreateSession_WithWebSocket(t *testing.T) {
	sessionsMu.Lock()
	sessions = make(map[string]*Session)
	sessionsMu.Unlock()

	ctx := context.Background()
	offerSDP := createValidOffer(t)

	mockTrans := &mockTransport{}
	dialer := &mockDialer{transport: mockTrans}

	sessionID, answerSDP, err := CreateSession(ctx, offerSDP, SessionOptions{
		WebSocketURL: "ws://localhost:8080/ws",
		Dialer:       dialer,
	})

	if err != nil {
		t.Fatalf("CreateSession failed: %v", err)
	}

	if sessionID == "" {
		t.Error("sessionID should not be empty")
	}

	if answerSDP == "" {
		t.Error("answerSDP should not be empty")
	}

	// make sure Dialer.Dial was called with the correct URL
	if dialer.dialURL != "ws://localhost:8080/ws" {
		t.Errorf("expected dial URL 'ws://localhost:8080/ws', got '%s'", dialer.dialURL)
	}

	// make sure OnMessage handler is set
	mockTrans.mu.Lock()
	hasHandler := mockTrans.onMessageFunc != nil
	mockTrans.mu.Unlock()

	if !hasHandler {
		t.Error("OnMessage handler should be set")
	}

	// Clean up
	DeleteSession(sessionID)
}

func TestCreateSession_WebSocketDialError(t *testing.T) {
	sessionsMu.Lock()
	sessions = make(map[string]*Session)
	sessionsMu.Unlock()

	ctx := context.Background()
	offerSDP := createValidOffer(t)

	dialer := &mockDialer{shouldErr: true}

	_, _, err := CreateSession(ctx, offerSDP, SessionOptions{
		WebSocketURL: "ws://localhost:8080/ws",
		Dialer:       dialer,
	})

	if err == nil {
		t.Error("CreateSession should fail when WebSocket dial fails")
	}
}

func TestCreateSession_InvalidSDP(t *testing.T) {
	sessionsMu.Lock()
	sessions = make(map[string]*Session)
	sessionsMu.Unlock()

	ctx := context.Background()
	invalidSDP := "invalid sdp"

	_, _, err := CreateSession(ctx, invalidSDP, SessionOptions{})

	if err == nil {
		t.Error("CreateSession should fail with invalid SDP")
	}
}

func TestDeleteSession_Success(t *testing.T) {
	sessionsMu.Lock()
	sessions = make(map[string]*Session)
	sessionsMu.Unlock()

	ctx := context.Background()
	offerSDP := createValidOffer(t)

	sessionID, _, err := CreateSession(ctx, offerSDP, SessionOptions{})
	if err != nil {
		t.Fatalf("CreateSession failed: %v", err)
	}

	err = DeleteSession(sessionID)
	if err != nil {
		t.Fatalf("DeleteSession failed: %v", err)
	}

	sessionsMu.Lock()
	_, exists := sessions[sessionID]
	sessionsMu.Unlock()

	if exists {
		t.Error("session should be deleted from sessions map")
	}
}

func TestDeleteSession_WithTransport(t *testing.T) {
	sessionsMu.Lock()
	sessions = make(map[string]*Session)
	sessionsMu.Unlock()

	ctx := context.Background()
	offerSDP := createValidOffer(t)

	mockTrans := &mockTransport{}
	dialer := &mockDialer{transport: mockTrans}

	sessionID, _, err := CreateSession(ctx, offerSDP, SessionOptions{
		WebSocketURL: "ws://localhost:8080/ws",
		Dialer:       dialer,
	})
	if err != nil {
		t.Fatalf("CreateSession failed: %v", err)
	}

	err = DeleteSession(sessionID)
	if err != nil {
		t.Fatalf("DeleteSession failed: %v", err)
	}

	mockTrans.mu.Lock()
	closed := mockTrans.closeCalled
	mockTrans.mu.Unlock()

	if !closed {
		t.Error("Transport.Close should be called during DeleteSession")
	}
}

func TestDeleteSession_NotFound(t *testing.T) {
	sessionsMu.Lock()
	sessions = make(map[string]*Session)
	sessionsMu.Unlock()

	err := DeleteSession("non-existent-session-id")

	if err == nil {
		t.Error("DeleteSession should fail for non-existent session")
	}
}

func TestSession_Cleanup(t *testing.T) {
	mockTrans := &mockTransport{}

	pc, err := webrtc.NewPeerConnection(webrtc.Configuration{})
	if err != nil {
		t.Fatalf("failed to create peer connection: %v", err)
	}

	session := &Session{
		ID:        "test-session",
		PC:        pc,
		Transport: mockTrans,
	}

	session.cleanup()

	mockTrans.mu.Lock()
	closed := mockTrans.closeCalled
	mockTrans.mu.Unlock()

	if !closed {
		t.Error("Transport.Close should be called during cleanup")
	}
}

func TestSession_CleanupWithNilTransport(t *testing.T) {
	pc, err := webrtc.NewPeerConnection(webrtc.Configuration{})
	if err != nil {
		t.Fatalf("failed to create peer connection: %v", err)
	}

	session := &Session{
		ID:        "test-session",
		PC:        pc,
		Transport: nil,
	}

	session.cleanup()
}

func TestGenerateSessionID(t *testing.T) {
	id1 := generateSessionID()
	id2 := generateSessionID()

	if id1 == "" {
		t.Error("generateSessionID should not return empty string")
	}

	if id1 == id2 {
		t.Error("generateSessionID should generate unique IDs")
	}

	if len(id1) != 36 {
		t.Errorf("expected UUID length 36, got %d", len(id1))
	}
}

func TestMultipleSessions(t *testing.T) {
	sessionsMu.Lock()
	sessions = make(map[string]*Session)
	sessionsMu.Unlock()

	ctx := context.Background()
	offerSDP := createValidOffer(t)

	var sessionIDs []string
	for i := 0; i < 3; i++ {
		sessionID, _, err := CreateSession(ctx, offerSDP, SessionOptions{})
		if err != nil {
			t.Fatalf("CreateSession %d failed: %v", i, err)
		}
		sessionIDs = append(sessionIDs, sessionID)
	}

	sessionsMu.Lock()
	if len(sessions) != 3 {
		t.Errorf("expected 3 sessions, got %d", len(sessions))
	}
	sessionsMu.Unlock()

	for _, id := range sessionIDs {
		DeleteSession(id)
	}

	sessionsMu.Lock()
	if len(sessions) != 0 {
		t.Errorf("expected 0 sessions after cleanup, got %d", len(sessions))
	}
	sessionsMu.Unlock()
}
