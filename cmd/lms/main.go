package main

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"log"
	"os"
	"os/signal"
	"strconv"
	"sync"
	"syscall"
	"time"

	"github.com/maxim-nazarenko/fiskil-lms/internal/lms"
	"github.com/maxim-nazarenko/fiskil-lms/internal/lms/storage"
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
	// todo(maksym): populate this config using env variables
	mysqlConfig := storage.NewMysqlConfig()
	mysqlConfig.User = "root"
	mysqlConfig.Passwd = "root"
	mysqlConfig.DBName = "lms"
	mysqlConfig.Net = "tcp"
	mysqlConfig.Addr = "127.0.0.1:13306"

	mysqlStorage, err := storage.NewMysqlStorage(mysqlConfig)
	if err != nil {
		return err
	}
	defer func() {
		if err := mysqlStorage.Close(); err != nil {
			appLogger.Error("could not close database connection: %v", err)
		}
	}()

	dbPingCtx, dbPingCancel := context.WithTimeout(appCtx, 10*time.Second)
	defer dbPingCancel()

	dbUpWaitFunc := func(db *sql.DB) (bool, error) {
		for {
			select {
			case <-dbPingCtx.Done():
				return false, dbPingCtx.Err()
			case <-time.After(1 * time.Second):
				if err := db.PingContext(dbPingCtx); err != nil {
					appLogger.Info("db ping failed: %v", err)
					return true, err
				}
				return false, nil
			}
		}
	}
	if err := mysqlStorage.Wait(dbUpWaitFunc); err != nil {
		return err
	}

	if err := storage.Migrate("file://migrations/", mysqlStorage.DB()); err != nil {
		return fmt.Errorf("migraions failed: %v", err)
	}

	appLogger.Info("migration completed")
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
