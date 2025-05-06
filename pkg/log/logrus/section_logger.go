package logrus

import "github.com/vmware-labs/distribution-tooling-for-helm/pkg/log"

// SectionLogger defines a SectionLogger implemented by logrus
type SectionLogger struct {
	*Logger
}

// ExecuteStep executes a function while showing an indeterminate progress animation
func (l *SectionLogger) ExecuteStep(title string, fn func() error) error {
	l.Info(title)
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
	return log.NewLoggedProgressBar(l.Logger)
}

// Successf logs a new success message (more efusive than Infof)
func (l *SectionLogger) Successf(format string, args ...interface{}) {
	l.Infof(format, args...)
}

// Section executes the provided function inside a new section
func (l *SectionLogger) Section(title string, fn func(log.SectionLogger) error) error {
	l.Info(title)
	return fn(l)
}

// NewSectionLogger returns a new SectionLogger implemented by logrus
func NewSectionLogger() *SectionLogger {
	return &SectionLogger{NewLogger()}
}
