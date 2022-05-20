package main

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"log"
	"math/rand"
	"os"
	"os/signal"
	"strconv"
	"sync"
	"syscall"
	"time"

	"cloud.google.com/go/pubsub"
	"cloud.google.com/go/pubsub/pstest"
	"github.com/maxim-nazarenko/fiskil-lms/internal/lms"
	"github.com/maxim-nazarenko/fiskil-lms/internal/lms/app"
	lmspubsub "github.com/maxim-nazarenko/fiskil-lms/internal/lms/pubsub"
	"github.com/maxim-nazarenko/fiskil-lms/internal/lms/storage"
	"github.com/maxim-nazarenko/fiskil-lms/internal/lms/utils"
)

// it was added last second, so doesn't look really nice
type messagesWrapper struct {
	pubsubMessages []*pubsub.Message
	mu             sync.Mutex
}

func newMessagesWrapper() *messagesWrapper {
	return &messagesWrapper{
		pubsubMessages: []*pubsub.Message{},
		mu:             sync.Mutex{},
	}
}

func (mw *messagesWrapper) Append(m *pubsub.Message) int {
	mw.mu.Lock()
	defer mw.mu.Unlock()
	mw.pubsubMessages = append(mw.pubsubMessages, m)

	return len(mw.pubsubMessages)
}

func (mw *messagesWrapper) Clean() {
	mw.mu.Lock()
	defer mw.mu.Unlock()

	mw.pubsubMessages = []*pubsub.Message{}
}

