# CLAUDE.md

This file provides guidance to Claude Code (claude.ai/code) when working with code in this repository.

## Project Overview

SyncAI WebRTC Worker is a Go service that bridges WebRTC connections to robot systems for real-time audio/video streaming. It uses WHIP (ingestion) for audio input, and WHEP (egress) for video output.

## Build & Run Commands

```bash
# Build (requires GStreamer dev libraries)
go build -o syncai-webrtc-worker cmd/main.go

# Docker build (outputs binary to ./dist/, mirrors CI)
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

**Commit messages** follow Conventional Commits (`feat:`, `fix:`, `docs:`, `chore:`, ...) — see `.github/prompt/copilot-commit-message-instructions.md`.

## Architecture

The service runs two independent WebRTC session types, each with its own endpoint pair. All WHIP/WHEP endpoints take `Content-Type: application/sdp` with an SDP offer in the body, return the answer SDP plus a `Location` header (`/<kind>/<proto>/:id`), and are closed via `DELETE` on that location.

- **Audio** (`POST /audio/whip`) — WHIP session receiving audio tracks from the client, piped through GStreamer for processing.
- **Video** (`POST /video/whep`) — WHEP session sending video (H264) + audio (Opus) tracks to the client from either a physical camera or a ROS UDP source.

### Key data flow

```
Client → WebRTC audio track → AudioSession → GStreamer pipeline → RTP (port 5004)
GStreamer capture → RTP (ports 5006/5007) → VideoSession UDP listeners → WebRTC tracks → Client
```

### Video source auto-detection

`config.detectVideoSource()` picks the source once at startup: **camera** mode if `/dev/syncai_camera` exists, otherwise **ROS** mode. In camera mode, creating a video session also spawns the GStreamer capture pipeline. In ROS mode, the ROS node is expected to already be publishing RTP to the video ports — the worker no longer signals it to start/stop.

### Session lifecycle

Each session type keeps its live sessions in a package-level map in `internal/webrtc/` keyed by UUID. Video sessions self-close on connection failure. Cleanup happens both on `DELETE` and on WebRTC connection-state changes.

### Package structure

- `cmd/main.go` — Entry point: initializes GStreamer, loads `.env`, starts Gin router on :18000
- `internal/api/` — Gin router (CORS config, dependency wiring) and HTTP handlers for WHIP/WHEP endpoints
- `internal/webrtc/` — Session types (audio, video)
- `internal/transport/` — Pluggable transport layer (WebSocket) (currently unwired — left over from the removed control session)
- `internal/proc/` — GStreamer pipeline construction for video capture and audio processing
- `internal/config/` — Environment-based configuration (AppConfig, GStreamerConfig)

### Testing approach

Tests use interface-based mocking (PeerConnectionInterface, PipelineFactory, SessionManager, Transport) with `stretchr/testify` for assertions. Mocks live in `mock_test.go` files next to the code they test. No external test frameworks or test databases needed; tests run without GStreamer/robot hardware.

## Configuration

Environment variables (or `.env` file in the working directory):

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

## CI & Deployment

- `test.yml` — PRs to `main`/`stage`/`dev`: `go vet` + `go test -v -race ./...` inside the `syncai-webrtc-worker-base` container image
- `build.yml` — pushes to those branches: builds the binary, smoke-tests it on :18000, packages binary + systemd unit into a timestamped tarball artifact
- `publish-image.yml` — Dockerfile changes: pushes the builder stage to GHCR as the build base image (not a runtime image)

Production runs the binary under systemd on the target device: `scripts/deploy.sh <tarball>` installs to `/opt/syncai-webrtc-worker/` and enables `syncai-webrtc-worker.service` (which loads `.env` via `EnvironmentFile=`). See README for details.
