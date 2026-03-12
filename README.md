# SyncAI WebRTC Worker

A Go-based WebRTC worker service for robot control and real-time audio/video streaming. Bridges WebRTC connections to robot systems via WHIP/WHEP protocols.

## Features

- **WebRTC WHIP/WHEP Support**
  - WHIP (WebRTC-HTTP Ingestion Protocol) for control commands and audio input
  - WHEP (WebRTC-HTTP Egress Protocol) for video output
- **Robot Control**
  - Velocity commands via gRPC streaming
  - Real-time control messages via DataChannel
  - Task/command execution system (Move, Head, TTS, Wait)
- **Multiple Video Sources**
  - Direct camera access (`/dev/video0`)
  - ROS node integration (UDP/RTSP)
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
| Containerization | Docker |

## Requirements

- Go 1.25+
- GStreamer 1.0 development libraries
- Docker (optional, for building)

## Installation

### Build with Docker

```bash
docker-compose up build-service
```

Build output will be located at `./dist/syncai-webrtc-worker`

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

Video source is auto-detected:
- Uses camera mode if `/dev/video0` exists
- Otherwise uses ROS mode

## Running

```bash
./syncai-webrtc-worker
```

The service starts on port `:18000`.

## API Endpoints

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

All endpoints require `Content-Type: application/sdp` with SDP Offer in the request body.

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
go test ./...
```

## License

© SyncRobotic
