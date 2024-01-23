// Package main implements the dt tool
package main

import (
	"fmt"
	"os"

	"github.com/vmware-labs/distribution-tooling-for-helm/cmd/dt/config"
)

func main() {
	// Make sure we clean up after ourselves
	defer config.CleanGlobalTempWorkDir()

	if err := rootCmd.Execute(); err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
}
