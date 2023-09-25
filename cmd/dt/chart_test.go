package main

import (
	"fmt"
	"testing"
)

func (suite *CmdSuite) TestChartsHelp() {
	t := suite.T()
	t.Run("Shows Help", func(t *testing.T) {
		res := dt("charts")
		res.AssertSuccess(t)
		for _, reStr := range []string{
			`annotate\s+Annotates a Helm chart`,
			`carvelize\s+Adds a Carvel bundle to the Helm chart`,
			`relocate\s+Relocates a Helm chart`,
		} {
			res.AssertSuccessMatch(t, fmt.Sprintf(`(?s).*Available Commands:.*\n\s*%s.*`, reStr))
		}
	})
}
