package main

import (
	"fmt"
	"os"
	"strings"

	"github.com/spf13/cobra"
)

// Version is the tool version
var Version = "0.4.9"

// BuildDate is the tool build date
var BuildDate = ""

// Commit is the commit sha of the code used to build the tool
var Commit = ""

var versionCmd = &cobra.Command{
	Use:   "version",
	Short: "Prints the version",
	Run: func(_ *cobra.Command, _ []string) {
		msg := fmt.Sprintf("Distribution Tooling for Helm %s\n", Version)
		if BuildDate != "" {
			msg += fmt.Sprintf("Built on: %s\n", BuildDate)
		}
		if Commit != "" {
			msg += fmt.Sprintf("Git Commit: %s\n", Commit)
		}
		fmt.Print(msg)
		os.Exit(0)
	},
}

func init() {
	Version = strings.TrimSpace(Version)
	BuildDate = strings.TrimSpace(BuildDate)
	Commit = strings.TrimSpace(Commit)
}
