package storage

import (
	"context"
	"math/rand"
	"os"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/go-sql-driver/mysql"
	"github.com/maxim-nazarenko/fiskil-lms/internal/lms/app"
	"github.com/maxim-nazarenko/fiskil-lms/internal/lms/utils"
)

var (
	once        sync.Once
	mysqlConfig *mysql.Config
	appConfig   *app.Configuration
)

func NewTestDatabase(ctx context.Context, t *testing.T) *mysqlStorage {

	once.Do(func() {
		var err error
		appConfig, err = app.BuildConfiguration([]string{}, os.Getenv)
		if err != nil {
			t.Fatal(err)
		}
		mysqlConfig = NewMysqlConfig()
		mysqlConfig.User = appConfig.DB.User
		mysqlConfig.Passwd = appConfig.DB.Password
		mysqlConfig.DBName = appConfig.DB.Name
		mysqlConfig.Net = "tcp"
		mysqlConfig.Addr = appConfig.DB.Address

	})

	dbName := "lms-test-" + tempDBName(10)

	mysqlStorage, err := NewMysqlStorage(mysqlConfig)
	if err != nil {
		t.Fatal(err)
	}
	if err := mysqlStorage.Wait(TimeoutPingWaiter(ctx, 10*time.Second)); err != nil {
		t.Fatal(err)
	}

	_, err = mysqlStorage.DB().ExecContext(ctx, "CREATE DATABASE IF NOT EXISTS `"+dbName+"`")
	if err != nil {
		t.Fatal(err)
	}

	mysqlConfig.DBName = dbName
	mysqlStorage, err = NewMysqlStorage(mysqlConfig)
	if err != nil {
		t.Fatal(err)
	}

	projectRoot := utils.ProjectRootDir()
	if err := Migrate("file://"+projectRoot+"/migrations", mysqlStorage.DB()); err != nil {
		t.Fatal(err)
	}

	// clean schema
	if _, err := mysqlStorage.DB().ExecContext(ctx, `delete from service_logs; delete from service_severity;`); err != nil {
		t.Fatal(err)
	}

	return mysqlStorage
}

func tempDBName(n int) string {
	builder := strings.Builder{}
	rand.Seed(time.Now().UnixMicro())
	for i := 0; i < n; i++ {
		builder.WriteByte(byte(rand.Intn(26) + 'a'))
	}

	return builder.String()
}
