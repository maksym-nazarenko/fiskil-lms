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

	// Storage defines interface to be satisfied by concrete storage implementation
	Storage interface {
		SaveMessages(ctx context.Context, messages []*Message) error
		WithTransaction(context.Context, func(context.Context, Querier) error) error
		Wait(f WaiterFunc) error
		Close() error
	}
)
