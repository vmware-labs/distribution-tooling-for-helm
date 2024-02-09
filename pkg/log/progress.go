package log

// LoggedProgressBar defines a widget that supports the ProgressBar interface but just logs messages
type LoggedProgressBar struct {
	Logger
	totalSteps   int
	currentSteps int
}

// NewLoggedProgressBar returns a progress bar that just log messages
func NewLoggedProgressBar(l Logger) *LoggedProgressBar {
	return &LoggedProgressBar{Logger: l}
}

// Stop stops the progress bar
func (p *LoggedProgressBar) Stop() {
}

// Start initiates the progress bar
func (p *LoggedProgressBar) Start(...interface{}) (ProgressBar, error) {
	return p, nil
}

// WithTotal sets the progress bar total steps
func (p *LoggedProgressBar) WithTotal(steps int) ProgressBar {
	p.totalSteps = steps
	return p
}

// Error shows an error message
func (p *LoggedProgressBar) Error(fmt string, args ...interface{}) {
	p.Errorf(fmt, args...)
}

// Info shows an info message
func (p *LoggedProgressBar) Info(fmt string, args ...interface{}) {
	p.Infof(fmt, args...)
}

// Successf displays a success message
func (p *LoggedProgressBar) Successf(fmt string, args ...interface{}) {
	p.Infof(fmt, args...)
}

// Warning displays a warning message
func (p *LoggedProgressBar) Warning(fmt string, args ...interface{}) {
	p.Warnf(fmt, args...)
}

// UpdateTitle updates the progress bar title
func (p *LoggedProgressBar) UpdateTitle(str string) ProgressBar {
	p.Infof("[ %3d/%3d ] %s", p.currentSteps, p.totalSteps, str)
	return p
}

// Add increments the progress bar the specified amount
func (p *LoggedProgressBar) Add(steps int) ProgressBar {
	newSteps := p.currentSteps + steps
	if newSteps > p.totalSteps {
		newSteps = p.totalSteps
	}
	p.currentSteps = newSteps
	return p
}
