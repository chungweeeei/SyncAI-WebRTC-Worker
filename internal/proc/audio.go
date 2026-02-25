package proc

import (
	"fmt"
	"io"
	"os"
	"os/exec"
	"sync"
	"syscall"
)

type AudioGStreamerPipeline struct {
	cmd    *exec.Cmd
	stdin  io.WriteCloser
	mu     sync.Mutex
	closed bool
}

func NewAudioGStreamerPipeline(port int) (*AudioGStreamerPipeline, error) {

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
	cmd.Stderr = os.Stderr

	// setup process group so we can kill the entire pipeline if needed
	cmd.SysProcAttr = &syscall.SysProcAttr{
		Setpgid: true,
	}

	if err := cmd.Start(); err != nil {
		return nil, fmt.Errorf("failed to start GStreamer pipeline: %w", err)
	}

	return &AudioGStreamerPipeline{
		cmd: cmd,
	}, nil
}

func (g *AudioGStreamerPipeline) Close() error {
	g.mu.Lock()
	defer g.mu.Unlock()

	g.closed = true

	if g.stdin != nil {
		g.stdin.Close()
	}

	if g.cmd != nil && g.cmd.Process != nil {
		pgid, err := syscall.Getpgid(g.cmd.Process.Pid)
		if err == nil {
			syscall.Kill(-pgid, syscall.SIGTERM)
		}
		g.cmd.Wait()
	}
	return nil
}
