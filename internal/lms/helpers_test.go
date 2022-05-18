package lms

import (
	"context"
	"testing"

	"github.com/maxim-nazarenko/fiskil-lms/internal/lms/storage"
)

type testingLogger struct {
	t *testing.T
}

func (tl *testingLogger) Info(msg string, args ...interface{}) {
	tl.t.Logf("INFO: "+msg, args...)
}
func (tl *testingLogger) Error(msg string, args ...interface{}) {
	tl.t.Logf("ERROR: "+msg, args...)
}

func newTestLogger(t *testing.T) *testingLogger {
	return &testingLogger{t: t}
}

func TestMysql(t *testing.T) {
	db := storage.NewTestDatabase(context.Background())
	db.Close()
}