func (mw *messagesWrapper) SendAcksAll() {
	mw.mu.Lock()
	defer mw.mu.Unlock()

	for _, m := range mw.pubsubMessages {
		m.Ack()
	}
	mw.pubsubMessages = []*pubsub.Message{}
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
	config, err := app.BuildConfiguration(args, os.Getenv)
	if err != nil {
		return err
	}

	go func() {
		select {
		case <-appCtx.Done():
		case <-time.After(config.StopAfter):
			appLogger.Info("LMS_STOP_AFTER (%s) period ended, stopping the app", config.StopAfter)
			cancel()
		}
	}()
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

	mw := newMessagesWrapper()
	var flusher lms.FlushFunc = func(ctx context.Context) error {
		if err := dc.Flusher()(ctx); err != nil {
			return err
		}
		mw.SendAcksAll()

		return nil
	}
	dc.WithProcessHooks(bufLengthHookFlusher(appCtx, config.FlushSize, flusher, appLogger))

	wg := sync.WaitGroup{}

	// channel connects pubsub topic -> data collector
	inChan := make(chan *lms.Message, config.FlushSize*2)

	wg.Add(1)
	go func() {
		defer wg.Done()
		defer appLogger.Info("closed message channel")
		<-appCtx.Done()
		close(inChan)
	}()

	// bridge connects incoming channel of messages and data collector
	wg.Add(1)
	go func(logger lms.Logger, inCh chan *lms.Message) {
		defer wg.Done()
		defer logger.Info("shutting down")
		for message := range inCh {
			logger.Info("received message from channel")
			if err := dc.ProcessMessage(message); err != nil {
				logger.Error("failed to process message: %v", err)
			}
		}
	}(appLogger.SubLogger("bridge"), inChan)

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

	// pub/sub implementation
	pubsubLogger := appLogger.SubLogger("pubsub")
	pubsubServer := lmspubsub.StartServer(appCtx, pubsubLogger.SubLogger("server"))
	pubsubProject := "lms"
	pubsubTopic := config.Pubsub.Topic
	pubsubClient, pubsubClientCancel, err := lmspubsub.NewClient(appCtx, pubsubServer.Addr, pubsubProject)
	if err != nil {
		return err
	}
	go func(ctx context.Context, logger lms.Logger) {
		defer pubsubClientCancel()
		defer logger.Info("shutting down")
		<-ctx.Done()
	}(appCtx, pubsubLogger.SubLogger("client"))

	// monitoring goroutine that prints number of not ACKed messages in queue
	wg.Add(1)
	go func(ctx context.Context, server *pstest.Server, logger lms.Logger) {
		defer wg.Done()
		defer logger.Info("shutting down")
		for {
			select {
			case <-ctx.Done():
				return
			case <-time.After(2 * time.Second):
				unacks := 0
				for _, m := range server.Messages() {
					if m.Acks == 0 {
						unacks++
					}
				}
				logger.Info("not ACKed messages in queue: %d", unacks)
			}
		}
	}(appCtx, pubsubServer, pubsubLogger.SubLogger("monitoring"))

	topic, err := pubsubClient.CreateTopic(appCtx, pubsubTopic)
	if err != nil {
		return err
	}
	defer topic.Stop()

	subscription, err := pubsubClient.CreateSubscription(appCtx, "consumer", pubsub.SubscriptionConfig{Topic: topic})
	if err != nil {
		return err
	}

	// pubsub producer of random messages
	producedMessagesStats := []*storage.LogStat{}
	wg.Add(1)
	go func(logger lms.Logger, results *[]*storage.LogStat) {
		defer wg.Done()
		defer logger.Info("shutting down")

		statsMap := map[string]*storage.LogStat{}
		for {
			select {
			case <-appCtx.Done():
				return
			case <-time.After(time.Duration(rand.Intn(5)+1) * time.Second):
				m := randomLogRecord(3)
				data, err := json.Marshal(m)
				if err != nil {
					logger.Error("%v", err)
					continue
				}
				_ = topic.Publish(appCtx, &pubsub.Message{Data: data})
				key := m.ServiceName + string(m.Severity)
				ls, ok := statsMap[key]
				if !ok {
					newStat := &storage.LogStat{
						ServiceName: m.ServiceName,
						Severity:    string(m.Severity),
						Count:       1,
					}
					*results = append(*results, newStat)
					statsMap[key] = newStat
				} else {
					ls.Count += 1
				}
			}
		}
	}(pubsubLogger.SubLogger("producer"), &producedMessagesStats)

	// time-based flusher periodically flushes data in data collector
	wg.Add(1)
	go func(logger lms.Logger) {
		defer wg.Done()
		defer logger.Info("shutting down")

		if err := timeBasedFlusher(config.FlushInterval, flusher, appLogger.SubLogger("flushtimer"))(appCtx); err != nil && !errors.Is(err, context.Canceled) {
			appLogger.Error("interval flusher exited with err: %v", err)
		}
	}(appLogger.SubLogger("interval flusher"))

	appLogger.Info("waiting for all background tasks to be completed")

	func(ctx context.Context, subscription *pubsub.Subscription, logger lms.Logger) {
		defer logger.Info("shutting down")

		err := subscription.Receive(
			ctx,
			func(ctx context.Context, m *pubsub.Message) {
				logger.Info("received message from pubsub topic")

				var message lms.Message
				if err := json.Unmarshal(m.Data, &message); err != nil {
					logger.Error("failed to parse incoming message: %v", err)
				}
				select {
				case <-ctx.Done():
				case inChan <- &message:
					mw.Append(m)
				}
			},
		)
		if err != nil {
			logger.Error(err.Error())
		}
	}(appCtx, subscription, pubsubLogger.SubLogger("consumer"))

	pubsubServer.Wait()
	wg.Wait()

	// flush data collector messages before stopping
	flushCtx, flushCancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer flushCancel()

	if err := flusher(flushCtx); err != nil {
		appLogger.Error(err.Error())
	}
	severityStatsCtx, severityStatsCancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer severityStatsCancel()
	severityStats, err := mysqlStorage.SeverityStats(severityStatsCtx)
	if err != nil {
		return err
	}

	logsStatsCtx, logsStatsCancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer logsStatsCancel()
	logsStats, err := mysqlStorage.SeverityStats(logsStatsCtx)
	if err != nil {
		return err
	}

	appLogger.Info("Statistics")
	appLogger.Info("----------")
	appLogger.Info("Produced messages")
	producedStatsMap := flattenLogStats(producedMessagesStats)
	totalProduced := 0
	for key, cnt := range producedStatsMap {
		appLogger.Info(`  %s: %d`, key, cnt)
		totalProduced += cnt
	}
	appLogger.Info("Total: %d", totalProduced)

	appLogger.Info("")
	appLogger.Info("service_logs table stats")
	logsStatsMap := flattenLogStats(logsStats)
	for key, cnt := range logsStatsMap {
		appLogger.Info(`  %s: %d`, key, cnt)
	}

	appLogger.Info("")
	appLogger.Info("service_severity table stats")
	severityStatsMap := flattenLogStats(severityStats)
	for key, cnt := range severityStatsMap {
		appLogger.Info(`  %s: %d`, key, cnt)
	}
	appLogger.Info("")
	appLogger.Info("Diffs")
	noDiffs := true
	for key, cnt := range producedStatsMap {
		if v, ok := logsStatsMap[key]; !ok {
			appLogger.Info("  service_logs table is missing '%s'", key)
			noDiffs = false
		} else if v != cnt {
			appLogger.Info("  service_logs table reports wrong value for '%s': want %d, got %d", key, cnt, v)
			noDiffs = false
		}
		if v, ok := severityStatsMap[key]; !ok {
			appLogger.Info("  service_severity table is missing '%s'", key)
			noDiffs = false
		} else if v != cnt {
			appLogger.Info("  service_severity table reports wrong value for '%s': want %d, got %d", key, cnt, v)
			noDiffs = false
		}
	}
	if noDiffs {
		appLogger.Info("  all good, everything is in sync")
	}

	return nil
}

func timeBasedFlusher(interval time.Duration, flush lms.FlushFunc, logger lms.Logger) lms.FlushFunc {
	return func(ctx context.Context) error {
		for {
			select {
			case <-ctx.Done():
				if err := flush(ctx); err != nil {
					logger.Error("flush error: %v", err)
				}
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

func randomLogRecord(maxIndex int) *lms.Message {
	n := rand.Intn(maxIndex) + 1
	possibleSeverities := []lms.Severity{
		lms.SEVERITY_DEBUG,
		lms.SEVERITY_INFO,
		lms.SEVERITY_WARN,
		lms.SEVERITY_ERROR,
		lms.SEVERITY_FATAL,
	}
	return &lms.Message{
		ServiceName: "service-" + strconv.Itoa(n),
		Payload:     "payload here",
		Severity:    possibleSeverities[rand.Intn(len(possibleSeverities))],
		Timestamp:   time.Now().UTC(),
	}
}

func flattenLogStats(stats []*storage.LogStat) map[string]int {
	statsMap := map[string]int{}
	for _, m := range stats {
		key := m.ServiceName + "," + string(m.Severity)
		cnt, ok := statsMap[key]
		newCount := m.Count
		if newCount < 1 {
			newCount = 1
		}
		if !ok {
			statsMap[key] = newCount
		} else {
			statsMap[key] = cnt + newCount
		}
	}
	return statsMap
}
