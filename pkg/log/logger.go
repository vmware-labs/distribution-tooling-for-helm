// Package log defines the Logger interfaces
package log

import (
	"io"

	"github.com/sirupsen/logrus"
)

// Level defines a type for log levels
type Level logrus.Level

const (
	// PanicLevel level, highest level of severity. Logs and then calls panic with the
	// message passed to Debug, Info, ...
	PanicLevel = Level(logrus.PanicLevel)
	// FatalLevel level. Logs and then calls `logger.Exit(1)`. It will exit even if the
	// logging level is set to Panic.
	FatalLevel = Level(logrus.FatalLevel)
	// ErrorLevel level. Logs. Used for errors that should definitely be noted.
	// Commonly used for hooks to send errors to an error tracking service.
	ErrorLevel = Level(logrus.ErrorLevel)
	// WarnLevel level. Non-critical entries that deserve eyes.
	WarnLevel = Level(logrus.WarnLevel)
	// InfoLevel level. General operational entries about what's going on inside the
	// application.
	InfoLevel = Level(logrus.InfoLevel)
	// DebugLevel level. Usually only enabled when debugging. Very verbose logging.
	DebugLevel = Level(logrus.DebugLevel)
	// TraceLevel level. Designates finer-grained informational events than the Debug.
	TraceLevel = Level(logrus.TraceLevel)
)

const (
	// AlwaysLevel is a level to indicate we want to always log
	AlwaysLevel = Level(0)
)

// LoggedError indicates an error that has been already logged
type LoggedError struct {
	Err error
}

// Error returns the wrapped error
func (e *LoggedError) Error() string { return e.Err.Error() }

// Unwrap returns the wrapped error
func (e *LoggedError) Unwrap() error { return e.Err }

// ParseLevel returns a Level from its string representation
func ParseLevel(level string) (Level, error) {
	l, err := logrus.ParseLevel(level)
	return Level(l), err
}

// Logger defines a common interface for loggers
type Logger interface {
	Infof(format string, args ...interface{})
	Errorf(format string, args ...interface{})
	Debugf(format string, args ...interface{})
	Warnf(format string, args ...interface{})
	Printf(format string, args ...interface{})
	SetWriter(w io.Writer)
	SetLevel(level Level)
	Failf(format string, args ...interface{}) error
}
