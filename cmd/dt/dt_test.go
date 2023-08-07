package main

import (
	"bytes"
	"flag"
	"os"
	"os/exec"
	"regexp"
	"syscall"
	"testing"

	tu "github.com/bitnami/gonit/testutils"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/suite"
)

func TestMain(m *testing.M) {
	if os.Getenv("BE_DT") == "1" {
		main()
		os.Exit(0)
		return
	}
	flag.Parse()
	c := m.Run()
	os.Exit(c)
}

type CmdSuite struct {
	suite.Suite
	sb *tu.Sandbox
}

func (suite *CmdSuite) TearDownSuite() {
	suite.sb.Cleanup()
}

func (suite *CmdSuite) AssertPanicsMatch(fn func(), re *regexp.Regexp) bool {
	return tu.AssertPanicsMatch(suite.T(), fn, re)
}

func (suite *CmdSuite) SetupSuite() {
	suite.sb = tu.NewSandbox()
}

// dt calls the dt command externally via exec
func dt(cmdArgs ...string) CmdResult {
	return execCommand(cmdArgs...)
}

func TestDtToolCommand(t *testing.T) {
	suite.Run(t, new(CmdSuite))
}

func execCommand(args ...string) CmdResult {
	var buffStdout, buffStderr bytes.Buffer
	code := 0

	cmd := exec.Command(os.Args[0], args...)
	cmd.Stdout = &buffStdout
	cmd.Stderr = &buffStderr

	cmd.Env = append(os.Environ(), "BE_DT=1")

	err := cmd.Run()

	if err != nil {
		code = err.(*exec.ExitError).Sys().(syscall.WaitStatus).ExitStatus()
	}

	return CmdResult{code: code, stdout: buffStdout.String(), stderr: buffStderr.String()}
}

type CmdResult struct {
	code   int
	stdout string
	stderr string
}

func (r CmdResult) AssertErrorMatch(t *testing.T, re interface{}) bool {
	if r.AssertError(t) {
		return assert.Regexp(t, re, r.stderr)
	}
	return true
}

func (r CmdResult) AssertSuccessMatch(t *testing.T, re interface{}) bool {
	if r.AssertSuccess(t) {
		return assert.Regexp(t, re, r.stdout)
	}
	return true
}
func (r CmdResult) AssertCode(t *testing.T, code int) bool {
	return assert.Equal(t, code, r.code, "Expected %d code but got %d", code, r.code)
}
func (r CmdResult) AssertSuccess(t *testing.T) bool {
	return assert.True(t, r.Success(), "Expected command to success but got code=%d stderr=%s", r.code, r.stderr)
}

func (r CmdResult) AssertError(t *testing.T) bool {
	return assert.False(t, r.Success(), "Expected command to fail")
}

func (r CmdResult) Success() bool {
	return r.code == 0
}
