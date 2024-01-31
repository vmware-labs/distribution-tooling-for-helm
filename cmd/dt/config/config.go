// Package config defines the configuration of the dt tool
package config

import (
	"context"
	"fmt"
	"os"
	"os/signal"
	"sync"
	"syscall"

	"github.com/vmware-labs/distribution-tooling-for-helm/pkg/imagelock"
	"github.com/vmware-labs/distribution-tooling-for-helm/pkg/log"
)

// Config defines the configuration of the dt tool
type Config struct {
	Insecure       bool
	logger         log.SectionLogger
	Context        context.Context
	AnnotationsKey string
	TempDirectory  string
	UsePlainHTTP   bool

	LogLevel    string
	UsePlainLog bool
}

// NewConfig returns a new Config
func NewConfig() *Config {
	return &Config{
		Context:        context.Background(),
		AnnotationsKey: imagelock.DefaultAnnotationsKey,
		LogLevel:       "info",
		UsePlainLog:    false,
	}
}

// Logger returns the current SectionLogger, creating it if necessary
func (c *Config) Logger() log.SectionLogger {
	if c.logger == nil {

		var l log.SectionLogger
		if c.UsePlainLog {
			l = log.NewLogrusSectionLogger()
		} else {
			l = log.NewPtermSectionLogger()
		}
		lvl, err := log.ParseLevel(c.LogLevel)

		if err != nil {
			l.Warnf("Invalid log level %s: %v", c.LogLevel, err)
		}

		l.SetLevel(lvl)
		c.logger = l
	}
	return c.logger
}

// GetTemporaryDirectory returns the temporary directory of the Config
func (c *Config) GetTemporaryDirectory() (string, error) {
	if c.TempDirectory != "" {
		return c.TempDirectory, nil
	}

	dir, err := os.MkdirTemp("", "chart-*")
	if err != nil {
		return "", err
	}
	c.TempDirectory = dir
	return dir, nil
}

// ContextWithSigterm returns a context that is canceled when the process receives a SIGTERM
func (c *Config) ContextWithSigterm() (context.Context, context.CancelFunc) {
	ctx, stop := signal.NotifyContext(c.Context, os.Interrupt, syscall.SIGTERM)
	// If we are done, call stop right away so we restore signal behavior
	go func() {
		defer stop()
		<-ctx.Done()
	}()
	return ctx, stop

}

var (
	// KeepArtifacts is a flag that indicates whether artifacts should be kept
	KeepArtifacts bool

	// global temporary directory used to store different assets
	globalTempWorkDir      string
	globalTempWorkDirMutex = &sync.RWMutex{}
)

// CleanGlobalTempWorkDir removes the global temporary directory
func CleanGlobalTempWorkDir() error {
	globalTempWorkDirMutex.Lock()
	defer globalTempWorkDirMutex.Unlock()

	if globalTempWorkDir == "" || KeepArtifacts {
		return nil
	}
	if err := os.RemoveAll(globalTempWorkDir); err != nil {
		return fmt.Errorf("failed to remove temporary directory %q: %w", globalTempWorkDir, err)
	}
	globalTempWorkDir = ""
	return nil
}

// GetGlobalTempWorkDir returns the current global directory or
// creates a new one if none has been created yet
func GetGlobalTempWorkDir() (string, error) {
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
