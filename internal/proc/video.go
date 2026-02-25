package proc

import (
	"fmt"
	"os"
	"os/exec"
	"sync"
	"syscall"
)

type VideoGStreamerPipeline struct {
	cmd    *exec.Cmd
	mu     sync.Mutex
	closed bool
}

func NewVideoGStreamerPipeline(videoPort, audioPort int) (*VideoGStreamerPipeline, error) {
	pipeline := fmt.Sprintf(
		"v4l2src device=/dev/syncai_camera "+
			"! videoconvert "+
			"! video/x-raw,width=640,height=480,framerate=30/1 "+
			"! queue "+
			"! nvvidconv "+
			"! 'video/x-raw(memory:NVMM),format=NV12' "+
			"! nvv4l2h264enc bitrate=4000000 insert-sps-pps=true "+
			"! h264parse "+
			"! rtph264pay config-interval=1 pt=96 "+
			"! udpsink host=127.0.0.1 port=%d sync=false "+
			"alsasrc device=hw:0,0 "+
			"! audioconvert "+
			"! audioresample "+
			"! audio/x-raw,rate=48000,channels=2 "+
			"! queue "+
			"! opusenc bitrate=64000 "+
			"! rtpopuspay pt=111 "+
			"! udpsink host=127.0.0.1 port=%d sync=false ",
		videoPort, audioPort)

	cmd := exec.Command("sh", "-c", fmt.Sprintf("gst-launch-1.0 -v %s", pipeline))
	cmd.Stderr = os.Stderr

	// setup process group so we can kill the entire pipeline if needed
	cmd.SysProcAttr = &syscall.SysProcAttr{
		Setpgid: true,
	}

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
		pgid, err := syscall.Getpgid(g.cmd.Process.Pid)
		if err == nil {
			syscall.Kill(-pgid, syscall.SIGTERM)
		}
		g.cmd.Wait()
	}
	return nil
}
