package lms

type dataCollector struct {
	logger         Logger
	onProcessHooks []ProcessHookFunc
	buffer         MessageBuffer
}

func (dc *dataCollector) Flush() error {
	return nil
}

func (dc *dataCollector) Flusher() FlushFunc {
	return dc.flush
}

func (dc *dataCollector) WithProcessHooks(hooks ...ProcessHookFunc) *dataCollector {
	dc.onProcessHooks = hooks
	return dc
}

func (dc *dataCollector) flush() error {
	dc.logger.Info("flushing buffer")
	return nil
}

func (dc *dataCollector) processMessage(msg *Message) error {
	for _, h := range dc.onProcessHooks {
		h(msg, dc.buffer)
	}
	return nil
}

// NewDataCollector constructs initialized data collector
func NewDataCollector(logger Logger) *dataCollector {
	return &dataCollector{
		logger: logger,
	}
}
