package lms

import (
	"context"
	"sync"
	"time"
)

const (
	SEVERITY_DEBUG Severity = "debug"
	SEVERITY_INFO  Severity = "info"
	SEVERITY_WARN  Severity = "warn"
	SEVERITY_ERROR Severity = "error"
	SEVERITY_FATAL Severity = "fatal"
)

type (
	Severity string
	// Message represents core message this system works with
	Message struct {
		ServiceName string    `json:"service_name"`
		Payload     string    `json:"payload"`
		Severity    Severity  `json:"severity"`
		Timestamp   time.Time `json:"timestamp"`
	}
)

type (
	Logger interface {
		Info(msg string, args ...interface{})
		Error(msg string, args ...interface{})

		// SubLogger creates new sublogger with provided instance name
		SubLogger(name string) Logger
	}

	FlushFunc       func(context.Context) error
	ProcessHookFunc func(*Message, MessageBuffer) bool

	MessageBuffer interface {
		sync.Locker

		// Append adds new message to the buffer and returns updated buffer length
		Append(Message) (int, error)

		// Len calculates current buffer length
		Len() int

		// GetAll returns all messages in buffer
		GetAll() []*Message

		// Clean empties the buffer
		Clean() error
	}
)
