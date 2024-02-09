// Package logrus provides a logger implementation using the logrus library
package logrus

import (
	"fmt"
	"io"

	"github.com/sirupsen/logrus"
	"github.com/vmware-labs/distribution-tooling-for-helm/pkg/log"
)

// Logger defines a Logger implemented by logrus
type Logger struct {
	*logrus.Logger
}

// Failf logs a formatted error and returns it back
func (l *Logger) Failf(format string, args ...interface{}) error {
	err := fmt.Errorf(format, args...)
	l.Errorf("%v", err)
	return &log.LoggedError{Err: err}
}

// SetLevel sets the log level
func (l *Logger) SetLevel(level log.Level) {
	l.Logger.SetLevel(logrus.Level(level))
}

// SetWriter sets the internal writer used by the log
func (l *Logger) SetWriter(w io.Writer) {
	l.Logger.SetOutput(w)
}

// Printf prints a message in the log
func (l *Logger) Printf(format string, args ...interface{}) {
	l.Infof(format, args...)
}

// NewLogger returns a Logger implemented by logrus
func NewLogger() *Logger {
	return &Logger{Logger: logrus.New()}
}
