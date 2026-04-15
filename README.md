# SyncAI WebRTC Worker

A Go-based WebRTC worker service for robot control and real-time audio/video streaming. Bridges WebRTC connections to robot systems via WHIP/WHEP protocols.

## Features

- **WebRTC WHIP/WHEP Support**
  - WHIP (WebRTC-HTTP Ingestion Protocol) for control commands and audio input
  - WHEP (WebRTC-HTTP Egress Protocol) for video output
- **Robot Control**
  - Velocity commands via gRPC streaming (`127.0.0.1:50052`)
  - Real-time control messages via DataChannel
  - Task/command execution system (Move, Head, TTS, Wait, StandUp, SitDown)
- **Support Multiple Video Sources**
  - Direct camera access (`/dev/syncai_camera`)
  - ROS node integration (UDP/RTSP via Unix socket `/tmp/rtsp_pusher.sock`)
- **Audio Processing**
  - WHIP audio input
  - GStreamer audio processing with volume control

## Tech Stack

| Category | Technology |
|----------|------------|
| Language | Go 1.25 |
| WebRTC | Pion WebRTC v4 |
| Media Processing | GStreamer (go-gst) |
| Web Framework | Gin |
| RPC | gRPC + Protocol Buffers |
| WebSocket | Gorilla WebSocket |
| Containerization | Docker (multi-stage build) |

## Requirements

- Go 1.25+
- GStreamer 1.0 development libraries
- Docker (optional, for building)

## Installation

### Build with Docker

```bash
# Build runtime image (multi-stage)
docker-compose up syncai-webrtc-worker

# Build binary to ./dist/ (local development)
docker-compose up build-service
```

### Local Build

```bash
# Install GStreamer dependencies (Debian/Ubuntu)
sudo apt-get install -y \
    pkg-config \
    libgstreamer1.0-dev \
    libgstreamer-plugins-base1.0-dev \
    gstreamer1.0-plugins-good \
    gstreamer1.0-plugins-bad \
    gstreamer1.0-plugins-ugly

# Build
go build -o syncai-webrtc-worker cmd/main.go
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

Video source is auto-detected:
- Uses camera mode if `/dev/syncai_camera` exists
- Otherwise uses ROS mode

## Running

```bash
./syncai-webrtc-worker
```

The service starts on port `:18000`.

## API Endpoints

All endpoints require `Content-Type: application/sdp` with SDP Offer in the request body.

CORS is enabled for `https://*`, `http://*`, and `http://localhost:5173`.

### Control (WHIP)

| Method | Path | Description |
|--------|------|-------------|
| `POST` | `/control/whip` | Create WebRTC control session |
| `DELETE` | `/control/whip/:id` | Close control session |

### Audio (WHIP)

| Method | Path | Description |
|--------|------|-------------|
| `POST` | `/audio/whip` | Create audio input session |
| `DELETE` | `/audio/whip/:id` | Close audio session |

### Video (WHEP)

| Method | Path | Description |
|--------|------|-------------|
| `POST` | `/video/whep` | Create video output session |
| `DELETE` | `/video/whep/:id` | Close video session |

## DataChannel Message Format

### Control Command (CONTROL)

```json
{
  "action": "CONTROL",
  "id": "uuid",
  "timestamp": 1234567890,
  "payload": {
    "linear": { "x": 0.5, "y": 0, "z": 0 },
    "angular": { "x": 0, "y": 0, "z": 0.1 }
  }
}
```

### Task Command (TASK)

```json
{
  "action": "TASK",
  "id": "uuid",
  "timestamp": 1234567890,
  "payload": {
    "steps": [
      { "id": "1", "name": "move", "type": "MOVE", "params": {...} },
      { "id": "2", "name": "speak", "type": "TTS", "params": {...} }
    ]
  }
}
```

Supported task types: `MOVE`, `HEAD`, `TTS`, `WAIT`, `STANDUP`, `SITDOWN`

## Project Structure

```
├── cmd/
│   └── main.go              # Application entry point
├── internal/
│   ├── api/                 # HTTP API layer
│   │   ├── router.go        # Route configuration
│   │   └── handlers/        # Request handlers
│   ├── config/              # Configuration management
│   ├── gateway/             # Robot gateway (gRPC/Socket)
│   ├── pb/                  # Protocol Buffers generated files
│   ├── proc/                # GStreamer processors
│   ├── transport/           # Transport layer (gRPC/WebSocket)
│   └── webrtc/              # WebRTC session management
│       └── actions/         # Action command handlers
├── protos/                  # Protocol Buffers definitions
├── scripts/                 # Build scripts
├── Dockerfile
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

## CI/CD

GitHub Actions workflows:

- **test.yml** — Runs `go vet` and `go test -v -race ./...` on PRs to main/stage/dev
- **build.yml** — Builds binary and runs smoke test on push to main/stage/dev
- **publish-image.yml** — Publishes Docker image to GHCR on Dockerfile changes

## License

© SyncRobotic
