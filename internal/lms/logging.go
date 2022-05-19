package lms

import (
	"io"
	"log"
)

type instanceLogger struct {
	stdLogger *log.Logger
	instance  string
	out       io.Writer
}

// Info implements Info method of Logger interface
func (il *instanceLogger) Info(msg string, args ...interface{}) {
	il.stdLogger.Printf(msg, args...)
}

// Error implements Info method of Logger interface
func (il *instanceLogger) Error(msg string, args ...interface{}) {
	il.stdLogger.Printf("ERROR: "+msg, args...)
}

// SubLogger implements interface
func (il *instanceLogger) SubLogger(name string) Logger {
	return NewInstanceLogger(il.out, il.instance+":"+name)
}

// NewInstanceLogger create new logger with a given instance name
func NewInstanceLogger(out io.Writer, instance string) *instanceLogger {
	return &instanceLogger{
		instance:  instance,
		out:       out,
		stdLogger: log.New(out, "["+instance+"] ", log.Flags()|log.Lmsgprefix),
	}

}
