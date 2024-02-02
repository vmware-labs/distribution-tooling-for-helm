// Package login implements the command to login to OCI registries
package login

import (
	"io"
	"os"
	"strings"

	dockercfg "github.com/docker/cli/cli/config"
	"github.com/docker/cli/cli/config/types"
	"github.com/google/go-containerregistry/pkg/authn"
	"github.com/google/go-containerregistry/pkg/name"
	"github.com/spf13/cobra"
	"github.com/vmware-labs/distribution-tooling-for-helm/cmd/dt/config"
	"github.com/vmware-labs/distribution-tooling-for-helm/pkg/log"
)

type loginOptions struct {
	serverAddress string
	user          string
	password      string
	passwordStdin bool
}

// NewCmd returns a new dt login command
func NewCmd(cfg *config.Config) *cobra.Command {
	var opts loginOptions

	cmd := &cobra.Command{
		Use:   "login REGISTRY",
		Short: "Log in to an OCI registry (Experimental)",
		Long:  "Experimental. Log in to an OCI registry using the Docker configuration file",
		Example: `  # Log in to index.docker.io 
  $ dt auth login index.docker.io -u my_username -p my_password

  # Log in to index.docker.io with a password from stdin
  $ dt auth login index.docker.io -u my_username --password-stdin < <(echo my_password)`,
		Args:          cobra.ExactArgs(1),
		SilenceUsage:  true,
		SilenceErrors: true,
		RunE: func(_ *cobra.Command, args []string) error {
			l := cfg.Logger()

			reg, err := name.NewRegistry(args[0])
			if err != nil {
				return l.Failf("failed to load registry %s: %v", args[0], err)
			}
			opts.serverAddress = reg.Name()

			return login(opts, l)
		},
	}

	flags := cmd.Flags()

	flags.StringVarP(&opts.user, "username", "u", "", "Username")
	flags.StringVarP(&opts.password, "password", "p", "", "Password")
	flags.BoolVarP(&opts.passwordStdin, "password-stdin", "", false, "Take the password from stdin")

	return cmd
}

// from https://github.com/google/go-containerregistry/blob/main/cmd/crane/cmd/auth.go
func login(opts loginOptions, l log.SectionLogger) error {
	if opts.passwordStdin {
		contents, err := io.ReadAll(os.Stdin)
		if err != nil {
			return l.Failf("failed to read from stdin: %v", err)
		}

		opts.password = strings.TrimSuffix(string(contents), "\n")
		opts.password = strings.TrimSuffix(opts.password, "\r")
	}
	if opts.user == "" && opts.password == "" {
		return l.Failf("username and password required")
	}
	l.Infof("log in to %s as user %s", opts.serverAddress, opts.user)
	cf, err := dockercfg.Load(os.Getenv("DOCKER_CONFIG"))
	if err != nil {
		return l.Failf("failed to load configuration: %v", err)
	}
	creds := cf.GetCredentialsStore(opts.serverAddress)
	if opts.serverAddress == name.DefaultRegistry {
		opts.serverAddress = authn.DefaultAuthKey
	}
	if err := creds.Store(types.AuthConfig{
		ServerAddress: opts.serverAddress,
		Username:      opts.user,
		Password:      opts.password,
	}); err != nil {
		return l.Failf("failed to store credentials: %v", err)
	}

	if err := cf.Save(); err != nil {
		return l.Failf("failed to save authorization information: %v", err)
	}
	l.Successf("logged in via %s", cf.Filename)
	return nil
}
