package main

import (
	"github.com/spf13/cobra"
	"github.com/vmware-labs/distribution-tooling-for-helm/cmd/dt/login"
	"github.com/vmware-labs/distribution-tooling-for-helm/cmd/dt/logout"
)

var authCmd = &cobra.Command{
	Use:           "auth",
	Short:         "Authentication commands",
	SilenceUsage:  true,
	SilenceErrors: true,
	Run: func(cmd *cobra.Command, _ []string) {
		_ = cmd.Help()
	},
}

func init() {
	authCmd.AddCommand(login.NewCmd(mainConfig), logout.NewCmd(mainConfig))
}
