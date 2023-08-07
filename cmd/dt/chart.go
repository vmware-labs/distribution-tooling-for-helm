package main

import (
	"github.com/spf13/cobra"
)

var chartCmd = &cobra.Command{
	Use:           "charts",
	Short:         "Helm chart management commands",
	SilenceUsage:  true,
	SilenceErrors: true,
	Run: func(cmd *cobra.Command, args []string) {
		_ = cmd.Help()
	},
}

func init() {
	chartCmd.AddCommand(relocateCmd, annotateCmd)
}
