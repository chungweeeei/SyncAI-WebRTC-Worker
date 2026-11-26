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

The public surface is the `internal/webrtc` package — three WebRTC session types, each managed by a create/delete function pair that takes an SDP offer string and returns a session ID plus the answer SDP:

- **Audio** — `CreateAudioSession` / `DeleteAudioSession`: WHIP-style session receiving audio tracks from the client, relayed as RTP into a GStreamer pipeline that decodes Opus and plays it out of `AUDIO_SINK`. ALSA by card name, not PulseAudio: the robot container has no PulseAudio daemon at all, so the old `pulsesink` default could only ever have failed there, and the speaker is a USB dongle whose card *number* moves with probe order. The sink is exclusive, so a live WHIP session makes the backend's TTS `/speak` (which shells out to `aplay` on the same device) fail with "Device or resource busy" — sharing would mean moving both sides to dmix.
- **Video** — `CreateVideoSession` / `DeleteVideoSession`: WHEP-style session sending video (H264) + audio (Opus) tracks to the client from the physical camera.
- **Duplex** — `CreateDuplexSession` / `DeleteDuplexSession`: both directions on one peer connection — camera out, browser mic in to the speaker. Expects an offer with `m=video recvonly` + `m=audio sendrecv`. It buys one ICE/DTLS handshake instead of two and shared fate for both directions. It contends with the single-direction sessions for the camera and speaker, and nothing in this module arbitrates that — the host owns the slots.

`CreateVideoSession` / `CreateDuplexSession` block until ICE gathering completes (can take up to a few seconds), so hosts must not call them on a latency-sensitive thread/event loop.

### Key data flow

```
Client → WebRTC audio track → AudioSession → GStreamer pipeline → RTP (port 5004)
GStreamer capture → RTP (ports 5006/5007) → VideoSession UDP listeners → WebRTC tracks → Client
```

### Video source

Creating a video (or duplex) session spawns a GStreamer pipeline whose source is chosen by `VIDEO_FORMAT`:

- `mjpeg` (default) / `uyvy`: the physical camera at `VIDEO_DEVICE`, opened by `v4l2src` for the life of the session.
- `rtpjpeg`: no device. RTP/JPEG on `127.0.0.1:VIDEO_RTP_IN_PORT` from the camera feeder (SyncAI-Robot-Camera-Feeder), which is then the camera's only opener so ROS can stream it at the same time. `udpsrc ! rtpjpegdepay ! nvjpegdec`, then the same encode half as `mjpeg`. No `extra-controls` (the feeder writes them) and no `rtpjitterbuffer` (loopback does not reorder; it would only add its 200 ms default latency).
- `rtsp`: no device. A camera another computer encodes and serves over RTSP — the Lynx M20's, H.265 from its perception board at `VIDEO_RTSP_URL`. `rtspsrc protocols=tcp ! rtph265depay ! h265parse config-interval=-1 ! nvv4l2decoder`, then the same encode half. Re-encoded rather than passed through: the WHEP track is H.264, and with no PLI handling only our own encoder can promise an IDR every half second (the M20's GOP is 1 s). `config-interval=-1` is load-bearing: the M20 sends VPS/SPS/PPS only in the SDP, `nvv4l2decoder` ignores `codec_data`, and without the re-insertion it decodes nothing until EOS. An empty URL or an unknown `VIDEO_RTSP_CODEC` is refused before the pipeline is built (`validateVideoConfig`). `VIDEO_RTSP_URL_2` adds a second camera as a second video track of the same WHEP session (picture in picture in the console), its own pipeline on `VIDEO_RTP_PORT_2` so one camera failing does not stop the other; it is sent only when the offer has a second video m-line (`wantsSecondCamera` / `countVideoMLines` in `videosession.go`), the first video m-line always being the first camera. Duplex sessions stay one camera.

This is a format value, not a revival of the removed ROS source mode (`VideoSourceType` enum + RTP-pusher Unix-socket signaling): there is no signaling and no lifecycle coupling — the feeder sends to a fixed port and the session listens on it.

The capture half is env-configurable because it is what differs per robot, and two of its failure modes are silent by default:

