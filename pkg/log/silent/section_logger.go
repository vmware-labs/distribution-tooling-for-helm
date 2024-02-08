// Package silent implements a silent logger
package silent

import (
	"github.com/vmware-labs/distribution-tooling-for-helm/pkg/log"
)

// SectionLogger is a SectionLogger that does not output anything
type SectionLogger struct {
	*Logger
}

// NewSectionLogger creates a new SilentSectionLogger
func NewSectionLogger() *SectionLogger {
	return &SectionLogger{&Logger{}}
}

// ExecuteStep executes a function while showing an indeterminate progress animation
func (l *SectionLogger) ExecuteStep(_ string, fn func() error) error {
	return fn()
}

// PrefixText returns the indented version of the provided text
func (l *SectionLogger) PrefixText(txt string) string {
	return txt
}

// StartSection starts a new log section
func (l *SectionLogger) StartSection(string) log.SectionLogger {
	return l
}

// ProgressBar returns a new silent progress bar
func (l *SectionLogger) ProgressBar() log.ProgressBar {
	return NewProgressBar()
}

// Successf logs a new success message (more efusive than Infof)
func (l *SectionLogger) Successf(string, ...interface{}) {
}

// Section executes the provided function inside a new section
func (l *SectionLogger) Section(_ string, fn func(log.SectionLogger) error) error {
	return fn(l)
}
