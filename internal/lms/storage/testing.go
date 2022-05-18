package storage

import (
	"context"
	"os"
	"strings"
	"sync"
	"time"
)

var once sync.Once

func NewTestDatabase(ctx context.Context) *mysqlStorage {
	dbName := "lms-test"

	config := NewMysqlConfig()
	config.DBName = dbName
	config.User = "root"
	config.Passwd = "root"
	config.Net = "tcp"
	config.Addr = "127.0.0.1:13306"

	mysqlStorage, err := NewMysqlStorage(config)
	if err != nil {
		panic(err)
	}
	if err := mysqlStorage.Wait(TimeoutPingWaiter(ctx, 10*time.Second)); err != nil {
		panic(err)
	}

	once.Do(func() {
		_, err = mysqlStorage.DB().ExecContext(ctx, "CREATE DATABASE IF NOT EXISTS `"+dbName+"`")
		if err != nil {
			panic(err)
		}

		// fixme(maksym): an ugly solution to find working directory
		// needs to be refactored to something more robust
		cwd, _ := os.Getwd()
		cwd = cwd[:strings.LastIndex(cwd, "/fiskil-lms")+len("/fiskil-lms")]
		if err := Migrate("file://"+cwd+"/migrations", mysqlStorage.DB()); err != nil {
			panic(err)
		}
		mysqlStorage, err = NewMysqlStorage(config)
		if err != nil {
			panic(err)
		}
	})

	if _, err := mysqlStorage.DB().ExecContext(ctx, `delete from service_logs; delete from service_severity;`); err != nil {
		panic(err)
	}

	return mysqlStorage
}
