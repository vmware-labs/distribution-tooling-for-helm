package pterm

import (
	"fmt"
	"io"
	"strings"

	"github.com/pterm/pterm"
	"github.com/vmware-labs/distribution-tooling-for-helm/internal/widgets"
	"github.com/vmware-labs/distribution-tooling-for-helm/pkg/log"
)

const (
	// Number of spaces for each nesting level
	nestSpacing = 3
)

// Logger defines a logger implemented using pterm
type Logger struct {
	writer io.Writer
	level  log.Level
	prefix string
}

func (l *Logger) printMessage(messageLevel log.Level, printer *pterm.PrefixPrinter, format string, args ...interface{}) {
	if messageLevel > l.level {
		return
	}
	pterm.Fprintln(l.writer, l.prefix+printer.Sprint(fmt.Sprintf(format, args...)))
}

// SetWriter sets the internal writer used by the log
func (l *Logger) SetWriter(w io.Writer) {
	l.writer = w
}

// SetLevel sets the log level
func (l *Logger) SetLevel(level log.Level) {
	l.level = level
}

// Failf logs a formatted error and returns it back
func (l *Logger) Failf(format string, args ...interface{}) error {
	err := fmt.Errorf(format, args...)
	l.Errorf("%v", err)
	return &log.LoggedError{Err: err}
}

// Printf prints a message in the log
func (l *Logger) Printf(format string, args ...interface{}) {
	l.printMessage(log.AlwaysLevel, Plain, format, args...)
}

// Errorf logs an error message
func (l *Logger) Errorf(format string, args ...interface{}) {
	l.printMessage(log.ErrorLevel, Error, format, args...)
}

// Infof logs an information message
func (l *Logger) Infof(format string, args ...interface{}) {
	l.printMessage(log.InfoLevel, Info, format, args...)
}

// Debugf logs a debug message
func (l *Logger) Debugf(format string, args ...interface{}) {
	l.printMessage(log.DebugLevel, Debug, format, args...)
}

// Warnf logs a warning message
func (l *Logger) Warnf(format string, args ...interface{}) {
	l.printMessage(log.WarnLevel, Warning, format, args...)
}

// NewSectionLogger returns a new SectionLogger implemented by pterm
func NewSectionLogger() *SectionLogger {
	return &SectionLogger{Logger: NewLogger()}
}

// SectionLogger defines a SectionLogger using pterm
type SectionLogger struct {
	*Logger
	nestLevel int
}

// ProgressBar returns a new ProgressBar
func (l *SectionLogger) ProgressBar() log.ProgressBar {
	return NewProgressBar(l.prefix)
}

// Successf logs a new success message (more efusive than Infof)
func (l *SectionLogger) Successf(format string, args ...interface{}) {
	l.printMessage(log.InfoLevel, Success, format, args...)
}

// PrefixText returns the indented version of the provided text
func (l *SectionLogger) PrefixText(txt string) string {
	// We include a leading " " as this is intended to align with our printers,
	// and all printers do  " " + printer.Text + " " + Text
	// so the extra space aligns the txt with the printer.Text char
	lines := make([]string, 0)
	for _, line := range strings.Split(txt, "\n") {
		lines = append(lines, fmt.Sprintf(" %s%s", l.prefix, line))
	}
	return strings.Join(lines, "\n")
}

// ExecuteStep executes a function while showing an indeterminate progress animation
func (l *SectionLogger) ExecuteStep(title string, fn func() error) error {
	err := widgets.ExecuteWithSpinner(
		widgets.DefaultSpinner.WithPrefix(l.prefix),
		title,
		func() error {
			return fn()
		},
	)
	return err
}

// Section executes the provided function inside a new section
func (l *SectionLogger) Section(title string, fn func(log.SectionLogger) error) error {
	childLog := l.StartSection(title)
	return fn(childLog)
}

// StartSection starts a new log section, with nested indentation
func (l *SectionLogger) StartSection(str string) log.SectionLogger {
	l.printMessage(log.AlwaysLevel, Fold, str)
	return l.nest()
}
func (l *SectionLogger) nest() log.SectionLogger {
	newLog := &SectionLogger{nestLevel: l.nestLevel + 1, Logger: NewLogger()}
	newLog.prefix = strings.Repeat(" ", newLog.nestLevel*nestSpacing)
	newLog.level = l.level
	return newLog
}
