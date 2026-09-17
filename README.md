# SyncAI WebRTC Worker

A Go-based WebRTC session core for real-time robot audio/video streaming: WHIP-style audio ingestion and WHEP-style video egress, with GStreamer handling media capture and processing.

> **Architecture note** — this module is transitioning from a standalone HTTP
> service to an embeddable library. The Gin HTTP layer has been removed; the
> WHIP/WHEP endpoints will be served by a host application (Python/FastAPI)
> that calls into the Go session core through a c-shared library
> (`go build -buildmode=c-shared`).

## Features

- **WebRTC Sessions (Pion)**
  - Audio input session (WHIP-style): receives client audio tracks, processes them through GStreamer, and outputs RTP
  - Video output session (WHEP-style): sends H264 video + Opus audio tracks captured from the physical camera
- **Video Source**
  - Direct camera access (`/dev/syncai_camera`) via GStreamer (NVIDIA `nvv4l2h264enc` on supported hardware)
- **Audio Processing**
  - GStreamer audio pipeline with volume control

## Tech Stack

| Category | Technology |
|----------|------------|
| Language | Go 1.25 |
| WebRTC | Pion WebRTC v4 |
| Media Processing | GStreamer (go-gst) |
| Containerization | Docker (multi-stage build) |

## Requirements

- Go 1.25+
- GStreamer 1.0 development libraries (build) and runtime plugins (run)
- Docker (optional, for building)

## Installation

### Build the shared library (for the Python/FastAPI host)

```bash
# Local build — outputs dist/libsyncai_worker.so and dist/libsyncai_worker.h
# (requires the GStreamer dev libraries below)
go build -buildmode=c-shared -o dist/libsyncai_worker.so ./cmd/lib

# Or via Docker (builds for Linux inside the builder stage)
docker-compose up build-lib
```

### GStreamer dependencies (Debian/Ubuntu)

```bash
sudo apt-get install -y \
    pkg-config \
    libgstreamer1.0-dev \
    libgstreamer-plugins-base1.0-dev \
    gstreamer1.0-plugins-good \
    gstreamer1.0-plugins-bad \
    gstreamer1.0-plugins-ugly
```

## Configuration

Configure via environment variables or `.env` file:

| Variable | Description | Default |
|----------|-------------|---------|
| `AUDIO_RTP_PORT` | Audio RTP port | `5004` |
| `VIDEO_RTP_PORT` | Video RTP port | `5006` |
| `VIDEO_AUDIO_RTP_PORT` | Video audio track RTP port | `5007` |
| `AUDIO_VOLUME` | Audio volume (0.0-1.0) | `0.2` |
| `STUN_SERVERS` | Comma-separated STUN server URLs | `stun:stun.l.google.com:19302` |
| `TURN_SERVERS` | Comma-separated TURN server URLs | |
| `TURN_USERNAME` | TURN authentication username | |
| `TURN_PASSWORD` | TURN authentication password | |

The video source is fixed to camera mode (`/dev/syncai_camera`).

## Session API

The host application creates and tears down WebRTC sessions through the
functions exported by `libsyncai_worker.so` (implemented in `cmd/lib`, backed
by the `internal/webrtc` package). Every export returns a JSON string —
`{"ok":true,...}` on success or `{"ok":false,"error":"..."}` on failure — that
the host must release with `FreeString`:

| Export | Description |
|--------|-------------|
| `InitWorker()` | Initialize GStreamer + load config from env/`.env`; call once before anything else |
| `CreateAudioSession(offerSDP)` | Create audio input session → `{"ok":true,"session_id":"...","answer_sdp":"..."}` |
| `DeleteAudioSession(id)` | Close audio session (unknown ID → not-found error) |
| `CreateVideoSession(offerSDP)` | Create video output session → same envelope as audio |
| `DeleteVideoSession(id)` | Close video session (idempotent — deleting an already-gone session succeeds) |
| `FreeString(ptr)` | Release a string returned by any export |

Notes for hosts:

- `Create*Session` blocks until ICE gathering completes (up to a few seconds) —
  call it from a worker thread, not an async event loop.
- Video sessions clean themselves up on WebRTC connection failure, which is why
  `DeleteVideoSession` treats a missing session as success.
- A Go c-shared library cannot be unloaded — restarting the worker requires
  restarting the host process.

## Project Structure

```
├── cmd/
│   └── lib/                 # c-shared entry point (libsyncai_worker.so)
├── internal/
│   ├── config/              # Configuration management
│   ├── proc/                # GStreamer pipeline construction
│   └── webrtc/              # WebRTC session management (public surface)
├── scripts/                 # Build / legacy deploy scripts
├── Dockerfile               # Build environment (builder stage only)
└── docker-compose.yml
```

## Testing

```bash
# Run all tests
go test ./...

# Run with race detection
go test -v -race ./...

# Lint
go vet ./...
```

Tests use interface-based mocks and run without GStreamer pipelines or robot
hardware.

## Legacy Deployment (systemd)

`scripts/deploy.sh` and `syncai-webrtc-worker.service` date from the
standalone HTTP-service era: they install a release tarball (binary + systemd
unit) to `/opt/syncai-webrtc-worker/`. The standalone binary and its build
targets have been removed, so this flow is currently non-functional; it will
be replaced once the c-shared/FastAPI integration owns deployment.

## License

© SyncRobotic
