package proc

import (
	"fmt"
	"log"
	"strings"
	"sync"
	"time"

	"SyncAI-WebRTC-Worker/internal/config"

	"github.com/go-gst/go-gst/gst"
)

// How long each bus poll blocks before the watcher re-checks whether the
// pipeline is closing. Short enough that Close() is not perceptibly delayed,
// long enough that an idle pipeline is not a spin loop.
const busPollInterval = 200 * time.Millisecond

type VideoGStreamerPipeline struct {
	pipeline Pipeline
	busDone  chan struct{}
	busWG    sync.WaitGroup
	mu       sync.Mutex
	closed   bool
}

// buildSensorControls renders v4l2src's extra-controls string.
//
// Order is load-bearing: each auto_* switch has to precede the manual value it
// gates, or the driver rejects the manual write because the mode is still
// automatic. A gated value is emitted only when its automatic mode is off --
// writing it in auto mode is at best ignored and at worst a warning for a
// value that was never going to apply.
//
// See config.VideoConfig for the polarity trap (auto_exposure 0 and
// white_balance_automatic 1 both mean "automatic").
func buildSensorControls(cfg config.VideoConfig) string {
	controls := fmt.Sprintf("c,auto_exposure=%d", cfg.AutoExposure)
	if cfg.AutoExposure == 1 {
		controls += fmt.Sprintf(",exposure_time_absolute=%d", cfg.ExposureTime)
	}
	controls += fmt.Sprintf(",white_balance_automatic=%d", cfg.AutoWhiteBalance)
	if cfg.AutoWhiteBalance == 0 {
		controls += fmt.Sprintf(",white_balance_temperature=%d", cfg.WhiteBalanceTemp)
	}
	controls += fmt.Sprintf(",gain=%d,brightness=%d,power_line_frequency=%d",
		cfg.Gain, cfg.Brightness, cfg.PowerLineFreq)
	return controls
}

// rtpJPEGCaps describes what the camera feeder sends: RTP/JPEG (RFC 2435),
// static payload type 26. udpsrc cannot discover caps on its own.
const rtpJPEGCaps = "application/x-rtp,media=video,encoding-name=JPEG,payload=26,clock-rate=90000"

// rtpInTimeout is how long the "rtpjpeg" branch goes without a packet before
// udpsrc posts a GstUDPSrcTimeout element message, which the bus watcher logs.
// Without it a missing camera feeder is a black view and nothing else: udpsrc
// does not fail when nobody sends.
const rtpInTimeout = 3 * time.Second

