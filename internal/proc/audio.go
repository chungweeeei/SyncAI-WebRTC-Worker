package proc

import (
	"fmt"
	"io"
	"os/exec"
	"sync"
)

type GStreamerPipeline struct {
	cmd    *exec.Cmd
	stdin  io.WriteCloser
	mu     sync.Mutex
	closed bool
}

func NewGStreamerPipeline(port int) (*GStreamerPipeline, error) {

	// GStreamer pipeline: 接收 RTP Opus → 解碼 → 播放
	pipeline := fmt.Sprintf(
		"udpsrc port=%d caps=\"application/x-rtp,media=audio,encoding-name=OPUS,payload=111\" "+
			"! rtpopusdepay "+
			"! opusdec "+
			"! audioconvert "+
			"! autoaudiosink",
		port,
	)

	cmd := exec.Command("sh", "-c", fmt.Sprintf("gst-launch-1.0 -v %s", pipeline))

	if err := cmd.Start(); err != nil {
		return nil, fmt.Errorf("failed to start GStreamer pipeline: %w", err)
	}

	return &GStreamerPipeline{
		cmd: cmd,
	}, nil
}

// Close 關閉 pipeline
func (g *GStreamerPipeline) Close() error {
	g.mu.Lock()
	defer g.mu.Unlock()

	g.closed = true

	if g.stdin != nil {
		g.stdin.Close()
	}

	if g.cmd != nil && g.cmd.Process != nil {
		return g.cmd.Process.Kill()
	}
	return nil
}
