package proc

import (
	"fmt"
	"io"
	"sync"

	"github.com/go-gst/go-gst/gst"
)

type AudioGStreamerPipeline struct {
	pipeline *gst.Pipeline
	stdin    io.WriteCloser
	mu       sync.Mutex
	closed   bool
}

func NewAudioGStreamerPipeline(port int) (*AudioGStreamerPipeline, error) {

	// GStreamer pipeline: 接收 RTP Opus → 解碼 → 播放
	pipelineStr := fmt.Sprintf(
		"udpsrc port=%d caps=\"application/x-rtp,media=audio,encoding-name=OPUS,payload=111\" "+
			"! rtpopusdepay "+
			"! opusdec "+
			"! audioconvert "+
			"! autoaudiosink",
		port,
	)

	pipeline, err := gst.NewPipelineFromString(pipelineStr)
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
