// Package main implements the dt tool
package main

import (
	"fmt"
	"os"
	"sync"
)

var (
	// this variable is modified externally through the --keep-artifacts global flag
	keepArtifacts bool

	// global temporary directory used to store different assets
	globalTempWorkDir      string
	globalTempWorkDirMutex = &sync.RWMutex{}
)

func cleanGlobalTempWorkDir() error {
	globalTempWorkDirMutex.Lock()
	defer globalTempWorkDirMutex.Unlock()

	if globalTempWorkDir == "" || keepArtifacts {
		return nil
	}
	if err := os.RemoveAll(globalTempWorkDir); err != nil {
		return fmt.Errorf("failed to remove temporary directory %q: %w", globalTempWorkDir, err)
	}
	globalTempWorkDir = ""
	return nil
}

// getGlobalTempWorkDir returns the current global directory or
// creates a new one if none has been created yet
func getGlobalTempWorkDir() (string, error) {
	globalTempWorkDirMutex.Lock()
	defer globalTempWorkDirMutex.Unlock()

	if globalTempWorkDir == "" {
		dir, err := os.MkdirTemp("", "chart-*")
		if err != nil {
			return "", fmt.Errorf("failed to create temporary directory: %w", err)
		}
		globalTempWorkDir = dir
	}
	return globalTempWorkDir, nil
}

func main() {
	// Make sure we clean up after ourselves
	defer cleanGlobalTempWorkDir()

	if err := rootCmd.Execute(); err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
}
