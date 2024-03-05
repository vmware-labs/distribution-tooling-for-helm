package main

import (
	"path/filepath"

	"os"

	"github.com/spf13/cobra"
	"github.com/vmware-labs/distribution-tooling-for-helm/cmd/dt/config"
	"github.com/vmware-labs/distribution-tooling-for-helm/cmd/dt/info"
	"github.com/vmware-labs/distribution-tooling-for-helm/cmd/dt/unwrap"
	"github.com/vmware-labs/distribution-tooling-for-helm/cmd/dt/wrap"
)

var rootCmd = newRootCmd()

var mainConfig = config.NewConfig()

func newRootCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use: filepath.Base(os.Args[0]),
		Run: func(cmd *cobra.Command, _ []string) {
			_ = cmd.Help()
		},
	}
	cmd.PersistentFlags().BoolVar(&mainConfig.Insecure, "insecure", mainConfig.Insecure, "skip TLS verification")
	cmd.PersistentFlags().BoolVar(&mainConfig.UsePlainHTTP, "use-plain-http", mainConfig.UsePlainHTTP, "use plain HTTP when pulling and pushing charts")
	cmd.PersistentFlags().StringVar(&mainConfig.AnnotationsKey, "annotations-key", mainConfig.AnnotationsKey, "annotation key used to define the list of included images")

	cmd.PersistentFlags().StringVar(&mainConfig.LogLevel, "log-level", mainConfig.LogLevel, "set log level: (trace, debug, info, warn, error, fatal, panic)")
	cmd.PersistentFlags().BoolVar(&mainConfig.UsePlainLog, "plain", mainConfig.UsePlainLog, "suppress the progress bar and symbols in messages and display only plain log messages")
	cmd.PersistentFlags().BoolVar(&config.KeepArtifacts, "keep-artifacts", config.KeepArtifacts, "keep temporary artifacts created during the tool execution")

	// Do not show completion command
	cmd.CompletionOptions.DisableDefaultCmd = true

	cmd.AddCommand(authCmd)
	cmd.AddCommand(chartCmd)
	cmd.AddCommand(imagesCmd)
	cmd.AddCommand(versionCmd)
	cmd.AddCommand(wrap.NewCmd(mainConfig), unwrap.NewCmd(mainConfig), info.NewCmd(mainConfig))

	return cmd
}
