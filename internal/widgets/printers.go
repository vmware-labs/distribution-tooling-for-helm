package widgets

import "github.com/pterm/pterm"

var (
	// Fold defines a printer that prefixes text by a 'fold' symbol
	Fold = pterm.Info.WithMessageStyle(&pterm.ThemeDefault.InfoMessageStyle).WithPrefix(pterm.Prefix{
		Style: pterm.NewStyle(pterm.FgBlue),
		Text:  "\u00BB", // "»"
	})
	// Plain defines a printer with empty prefix
	Plain = pterm.Info.WithMessageStyle(&pterm.ThemeDefault.InfoMessageStyle).WithPrefix(pterm.Prefix{
		Style: pterm.NewStyle(pterm.FgBlue),
		Text:  " ",
	})
	// Error defines a printer for errors
	Error = pterm.Error.WithMessageStyle(&pterm.ThemeDefault.ErrorMessageStyle).WithPrefix(pterm.Prefix{
		Style: pterm.NewStyle(pterm.FgRed),
		Text:  "\u2718", // "✘"
	})
	// Warning defines a printer for warnings
	Warning = pterm.Warning.WithMessageStyle(&pterm.ThemeDefault.WarningMessageStyle).WithPrefix(pterm.Prefix{
		Style: pterm.NewStyle(pterm.FgYellow),
		Text:  "\u26A0\uFE0F",
	})
	// Debug defines a printer for debug messages
	Debug = pterm.Debug.WithMessageStyle(&pterm.ThemeDefault.DebugMessageStyle).WithDebugger(false).WithPrefix(pterm.Prefix{
		Style: pterm.NewStyle(pterm.FgGray),
		Text:  "\U0001F50D",
	})
	// Info defines a printer for info messages
	Info = pterm.Success.WithMessageStyle(&pterm.ThemeDefault.SuccessMessageStyle).WithPrefix(pterm.Prefix{
		Style: pterm.NewStyle(pterm.FgLightGreen),
		Text:  "\u2714", // "✔"
	})
	// Success defines a printer for success messages
	Success = pterm.Success.WithMessageStyle(&pterm.ThemeDefault.SuccessMessageStyle).WithPrefix(pterm.Prefix{
		Style: pterm.NewStyle(pterm.FgLightGreen),
		// This is a rocket
		// Text:  "\U0001F680",
		Text: "\U0001F389",
	})
)
