package main

import (
	"encoding/json"
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestSuccessResult(t *testing.T) {
	assert.JSONEq(t, `{"ok":true}`, successResult())
}

func TestSessionResult(t *testing.T) {
	got := sessionResult("session-123", "v=0\r\na=answer\r\n")

	var r result
	assert.NoError(t, json.Unmarshal([]byte(got), &r))
	assert.True(t, r.OK)
	assert.Equal(t, "session-123", r.SessionID)
	assert.Equal(t, "v=0\r\na=answer\r\n", r.AnswerSDP)
	assert.Empty(t, r.Error)
}

func TestErrorResult(t *testing.T) {
	got := errorResult("session not found: abc")

	var r result
	assert.NoError(t, json.Unmarshal([]byte(got), &r))
	assert.False(t, r.OK)
	assert.Equal(t, "session not found: abc", r.Error)
	assert.Empty(t, r.SessionID)
	assert.Empty(t, r.AnswerSDP)
}

func TestErrorResult_EscapesSpecialCharacters(t *testing.T) {
	got := errorResult(`failed: "quoted" and \backslash`)

	var r result
	assert.NoError(t, json.Unmarshal([]byte(got), &r))
	assert.Equal(t, `failed: "quoted" and \backslash`, r.Error)
}
