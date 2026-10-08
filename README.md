# SyncAI WebRTC Worker

A Go-based WebRTC session core for real-time robot audio/video streaming: WHIP-style audio ingestion, WHEP-style video egress, and a duplex session carrying both on one peer connection, with GStreamer handling media capture and processing.

> **Architecture note** — this module is transitioning from a standalone HTTP
> service to an embeddable library. The Gin HTTP layer has been removed; the
> WHIP/WHEP endpoints will be served by a host application (Python/FastAPI)
> that calls into the Go session core through a c-shared library
> (`go build -buildmode=c-shared`).

## Features

- **WebRTC Sessions (Pion)**
  - Audio input session (WHIP-style): receives client audio tracks, processes them through GStreamer, and outputs RTP
  - Video output session (WHEP-style): sends H264 video + Opus audio tracks captured from the physical camera
  - Duplex session: camera out and browser microphone in over a single peer connection (one ICE/DTLS handshake)
- **Video Source**
  - Direct camera access (`VIDEO_DEVICE`, default `/dev/syncai/camera0`) via GStreamer (NVIDIA `nvv4l2h264enc` on supported hardware)
  - Or RTP/JPEG from the camera feeder (`VIDEO_FORMAT=rtpjpeg`), so ROS can stream the same camera at the same time
- **Audio Processing**
  - GStreamer audio pipeline with volume control

## Tech Stack

| Category | Technology |
|----------|------------|
| Language | Go 1.25 |
| WebRTC | Pion WebRTC v4 |
| Media Processing | GStreamer 1.20 (go-gst v1.0.0, pinned) |
| Containerization | Docker (Ubuntu 22.04 builder stage) |

## Requirements

- Go 1.25+
- GStreamer 1.0 development libraries (build) and runtime plugins (run)
- Docker (optional, for building)

> **Do not upgrade `go-gst`.** The deployment target (Ubuntu 22.04 robot
> container) ships GStreamer 1.20.3 with the NVIDIA Tegra elements built
> against it. Every go-gst release after v1.0.0 calls a 1.22-only symbol, so a
> library built against a newer GStreamer compiles fine and then fails to load
> in the host with `undefined symbol: gst_debug_message_get_id`. v1.0.0 is
> retracted upstream, so `go get -u` will try to move off it.

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
| `VIDEO_FORMAT` | Source branch: `mjpeg` (v4l2src → nvjpegdec), `uyvy` (v4l2src → videoconvert) or `rtpjpeg` (RTP/JPEG from the camera feeder → nvjpegdec) | `mjpeg` |
| `VIDEO_RTP_IN_PORT` | Port the `rtpjpeg` branch listens on (127.0.0.1); must match the feeder's `rtp_port` | `5008` |
| `VIDEO_WIDTH` | Capture width | `1280` |
| `VIDEO_HEIGHT` | Capture height | `720` |
| `VIDEO_FRAMERATE` | Capture framerate (fps) | `60` |
| `VIDEO_BITRATE` | H.264 bitrate (bps) | `4000000` |
| `VIDEO_AUTO_EXPOSURE` | `0` = Auto Mode, `1` = Manual Mode (UVC menu numbering) | `0` |
| `VIDEO_EXPOSURE` | Exposure time in 100 us units; applied only when `VIDEO_AUTO_EXPOSURE=1` | `330` |
| `VIDEO_AUTO_WB` | `1` = automatic white balance, `0` = manual | `0` |
| `VIDEO_WB_TEMP` | White balance temperature in K (2300–15000); applied only when `VIDEO_AUTO_WB=0` | `5200` |
| `VIDEO_GAIN` | Sensor gain | `1` |
| `VIDEO_BRIGHTNESS` | Sensor brightness | `16` |
| `VIDEO_POWER_LINE_FREQ` | Flicker correction: `0` disabled, `1` = 50 Hz, `2` = 60 Hz | `1` |
| `VIDEO_AUDIO_SRC` | Full GStreamer source element for the microphone branch; empty drops the branch | |

The video source is the camera at `VIDEO_DEVICE`. The default is a udev symlink
rather than `/dev/videoN` — each camera exposes two nodes, and probe order
decides which minor numbers they get.

### Sharing the camera with ROS (`VIDEO_FORMAT=rtpjpeg`)

A V4L2 capture device takes one streaming opener at a time, so a session that
opens the camera itself and a ROS node publishing the same camera cannot both
run — whichever starts second fails with `EBUSY`. With `VIDEO_FORMAT=rtpjpeg`
the worker opens no device at all: the camera feeder
([SyncAI-Robot-Camera-Feeder](https://github.com/chungweeeei/SyncAI-Robot-Camera-Feeder))
is the camera's only opener and sends its MJPEG, undecoded, as RTP/JPEG to
`127.0.0.1:$VIDEO_RTP_IN_PORT`. The branch is `udpsrc ! rtpjpegdepay !
nvjpegdec`, and everything from `nvjpegdec` on is the same as `mjpeg`.

In this mode:

- `VIDEO_DEVICE` and the sensor controls are ignored; the feeder writes the
  same controls with the same defaults.
- `VIDEO_FRAMERATE` should match what the feeder sends (60): it sets the
  encoder's IDR interval.
- A feeder that is not running is not an error — `udpsrc` simply receives
  nothing. The bus watcher logs `no RTP from the camera feeder` every 3 s while
  that lasts, so a black view has a log line explaining it.

`VIDEO_WIDTH` / `VIDEO_HEIGHT` / `VIDEO_FRAMERATE` must be a combination the
sensor actually advertises (`v4l2-ctl -d $VIDEO_DEVICE --list-formats-ext`); the
AR0234 offers 640x480 at 120/60 fps but **not** at 30. A pair it does not offer
fails as a `not-negotiated` error on the GStreamer bus, which the pipeline's bus
watcher logs — `SetState(PLAYING)` itself still returns success.

The sensor controls are written through `v4l2src extra-controls` at capture
start (by the two `v4l2src` branches), and they are not decorative. UVC control state lives in the camera
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
| `CreateDuplexSession(offerSDP)` | Create bidirectional session (camera out, mic in) → same envelope; offer must carry `m=video recvonly` + `m=audio sendrecv` |
| `DeleteDuplexSession(id)` | Close duplex session (idempotent) |
| `FreeString(ptr)` | Release a string returned by any export |

Notes for hosts:

- `Create*Session` blocks until ICE gathering completes (up to a few seconds) —
  call it from a worker thread, not an async event loop.
- Video and duplex sessions clean themselves up on WebRTC connection failure,
  which is why their delete calls treat a missing session as success.
- The camera and the speaker are one device each, so a duplex session contends
  with video/audio sessions for them. The library does not arbitrate this — the
  host must ensure only one owner at a time.
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

`scripts/deploy.sh` dates from the standalone HTTP-service era (its
`syncai-webrtc-worker.service` unit has since been removed): it installs a
release tarball (binary + systemd unit) to `/opt/syncai-webrtc-worker/`. The standalone binary and its build
targets have been removed, so this flow is currently non-functional; it will
be replaced once the c-shared/FastAPI integration owns deployment.

## License

© SyncRobotic
