# CLAUDE.md

This file provides guidance to Claude Code (claude.ai/code) when working with code in this repository.

## Project Overview

SyncAI WebRTC Worker is a Go service that bridges WebRTC connections to robot systems for real-time control and audio/video streaming. It uses WHIP (ingestion) for control and audio input, and WHEP (egress) for video output.

## Build & Run Commands

```bash
# Build (requires GStreamer dev libraries)
go build -o syncai-webrtc-worker cmd/main.go

# Docker build (outputs to ./dist/)
docker-compose up build-service

# Run (listens on :18000)
./syncai-webrtc-worker

# Test
go test ./...

# Test with race detection (CI mode)
go test -v -race ./...

# Run a single test
go test -v -run TestFunctionName ./internal/webrtc/

# Lint
go vet ./...
```

**GStreamer dependencies (Ubuntu/Debian):**
```bash
sudo apt-get install -y pkg-config libgstreamer1.0-dev libgstreamer-plugins-base1.0-dev \
    gstreamer1.0-plugins-good gstreamer1.0-plugins-bad gstreamer1.0-plugins-ugly
```

## Architecture

The service runs three independent WebRTC session types, each with its own endpoint:

- **Control** (`POST /control/whip`) — DataChannel-based session. Client sends JSON messages with action type `CONTROL` (velocity commands → gRPC) or `TASK` (robot commands like MOVE, TTS → WebSocket).
- **Audio** (`POST /audio/whip`) — WHIP session receiving audio tracks from the client, piped through GStreamer for processing.
- **Video** (`POST /video/whep`) — WHEP session sending video+audio tracks to the client from either a physical camera (`/dev/syncai_camera`) or a ROS UDP source.

### Key data flow

```
Client → DataChannel → Session → actions parser → RobotGateway → gRPC (velocity) / WebSocket (tasks)
Client → WebRTC audio track → AudioSession → GStreamer pipeline → RTP (port 5004)
GStreamer capture → RTP (ports 5006/5007) → VideoSession → WebRTC tracks → Client
```

### Package structure

- `cmd/main.go` — Entry point: initializes GStreamer, loads config, starts Gin router
- `internal/api/` — Gin router and HTTP handlers for WHIP/WHEP endpoints
- `internal/webrtc/` — Session types (control, audio, video) and action/command parsing
- `internal/gateway/` — RobotGateway: communicates with robot via gRPC (velocity) and Unix socket (RTSP pusher)
- `internal/transport/` — Pluggable transport layer (gRPC client for velocity streaming, WebSocket for task commands)
- `internal/proc/` — GStreamer pipeline construction for video capture and audio processing
- `internal/config/` — Environment-based configuration (AppConfig, GStreamerConfig)
- `internal/pb/` — Generated protobuf code from `protos/control_service.proto`
- `protos/` — Proto definitions (ControlService with StreamVelocity RPC)

### Testing approach

Tests use interface-based mocking (PeerConnectionInterface, PipelineFactory, SessionManager, Transport) with `stretchr/testify` for assertions. No external test frameworks or test databases needed.

## Configuration

Environment variables (or `.env` file):

| Variable | Default | Description |
|----------|---------|-------------|
| `AUDIO_RTP_PORT` | `5004` | Audio RTP port |
| `VIDEO_RTP_PORT` | `5006` | Video RTP port |
| `VIDEO_AUDIO_RTP_PORT` | `5007` | Video audio track RTP port |
| `AUDIO_VOLUME` | `0.2` | Audio volume (0.0-1.0) |
| `STUN_SERVERS` | `stun:stun.l.google.com:19302` | Comma-separated STUN URLs |
| `TURN_SERVERS` | (empty) | Comma-separated TURN URLs |
| `TURN_USERNAME` | (empty) | TURN auth username |
| `TURN_PASSWORD` | (empty) | TURN auth password |

## External Dependencies

- **GStreamer 1.0** for media capture/processing (uses NVIDIA `nvv4l2h264enc` on supported hardware)
- **gRPC server** expected at `127.0.0.1:50052` for robot velocity control
- **Unix socket** at `/tmp/rtsp_pusher.sock` for ROS RTSP integration

## Proto Generation

The gRPC service is defined in `protos/control_service.proto`. Generated Go code lives in `internal/pb/`.
