package storage

import (
	"context"
	"database/sql"
	"fmt"
	"strings"
	"time"

	"github.com/go-sql-driver/mysql"
	_ "github.com/go-sql-driver/mysql"
)

type (
	mysqlStorage struct {
		db *sql.DB
	}

	Querier interface {
		ExecContext(ctx context.Context, query string, args ...any) (sql.Result, error)
	}
)

// SaveMessages implements Storage interface
//
// todo(maksym): test performance, since big chunks may create huge transaction logs
// and possible timeouts, deadlocks or disk usage increase
func (m *mysqlStorage) SaveMessages(ctx context.Context, messages []*Message) error {
	if len(messages) < 1 {
		return nil
	}

	values := strings.Repeat("(?, ?, ?),", len(messages))
	values = values[:len(values)-1]
	args := []interface{}{}
	stmt := `INSERT INTO service_logs (service_name, payload, severity)
	values ` + values
	for _, v := range messages {
		args = append(args, v.ServiceName, v.Payload, v.Severity)
	}

	if err := m.WithTransaction(ctx, func(ctx context.Context, q Querier) error {
		_, err := q.ExecContext(ctx, stmt, args...)
		if err != nil {
			return err
		}
		return nil
	}); err != nil {
		return err
	}

	return nil
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

func (m *mysqlStorage) Wait(f func(db *sql.DB) (bool, error)) error {
	cont, err := f(m.db)
	for cont {
		cont, err = f(m.db)
	}

	return err
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
