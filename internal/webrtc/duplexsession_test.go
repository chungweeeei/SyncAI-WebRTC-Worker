package webrtc

import (
	"sync"
	"testing"

	"github.com/stretchr/testify/assert"
)

func resetDuplexSessions() {
	duplexSessionsMu.Lock()
	duplexSessions = make(map[string]*DuplexSession)
	duplexSessionsMu.Unlock()
}

// Idempotent like the WHEP export, not like the WHIP one: a duplex session
// self-closes on connection failure, so "already gone" is the normal case for
// a browser that dropped off the network.
func TestDeleteDuplexSession_NotFound(t *testing.T) {
	resetDuplexSessions()

	err := DeleteDuplexSession("non-existent-session-id")

	assert.NoError(t, err)
}

func TestDeleteDuplexSession_Success(t *testing.T) {
	resetDuplexSessions()

	mockPC := new(MockPeerConnection)
	mockPC.On("Close").Return(nil)

	session := &DuplexSession{ID: "test-duplex-session", PC: mockPC}

	duplexSessionsMu.Lock()
	duplexSessions["test-duplex-session"] = session
	duplexSessionsMu.Unlock()

	err := DeleteDuplexSession("test-duplex-session")

	assert.NoError(t, err)

	duplexSessionsMu.Lock()
	_, exists := duplexSessions["test-duplex-session"]
	duplexSessionsMu.Unlock()

	assert.False(t, exists, "session should be deleted from duplexSessions map")
	mockPC.AssertCalled(t, "Close")
}

func TestDuplexSession_CloseWithAllNil(t *testing.T) {
	session := &DuplexSession{ID: "test-duplex-session"}

	assert.NotPanics(t, func() { session.Close() })
	assert.True(t, session.closed)
}

// Close runs from two places -- DeleteDuplexSession and the connection-state
// callback -- so the second one has to be free. Both devices would otherwise
// be released twice.
func TestDuplexSession_Close_Idempotent(t *testing.T) {
	resetDuplexSessions()

	mockPC := new(MockPeerConnection)
	mockPC.On("Close").Return(nil)

	session := &DuplexSession{ID: "test-duplex-session", PC: mockPC}

	session.Close()
	session.Close()

	mockPC.AssertNumberOfCalls(t, "Close", 1)
}

func TestDuplexSession_Close_Concurrent(t *testing.T) {
	mockPC := new(MockPeerConnection)
	mockPC.On("Close").Return(nil)

	session := &DuplexSession{ID: "test-duplex-session", PC: mockPC}

	var wg sync.WaitGroup
	for i := 0; i < 8; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			session.Close()
		}()
	}
	wg.Wait()

	mockPC.AssertNumberOfCalls(t, "Close", 1)
}

func TestDuplexSessionOptions_CarriesBothDirections(t *testing.T) {
	opts := DuplexSessionOptions{}

	// The three ports are distinct roles, and mixing them up is the kind of
	// mistake that produces a session where the browser hears the robot
	// through its own microphone port. Named here so a reordering of the
	// struct cannot silently swap them.
	assert.Equal(t, 0, opts.VideoRTPPort, "camera video out")
	assert.Equal(t, 0, opts.AudioRTPPort, "robot audio out")
	assert.Equal(t, 0, opts.MicRTPPort, "browser microphone in")
	assert.Nil(t, opts.PCFactory)
	assert.Nil(t, opts.PipelineFactory)
}
