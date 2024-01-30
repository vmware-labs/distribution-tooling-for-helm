package main

import "github.com/spf13/cobra"

var authCmd = &cobra.Command{
	Use:           "auth",
	Short:         "Log in to a registry",
	SilenceUsage:  true,
	SilenceErrors: true,
	Run: func(cmd *cobra.Command, _ []string) {
		_ = cmd.Help()
	},
}

func init() {
	authCmd.AddCommand(loginCmd, logoutCmd)
}
