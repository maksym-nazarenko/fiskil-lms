package main

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"log"
	"math/rand"
	"os"
	"os/signal"
	"strconv"
	"strings"
	"sync"
	"syscall"
	"time"

	"github.com/maxim-nazarenko/fiskil-lms/internal/lms"
	"github.com/maxim-nazarenko/fiskil-lms/internal/lms/storage"
	"github.com/maxim-nazarenko/fiskil-lms/internal/lms/utils"
)

type configuration struct {
	flushInterval time.Duration
	flushSize     int
	db            struct {
		address  string
		user     string
		password string
		name     string
	}
	pubsub struct {
		topic string
	}
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
		appLogger.Info("received interruption request, closing the app")
		cancel()
	}()
	config, err := parseFlags(args)
	if err != nil {
		return err
	}
	// todo(maksym): populate this config using env variables
	mysqlConfig := storage.NewMysqlConfig()
	mysqlConfig.User = config.db.user
	mysqlConfig.Passwd = config.db.password
	mysqlConfig.DBName = config.db.name
	mysqlConfig.Net = "tcp"
	mysqlConfig.Addr = config.db.address

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
	flusher := dc.Flusher()

	wg := sync.WaitGroup{}

	// actual work
	// pubsubServer := lmspubsub.StartServer(appCtx, appLogger)
	// pubsubProject := "lms"
	// pubsubTopic := config.pubsub.topic
	// pubsubClient, err := lmspubsub.NewClient(appCtx, pubsubServer.Addr, pubsubProject)
	// if err != nil {
	// 	return err
	// }

	inChan := make(chan *lms.Message, 100)

	// bridge connects incoming channel of messages and data collector
	consumerLogger := appLogger.SubLogger("bridge")
	wg.Add(1)
	go func(logger lms.Logger, inCh chan *lms.Message) {
		defer wg.Done()
		for message := range inCh {
			logger.Info("processing message")
			if err := dc.ProcessMessage(message); err != nil {
				logger.Error("failed to process message: %v", err)
			}
			logger.Info("done")
		}
		logger.Info("shutting down")
	}(consumerLogger, inChan)

	// Fake producer sends random data to the channel
	producerLogger := appLogger.SubLogger("producer")
	wg.Add(1)
	go func(logger lms.Logger, inCh chan *lms.Message) {
		defer wg.Done()
		for {
			select {
			case <-appCtx.Done():
				logger.Info("shutting down")
				return
			case <-time.After(time.Duration(rand.Intn(5)+1) * time.Second):
				logger.Info("sending message")
				n := rand.Intn(3) + 1
				inCh <- &lms.Message{
					ServiceName: "service-" + strconv.Itoa(n),
					Payload:     "payload here",
					Severity:    lms.SEVERITY_INFO,
					Timestamp:   time.Now().UTC(),
				}
			}
		}
	}(producerLogger, inChan)

	// pubsubLogger := appLogger.SubLogger("consumer")
	// pubsubClient.SubscriptionInProject(pubsubTopic, pubsubProject).Receive(
	// 	appCtx,
	// 	func(ctx context.Context, m *pubsub.Message) {
	// 		var message lms.Message
	// 		if err := json.Unmarshal(m.Data, &message); err != nil {
	// 			pubsubLogger.Error("failed to parse incoming message: %v", err)
	// 		}
	// 		pubsubLogger.Info("processing message: %v", message)
	// 	},
	// )

	// time-based flusher periodically flushes data in data collector
	wg.Add(1)
	go func() {
		defer wg.Done()
		defer appLogger.Info("stopping interval flusher")
		if err := timeBasedFlusher(config.flushInterval, flusher, func(err error) {
			appLogger.Error("flushing failed: %v", err)
		})(appCtx); err != nil && !errors.Is(err, context.Canceled) {
			appLogger.Error("interval flusher exited with err: %v", err)
		}
	}()
	appLogger.Info("waiting for all background tasks to be completed")
	wg.Wait()

	return nil
}

// parseFlags builds configuration of the app based on env
// todo(maksym): consider using CLI flags if feasible
// todo(maksym): consider using something like github.com/spf13/viper or build own solution
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

	config.db.address = os.Getenv("LMS_DB_ADDRESS")
	config.db.name = os.Getenv("LMS_DB_NAME")
	config.db.password = os.Getenv("LMS_DB_PASSWORD")
	config.db.user = os.Getenv("LMS_DB_USER")

	config.pubsub.topic = os.Getenv("LMS_PUBSUB_TOPIC")
	if strings.TrimSpace(config.pubsub.topic) == "" {
		config.pubsub.topic = "lms"
	}

	return &config, nil
}

func timeBasedFlusher(interval time.Duration, flush lms.FlushFunc, errorHandler func(error)) lms.FlushFunc {
	return func(ctx context.Context) error {
		for {
			select {
			case <-ctx.Done():
				return ctx.Err()
			case <-time.After(interval):
				if err := flush(ctx); err != nil {
					errorHandler(err)
				}
			}
		}
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
