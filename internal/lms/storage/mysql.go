package storage

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/go-sql-driver/mysql"
	_ "github.com/go-sql-driver/mysql"
)

type (
	WaiterFunc   func(db *sql.DB) (bool, error)
	mysqlStorage struct {
		db *sql.DB
	}

	Querier interface {
		ExecContext(ctx context.Context, query string, args ...any) (sql.Result, error)
		QueryContext(ctx context.Context, query string, args ...any) (*sql.Rows, error)
	}
)

// TimeoutPingWaiter waits until the DB is available or fails on timeout
var TimeoutPingWaiter func(context.Context, time.Duration) WaiterFunc = func(parentCtx context.Context, timeout time.Duration) WaiterFunc {
	var err error
	ctx, cancel := context.WithTimeout(parentCtx, timeout)
	return func(db *sql.DB) (bool, error) {
		for {
			select {
			case <-ctx.Done():
				cancel()
				if err != nil {
					err = errors.New(ctx.Err().Error() + ": " + err.Error())
				}
				return false, err
			case <-time.After(1 * time.Second):
				if err = db.PingContext(ctx); err == nil {
					cancel()
					return false, nil
				}
				return true, err
			}
		}
	}
}

// NewMysqlStorage creates and initializes new MySQL storage instance
func NewMysqlStorage(config *mysql.Config) (*mysqlStorage, error) {
	db, err := sql.Open("mysql", config.FormatDSN())
	if err != nil {
		return nil, err
	}
	db.SetMaxOpenConns(30)
	db.SetMaxIdleConns(10)
	db.SetConnMaxLifetime(5 * time.Minute)

	return &mysqlStorage{
		db: db,
	}, nil
}

// SaveMessages implements Storage interface
//
// todo(maksym): test performance, since big chunks may create huge transaction logs
// and possible timeouts, deadlocks or disk usage increase
func (m *mysqlStorage) SaveMessages(ctx context.Context, messages []*Message) error {
	if len(messages) < 1 {
		return nil
	}

	// write temporary stats map keyed by group (service_name, severity) => number of occurrences
	statsMap := map[string]*LogStat{}
	stats := []*LogStat{}
	for _, m := range messages {
		key := m.ServiceName + m.Severity
		ls, ok := statsMap[key]
		if !ok {
			newStat := &LogStat{
				ServiceName: m.ServiceName,
				Severity:    m.Severity,
				Count:       1,
			}
			stats = append(stats, newStat)
			statsMap[key] = newStat
		} else {
			ls.Count += 1
		}
	}

	if err := m.WithTransaction(ctx, func(ctx context.Context, q Querier) error {
		if err := m.saveLogsRecords(ctx, messages, q); err != nil {
			return err
		}

		return m.saveSeverityRecords(ctx, stats, q)
	}); err != nil {
		return err
	}

	return nil
}

func (m *mysqlStorage) ListMessages(ctx context.Context) ([]*Message, error) {
	stmt := `
		select
			service_name, payload, severity, timestamp, created_at
		from
			service_logs
	`
	messages := []*Message{}
	if err := m.WithTransaction(ctx, func(ctx context.Context, q Querier) error {
		rows, err := q.QueryContext(ctx, stmt)
		if err != nil {
			return err
		}
		defer rows.Close()

		for rows.Next() {
			msg := &Message{}
			if err := rows.Scan(
				&msg.ServiceName,
				&msg.Payload,
				&msg.Severity,
				&msg.Timestamp,
				&msg.CreatedAt,
			); err != nil {
				return err
			}

			messages = append(messages, msg)
		}

		return rows.Err()
	}); err != nil {
		return nil, err
	}

	return messages, nil
}

