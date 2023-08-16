package main

import (
	"fmt"
	"testing"
)

func (suite *CmdSuite) TestImagesHelp() {
	t := suite.T()
	t.Run("Shows Help", func(t *testing.T) {
		res := dt("images")
		res.AssertSuccess(t)
		for _, reStr := range []string{
			`lock\s+Creates the lock file`,
			`pull\s+Pulls the images from the Images\.lock`,
			`push\s+Pushes the images from Images\.lock`,
			`verify\s+Verifies the images in an Images\.lock`,
		} {
			res.AssertSuccessMatch(t, fmt.Sprintf(`(?s).*Available Commands:.*\n\s*%s.*`, reStr))
		}
	})
}
