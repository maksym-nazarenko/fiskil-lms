package lms

type dataCollector struct {
	logger Logger
}

func (dc *dataCollector) Flush() error {
	return nil
}

func (dc *dataCollector) Flusher() FlushFunc {
	return dc.flush
}

func (dc *dataCollector) flush() error {
	dc.logger.Info("flushing buffer")
	return nil
}

// NewDataCollector constructs initialized data collector
func NewDataCollector(logger Logger) *dataCollector {
	return &dataCollector{
		logger: logger,
	}
}
