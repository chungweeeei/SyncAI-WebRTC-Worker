# SyncAI WebRTC Worker

A Go-based WebRTC worker service for real-time audio/video streaming. Bridges WebRTC connections to robot systems via WHIP/WHEP protocols.

## Features

- **WebRTC WHIP/WHEP Support**
  - WHIP (WebRTC-HTTP Ingestion Protocol) for audio input
  - WHEP (WebRTC-HTTP Egress Protocol) for video output
- **Support Multiple Video Sources**
  - Direct camera access (`/dev/syncai_camera`)
  - ROS node integration (RTP over UDP)
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
| WebSocket | Gorilla WebSocket |
| Containerization | Docker (multi-stage build) |

## Requirements

- Go 1.25+
- GStreamer 1.0 development libraries
- Docker (optional, for building)

## Installation

### Build with Docker

```bash
# Build binary to ./dist/ (uses the builder stage, mirrors CI)
docker-compose up build-service

# Build & run the runtime image locally (for smoke testing)
docker-compose up syncai-webrtc-worker
```

> The runtime image is intended for local smoke testing only. Production
> deployments use the binary produced by `build-service` (or by CI) and run it
> under systemd — see [Deployment](#deployment-systemd).

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

## Deployment (systemd)

The service is deployed as a systemd unit on the target device. The recommended
flow is to consume the tarball produced by CI (`build.yml`, see [CI/CD](#cicd))
and hand it to `scripts/deploy.sh`, which fully automates the install.

### Recommended: deploy from CI tarball

1. Download the `syncai-webrtc-worker-<timestamp>.tar.gz` artifact from the
   `Build` workflow run on GitHub Actions, and copy it to the target device.
2. On the target device, run the deploy script:

   ```bash
   ./scripts/deploy.sh syncai-webrtc-worker-<timestamp>.tar.gz
   ```

   The script will:
   - Stop the service if it is already running
   - Extract the tarball to `/opt/syncai-webrtc-worker/`
   - Install `syncai-webrtc-worker.service` into `/etc/systemd/system/`
   - `daemon-reload`, `enable`, and `start` the service
   - Verify the service is active and print status / log hints on failure

3. (Optional) Drop a `.env` file at `/opt/syncai-webrtc-worker/.env` to override
   defaults — the unit file picks it up via `EnvironmentFile=`.

### Manual install (alternative)

If you have a locally built binary instead of the CI tarball:

```bash
# 1. Build the binary
docker-compose up build-service
# Binary output: ./dist/syncai-webrtc-worker

# 2. Copy artifacts to /opt/syncai-webrtc-worker/
sudo mkdir -p /opt/syncai-webrtc-worker
sudo cp dist/syncai-webrtc-worker /opt/syncai-webrtc-worker/
sudo cp syncai-webrtc-worker.service /opt/syncai-webrtc-worker/
sudo cp .env /opt/syncai-webrtc-worker/   # optional

# 3. Install and start the systemd service
sudo cp syncai-webrtc-worker.service /etc/systemd/system/
sudo systemctl daemon-reload
sudo systemctl enable syncai-webrtc-worker
sudo systemctl start syncai-webrtc-worker
```

### Managing the service

```bash
# Check status
sudo systemctl status syncai-webrtc-worker

# View logs (follow mode)
journalctl -u syncai-webrtc-worker -f

# Restart after updating binary
sudo systemctl restart syncai-webrtc-worker
```

## API Endpoints

All endpoints require `Content-Type: application/sdp` with SDP Offer in the request body.

CORS is enabled for `https://*`, `http://*`, and `http://localhost:5173`.

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

## Project Structure

```
├── cmd/
│   └── main.go              # Application entry point
├── internal/
│   ├── api/                 # HTTP API layer
│   │   ├── router.go        # Route configuration
│   │   └── handlers/        # Request handlers
│   ├── config/              # Configuration management
│   ├── proc/                # GStreamer processors
│   ├── transport/           # Transport layer (WebSocket, currently unwired)
│   └── webrtc/              # WebRTC session management
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

- **test.yml** — On PRs to `main`/`stage`/`dev`: runs `go vet` and
  `go test -v -race ./...` inside the `syncai-webrtc-worker-base` container
  image.
- **build.yml** — On push to `main`/`stage`/`dev`: builds the binary, runs a
  smoke test against port `18000`, then packages the binary together with
  `syncai-webrtc-worker.service` into `syncai-webrtc-worker-<timestamp>.tar.gz`
  and uploads it as a workflow artifact (retained 7 days). This tarball is what
  `scripts/deploy.sh` consumes on the target device.
- **publish-image.yml** — On push to `main`/`stage`/`dev` that touches the
  `Dockerfile`: builds the `builder` stage and pushes it to GHCR as
  `ghcr.io/<owner>/syncai-webrtc-worker-base:latest`. This is the **build base
  image** used by `test.yml` and `build.yml` as their container — not a runtime
  image for end users.

## License

© SyncRobotic
