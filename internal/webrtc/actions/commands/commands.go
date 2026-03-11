package commands

import (
	"encoding/json"
	"fmt"
	"math"

	"github.com/google/uuid"
)

type CommandType string

const (
	CommandTypeMove    CommandType = "MOVE"
	CommandTypeStandup CommandType = "STANDUP"
	CommandTypeSitdown CommandType = "SITDOWN"
	CommandTypeHead    CommandType = "HEAD"
	CommandTypeTTS     CommandType = "TTS"
	CommandTypeWait    CommandType = "WAIT"
)

const (
	RobotMoveCommandID   = 1005
	RobotStatusID        = 1009
	RobotMotionControlID = 1013
	RobotTTSCommandID    = 1014
)

const (
	StandUpActionID = 3
	SitDownActionID = 4
)

// Command Params for each command type
type CommandParams interface{}

type CommandRequest struct {
	Op      string `json:"op"`
	Service string `json:"service"`
	Type    string `json:"type"`
	Args    struct {
		Data any `json:"data"`
	} `json:"args"`
}

// Parse Params for each command type
func GenerateCommandRequest(cmdType CommandType, rawData json.RawMessage) (*CommandRequest, error) {
	var cmdReq CommandRequest
	switch CommandType(cmdType) {
	case CommandTypeMove:
		var params MoveParams
		if err := json.Unmarshal(rawData, &params); err != nil {
			return nil, err
		}

		// generate data
		requestData := map[string]interface{}{
			"api_id":      RobotMoveCommandID,
			"mission_id":  uuid.New().String(),
			"x":           params.TargetX,
			"y":           params.TargetY,
			"orientation": params.TargetOrientation * (math.Pi / 180.0), //degree to radian
		}

		requestDataBytes, err := json.Marshal(requestData)
		if err != nil {
			return nil, err
		}

		cmdReq.Op = "call_service"
		cmdReq.Service = "/api/system"
		cmdReq.Type = "custom_msgs/srv/Api"
		cmdReq.Args.Data = string(requestDataBytes)

		return &cmdReq, nil

	case CommandTypeStandup:

		requestData := map[string]interface{}{
			"api_id":     RobotMotionControlID,
			"mission_id": uuid.New().String(),
			"action":     StandUpActionID,
		}

		requestDataBytes, err := json.Marshal(requestData)
		if err != nil {
			return nil, err
		}

		cmdReq.Op = "call_service"
		cmdReq.Service = "/api/system"
		cmdReq.Type = "custom_msgs/srv/Api"
		cmdReq.Args.Data = string(requestDataBytes)

		return &cmdReq, nil

	case CommandTypeSitdown:

		requestData := map[string]interface{}{
			"api_id":     RobotMotionControlID,
			"mission_id": uuid.New().String(),
			"action":     SitDownActionID,
		}

		requestDataBytes, err := json.Marshal(requestData)
		if err != nil {
			return nil, err
		}

		cmdReq.Op = "call_service"
		cmdReq.Service = "/api/system"
		cmdReq.Type = "custom_msgs/srv/Api"
		cmdReq.Args.Data = string(requestDataBytes)

		return &cmdReq, nil

	case CommandTypeHead:
		var params HeadParams
		if err := json.Unmarshal(rawData, &params); err != nil {
			return nil, err
		}

		cmdReq.Op = "call_service"
		cmdReq.Service = "/set_angle_tag"
		cmdReq.Type = "custom_msgs/srv/SetFloat"
		cmdReq.Args.Data = params.Angle

		return &cmdReq, nil

	case CommandTypeTTS:
		var params TTSParams
		if err := json.Unmarshal(rawData, &params); err != nil {
			return nil, err
		}

		requestData := map[string]interface{}{
			"api_id":     RobotTTSCommandID,
			"text":       params.Text,
			"voice_name": "English-US.Male-1",
		}

		requestDataBytes, err := json.Marshal(requestData)
		if err != nil {
			return nil, err
		}

		cmdReq.Op = "call_service"
		cmdReq.Service = "/api/system"
		cmdReq.Type = "custom_msgs/srv/Api"
		cmdReq.Args.Data = string(requestDataBytes)
		return &cmdReq, nil

	default:
		return nil, fmt.Errorf("unknown command type: %s", cmdType)
	}
}
