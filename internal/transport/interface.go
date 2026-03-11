package transport

import "context"

type MessageHandler func(data []byte)

type Transport interface {
	Send(data []byte) error
	OnMessage(handler MessageHandler)
	Close() error
}

type Dialer interface {
	Dial(ctx context.Context, url string) (Transport, error)
}
