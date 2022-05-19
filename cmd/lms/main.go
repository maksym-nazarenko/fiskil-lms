package main

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"log"
	"os"
	"os/signal"
	"sync"
	"syscall"
	"time"

	"cloud.google.com/go/pubsub"
	"github.com/maxim-nazarenko/fiskil-lms/internal/lms"
	"github.com/maxim-nazarenko/fiskil-lms/internal/lms/app"
	lmspubsub "github.com/maxim-nazarenko/fiskil-lms/internal/lms/pubsub"
	"github.com/maxim-nazarenko/fiskil-lms/internal/lms/storage"
	"github.com/maxim-nazarenko/fiskil-lms/internal/lms/utils"
)

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
		appLogger.Info("received interruption request, closing the app")
		cancel()
	}()
	config, err := app.BuildConfiguration(args, os.Getenv)
	if err != nil {
		return err
	}

	mysqlConfig := storage.NewMysqlConfig()
	mysqlConfig.User = config.DB.User
	mysqlConfig.Passwd = config.DB.Password
	mysqlConfig.DBName = config.DB.Name
	mysqlConfig.Net = "tcp"
	mysqlConfig.Addr = config.DB.Address

	mysqlStorage, err := storage.NewMysqlStorage(mysqlConfig)
	if err != nil {
		return err
	}
	defer func() {
		if err := mysqlStorage.Close(); err != nil {
			appLogger.Error("could not close database connection: %v", err)
		}
	}()
	if err := dbConnect(appCtx, mysqlStorage, appLogger); err != nil {
		return err
	}
	projectRoot := utils.ProjectRootDir()
	if err := storage.Migrate("file://"+projectRoot+"/migrations/", mysqlStorage.DB()); err != nil {
		return fmt.Errorf("migrations failed: %v", err)
	}
	appLogger.Info("migration completed")

	dc := lms.NewDataCollector(appLogger, lms.NewSliceBuffer(), mysqlStorage)
	dc.WithProcessHooks(bufLengthHookFlusher(appCtx, config.FlushSize, dc.Flusher(), appLogger))

	wg := sync.WaitGroup{}

	// actual work
	inChan := make(chan *lms.Message, 100)

	// bridge connects incoming channel of messages and data collector
	consumerLogger := appLogger.SubLogger("bridge")
	wg.Add(1)
	go func(logger lms.Logger, inCh chan *lms.Message) {
		defer wg.Done()
		for message := range inCh {
			if err := dc.ProcessMessage(message); err != nil {
				logger.Error("failed to process message: %v", err)
			}
		}
		logger.Info("shutting down")
	}(consumerLogger, inChan)

	// Fake producer sends random data to the channel
	// producerLogger := appLogger.SubLogger("producer")
	// wg.Add(1)
	// go func(logger lms.Logger, inCh chan *lms.Message) {
	// 	defer wg.Done()
	// 	for {
	// 		select {
	// 		case <-appCtx.Done():
	// 			logger.Info("shutting down")
	// 			return
	// 		case <-time.After(time.Duration(rand.Intn(5)+1) * time.Second):
	// 			n := rand.Intn(3) + 1
	// 			inCh <- &lms.Message{
	// 				ServiceName: "service-" + strconv.Itoa(n),
	// 				Payload:     "payload here",
	// 				Severity:    lms.SEVERITY_INFO,
	// 				Timestamp:   time.Now().UTC(),
	// 			}
	// 		}
	// 	}
	// }(producerLogger, inChan)
	pubsubServer := lmspubsub.StartServer(appCtx, appLogger)
	pubsubProject := "lms"
	pubsubTopic := config.Pubsub.Topic
	pubsubClient, err := lmspubsub.NewClient(appCtx, pubsubServer.Addr, pubsubProject)
	if err != nil {
		return err
	}
	pubsubLogger := appLogger.SubLogger("pubsub")
	pubsubClient.SubscriptionInProject(pubsubTopic, pubsubProject).Receive(
		appCtx,
		func(ctx context.Context, m *pubsub.Message) {
			var message lms.Message
			if err := json.Unmarshal(m.Data, &message); err != nil {
				pubsubLogger.Error("failed to parse incoming message: %v", err)
			}
			pubsubLogger.Info("processing message: %v", message)
			inChan <- &message
		},
	)

	// time-based flusher periodically flushes data in data collector
	wg.Add(1)
	go func() {
		defer wg.Done()
		defer appLogger.Info("stopping interval flusher")

		if err := timeBasedFlusher(config.FlushInterval, dc.Flusher(), appLogger.SubLogger("flushtimer"))(appCtx); err != nil && !errors.Is(err, context.Canceled) {
			appLogger.Error("interval flusher exited with err: %v", err)
		}
	}()

	appLogger.Info("waiting for all background tasks to be completed")
	wg.Wait()

	return nil
}

func timeBasedFlusher(interval time.Duration, flush lms.FlushFunc, logger lms.Logger) lms.FlushFunc {
	return func(ctx context.Context) error {
		for {
			select {
			case <-ctx.Done():
				return ctx.Err()
			case <-time.After(interval):
				logger.Info("flushing data on timer (every %s)", interval)
				if err := flush(ctx); err != nil {
					logger.Error("flush error: %v", err)
				}
			}
		}
	}
}

func bufLengthHookFlusher(ctx context.Context, max int, flusher lms.FlushFunc, logger lms.Logger) lms.ProcessHookFunc {
	return func(m *lms.Message, mb lms.MessageBuffer) bool {
		if mb.Len() >= max {
			logger.Info("bufsize is >= %d, flushing data to storage", max)
			if err := flusher(ctx); err != nil {
				logger.Error("flush error: %v", err)
			}
		}

		return true
	}
}

func dbConnect(ctx context.Context, mysqlStorage storage.Storage, appLogger lms.Logger) error {
	dbPingCtx, dbPingCancel := context.WithTimeout(ctx, 10*time.Second)
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

	return nil
}
