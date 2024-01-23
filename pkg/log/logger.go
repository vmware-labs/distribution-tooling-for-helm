// Package log defines the Logger interfaces
package log

import (
	"fmt"
	"io"
	"os"

	"github.com/pterm/pterm"
	"github.com/sirupsen/logrus"
	"github.com/vmware-labs/distribution-tooling-for-helm/internal/widgets"
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
	// Internal level to indicate we want to always log
	alwaysLevel = Level(0)
)

var (
	// SilentLog implement a Logger that does not print anything
	SilentLog = NewSilentLogger()
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

// NewPtermLogger returns a new Logger implemented by pterm
func NewPtermLogger() Logger {
	return newPtermLogger()
}

func newPtermLogger() *PtermLogger {
	return &PtermLogger{writer: os.Stdout, level: InfoLevel}
}

// PtermLogger defines a logger implemented using pterm
type PtermLogger struct {
	writer io.Writer
	level  Level
	prefix string
}

func (l *PtermLogger) printMessage(messageLevel Level, printer *pterm.PrefixPrinter, format string, args ...interface{}) {
	if messageLevel > l.level {
		return
	}
	pterm.Fprintln(l.writer, l.prefix+printer.Sprint(fmt.Sprintf(format, args...)))
}

// SetWriter sets the internal writer used by the log
func (l *PtermLogger) SetWriter(w io.Writer) {
	l.writer = w
}

// SetLevel sets the log level
func (l *PtermLogger) SetLevel(level Level) {
	l.level = level
}

// Failf logs a formatted error and returns it back
func (l *PtermLogger) Failf(format string, args ...interface{}) error {
	err := fmt.Errorf(format, args...)
	l.Errorf("%v", err)
	return &LoggedError{Err: err}
}

// Printf prints a message in the log
func (l *PtermLogger) Printf(format string, args ...interface{}) {
	l.printMessage(alwaysLevel, widgets.Plain, format, args...)
}

// Errorf logs an error message
func (l *PtermLogger) Errorf(format string, args ...interface{}) {
	l.printMessage(ErrorLevel, widgets.Error, format, args...)
}

// Infof logs an information message
func (l *PtermLogger) Infof(format string, args ...interface{}) {
	l.printMessage(InfoLevel, widgets.Info, format, args...)
}

// Debugf logs a debug message
func (l *PtermLogger) Debugf(format string, args ...interface{}) {
	l.printMessage(DebugLevel, widgets.Debug, format, args...)
}

// Warnf logs a warning message
func (l *PtermLogger) Warnf(format string, args ...interface{}) {
	l.printMessage(WarnLevel, widgets.Warning, format, args...)
}

// LogrusLogger defines a Logger implemented by logrus
type LogrusLogger struct {
	*logrus.Logger
}

// Failf logs a formatted error and returns it back
func (l *LogrusLogger) Failf(format string, args ...interface{}) error {
	err := fmt.Errorf(format, args...)
	l.Errorf("%v", err)
	return &LoggedError{Err: err}
}

// SetLevel sets the log level
func (l *LogrusLogger) SetLevel(level Level) {
	l.Logger.SetLevel(logrus.Level(level))
}

// SetWriter sets the internal writer used by the log
func (l *LogrusLogger) SetWriter(w io.Writer) {
	l.Logger.SetOutput(w)
}

// Printf prints a message in the log
func (l *LogrusLogger) Printf(format string, args ...interface{}) {
	l.Infof(format, args...)
}

// NewLogrusLogger returns a Logger implemented by logrus
func NewLogrusLogger() Logger {
	return newLogrusLogger()
}

func newLogrusLogger() *LogrusLogger {
	return &LogrusLogger{Logger: logrus.New()}
}

// NewSilentLogger returns a new Logger that does not log any message
func NewSilentLogger() Logger {
	l := newLogrusSectionLogger()
	l.SetOutput(io.Discard)
	return l
}
