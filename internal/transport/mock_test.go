package transport

import (
	"context"
	"time"

	"github.com/stretchr/testify/mock"
)

var _ Transport = (*MockTransport)(nil)
var _ Dialer = (*MockDialer)(nil)

type MockTransport struct {
	mock.Mock
}

func (m *MockTransport) SendAndReceive(data []byte, timeout time.Duration) ([]byte, error) {
	args := m.Called(data, timeout)
	return args.Get(0).([]byte), args.Error(1)
}

func (m *MockTransport) OnMessage(handler MessageHandler) {
	m.Called(handler)
}

func (m *MockTransport) Close() error {
	args := m.Called()
	return args.Error(0)
}

type MockDialer struct {
	mock.Mock
}

func (m *MockDialer) Dial(ctx context.Context, url string) (Transport, error) {
	args := m.Called(ctx, url)
	if args.Get(0) == nil {
		return nil, args.Error(1)
	}
	return args.Get(0).(Transport), args.Error(1)
}
