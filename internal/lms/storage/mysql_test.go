package storage

import (
	"context"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestSaveMessages(t *testing.T) {
	testCtx, cancel := context.WithTimeout(context.Background(), 1*time.Minute)
	defer cancel()

	mysqlStorage := NewTestDatabase(testCtx)
	cases := []struct {
		name     string
		messages []*Message
	}{
		{
			name: "happy path",
			messages: []*Message{
				{
					ServiceName: "service-1",
					Payload:     "some payload 1",
					Timestamp:   time.Now().UTC().Truncate(time.Second),
					Severity:    "warn",
				},
				{
					ServiceName: "service-2",
					Payload:     "some payload 2",
					Timestamp:   time.Now().UTC().Truncate(time.Second),
					Severity:    "info",
				},
				{
					ServiceName: "service-3",
					Payload:     "some payload 3",
					Timestamp:   time.Now().UTC().Truncate(time.Second),
					Severity:    "fatal",
				},
			},
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			ctx, cancel := context.WithTimeout(testCtx, 10*time.Second)
			defer cancel()

			err := mysqlStorage.SaveMessages(ctx, tc.messages)
			require.NoError(t, err)

			readMessages, err := mysqlStorage.ListMessages(ctx)
			require.NoError(t, err)
			// ignore createdAt, since it is set by MySQL
			for _, m := range readMessages {
				m.CreatedAt = time.Time{}
			}
			assert.ElementsMatch(t, tc.messages, readMessages)
		})
	}
}
