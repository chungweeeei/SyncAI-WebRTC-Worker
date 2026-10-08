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

// BuildVideoPipelineString builds the capture pipeline feeding the WHEP
// session's two RTP listeners.
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

func BuildVideoPipelineString(videoPort, audioPort int, cfg config.VideoConfig) string {
	// Half a second of frames, matching the host publisher's IDR_INTERVAL.
	idrInterval := cfg.Framerate / 2
	if idrInterval < 1 {
		idrInterval = 1
	}

	sensorControls := buildSensorControls(cfg)

	var source string
	if strings.EqualFold(cfg.Format, "uyvy") {
		// Raw capture: videoconvert is what lets nvvidconv take the frame into
		// NVMM. The sensor advertises UYVY only at the higher framerates
		// (640x480 is 120/60 fps, no 30), so a mismatched VIDEO_FRAMERATE
		// fails here as not-negotiated.
		source = fmt.Sprintf(
			"v4l2src device=%s io-mode=2 extra-controls=\"%s\" do-timestamp=true "+
				"! video/x-raw,format=UYVY,width=%d,height=%d,framerate=%d/1 "+
				"! queue max-size-buffers=3 max-size-time=0 max-size-bytes=0 leaky=downstream "+
				"! videoconvert ",
			cfg.Device, sensorControls, cfg.Width, cfg.Height, cfg.Framerate)
	} else {
		// MJPEG: the queue sits before the decoder on purpose -- dropping a
		// compressed frame under back-pressure is cheaper than dropping a
		// decoded one, and leaky=downstream is what keeps a slow encoder from
		// stalling the capture thread.
		source = fmt.Sprintf(
			"v4l2src device=%s io-mode=2 extra-controls=\"%s\" do-timestamp=true "+
				"! image/jpeg,width=%d,height=%d,framerate=%d/1 "+
				"! queue max-size-buffers=3 max-size-time=0 max-size-bytes=0 leaky=downstream "+
				"! nvjpegdec ! video/x-raw(memory:NVMM) ",
			cfg.Device, sensorControls, cfg.Width, cfg.Height, cfg.Framerate)
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

// NewVideoGStreamerPipeline uses the default PipelineFactory.
func NewVideoGStreamerPipeline(videoPort, audioPort int, cfg config.VideoConfig) (*VideoGStreamerPipeline, error) {
	return NewVideoGStreamerPipelineWithFactory(videoPort, audioPort, cfg, defaultFactory)
}

// NewVideoGStreamerPipelineWithFactory allows injecting a PipelineFactory (for tests).
func NewVideoGStreamerPipelineWithFactory(videoPort, audioPort int, cfg config.VideoConfig, factory PipelineFactory) (*VideoGStreamerPipeline, error) {
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
				gst.MessageError|gst.MessageWarning|gst.MessageEOS,
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
