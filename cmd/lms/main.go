package main

import (
	"context"
	"errors"
	"log"
	"os"
	"os/signal"
	"strconv"
	"sync"
	"syscall"
	"time"

	"github.com/maxim-nazarenko/fiskil-lms/internal/lms"
)

const (
	messageChanBufSize int = 1000
)

type configuration struct {
	flushInterval time.Duration
	flushSize     int
}

func main() {
	if err := run(os.Args[1:]); err != nil {
		log.Fatal(err)
		os.Exit(1)

	}
	os.Exit(0)
}

func run(args []string) error {
	appCtx, cancel := context.WithCancel(context.Background())
	defer cancel()

	appLogger := lms.NewInstanceLogger(os.Stdout, "DCL")

	signalChan := make(chan os.Signal, 1)
	signal.Notify(signalChan, os.Interrupt, syscall.SIGQUIT, syscall.SIGHUP, syscall.SIGTERM)
	go func() {
		<-signalChan
		appLogger.Info("received interuption request, closing the app")
		cancel()
	}()
	config, err := parseFlags(args)
	if err != nil {
		return err
	}
	// inCh := make(chan lms.Message, messageChanBufSize)
	dc := lms.NewDataCollector(appLogger, lms.NewSliceBuffer())
	flusher := dc.Flusher()

	wg := sync.WaitGroup{}
	wg.Add(1)
	go func() {
		defer wg.Done()
		defer appLogger.Info("stopping interval flusher")
		if err := timeBasedFlusher(appCtx, config.flushInterval, flusher, func(err error) {
			appLogger.Error("flushing failed: %v", err)
		})(); err != nil && !errors.Is(err, context.Canceled) {
			appLogger.Error("interval flusher exited with err: %v", err)
		}
	}()
	wg.Wait()

	return nil
}

// parseFlags builds configuration of the app based on env
// todo(maksym): consider using CLI flags if feasible
func parseFlags(args []string) (*configuration, error) {
	config := configuration{
		flushInterval: 1 * time.Minute,
		flushSize:     5000,
	}

	flushIntervalStr := os.Getenv("LMS_FLUSH_INTERVAL")
	if flushIntervalStr != "" {
		flushInterval, err := time.ParseDuration(flushIntervalStr)
		if err != nil {
			return nil, err
		}
		config.flushInterval = flushInterval
	}

	flushSizeStr := os.Getenv("LMS_FLUSH_SIZE")
	if flushSizeStr != "" {
		flushSize, err := strconv.Atoi(flushSizeStr)
		if err != nil {
			return nil, err
		}
		config.flushSize = flushSize
	}

	return &config, nil
}

func timeBasedFlusher(ctx context.Context, interval time.Duration, flush lms.FlushFunc, errorHandler func(error)) lms.FlushFunc {
	return func() error {
		for {
			select {
			case <-ctx.Done():
				return ctx.Err()
			case <-time.After(interval):
				if err := flush(); err != nil {
					errorHandler(err)
				}
			}
		}
	}
}
