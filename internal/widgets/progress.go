// Package widgets groups a set of cui widgets
package widgets

import (
	"fmt"
	"io"

	"github.com/pterm/pterm"
	"github.com/sirupsen/logrus"
	"github.com/vmware-labs/distribution-tooling-for-helm/utils"
)

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

// PrettyProgressBar defines a progress bar with fancy cui effects
type PrettyProgressBar struct {
	*pterm.ProgressbarPrinter
	padding string
}

func (p *PrettyProgressBar) printMessage(printer *pterm.PrefixPrinter, format string, args ...interface{}) {
	pterm.Fprintln(printer.Writer, p.padding+printer.Sprint(fmt.Sprintf(format, args...)))
}

// Stop stops the progress bar
func (p *PrettyProgressBar) Stop() {
	_, _ = p.ProgressbarPrinter.Stop()
}

// Start initiates the progress bar
func (p *PrettyProgressBar) Start(title ...interface{}) (ProgressBar, error) {
	res, err := p.ProgressbarPrinter.Start(title...)
	if err != nil {
		return p, fmt.Errorf("failed to start progress bar: %w", err)
	}
	p.ProgressbarPrinter = res
	return p, nil
}

// WithTotal sets the progress bar total steps
func (p *PrettyProgressBar) WithTotal(n int) ProgressBar {
	p.ProgressbarPrinter = p.ProgressbarPrinter.WithTotal(n)
	return p
}

// Errorf shows an error message
func (p *PrettyProgressBar) Errorf(fmt string, args ...interface{}) {
	//p.printMessage(&pterm.Error, padding+fmt, args...)
	p.printMessage(Error, fmt, args...)
}

// Infof shows an info message
func (p *PrettyProgressBar) Infof(fmt string, args ...interface{}) {
	// p.printMessage(pterm.Error, fmt, args...)
	p.printMessage(Info, fmt, args...)
}

// Successf displays a success message
func (p *PrettyProgressBar) Successf(fmt string, args ...interface{}) {
	// p.printMessage(pterm.Success, fmt, args...)
	p.printMessage(Success, fmt, args...)
}

// Warnf displays a warning message
func (p *PrettyProgressBar) Warnf(fmt string, args ...interface{}) {
	p.printMessage(&pterm.Warning, fmt, args...)
}

func (p *PrettyProgressBar) formatTitle(title string) string {
	// We prefix with a leading " " so we align with other printers, that
	// start with a leading space
	paddedTitle := " " + p.padding + title
	maxTitleLength := int(float32(p.ProgressbarPrinter.MaxWidth) * 0.70)
	truncatedTitle := utils.TruncateStringWithEllipsis(paddedTitle, maxTitleLength)
	return fmt.Sprintf("%-*s", maxTitleLength, truncatedTitle)
}

// UpdateTitle updates the progress bar title
func (p *PrettyProgressBar) UpdateTitle(title string) ProgressBar {
	p.ProgressbarPrinter.UpdateTitle(p.formatTitle(title))
	return p
}

// Add increments the progress bar the specified amount
func (p *PrettyProgressBar) Add(inc int) ProgressBar {
	p.ProgressbarPrinter.Add(inc)
	return p
}

// NewPrettyProgressBar returns a new NewPrettyProgressBar
func NewPrettyProgressBar(padding string) ProgressBar {
	p := pterm.DefaultProgressbar.WithMaxWidth(pterm.GetTerminalWidth()).WithRemoveWhenDone(true)

	return &PrettyProgressBar{
		ProgressbarPrinter: p,
		padding:            padding,
	}
}

// LogProgressBar defines a widget that supports the ProgressBar interface but just logs messages
type LogProgressBar struct {
	*logrus.Logger
	totalSteps   int
	currentSteps int
}

// NewLogProgressBar returns a progress bar that just log messages
func NewLogProgressBar(l *logrus.Logger) ProgressBar {
	return newLogProgressBar(l)
}

func newLogProgressBar(l *logrus.Logger) *LogProgressBar {
	return &LogProgressBar{Logger: l}
}

// NewSilentProgressBar returns a new ProgressBar that does not produce any output
func NewSilentProgressBar() ProgressBar {
	pb := newLogProgressBar(logrus.New())
	pb.SetOutput(io.Discard)
	return pb
}

// Stop stops the progress bar
func (p *LogProgressBar) Stop() {
}

// Start initiates the progress bar
func (p *LogProgressBar) Start(...interface{}) (ProgressBar, error) {
	return p, nil
}

// WithTotal sets the progress bar total steps
func (p *LogProgressBar) WithTotal(steps int) ProgressBar {
	p.totalSteps = steps
	return p
}

// Error shows an error message
func (p *LogProgressBar) Error(fmt string, args ...interface{}) {
	p.Errorf(fmt, args...)
}

// Info shows an info message
func (p *LogProgressBar) Info(fmt string, args ...interface{}) {
	p.Infof(fmt, args...)
}

// Successf displays a success message
func (p *LogProgressBar) Successf(fmt string, args ...interface{}) {
	p.Infof(fmt, args...)
}

// Warning displays a warning message
func (p *LogProgressBar) Warning(fmt string, args ...interface{}) {
	p.Warnf(fmt, args...)
}

// UpdateTitle updates the progress bar title
func (p *LogProgressBar) UpdateTitle(str string) ProgressBar {
	p.Infof("[ %3d/%3d ] %s", p.currentSteps, p.totalSteps, str)
	return p
}

// Add increments the progress bar the specified amount
func (p *LogProgressBar) Add(steps int) ProgressBar {
	newSteps := p.currentSteps + steps
	if newSteps > p.totalSteps {
		newSteps = p.totalSteps
	}
	p.currentSteps = newSteps
	return p
}