// BuildVideoPipelineString builds the capture pipeline feeding the WHEP
// session's two RTP listeners.
//
// The source is one of four branches (see config.VideoConfig.Format); all of
// them hand the shared encode half below NVMM-ready frames.
//
// The encoder properties are the ones the robot workspace's host-side
// publisher (scripts/publish_camera_crop.sh) already proved on this hardware:
// baseline profile for browser compatibility, an IDR every half second, and
// h264parse config-interval=-1 so SPS/PPS ride in-band with every keyframe.
// The IDR interval matters more here than it does for RTSP -- the worker never
// answers an RTCP PLI, so a browser that joins mid-stream shows nothing until
// the next keyframe arrives on its own.
//
// The audio branch is appended only when cfg.AudioSource is set; see
// config.VideoConfig for why empty is the default.
func BuildVideoPipelineString(videoPort, audioPort int, cfg config.VideoConfig) string {
	// Half a second of frames, matching the host publisher's IDR_INTERVAL.
	idrInterval := cfg.Framerate / 2
	if idrInterval < 1 {
		idrInterval = 1
	}

	var source string
	switch {
	case strings.EqualFold(cfg.Format, "rtsp"):
		// A camera another computer encodes (the M20's perception board),
		// decoded here and re-encoded below; see config.VideoConfig.Format
		// for why not a passthrough.
		//
		// protocols=tcp: interleaved on the RTSP connection, so no UDP ports
		// to open or lose, and a reordering-free link for the short latency.
		//
		// config-interval=-1 is not optional. The M20's stream sends its
		// VPS/SPS/PPS only in the SDP, never in-band: its IDRs arrive bare.
		// h265parse passes the SDP's copy on as codec_data, and nvv4l2decoder
		// ignores that, so without the parser re-inserting the parameter
		// sets before every IDR the decoder opens, takes every frame and
		// decodes none ("No valid frames decoded"), with nothing on the bus
		// until end of stream. Measured on robot01, 2026-11-26.
		//
		// Unlike rtpjpeg this source is off the host, on the robot's internal
		// LAN; the URL is the operator's configuration, not something a
		// remote peer can supply.
		//
		// The leaky queue sits before the decoder, as in the other branches:
		// a compressed frame is the cheaper one to drop.
		depay, parse := "rtph265depay", "h265parse"
		if strings.EqualFold(cfg.RTSPCodec, "h264") {
			depay, parse = "rtph264depay", "h264parse"
		}
		source = fmt.Sprintf(
			"rtspsrc location=\"%s\" protocols=tcp latency=%d drop-on-latency=true "+
				"! %s ! %s config-interval=-1 "+
				"! queue max-size-buffers=3 max-size-time=0 max-size-bytes=0 leaky=downstream "+
				"! nvv4l2decoder ! video/x-raw(memory:NVMM) ",
			cfg.RTSPURL, cfg.RTSPLatency, depay, parse)
	case strings.EqualFold(cfg.Format, "rtpjpeg"):
		// The camera feeder owns the device and has already written the
		// sensor controls, so there is no v4l2src and no extra-controls here.
		// Bound to 127.0.0.1: the feeder runs on the same host, and nothing
		// off the robot should be able to inject frames into the view.
		//
		// No rtpjitterbuffer: loopback does not reorder, so its only effect
		// would be its latency (200 ms by default). A lost packet costs
		// rtpjpegdepay that one frame.
		//
		// The leaky queue sits before the decoder for the same reason as in
		// the MJPEG branch. VIDEO_FRAMERATE should still match what the
		// feeder sends: it sets the encoder's IDR interval below.
		source = fmt.Sprintf(
			"udpsrc address=127.0.0.1 port=%d timeout=%d caps=\"%s\" "+
				"! rtpjpegdepay "+
				"! queue max-size-buffers=3 max-size-time=0 max-size-bytes=0 leaky=downstream "+
				"! nvjpegdec ! video/x-raw(memory:NVMM) ",
			cfg.RTPInPort, rtpInTimeout.Nanoseconds(), rtpJPEGCaps)
	case strings.EqualFold(cfg.Format, "uyvy"):
		// Raw capture: videoconvert is what lets nvvidconv take the frame into
		// NVMM. The sensor advertises UYVY only at the higher framerates
		// (640x480 is 120/60 fps, no 30), so a mismatched VIDEO_FRAMERATE
		// fails here as not-negotiated.
		source = fmt.Sprintf(
			"v4l2src device=%s io-mode=2 extra-controls=\"%s\" do-timestamp=true "+
				"! video/x-raw,format=UYVY,width=%d,height=%d,framerate=%d/1 "+
				"! queue max-size-buffers=3 max-size-time=0 max-size-bytes=0 leaky=downstream "+
				"! videoconvert ",
			cfg.Device, buildSensorControls(cfg), cfg.Width, cfg.Height, cfg.Framerate)
	default:
		// MJPEG: the queue sits before the decoder on purpose -- dropping a
		// compressed frame under back-pressure is cheaper than dropping a
		// decoded one, and leaky=downstream is what keeps a slow encoder from
		// stalling the capture thread.
		source = fmt.Sprintf(
			"v4l2src device=%s io-mode=2 extra-controls=\"%s\" do-timestamp=true "+
				"! image/jpeg,width=%d,height=%d,framerate=%d/1 "+
				"! queue max-size-buffers=3 max-size-time=0 max-size-bytes=0 leaky=downstream "+
				"! nvjpegdec ! video/x-raw(memory:NVMM) ",
			cfg.Device, buildSensorControls(cfg), cfg.Width, cfg.Height, cfg.Framerate)
	}

	pipeline := source + fmt.Sprintf(
		"! nvvidconv ! video/x-raw(memory:NVMM),format=NV12 "+
			"! nvv4l2h264enc bitrate=%d profile=0 insert-sps-pps=true insert-vui=true "+
			"iframeinterval=%d idrinterval=%d control-rate=1 maxperf-enable=true "+
			"! h264parse config-interval=-1 "+
			"! rtph264pay config-interval=1 pt=96 "+
			"! udpsink host=127.0.0.1 port=%d sync=true",
		cfg.Bitrate, idrInterval, idrInterval, videoPort)

	if cfg.AudioSource != "" {
		pipeline += fmt.Sprintf(
			" %s "+
				"! audioconvert "+
				"! audioresample "+
				"! audio/x-raw,rate=48000,channels=2 "+
				"! queue max-size-buffers=3 max-size-time=0 max-size-bytes=0 "+
				"! opusenc bitrate=64000 "+
				"! rtpopuspay pt=111 "+
				"! udpsink host=127.0.0.1 port=%d sync=true",
			cfg.AudioSource, audioPort)
	}

	return pipeline
}

