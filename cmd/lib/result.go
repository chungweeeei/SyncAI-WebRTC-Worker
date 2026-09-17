package main

import "encoding/json"

// result is the JSON envelope returned by every exported function.
// success: {"ok":true} (+ session_id/answer_sdp for create calls)
// failure: {"ok":false,"error":"..."}
type result struct {
	OK        bool   `json:"ok"`
	SessionID string `json:"session_id,omitempty"`
	AnswerSDP string `json:"answer_sdp,omitempty"`
	Error     string `json:"error,omitempty"`
}

func marshalResult(r result) string {
	data, err := json.Marshal(r)
	if err != nil {
		// result only contains strings and a bool; marshaling cannot
		// realistically fail, but never return invalid JSON to the host.
		return `{"ok":false,"error":"failed to marshal result"}`
	}
	return string(data)
}

func successResult() string {
	return marshalResult(result{OK: true})
}

func sessionResult(sessionID, answerSDP string) string {
	return marshalResult(result{OK: true, SessionID: sessionID, AnswerSDP: answerSDP})
}

func errorResult(msg string) string {
	return marshalResult(result{OK: false, Error: msg})
}
