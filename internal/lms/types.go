package lms

import "time"

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

	FlushFunc func() error
)