// validateVideoConfig refuses the configurations that are certain to fail.
//
// Refused here, as a Create*Session error the host can show, because the
// pipeline would otherwise report them only on the bus after PLAYING has
// returned success: an rtspsrc with no location posts its error to a watcher
// that only logs, and the browser gets a session with no frames in it.
func validateVideoConfig(cfg config.VideoConfig) error {
	if !strings.EqualFold(cfg.Format, "rtsp") {
		return nil
	}
	if cfg.RTSPURL == "" {
		return fmt.Errorf("VIDEO_FORMAT=rtsp needs VIDEO_RTSP_URL")
	}
	if !strings.EqualFold(cfg.RTSPCodec, "h265") && !strings.EqualFold(cfg.RTSPCodec, "h264") {
		return fmt.Errorf("VIDEO_RTSP_CODEC must be h265 or h264, not %q", cfg.RTSPCodec)
	}
	return nil
}

// NewVideoGStreamerPipeline uses the default PipelineFactory.
func NewVideoGStreamerPipeline(videoPort, audioPort int, cfg config.VideoConfig) (*VideoGStreamerPipeline, error) {
	return NewVideoGStreamerPipelineWithFactory(videoPort, audioPort, cfg, defaultFactory)
}

// NewVideoGStreamerPipelineWithFactory allows injecting a PipelineFactory (for tests).
func NewVideoGStreamerPipelineWithFactory(videoPort, audioPort int, cfg config.VideoConfig, factory PipelineFactory) (*VideoGStreamerPipeline, error) {
	if err := validateVideoConfig(cfg); err != nil {
		return nil, err
	}

	pipelineStr := BuildVideoPipelineString(videoPort, audioPort, cfg)
	log.Printf("video pipeline: %s", pipelineStr)

	pipeline, err := factory.NewPipelineFromString(pipelineStr)
	if err != nil {
		return nil, err
	}

	if err := pipeline.SetState(gst.StatePlaying); err != nil {
		// Back to NULL before the wrapper goes out of scope. go-gst's
		// finalizer unrefs the pipeline in whatever state it was left in, and
		// disposing elements that are still PLAYING both keeps the camera
		// open and logs a GStreamer-CRITICAL that names no session.
		if nullErr := pipeline.BlockSetState(gst.StateNull); nullErr != nil {
			log.Printf("video pipeline did not reach NULL after a failed start: %v", nullErr)
		}
		return nil, err
	}

	g := &VideoGStreamerPipeline{
		pipeline: pipeline,
		busDone:  make(chan struct{}),
	}
	g.startBusWatch()

	return g, nil
}

