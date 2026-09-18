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
| `AUDIO_VOLUME` | Playback gain for the WHIP branch; 1.0 is unity, above it amplifies and clips | `1.0` |
| `AUDIO_SINK` | Complete GStreamer sink element the WHIP branch plays into | `alsasink device=plughw:CARD=CD002AUDIO` |
| `STUN_SERVERS` | Comma-separated STUN server URLs | `stun:stun.l.google.com:19302` |
| `TURN_SERVERS` | Comma-separated TURN server URLs | |
| `TURN_USERNAME` | TURN authentication username | |
| `TURN_PASSWORD` | TURN authentication password | |
| `VIDEO_DEVICE` | V4L2 capture node | `/dev/syncai/camera0` |
| `VIDEO_FORMAT` | Capture branch: `mjpeg` (nvjpegdec) or `uyvy` (videoconvert) | `mjpeg` |
| `VIDEO_WIDTH` | Capture width | `1280` |
| `VIDEO_HEIGHT` | Capture height | `720` |
| `VIDEO_FRAMERATE` | Capture framerate (fps) | `60` |
| `VIDEO_BITRATE` | H.264 bitrate (bps) | `4000000` |
| `VIDEO_AUTO_EXPOSURE` | `0` = Auto Mode, `1` = Manual Mode (UVC menu numbering) | `0` |
| `VIDEO_EXPOSURE` | Exposure time in 100 us units; applied only when `VIDEO_AUTO_EXPOSURE=1` | `330` |
| `VIDEO_AUTO_WB` | `1` = automatic white balance, `0` = manual | `1` |
| `VIDEO_WB_TEMP` | White balance temperature in K; applied only when `VIDEO_AUTO_WB=0` | `5000` |
| `VIDEO_GAIN` | Sensor gain | `1` |
| `VIDEO_BRIGHTNESS` | Sensor brightness | `16` |
| `VIDEO_POWER_LINE_FREQ` | Flicker correction: `0` disabled, `1` = 50 Hz, `2` = 60 Hz | `1` |
| `VIDEO_AUDIO_SRC` | Full GStreamer source element for the microphone branch; empty drops the branch | |

The video source is the camera at `VIDEO_DEVICE`. The default is a udev symlink
rather than `/dev/videoN` — where two identical cameras are fitted, minor
numbers swap on replug.

`VIDEO_WIDTH` / `VIDEO_HEIGHT` / `VIDEO_FRAMERATE` must be a combination the
sensor actually advertises (`v4l2-ctl -d $VIDEO_DEVICE --list-formats-ext`); the
AR0234 offers 640x480 at 120/60 fps but **not** at 30. A pair it does not offer
fails as a `not-negotiated` error on the GStreamer bus, which the pipeline's bus
watcher logs — `SetState(PLAYING)` itself still returns success.

The sensor controls are written through `v4l2src extra-controls` at capture
start, and they are not decorative. UVC control state lives in the camera
firmware and survives open/close, so a pipeline that sets nothing inherits
whatever the last process left behind — on this fleet that was manual exposure
at 4 ms and manual white balance frozen at 4000 K, an image so dark and so blue
it reads as a broken encoder. Note the two auto switches disagree on polarity:
`VIDEO_AUTO_EXPOSURE=0` and `VIDEO_AUTO_WB=1` both mean "automatic". Each
manual value is gated by its auto switch — the driver rejects the write while
the automatic mode is on, so the pipeline emits it only when that mode is off.

`VIDEO_AUDIO_SRC` is empty by default because the robot has no capture device
and the deployment container ships no `alsasrc` element; an audio branch that
cannot be parsed fails the whole pipeline string and takes the video with it.
The WHEP session still offers an Opus track when it is empty — the track simply
carries no packets.

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
