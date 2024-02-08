// Package pterm provides a logger implementation using the pterm library
package pterm

import (
	"os"

	"github.com/vmware-labs/distribution-tooling-for-helm/pkg/log"
)

// NewLogger returns a new Logger implemented by pterm
func NewLogger() *Logger {
	return &Logger{writer: os.Stdout, level: log.InfoLevel}
}
