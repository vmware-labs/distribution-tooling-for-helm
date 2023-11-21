package main

import (
	"context"
	"os/signal"
	"path/filepath"
	"syscall"

	"os"

	"github.com/spf13/cobra"
	"github.com/vmware-labs/distribution-tooling-for-helm/internal/log"
	"github.com/vmware-labs/distribution-tooling-for-helm/pkg/imagelock"
)

var rootCmd = newRootCmd()

const (
	// Text to print to terminal to separate sections to improve readability
	// An empty string will just add a new line
	terminalSpacer = ""
)

// Global flags
var (
	insecure       bool
	usePlainHTTP   bool
	annotationsKey string = imagelock.DefaultAnnotationsKey
	logLevel              = "info"
	usePlainLog           = false
)

func newRootCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use: filepath.Base(os.Args[0]),
		Run: func(cmd *cobra.Command, args []string) {
			_ = cmd.Help()
		},
	}
	cmd.PersistentFlags().BoolVar(&insecure, "insecure", insecure, "skip TLS verification")
	cmd.PersistentFlags().BoolVar(&usePlainHTTP, "use-plain-http", usePlainHTTP, "use plain HTTP when pulling and pushing charts")
	cmd.PersistentFlags().StringVar(&annotationsKey, "annotations-key", annotationsKey, "annotation key used to define the list of included images")

	cmd.PersistentFlags().StringVar(&logLevel, "log-level", logLevel, "set log level: (trace, debug, info, warn, error, fatal, panic)")
	cmd.PersistentFlags().BoolVar(&usePlainLog, "plain", usePlainLog, "suppress the progress bar and symbols in messages and display only plain log messages")
	cmd.PersistentFlags().BoolVar(&keepArtifacts, "keep-artifacts", keepArtifacts, "keep temporary artifacts created during the tool execution")

	// Do not show completion command
	cmd.CompletionOptions.DisableDefaultCmd = true

	cmd.AddCommand(chartCmd)
	cmd.AddCommand(imagesCmd)
	cmd.AddCommand(versionCmd)

	return cmd
}

func getAnnotationsKey() string {
	return annotationsKey
}

func getLogger() log.SectionLogger {
	var l log.SectionLogger
	if usePlainLog {
		l = log.NewLogrusSectionLogger()
	} else {
		l = log.NewPtermSectionLogger()
	}
	lvl, err := log.ParseLevel(logLevel)

	if err != nil {
		l.Warnf("Invalid log level %s: %v", logLevel, err)
		return l
	}

	l.SetLevel(lvl)
	return l
}

func contextWithSigterm(ctx context.Context) (context.Context, context.CancelFunc) {
	ctx, stop := signal.NotifyContext(ctx, os.Interrupt, syscall.SIGTERM)
	// If we are done, call stop right away so we restore signal behavior
	go func() {
		defer stop()
		<-ctx.Done()
	}()
	return ctx, stop
}
