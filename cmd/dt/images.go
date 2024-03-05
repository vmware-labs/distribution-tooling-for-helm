package main

import (
	"github.com/spf13/cobra"
	"github.com/vmware-labs/distribution-tooling-for-helm/cmd/dt/lock"
	"github.com/vmware-labs/distribution-tooling-for-helm/cmd/dt/pull"
	"github.com/vmware-labs/distribution-tooling-for-helm/cmd/dt/push"
	"github.com/vmware-labs/distribution-tooling-for-helm/cmd/dt/verify"
)

var imagesCmd = &cobra.Command{
	Use:           "images",
	SilenceUsage:  true,
	SilenceErrors: true,
	Short:         "Container image management commands",
	Run: func(cmd *cobra.Command, _ []string) {
		_ = cmd.Help()
	},
}

func init() {
	imagesCmd.AddCommand(lock.NewCmd(mainConfig), verify.NewCmd(mainConfig), pull.NewCmd(mainConfig), push.NewCmd(mainConfig))
}