func (m *mysqlStorage) LogStats(ctx context.Context) ([]*LogStat, error) {
	stmt := `
		select
			service_name, severity, sum(1)
		from
			service_logs
		group by
			service_name, severity
	`
	stats := []*LogStat{}
	if err := m.WithTransaction(ctx, func(ctx context.Context, q Querier) error {
		rows, err := m.db.QueryContext(ctx, stmt)
		if err != nil {
			return err
		}
		defer rows.Close()

		for rows.Next() {
			stat := &LogStat{}
			if err := rows.Scan(
				&stat.ServiceName,
				&stat.Severity,
				&stat.Count,
			); err != nil {
				return err
			}
			stats = append(stats, stat)
		}

		return rows.Err()
	}); err != nil {
		return nil, err
	}

	return stats, nil
}

func (m *mysqlStorage) SeverityStats(ctx context.Context) ([]*LogStat, error) {
	stmt := `
	select
		service_name, severity, sum(count)
	from
		service_severity
	group by
		service_name, severity
`
	stats := []*LogStat{}
	if err := m.WithTransaction(ctx, func(ctx context.Context, q Querier) error {
		rows, err := m.db.QueryContext(ctx, stmt)
		if err != nil {
			return err
		}
		defer rows.Close()

		for rows.Next() {
			stat := &LogStat{}
			if err := rows.Scan(
				&stat.ServiceName,
				&stat.Severity,
				&stat.Count,
			); err != nil {
				return err
			}
			stats = append(stats, stat)
		}

		return rows.Err()
	}); err != nil {
		return nil, err
	}

	return stats, nil
}

// WithTransaction implements Storage interface
func (m *mysqlStorage) WithTransaction(ctx context.Context, f func(context.Context, Querier) error) error {
	tx, err := m.db.BeginTx(ctx, &sql.TxOptions{Isolation: sql.LevelReadCommitted})
	if err != nil {
		return err
	}
	if err := f(ctx, tx); err != nil {
		if errTx := tx.Rollback(); err != nil {
			return fmt.Errorf("cannot rollback transaction: %v, original error: %v", errTx, err)
		}

		return err
	}

	if err := tx.Commit(); err != nil {
		return err
	}

	return nil
}

func (m *mysqlStorage) Close() error {
	return m.db.Close()
}

func (m *mysqlStorage) DB() *sql.DB {
	return m.db

}

func (m *mysqlStorage) Wait(f WaiterFunc) error {
	cont, err := f(m.db)
	for cont {
		cont, err = f(m.db)
	}

	return err
}

// NewMysqlConfig initializes new MySQL connection configuration with sane defaults
func NewMysqlConfig() *mysql.Config {
	mysqlConfig := mysql.NewConfig()
	mysqlConfig.AllowNativePasswords = true
	mysqlConfig.MultiStatements = true // if false, SQL with >1 statement (e.g. create table in migrations) will fail
	mysqlConfig.ParseTime = true

	return mysqlConfig
}

func (m *mysqlStorage) saveLogsRecords(ctx context.Context, messages []*Message, q Querier) error {
	values := strings.Repeat("(?, ?, ?, ?),", len(messages))
	values = values[:len(values)-1]
	args := []interface{}{}
	stmt := `
		INSERT INTO service_logs (service_name, payload, severity, timestamp)
		values ` + values
	for _, v := range messages {
		args = append(args, v.ServiceName, v.Payload, v.Severity, v.Timestamp)
	}

	_, err := q.ExecContext(ctx, stmt, args...)
	if err != nil {
		return err
	}

	return nil
}

func (m *mysqlStorage) saveSeverityRecords(ctx context.Context, stats []*LogStat, q Querier) error {
	values := strings.Repeat("(?, ?, ?),", len(stats))
	values = values[:len(values)-1]
	args := []interface{}{}
	stmt := `
		INSERT INTO service_severity (service_name, severity, ` + "`count`" + `)
		values ` + values
	for _, v := range stats {
		args = append(args, v.ServiceName, v.Severity, v.Count)
	}

	_, err := q.ExecContext(ctx, stmt, args...)
	if err != nil {
		return err
	}

	return nil
}
