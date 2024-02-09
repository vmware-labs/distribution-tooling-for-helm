package log

// ProgressBar defines a ProgressBar widget
type ProgressBar interface {
	WithTotal(total int) ProgressBar
	UpdateTitle(title string) ProgressBar
	Add(increment int) ProgressBar
	Start(title ...interface{}) (ProgressBar, error)
	Stop()
	Successf(fmt string, args ...interface{})
	Errorf(fmt string, args ...interface{})
	Infof(fmt string, args ...interface{})
	Warnf(fmt string, args ...interface{})
}

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
	ProgressBar() ProgressBar
}