- A resolution/framerate pair the sensor does not advertise (the AR0234 has no 30 fps mode at 640x480) fails as `not-negotiated`, *after* `SetState(PLAYING)` has already returned success. `VideoGStreamerPipeline` therefore polls the pipeline bus in a goroutine and logs ERROR/WARNING/EOS — polling rather than `Bus.AddWatch`, which would need a GLib main loop the Python host has no reason to run. It reports only; the host owns session teardown.
- Teardown has to **block** on the NULL state transition (`BlockSetState`, not `SetState`), because that transition is what closes the V4L2 device. Dropping the pipeline's reference while the transition is still in flight leaks the camera fd for the life of the host process — and the host is the robot's backend, which cannot unload this library, so every later session negotiates nothing (`not-negotiated`) until the backend is restarted. Observed exactly once, from a session torn down while its pipeline was still coming up.
- Under `rtpjpeg`, a feeder that is not running fails nothing — `udpsrc` just receives no packets. `udpsrc timeout` makes it post `GstUDPSrcTimeout` every 3 s instead, and the bus watcher logs that as `no RTP from the camera feeder`; that is the only signal a black view has.
- The sensor controls (`VIDEO_AUTO_WB` and friends) are written through `v4l2src extra-controls` at capture start (the two `v4l2src` branches only). UVC control state persists in the camera firmware across open/close, so setting nothing means inheriting the last process's settings rather than getting sane defaults. The two auto switches have opposite polarity: `auto_exposure=0` is automatic, `white_balance_automatic=1` is automatic.
- The microphone branch is off unless `VIDEO_AUDIO_SRC` is set. A missing element (the deployment container has no `alsasrc`) fails the *whole* pipeline string at parse time, so an unused audio branch takes the video down with it. The Opus track is still offered and simply carries nothing.

### Session lifecycle

Each session type keeps its live sessions in a package-level map in `internal/webrtc/` keyed by UUID. Video and duplex sessions self-close on connection failure. Cleanup happens both via `Delete*Session` calls and on WebRTC connection-state changes.

### Package structure

