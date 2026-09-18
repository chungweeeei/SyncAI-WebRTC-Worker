package proc

import (
	"fmt"
	"io"
	"log"
	"sync"

	"SyncAI-WebRTC-Worker/internal/config"

	"github.com/go-gst/go-gst/gst"
)

type AudioGStreamerPipeline struct {
	pipeline Pipeline
	stdin    io.WriteCloser
	mu       sync.Mutex
	closed   bool
}

func BuildAudioPipelineString(port int, cfg config.AudioConfig) string {
	return fmt.Sprintf(
		"udpsrc port=%d caps=\"application/x-rtp,media=audio,encoding-name=OPUS,payload=111\" "+
			"! rtpopusdepay "+
			"! opusdec "+
			"! audioconvert "+
			"! volume volume=%f "+
			"! audioresample "+
			"! %s",
		port, cfg.AudioVolume, cfg.Sink,
	)
}

// use default factory to create GStreamer pipeline
func NewAudioGStreamerPipeline(port int, cfg config.AudioConfig) (*AudioGStreamerPipeline, error) {
	return NewAudioGStreamerPipelineWithFactory(port, cfg, defaultFactory)
}

// allow factory dependency injection for testing
func NewAudioGStreamerPipelineWithFactory(port int, cfg config.AudioConfig, factory PipelineFactory) (*AudioGStreamerPipeline, error) {
	pipelineStr := BuildAudioPipelineString(port, cfg)

	pipeline, err := factory.NewPipelineFromString(pipelineStr)
	if err != nil {
		return nil, err
	}

	if err := pipeline.SetState(gst.StatePlaying); err != nil {
		// Back to NULL for the same reason the video pipeline's failed start
		// is: the finalizer unrefs whatever state the pipeline was left in,
		// and an ALSA sink disposed above NULL keeps the speaker.
		if nullErr := pipeline.BlockSetState(gst.StateNull); nullErr != nil {
			log.Printf("audio pipeline did not reach NULL after a failed start: %v", nullErr)
		}
		return nil, err
	}

	return &AudioGStreamerPipeline{
		pipeline: pipeline,
	}, nil
}

func (g *AudioGStreamerPipeline) Close() error {
	g.mu.Lock()
	defer g.mu.Unlock()

	if g.closed {
		return nil
	}
	g.closed = true

	if g.stdin != nil {
		g.stdin.Close()
	}

	if g.pipeline != nil {
		// Blocking for the same reason the video pipeline's teardown is; see
		// the Pipeline interface -- which is also why the reference is merely
		// dropped here rather than unreffed by hand.
		if err := g.pipeline.BlockSetState(gst.StateNull); err != nil {
			log.Printf("audio pipeline did not reach NULL cleanly: %v", err)
		}
		g.pipeline = nil
	}
	return nil
}

func (g *AudioGStreamerPipeline) IsClosed() bool {
	g.mu.Lock()
	defer g.mu.Unlock()
	return g.closed
}
