package widgets

import (
	"github.com/pterm/pterm"
	"github.com/pterm/pterm/putils"
)

var (
	// DefaultSpinner defines the default spinner widget
	DefaultSpinner Spinner
)

func prefixSequence(prefix string, sequence ...string) []string {
	newSequence := make([]string, len(sequence))
	for i, str := range sequence {
		newSequence[i] = prefix + str
	}
	return newSequence
}

// Spinner defines a widget that shows a indeterminate progress animation
type Spinner struct {
	*pterm.SpinnerPrinter
}

// WithPrefix returns a new Spinner the with the specified prefix
func (s *Spinner) WithPrefix(prefix string) *Spinner {
	return &Spinner{s.WithSequence(prefixSequence(prefix, s.Sequence...)...)}
}

func init() {
	DefaultSpinner = Spinner{pterm.DefaultSpinner.WithSequence(prefixSequence(" ", "⠋", "⠙", "⠹", "⠸", "⠼", "⠴", "⠦", "⠧", "⠇", "⠏")...)}
}

// ExecuteWithSpinner runs the provided function while executing spinner
func ExecuteWithSpinner(spinner *Spinner, message string, fn func() error) error {
	return putils.RunWithSpinner(spinner.WithRemoveWhenDone(true).WithText(message), func(_ *pterm.SpinnerPrinter) error {
		return fn()
	})
}

// ExecuteWithDefaultSpinner runs the provided function while executing the default spinner
func ExecuteWithDefaultSpinner(message string, fn func() error) error {
	return ExecuteWithSpinner(&DefaultSpinner, message, fn)
}