- `cmd/lib/` — c-shared entry point (the module's only main package): exports `InitWorker`, `Create/DeleteAudioSession`, `Create/DeleteVideoSession`, `Create/DeleteDuplexSession`, `FreeString` over a JSON result envelope (`result.go`)
- `internal/webrtc/` — Session types (audio, video, duplex) — the module's public surface
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
| `AUDIO_VOLUME` | `1.0` | Playback gain for the WHIP branch; unity, and above 1.0 amplifies and clips |
| `AUDIO_SINK` | `alsasink device=plughw:CARD=CD002AUDIO` | Whole GStreamer sink element for WHIP playback |
| `VIDEO_DEVICE` | `/dev/syncai/camera0` | V4L2 capture node (a udev symlink, not `/dev/videoN`) |
| `VIDEO_FORMAT` | `mjpeg` | Source branch: `mjpeg` (nvjpegdec), `uyvy` (videoconvert), `rtpjpeg` (RTP/JPEG from the camera feeder) or `rtsp` (an RTSP stream, nvv4l2decoder) |
| `VIDEO_RTP_IN_PORT` | `5008` | Port the `rtpjpeg` branch listens on (127.0.0.1); must match the feeder's `rtp_port` |
| `VIDEO_RTSP_URL` | (empty) | Stream the `rtsp` branch plays; required by it |
| `VIDEO_RTSP_URL_2` | (empty) | Second camera for `rtsp`, sent as a second video track when the offer has a second video m-line |
| `VIDEO_RTP_PORT_2` | `5010` | Port the second camera's pipeline sends its RTP to |
| `VIDEO_RTSP_CODEC` | `h265` | `h265` or `h264`; picks the depayloader and parser |
| `VIDEO_RTSP_LATENCY_MS` | `50` | `rtspsrc` jitter buffer (its own default, 2000, is sized for the internet) |
| `VIDEO_WIDTH` / `VIDEO_HEIGHT` | `1280` / `720` | Capture size |
| `VIDEO_FRAMERATE` | `60` | Capture framerate; must be one the sensor advertises |
| `VIDEO_BITRATE` | `4000000` | H.264 bitrate |
| `VIDEO_AUTO_EXPOSURE` / `VIDEO_EXPOSURE` | `0` / `330` | `0` = auto (UVC menu numbering); exposure in 100 µs units, applied only when manual |
| `VIDEO_AUTO_WB` / `VIDEO_WB_TEMP` | `0` / `5200` | `1` = auto; temperature in K, applied only when manual (5200 measured in the lab) |
| `VIDEO_GAIN` / `VIDEO_BRIGHTNESS` | `1` / `16` | Sensor gain / brightness |
| `VIDEO_POWER_LINE_FREQ` | `1` | Flicker correction: `0` off, `1` 50 Hz, `2` 60 Hz |
| `VIDEO_AUDIO_SRC` | (empty) | Whole GStreamer source element for the mic branch; empty drops the branch |
| `STUN_SERVERS` | `stun:stun.l.google.com:19302` | Comma-separated STUN URLs |
| `TURN_SERVERS` | (empty) | Comma-separated TURN URLs |
| `TURN_USERNAME` | (empty) | TURN auth username |
| `TURN_PASSWORD` | (empty) | TURN auth password |

## External Dependencies

- **`go-gst` is pinned to v1.0.0, and the pin is load-bearing.** Every later
  release (v1.1.0 through v1.4.1) calls `gst_debug_message_get_id`, which
  GStreamer added in 1.22. The deployment target is the robot container:
  Ubuntu 22.04, **GStreamer 1.20.3**, with the Tegra elements
  (`nvjpegdec`/`nvvidconv`/`nvv4l2h264enc`) injected by the nvidia container
  runtime and built against that same 1.20. Upgrading GStreamer there is not an
  option — it would break the hardware encoder, which is the whole point of the
  pipeline. So the ceiling is 1.20 and go-gst v1.0.0 is the last release under
  it.

  Two things this costs, both worth re-reading before touching the dependency:
  v1.0.0 is **retracted upstream** ("Handwritten bindings, not as stable as
  planned and not licensed open enough"), so `go get -u` will try to move off
  it; and building on any newer distro succeeds and then fails at *load* time
  in the host process with `undefined symbol: gst_debug_message_get_id`, which
  is why the builder image is based on the deployment distro.

  The way out of the pin is not a newer go-gst. It is dropping cgo altogether
  and supervising `gst-launch-1.0` as a child process, the shape the robot
  workspace's `doc/webrtc-worker-proposal.md` described: the RTP already
  crosses a UDP socket, so the media path would not change.
- **GStreamer 1.0** for media capture/processing (uses NVIDIA `nvv4l2h264enc` on supported hardware). The library is dynamically linked via cgo (go-gst), so the runtime environment — including a future Python host process — must have the GStreamer runtime installed.

## Migration Status (new software architecture)

The repo is mid-migration from a standalone HTTP service to an embeddable session core. Already removed:

- Robot-control DataChannel session (`/control/whip`), including the actions/commands rosbridge translation layer
- gRPC velocity streaming (`internal/gateway`, `internal/pb`, `protos/`) and the whole `internal/transport` layer (gRPC control client + rosbridge WebSocket)
- ROS video source mode, the RTP-pusher Unix-socket signaling, and the `VideoSourceType` enum
- Gin HTTP layer (`internal/api/`) and the GitHub Actions workflows
- Standalone binary (`cmd/main.go`), its Docker build targets (runtime stage, `build-service`), `scripts/build-entrypoint.sh` (now `build-lib-entrypoint.sh`), and the `syncai-webrtc-worker.service` systemd unit

The c-shared entry point is done: `cmd/lib` exports `InitWorker` / `Create*Session` / `Delete*Session` / `FreeString` (build with `go build -buildmode=c-shared -o dist/libsyncai_worker.so ./cmd/lib`, or `docker-compose up build-lib`). Contract for the Python host:

- Every export returns a malloc'd JSON string (`{"ok":true,...}` or `{"ok":false,"error":"..."}`); the host must call `FreeString` on each one.
- Call `InitWorker` once before any session function (repeat calls are no-op successes); calls before init return an error envelope.
- `Create*Session` blocks until ICE gathering completes — call from a worker thread, not an async event loop.
- `DeleteAudioSession` returns not-found for unknown IDs; `DeleteVideoSession` / `DeleteDuplexSession` are idempotent (these sessions self-close on connection failure, so deleting an already-gone session succeeds).
- The `.so` cannot be unloaded (`dlclose` unsupported for Go c-shared) — restarting the worker means restarting the host process.

Remaining legacy: `scripts/deploy.sh` (systemd install of the removed standalone binary) is currently non-functional and kept only as a reference until the FastAPI host owns deployment.
