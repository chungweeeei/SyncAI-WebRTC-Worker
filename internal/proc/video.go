package proc

import (
	"fmt"
	"sync"

	"github.com/go-gst/go-gst/gst"
)

type VideoGStreamerPipeline struct {
	pipeline *gst.Pipeline
	mu       sync.Mutex
	closed   bool
}

func NewVideoGStreamerPipeline(videoPort, audioPort int) (*VideoGStreamerPipeline, error) {
	pipelineStr := fmt.Sprintf(
		"v4l2src device=/dev/syncai_camera do-timestamp=true "+
			"! videoconvert "+
			"! video/x-raw,width=640,height=480,framerate=30/1 "+
			"! queue max-size-buffers=3 max-size-time=0 max-size-bytes=0 leaky=downstream "+
			"! nvvidconv "+
			"! video/x-raw(memory:NVMM),format=NV12 "+
			"! nvv4l2h264enc bitrate=4000000 insert-sps-pps=true "+
			"! h264parse "+
			"! rtph264pay config-interval=1 pt=96 "+
			"! udpsink host=127.0.0.1 port=%d sync=true "+
			"alsasrc device=hw:0,0 do-timestamp=true "+
			"! audioconvert "+
			"! audioresample "+
			"! audio/x-raw,rate=48000,channels=2 "+
			"! queue max-size-buffers=3 max-size-time=0 max-size-bytes=0 "+
			"! opusenc bitrate=64000 "+
			"! rtpopuspay pt=111 "+
			"! udpsink host=127.0.0.1 port=%d sync=true",
		videoPort, audioPort)

	fmt.Println(pipelineStr)
	pipeline, err := gst.NewPipelineFromString(pipelineStr)
	if err != nil {
		return nil, err
	}

	if err := pipeline.SetState(gst.StatePlaying); err != nil {
		return nil, err
	}

	return &VideoGStreamerPipeline{
		pipeline: pipeline,
	}, nil
}

func (g *VideoGStreamerPipeline) Close() error {
	g.mu.Lock()
	defer g.mu.Unlock()

	if g.closed {
		return nil
	}

	g.closed = true
	if g.pipeline != nil {
		g.pipeline.SetState(gst.StateNull)
		g.pipeline.Unref()
	}

	return nil
}
