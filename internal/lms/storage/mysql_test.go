package storage

import (
	"context"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestSaveMessages(t *testing.T) {
	if testing.Short() {
		t.Skip("Skipping integration tests")
	}
	testCtx, cancel := context.WithTimeout(context.Background(), 1*time.Minute)
	defer cancel()

	mysqlStorage, dbName := NewTestDatabase(testCtx, t)
	t.Logf("test DB name: %s", dbName)

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

func TestLogStats(t *testing.T) {
	if testing.Short() {
		t.Skip("Skipping integration tests")
	}
	testCtx, cancel := context.WithTimeout(context.Background(), 1*time.Minute)
	defer cancel()

	mysqlStorage, dbName := NewTestDatabase(testCtx, t)
	t.Logf("test DB name: %s", dbName)

	expectedResult := map[string]map[string]int{
		"service-1": {
			"info":  2,
			"debug": 5,
		},
		"service-2": {
			"warn":  3,
			"debug": 12,
		},
		"service-3": {
			"error": 2,
			"warn":  9,
			"info":  5,
		},
		"service-4": {
			"fatal": 1,
			"warn":  2,
		},
		"service-5": {
			"info": 1,
		},
	}
	messagesToSave := []*Message{}
	expectedLogStats := []*LogStat{}
	for serviceName, logStats := range expectedResult {
		for severity, count := range logStats {
			for i := 0; i < count; i++ {
				messagesToSave = append(messagesToSave, &Message{
					ServiceName: serviceName,
					Severity:    severity,
					Timestamp:   time.Now().UTC(),
				})
			}
			expectedLogStats = append(expectedLogStats,
				&LogStat{
					ServiceName: serviceName,
					Severity:    severity,
					Count:       count,
				})
		}
	}

	err := mysqlStorage.SaveMessages(testCtx, messagesToSave)
	require.NoError(t, err)

	stats, err := mysqlStorage.LogStats(testCtx)
	require.NoError(t, err)
	assert.ElementsMatch(t, expectedLogStats, stats)
}

func TestSeverityStats(t *testing.T) {
	if testing.Short() {
		t.Skip("Skipping integration tests")
	}
	testCtx, cancel := context.WithTimeout(context.Background(), 1*time.Minute)
	defer cancel()

	mysqlStorage, dbName := NewTestDatabase(testCtx, t)
	t.Logf("test DB name: %s", dbName)

	expectedResult := map[string]map[string]int{
		"service-1": {
			"info":  2,
			"debug": 5,
		},
		"service-2": {
			"warn":  3,
			"debug": 12,
		},
		"service-3": {
			"error": 2,
			"warn":  9,
			"info":  5,
		},
	}
	messagesToSave := []*Message{}
	expectedLogStats := []*LogStat{}
	batchesCount := 2
	for serviceName, logStats := range expectedResult {
		for severity, count := range logStats {
			for i := 0; i < count; i++ {
				messagesToSave = append(messagesToSave, &Message{
					ServiceName: serviceName,
					Severity:    severity,
					Timestamp:   time.Now().UTC(),
				})
			}
			expectedLogStats = append(expectedLogStats,
				&LogStat{
					ServiceName: serviceName,
					Severity:    severity,
					Count:       count * batchesCount,
				})
		}
	}

	err := mysqlStorage.SaveMessages(testCtx, messagesToSave)
	require.NoError(t, err)

	err = mysqlStorage.SaveMessages(testCtx, messagesToSave)
	require.NoError(t, err)

	stats, err := mysqlStorage.SeverityStats(testCtx)
	require.NoError(t, err)
	assert.ElementsMatch(t, expectedLogStats, stats)
}
