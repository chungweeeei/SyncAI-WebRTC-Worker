package proc

import "github.com/go-gst/go-gst/gst"

// Pipeline define GStreamer pipeline interface
type Pipeline interface {
	SetState(state gst.State) error
	Unref()
}

// PipelineFactory define GStreamer pipeline factory interface
type PipelineFactory interface {
	NewPipelineFromString(pipelineStr string) (Pipeline, error)
}

// DefaultPipelineFactory is the default implementation of PipelineFactory
type DefaultPipelineFactory struct{}

func (f *DefaultPipelineFactory) NewPipelineFromString(pipelineStr string) (Pipeline, error) {
	return gst.NewPipelineFromString(pipelineStr)
}

// default factory instance
var defaultFactory PipelineFactory = &DefaultPipelineFactory{}

// SetDefaultFactory setting default factory, used for testing
func SetDefaultFactory(factory PipelineFactory) {
	defaultFactory = factory
}

func ResetDefaultFactory() {
	defaultFactory = &DefaultPipelineFactory{}
}
