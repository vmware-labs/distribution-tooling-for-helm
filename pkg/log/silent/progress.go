package silent

import "github.com/vmware-labs/distribution-tooling-for-helm/pkg/log"

// ProgressBar defines a widget that supports the ProgressBar interface and does nothing
type ProgressBar struct {
}

// NewProgressBar returns a new ProgressBar that does not produce any output
func NewProgressBar() *ProgressBar {
	return &ProgressBar{}
}

// Stop stops the progress bar
func (p *ProgressBar) Stop() {
}

// Start initiates the progress bar
func (p *ProgressBar) Start(...interface{}) (log.ProgressBar, error) {
	return p, nil
}

// WithTotal sets the progress bar total steps
func (p *ProgressBar) WithTotal(int) log.ProgressBar {
	return p
}

// Errorf shows an error message
func (p *ProgressBar) Errorf(string, ...interface{}) {

}

// Infof shows an info message
func (p *ProgressBar) Infof(string, ...interface{}) {
}

// Successf displays a success message
func (p *ProgressBar) Successf(string, ...interface{}) {
}

// Warnf displays a warning message
func (p *ProgressBar) Warnf(string, ...interface{}) {
}

// UpdateTitle updates the progress bar title
func (p *ProgressBar) UpdateTitle(string) log.ProgressBar {
	return p
}

// Add increments the progress bar the specified amount
func (p *ProgressBar) Add(int) log.ProgressBar {
	return p
}