// startBusWatch reports asynchronous pipeline failures.
//
// Without it the worker is blind to everything that goes wrong after
// SetState(PLAYING) returns, and that is most of what goes wrong: a state
// change to PLAYING is asynchronous, so a camera already held by another
// process, a resolution/framerate pair the sensor does not advertise, or a
// missing Tegra encoder all return success here and surface only as an ERROR
// message on the bus. The host then hands its caller a session id, the browser
// completes ICE, and no frame ever arrives with nothing anywhere saying why.
//
// Polling rather than Bus.AddWatch: a watch needs a running GLib main loop,
// and this library is embedded in a Python host that has no reason to run one.
//
// Reporting only. Tearing the session down from here would race the host's own
// bookkeeping, which has no callback channel to learn about it.
func (g *VideoGStreamerPipeline) startBusWatch() {
	real, ok := g.pipeline.(*gst.Pipeline)
	if !ok {
		// A mock pipeline in tests. Nothing to watch.
		return
	}

	bus := real.GetPipelineBus()
	if bus == nil {
		return
	}

	g.busWG.Add(1)
	go func() {
		defer g.busWG.Done()
		for {
			select {
			case <-g.busDone:
				return
			default:
			}

			msg := bus.TimedPopFiltered(
				gst.ClockTime(busPollInterval),
				gst.MessageError|gst.MessageWarning|gst.MessageEOS|gst.MessageElement,
			)
			if msg == nil {
				continue
			}

			switch msg.Type() {
			case gst.MessageError:
				err := msg.ParseError()
				log.Printf("video pipeline ERROR from %s: %s (%s)",
					msg.Source(), err.Error(), err.DebugString())
			case gst.MessageWarning:
				err := msg.ParseWarning()
				log.Printf("video pipeline warning from %s: %s (%s)",
					msg.Source(), err.Error(), err.DebugString())
			case gst.MessageEOS:
				log.Printf("video pipeline reached end-of-stream (capture has stopped)")
			case gst.MessageElement:
				// Only the "rtpjpeg" branch's udpsrc posts this; it repeats
				// every rtpInTimeout for as long as nothing arrives.
				if st := msg.GetStructure(); st != nil && st.Name() == "GstUDPSrcTimeout" {
					log.Printf("video pipeline: no RTP from the camera feeder for %s "+
						"(is syncai_camera_feeder running and sending to this port?)", rtpInTimeout)
				}
			}
			// No Unref: TimedPopFiltered wraps the message with
			// FromGstMessageUnsafeFull, which takes no ref and installs a
			// finalizer. Unreffing here was the same double free as the
			// pipeline's -- see the Pipeline interface.
		}
	}()
}

func (g *VideoGStreamerPipeline) Close() error {
	g.mu.Lock()
	defer g.mu.Unlock()

	if g.closed {
		return nil
	}
	g.closed = true

	// Stop the watcher and wait for it before the pipeline goes: the
	// goroutine holds the bus, and reading it after the pipeline is freed is
	// a use-after-free.
	if g.busDone != nil {
		close(g.busDone)
		g.busWG.Wait()
		g.busDone = nil
	}

	if g.pipeline != nil {
		// Blocking, and that is the point: this is what closes the camera.
		// See the Pipeline interface. An error here is logged rather than
		// returned -- the caller is a session teardown that has nothing
		// better to do with it, and the reference is dropped either way.
		if err := g.pipeline.BlockSetState(gst.StateNull); err != nil {
			log.Printf("video pipeline did not reach NULL cleanly: %v", err)
		}
		// Dropping the reference is the release: go-gst's finalizer owns the
		// unref. See the Pipeline interface for what calling it by hand cost.
		g.pipeline = nil
	}
	return nil
}

func (g *VideoGStreamerPipeline) IsClosed() bool {
	g.mu.Lock()
	defer g.mu.Unlock()
	return g.closed
}
