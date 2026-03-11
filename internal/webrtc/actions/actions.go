package actions

import (
	"SyncAI-WebRTC-Worker/internal/webrtc/actions/commands"
	"encoding/json"
	"fmt"
)

type ActionType string

const (
	ActionTypeControl ActionType = "CONTROL"
	ActionTypeTask    ActionType = "TASK"
)

type BaseMessage struct {
	Action    ActionType      `json:"action"`
	ID        string          `json:"id"`
	Timestamp int64           `json:"timestamp"`
	Payload   json.RawMessage `json:"payload"`
}

type ControlPayload struct {
	Linear struct {
		X float64 `json:"x"`
		Y float64 `json:"y"`
		Z float64 `json:"z"`
	} `json:"linear"`
	Angular struct {
		X float64 `json:"x"`
		Y float64 `json:"y"`
		Z float64 `json:"z"`
	} `json:"angular"`
}

type CommandPayload struct {
	Steps []StepPayload `json:"steps"`
}

type StepPayload struct {
	ID     string               `json:"id"`
	Name   string               `json:"name"`
	Type   commands.CommandType `json:"type"`
	Params json.RawMessage      `json:"params"`
}

// ParseMessage parses raw bytes into a BaseMessage struct
func ParseMessage(data []byte) (*BaseMessage, error) {
	var msg BaseMessage
	if err := json.Unmarshal(data, &msg); err != nil {
		return nil, err
	}
	return &msg, nil
}

// ParseControlPayload parses the payload as ControlPayload
func (m *BaseMessage) ParseControlPayload() (*ControlPayload, error) {
	var payload ControlPayload
	if err := json.Unmarshal(m.Payload, &payload); err != nil {
		return nil, fmt.Errorf("failed to parse control payload: %w", err)
	}
	return &payload, nil
}

// ParseCommandPayload parses the payload as CommandPayload
func (m *BaseMessage) ParseCommandPayload() (*CommandPayload, error) {
	var payload CommandPayload
	if err := json.Unmarshal(m.Payload, &payload); err != nil {
		return nil, fmt.Errorf("failed to parse command payload: %w", err)
	}
	return &payload, nil
}
