package proc

import (
	"github.com/go-gst/go-gst/gst"
	"github.com/stretchr/testify/mock"
)

type MockPipeline struct {
	mock.Mock
}

func (m *MockPipeline) SetState(state gst.State) error {
	args := m.Called(state)
	return args.Error(0)
}

func (m *MockPipeline) BlockSetState(state gst.State) error {
	args := m.Called(state)
	return args.Error(0)
}

type MockPipelineFactory struct {
	mock.Mock
}

func (m *MockPipelineFactory) NewPipelineFromString(pipelineStr string) (Pipeline, error) {
	args := m.Called(pipelineStr)
	if args.Get(0) == nil {
		return nil, args.Error(1)
	}
	return args.Get(0).(Pipeline), args.Error(1)
}
