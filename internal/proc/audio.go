package proc

import (
	"fmt"
	"io"
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
			"! pulsesink",
		port, cfg.AudioVolume,
	)
}

// use default factory to create GStreamer pipeline
func NewAudioGStreamerPipeline(port int, cfg config.AudioConfig) (*AudioGStreamerPipeline, error) {
	return NewAudioGStreamerPipelineWithFactory(port, cfg, defaultFactory)
}

// allow factory dependency injection for testing
func NewAudioGStreamerPipelineWithFactory(port int, cfg config.AudioConfig, factory PipelineFactory) (*AudioGStreamerPipeline, error) {
	pipelineStr := BuildAudioPipelineString(port, cfg)

	fmt.Println(pipelineStr)
	pipeline, err := factory.NewPipelineFromString(pipelineStr)
	if err != nil {
		return nil, err
	}

	if err := pipeline.SetState(gst.StatePlaying); err != nil {
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
		g.pipeline.SetState(gst.StateNull)
		g.pipeline.Unref()
	}
	return nil
}

func (g *AudioGStreamerPipeline) IsClosed() bool {
	g.mu.Lock()
	defer g.mu.Unlock()
	return g.closed
}
