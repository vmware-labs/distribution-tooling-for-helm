package main

import "fmt"

func (suite *CmdSuite) TestVersionCommand() {
	dt("version").AssertSuccessMatch(suite.T(), fmt.Sprintf("^Distribution Tooling for Helm %s", Version))
}
