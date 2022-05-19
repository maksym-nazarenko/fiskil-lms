package storage

import (
	"context"
	"time"
)

type (
	// Message is a low-level object directly stored in persistent layer
	Message struct {
		ServiceName string
		Payload     string
		Severity    string
		Timestamp   time.Time
		CreatedAt   time.Time
	}

	// LogStat holds data for log statistics in storage
	LogStat struct {
		ServiceName string
		Severity    string
		Count       int
	}

	// Storage defines interface to be satisfied by concrete storage implementation
	Storage interface {
		// SaveMessages saves slice of messages to underlying storage
		SaveMessages(ctx context.Context, messages []*Message) error

		// LogStats calculates number of records for (service, severity) group
		LogStats(ctx context.Context) ([]*LogStat, error)

		// WithTransaction wraps functions in transaction and rolls it back if function returns error
		WithTransaction(context.Context, func(context.Context, Querier) error) error

		// Wait runs provided wait function until it returns true without error
		Wait(f WaiterFunc) error

		// Close closes underlying storage connection if supported by concrete implementation
		Close() error
	}
)
