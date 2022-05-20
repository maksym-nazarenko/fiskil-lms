package lms

import (
	"context"
	"sync"

	"github.com/maxim-nazarenko/fiskil-lms/internal/lms/storage"
)

type dataCollector struct {
	logger         Logger
	onProcessHooks []ProcessHookFunc
	buffer         MessageBuffer
	storage        storage.Storage

	// fmu guards calls to flush()
	fmu sync.Mutex
}

// NewDataCollector constructs initialized data collector
func NewDataCollector(logger Logger, buffer MessageBuffer, storage storage.Storage) *dataCollector {
	return &dataCollector{
		logger:  logger,
		buffer:  buffer,
		storage: storage,
		fmu:     sync.Mutex{},
	}
}

func (dc *dataCollector) Flusher() FlushFunc {
	return dc.flush
}

func (dc *dataCollector) WithProcessHooks(hooks ...ProcessHookFunc) *dataCollector {
	dc.onProcessHooks = hooks
	return dc
}

func (dc *dataCollector) Buffer() MessageBuffer {
	return dc.buffer
}

func (dc *dataCollector) ProcessMessage(msg *Message) error {
	_, err := dc.buffer.Append(*msg)
	for _, h := range dc.onProcessHooks {
		if !h(msg, dc.buffer) {
			break
		}
	}
	return err
}

func (dc *dataCollector) flush(ctx context.Context) error {
	dc.fmu.Lock()
	defer dc.fmu.Unlock()
	messages := dc.messagesToStorageMessages(dc.buffer.GetAll())
	if err := dc.storage.SaveMessages(ctx, messages); err != nil {
		return err
	}
	if err := dc.buffer.Clean(); err != nil {
		return err
	}

	return nil
}

func (dc *dataCollector) messagesToStorageMessages(messages []*Message) []*storage.Message {
	result := make([]*storage.Message, 0, len(messages))
	for _, m := range messages {
		result = append(result, &storage.Message{
			ServiceName: m.ServiceName,
			Payload:     m.Payload,
			Severity:    string(m.Severity),
			Timestamp:   m.Timestamp,
		})
	}
	return result
}
