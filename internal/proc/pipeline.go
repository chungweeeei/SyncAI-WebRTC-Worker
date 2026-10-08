package proc

import "github.com/go-gst/go-gst/gst"

// Pipeline define GStreamer pipeline interface
//
// Deliberately no Unref, and that is the fix for a crash rather than a
// simplification. go-gst hands every GstPipeline back through
// glib.TransferNone, which ref-sinks it and installs a runtime finalizer that
// unrefs when the Go wrapper is collected -- and glib.Object.Unref does not
// clear that finalizer. Calling it from Close() as well was therefore a double
// free: the last reference went at teardown, and the finalizer unreffed the
// same address at the next GC, by which time GLib had usually recycled it into
// the *following* session's pipeline. It surfaced as "Trying to dispose
// element v4l2srcN, but it is in PLAYING instead of the NULL state" logged
// against a pipeline that had just started, and then SIGSEGV in the host
// process -- reliably after two or three open/close cycles, never on the
// first.
//
// Releasing a pipeline is therefore just dropping the reference to it. What
// still has to be deterministic is the state change, below.
type Pipeline interface {
	SetState(state gst.State) error

	// BlockSetState sets the state and blocks until the transition has
	// completed. Teardown needs it and startup does not: a v4l2 source
	// releases the device on the way down to NULL, and SetState alone only
	// *requests* that transition. Dropping the last reference while it is
	// still in flight leaves the camera open for the life of the process --
	// and the process here is the robot's backend, which cannot unload this
	// library, so the next session finds the device wedged and negotiates
	// nothing.
	BlockSetState(state gst.State) error
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
