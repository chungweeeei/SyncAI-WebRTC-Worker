package transport

import (
	"context"
	"time"
)

type MessageHandler func(data []byte)

type Transport interface {
	SendAndReceive(data []byte, timeout time.Duration) ([]byte, error)
	OnMessage(handler MessageHandler)
	Close() error
}

type Dialer interface {
	Dial(ctx context.Context, url string) (Transport, error)
}
