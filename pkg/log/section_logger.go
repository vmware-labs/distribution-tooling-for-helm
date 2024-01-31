package log

import (
	"fmt"
	"strings"

	"github.com/vmware-labs/distribution-tooling-for-helm/internal/widgets"
)

const (
	// Number of spaces for each nesting level
	nestSpacing = 3
)

// SectionLogger defines an interface for loggers supporting nested levels of loggin
type SectionLogger interface {
	Logger
	Successf(format string, args ...interface{})
	PrefixText(string) string
	StartSection(title string) SectionLogger
	//	Nest(title string) SectionLogger
	//	NestLevel() int
	Section(title string, fn func(SectionLogger) error) error
	ExecuteStep(title string, fn func() error) error
	ProgressBar() widgets.ProgressBar
}

// NewPtermSectionLogger returns a new SectionLogger implemented by pterm
func NewPtermSectionLogger() SectionLogger {
	return newPtermSectionLogger()
}

func newPtermSectionLogger() *PtermSectionLogger {
	return &PtermSectionLogger{PtermLogger: newPtermLogger()}
}

// PtermSectionLogger defines a SectionLogger using pterm
type PtermSectionLogger struct {
	*PtermLogger
	nestLevel int
}

// ProgressBar returns a new ProgressBar
func (l *PtermSectionLogger) ProgressBar() widgets.ProgressBar {
	return widgets.NewPrettyProgressBar(l.prefix)
}

// Successf logs a new success message (more efusive than Infof)
func (l *PtermSectionLogger) Successf(format string, args ...interface{}) {
	l.printMessage(InfoLevel, widgets.Success, format, args...)
}

// PrefixText returns the indented version of the provided text
func (l *PtermSectionLogger) PrefixText(txt string) string {
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
func (l *PtermSectionLogger) ExecuteStep(title string, fn func() error) error {
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
func (l *PtermSectionLogger) Section(title string, fn func(SectionLogger) error) error {
	childLog := l.StartSection(title)
	return fn(childLog)
}

// StartSection starts a new log section, with nested indentation
func (l *PtermSectionLogger) StartSection(str string) SectionLogger {
	l.printMessage(alwaysLevel, widgets.Fold, str)
	return l.nest()
}
func (l *PtermSectionLogger) nest() SectionLogger {
	newLog := &PtermSectionLogger{nestLevel: l.nestLevel + 1, PtermLogger: newPtermLogger()}
	newLog.prefix = strings.Repeat(" ", newLog.nestLevel*nestSpacing)
	newLog.level = l.level
	return newLog
}

// LogrusSectionLogger defines a SectionLogger implemented by logrus
type LogrusSectionLogger struct {
	*LogrusLogger
}

// ExecuteStep executes a function while showing an indeterminate progress animation
func (l *LogrusSectionLogger) ExecuteStep(title string, fn func() error) error {
	l.Info(title)
	return fn()
}

// PrefixText returns the indented version of the provided text
func (l *LogrusSectionLogger) PrefixText(txt string) string {
	return txt
}

// StartSection starts a new log section
func (l *LogrusSectionLogger) StartSection(string) SectionLogger {
	return l
}

// ProgressBar returns a new silent progress bar
func (l *LogrusSectionLogger) ProgressBar() widgets.ProgressBar {
	return widgets.NewLogProgressBar(l.Logger)
}

// Successf logs a new success message (more efusive than Infof)
func (l *LogrusSectionLogger) Successf(format string, args ...interface{}) {
	l.Infof(format, args...)
}

// Section executes the provided function inside a new section
func (l *LogrusSectionLogger) Section(title string, fn func(SectionLogger) error) error {
	l.Infof(title)
	return fn(l)
}

// NewLogrusSectionLogger returns a new SectionLogger implemented by logrus
func NewLogrusSectionLogger() SectionLogger {
	return newLogrusSectionLogger()
}

func newLogrusSectionLogger() *LogrusSectionLogger {
	return &LogrusSectionLogger{newLogrusLogger()}
}
