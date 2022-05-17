package lms

import (
	"context"
	"sync"
	"time"
)

// Message represents core message this system works with
type Message struct {
	ServiceName string    `json:"service_name"`
	Payload     string    `json:"payload"`
	Severity    string    `json:"severity"`
	Timestamp   time.Time `json:"timestamp"`
}

type (
	Flusher interface {
		Flush() error
	}
	Logger interface {
		Info(msg string, args ...interface{})
		Error(msg string, args ...interface{})
	}

	FlushFunc       func() error
	ProcessHookFunc func(*Message, MessageBuffer) bool

	MessageBuffer interface {
		sync.Locker

		// Append adds new message to the buffer and returns updated buffer length
		Append(Message) (int, error)
		// Len calculates current buffer length
		Len() int
	}

	Querier interface {
		WithTransaction(ctx context.Context, q Querier) error
	}
)
