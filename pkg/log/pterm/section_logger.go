package pterm

import (
	"fmt"
	"strings"

	"github.com/vmware-labs/distribution-tooling-for-helm/internal/widgets"
	"github.com/vmware-labs/distribution-tooling-for-helm/pkg/log"
)

const (
	// Number of spaces for each nesting level
	nestSpacing = 3
)

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
