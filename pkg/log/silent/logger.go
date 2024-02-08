package silent

import (
	"fmt"
	"io"

	"github.com/vmware-labs/distribution-tooling-for-helm/pkg/log"
)

// Logger defines a logger that does not log anything
type Logger struct {
}

// NewLogger returns a new Logger that does not log any message
func NewLogger() *Logger {
	return &Logger{}
}

// Infof logs nothing
func (l *Logger) Infof(string, ...interface{}) {}

// Errorf logs nothing
func (l *Logger) Errorf(string, ...interface{}) {}

// Debugf logs nothing
func (l *Logger) Debugf(string, ...interface{}) {}

// Warnf logs nothing
func (l *Logger) Warnf(string, ...interface{}) {}

// Printf logs nothing
func (l *Logger) Printf(string, ...interface{}) {}

// SetWriter does nothing
func (l *Logger) SetWriter(io.Writer) {}

// SetLevel does nothing
func (l *Logger) SetLevel(log.Level) {}

// Failf returns a LoggedError
func (l *Logger) Failf(format string, args ...interface{}) error {
	return &log.LoggedError{Err: fmt.Errorf(format, args...)}
}
