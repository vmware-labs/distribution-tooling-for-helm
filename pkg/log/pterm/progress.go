package pterm

import (
	"fmt"

	"github.com/pterm/pterm"
	"github.com/vmware-labs/distribution-tooling-for-helm/pkg/log"
	"github.com/vmware-labs/distribution-tooling-for-helm/pkg/utils"
)

// ProgressBar defines a progress bar with fancy cui effects
type ProgressBar struct {
	*pterm.ProgressbarPrinter
	padding string
}

func (p *ProgressBar) printMessage(printer *pterm.PrefixPrinter, format string, args ...interface{}) {
	pterm.Fprintln(printer.Writer, p.padding+printer.Sprint(fmt.Sprintf(format, args...)))
}

// Stop stops the progress bar
func (p *ProgressBar) Stop() {
	_, _ = p.ProgressbarPrinter.Stop()
}

// Start initiates the progress bar
func (p *ProgressBar) Start(title ...interface{}) (log.ProgressBar, error) {
	res, err := p.ProgressbarPrinter.Start(title...)
	if err != nil {
		return p, fmt.Errorf("failed to start progress bar: %w", err)
	}
	p.ProgressbarPrinter = res
	return p, nil
}

// WithTotal sets the progress bar total steps
func (p *ProgressBar) WithTotal(n int) log.ProgressBar {
	p.ProgressbarPrinter = p.ProgressbarPrinter.WithTotal(n)
	return p
}

// Errorf shows an error message
func (p *ProgressBar) Errorf(fmt string, args ...interface{}) {
	p.printMessage(Error, fmt, args...)
}

// Infof shows an info message
func (p *ProgressBar) Infof(fmt string, args ...interface{}) {
	p.printMessage(Info, fmt, args...)
}

// Successf displays a success message
func (p *ProgressBar) Successf(fmt string, args ...interface{}) {
	p.printMessage(Success, fmt, args...)
}

// Warnf displays a warning message
func (p *ProgressBar) Warnf(fmt string, args ...interface{}) {
	p.printMessage(&pterm.Warning, fmt, args...)
}

func (p *ProgressBar) formatTitle(title string) string {
	// We prefix with a leading " " so we align with other printers, that
	// start with a leading space
	paddedTitle := " " + p.padding + title
	maxTitleLength := int(float32(p.ProgressbarPrinter.MaxWidth) * 0.70)
	truncatedTitle := utils.TruncateStringWithEllipsis(paddedTitle, maxTitleLength)
	return fmt.Sprintf("%-*s", maxTitleLength, truncatedTitle)
}

// UpdateTitle updates the progress bar title
func (p *ProgressBar) UpdateTitle(title string) log.ProgressBar {
	p.ProgressbarPrinter.UpdateTitle(p.formatTitle(title))
	return p
}

// Add increments the progress bar the specified amount
func (p *ProgressBar) Add(inc int) log.ProgressBar {
	p.ProgressbarPrinter.Add(inc)
	return p
}

// NewProgressBar returns a new NewProgressBar
func NewProgressBar(padding string) *ProgressBar {
	p := pterm.DefaultProgressbar.WithMaxWidth(pterm.GetTerminalWidth()).WithRemoveWhenDone(true)

	return &ProgressBar{
		ProgressbarPrinter: p,
		padding:            padding,
	}
}
