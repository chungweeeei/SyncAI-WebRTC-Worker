package proc

import (
	"fmt"
	"os/exec"
	"sync"
)

type VideoGStreamerPipeline struct {
	cmd    *exec.Cmd
	mu     sync.Mutex
	closed bool
}

func NewVideoGStreamerPipeline(port int) (*VideoGStreamerPipeline, error) {
	pipeline := "v4l2src=device=/dev/syncai_camera " +
		"! videoconvert " +
		"! 'video/x-raw,width=640,height=480,framerate=30/1' " +
		"! queue " +
		"! nvvidconv " +
		"! 'video/x-raw(memory:NVMM),format=NV12' " +
		"! nvv4l2h264enc bitrate=4000000 insert-sps-pps=true " +
		"! h264parse " +
		"! rtph264pay config-interval=1 pt=96 " +
		"! udpsink host=127.0.0.1 port=5006 sync=false"

	cmd := exec.Command("sh", "-c", fmt.Sprintf("gst-launch-1.0 -v %s", pipeline))

	if err := cmd.Start(); err != nil {
		return nil, fmt.Errorf("failed to start GStreamer video pipeline: %w", err)
	}

	return &VideoGStreamerPipeline{
		cmd: cmd,
	}, nil
}

func (g *VideoGStreamerPipeline) Close() error {
	g.mu.Lock()
	defer g.mu.Unlock()

	if g.closed {
		return nil
	}

	g.closed = true
	if g.cmd != nil && g.cmd.Process != nil {
		return g.cmd.Process.Kill()
	}
	return nil
}
