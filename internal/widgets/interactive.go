package widgets

import (
	"github.com/pterm/pterm"
)

// ShowYesNoQuestion shows the yes/no question message provided
func ShowYesNoQuestion(question string) bool {
	result, _ := pterm.DefaultInteractiveConfirm.Show(question)
	return result
}
