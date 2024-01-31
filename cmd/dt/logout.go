package main

import (
	"os"

	"github.com/docker/cli/cli/config"
	"github.com/google/go-containerregistry/pkg/authn"
	"github.com/google/go-containerregistry/pkg/name"
	"github.com/spf13/cobra"
	"github.com/vmware-labs/distribution-tooling-for-helm/internal/log"
)

var logoutCmd = newLogoutCmd()

func newLogoutCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "logout REGISTRY",
		Short: "Logout from an OCI registry (Experimental)",
		Long:  "Experimental. Logout from an OCI registry using the Docker configuration file",
		Args:  cobra.ExactArgs(1),
		Example: `  # Log out from index.docker.io
  $ dt auth logout index.docker.io`,
		SilenceUsage:  true,
		SilenceErrors: true,
		RunE: func(_ *cobra.Command, args []string) error {
			l := getLogger()

			reg, err := name.NewRegistry(args[0])
			if err != nil {
				return l.Failf("failed to load registry %s: %v", args[0], err)
			}
			serverAddress := reg.Name()

			return logout(serverAddress, l)
		},
	}

	return cmd
}

// from https://github.com/google/go-containerregistry/blob/main/cmd/crane/cmd/auth.go
func logout(serverAddress string, l log.SectionLogger) error {
	l.Infof("logout from %s", serverAddress)
	cf, err := config.Load(os.Getenv("DOCKER_CONFIG"))
	if err != nil {
		return l.Failf("failed to load configuration: %v", err)
	}
	creds := cf.GetCredentialsStore(serverAddress)
	if serverAddress == name.DefaultRegistry {
		serverAddress = authn.DefaultAuthKey
	}
	if err := creds.Erase(serverAddress); err != nil {
		return l.Failf("failed to store credentials: %v", err)
	}

	if err := cf.Save(); err != nil {
		return l.Failf("failed to save authorization information: %v", err)
	}
	l.Successf("logged out via %s", cf.Filename)
	return nil
}
