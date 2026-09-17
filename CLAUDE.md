# CLAUDE.md

This file provides guidance to Claude Code (claude.ai/code) when working with code in this repository.

## Project Overview

SyncAI WebRTC Worker is the Go WebRTC session core for robot audio/video streaming: WHIP-style audio ingestion from the client and WHEP-style video egress to the client. It is currently **headless** — the former Gin HTTP layer has been removed, and the plan is to compile this module as a c-shared library (`go build -buildmode=c-shared`) embedded in a Python/FastAPI host, which will own the HTTP surface (WHIP/WHEP endpoints, CORS, Location headers).

## Build & Run Commands

```bash
# Build the c-shared library for the Python/FastAPI host (also emits dist/libsyncai_worker.h)
# Requires GStreamer dev libraries
go build -buildmode=c-shared -o dist/libsyncai_worker.so ./cmd/lib

# Docker build (outputs libsyncai_worker.so + .h to ./dist/ via the builder stage)
docker-compose up build-lib

# Test
go test ./...

# Test with race detection
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

The public surface is the `internal/webrtc` package — two independent WebRTC session types, each managed by a create/delete function pair that takes an SDP offer string and returns a session ID plus the answer SDP:

- **Audio** — `CreateAudioSession` / `DeleteAudioSession`: WHIP-style session receiving audio tracks from the client, piped through a GStreamer pipeline and pushed out as RTP.
- **Video** — `CreateVideoSession` / `DeleteVideoSession`: WHEP-style session sending video (H264) + audio (Opus) tracks to the client from the physical camera.

`CreateVideoSession` blocks until ICE gathering completes (can take up to a few seconds), so hosts must not call it on a latency-sensitive thread/event loop.

### Key data flow

```
Client → WebRTC audio track → AudioSession → GStreamer pipeline → RTP (port 5004)
GStreamer capture → RTP (ports 5006/5007) → VideoSession UDP listeners → WebRTC tracks → Client
```

### Video source

The video source is the physical camera (`/dev/syncai_camera`): creating a video session also spawns the GStreamer capture pipeline reading from it. There is no source-type switch — the former `VideoSourceType` enum was removed along with the ROS source mode.

### Session lifecycle

Each session type keeps its live sessions in a package-level map in `internal/webrtc/` keyed by UUID. Video sessions self-close on connection failure. Cleanup happens both via `Delete*Session` calls and on WebRTC connection-state changes.

### Package structure

- `cmd/lib/` — c-shared entry point (the module's only main package): exports `InitWorker`, `Create/DeleteAudioSession`, `Create/DeleteVideoSession`, `FreeString` over a JSON result envelope (`result.go`)
- `internal/webrtc/` — Session types (audio, video) — the module's public surface
- `internal/proc/` — GStreamer pipeline construction for video capture and audio processing
- `internal/config/` — Environment-based configuration (AppConfig, GStreamerConfig)

### Testing approach

Tests use interface-based mocking (PeerConnectionInterface, PipelineFactory) with `stretchr/testify` for assertions. Mocks live in `mock_test.go` files next to the code they test. No external test frameworks or test databases needed; tests run without GStreamer/robot hardware.

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

- **GStreamer 1.0** for media capture/processing (uses NVIDIA `nvv4l2h264enc` on supported hardware). The library is dynamically linked via cgo (go-gst), so the runtime environment — including a future Python host process — must have the GStreamer runtime installed.

## Migration Status (new software architecture)

The repo is mid-migration from a standalone HTTP service to an embeddable session core. Already removed:

- Robot-control DataChannel session (`/control/whip`), including the actions/commands rosbridge translation layer
- gRPC velocity streaming (`internal/gateway`, `internal/pb`, `protos/`) and the whole `internal/transport` layer (gRPC control client + rosbridge WebSocket)
- ROS video source mode, the RTP-pusher Unix-socket signaling, and the `VideoSourceType` enum
- Gin HTTP layer (`internal/api/`) and the GitHub Actions workflows
- Standalone binary (`cmd/main.go`), its Docker build targets (runtime stage, `build-service`), and `scripts/build-entrypoint.sh`

The c-shared entry point is done: `cmd/lib` exports `InitWorker` / `Create*Session` / `Delete*Session` / `FreeString` (build with `go build -buildmode=c-shared -o dist/libsyncai_worker.so ./cmd/lib`, or `docker-compose up build-lib`). Contract for the Python host:

- Every export returns a malloc'd JSON string (`{"ok":true,...}` or `{"ok":false,"error":"..."}`); the host must call `FreeString` on each one.
- Call `InitWorker` once before any session function (repeat calls are no-op successes); calls before init return an error envelope.
- `Create*Session` blocks until ICE gathering completes — call from a worker thread, not an async event loop.
- `DeleteAudioSession` returns not-found for unknown IDs; `DeleteVideoSession` is idempotent (video sessions self-close on connection failure, so deleting an already-gone session succeeds).
- The `.so` cannot be unloaded (`dlclose` unsupported for Go c-shared) — restarting the worker means restarting the host process.

Remaining legacy: `scripts/deploy.sh` and `syncai-webrtc-worker.service` (systemd install of the removed standalone binary) are currently non-functional and kept only as a reference until the FastAPI host owns deployment.
