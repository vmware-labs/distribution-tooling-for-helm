// Package verify defines the verify command
package verify

import (
	"context"
	"fmt"
	"os"

	"github.com/spf13/cobra"
	"github.com/vmware-labs/distribution-tooling-for-helm/cmd/dt/config"
	"github.com/vmware-labs/distribution-tooling-for-helm/pkg/chartutils"
	"github.com/vmware-labs/distribution-tooling-for-helm/pkg/imagelock"
	"github.com/vmware-labs/distribution-tooling-for-helm/pkg/utils"
)

// Auth defines the authentication information to access the container registry
type Auth struct {
	Username string
	Password string
}

// Config defines the configuration of the verify command
type Config struct {
	AnnotationsKey string
	Insecure       bool
	Auth           Auth
}

// Lock verifies the images in an Images.lock
func Lock(chartPath string, lockFile string, cfg Config) error {
	if !utils.FileExists(chartPath) {
		return fmt.Errorf("chart %q does not exist", chartPath)
	}
	fh, err := os.Open(lockFile)
	if err != nil {
		return fmt.Errorf("failed to open Images.lock file: %v", err)
	}
	defer fh.Close()

	currentLock, err := imagelock.FromYAML(fh)
	if err != nil {
		return fmt.Errorf("failed to load Images.lock: %v", err)
	}
	calculatedLock, err := imagelock.GenerateFromChart(chartPath,
		imagelock.WithAnnotationsKey(cfg.AnnotationsKey),
		imagelock.WithContext(context.Background()),
		imagelock.WithAuth(cfg.Auth.Username, cfg.Auth.Password),
		imagelock.WithInsecure(cfg.Insecure),
	)

	if err != nil {
		return fmt.Errorf("failed to re-create Images.lock from Helm chart %q: %v", chartPath, err)
	}

	if err := calculatedLock.Validate(currentLock.Images); err != nil {
		return fmt.Errorf("validation failed for Images.lock: %w", err)
	}
	return nil
}

// NewCmd builds a new verify command
func NewCmd(cfg *config.Config) *cobra.Command {
	var lockFile string

	cmd := &cobra.Command{
		Use:   "verify CHART_PATH",
		Short: "Verifies the images in an Images.lock",
		Long:  "Verifies that the information in the Images.lock from the given Helm chart are the same images available on their registries for being pulled",
		Example: `  # Verifies integrity of the container images on the given Helm chart
  $ dt images verify examples/mariadb`,
		Args:          cobra.ExactArgs(1),
		SilenceUsage:  true,
		SilenceErrors: true,
		RunE: func(_ *cobra.Command, args []string) error {
			chartPath := args[0]

			l := cfg.Logger()

			if !utils.FileExists(chartPath) {
				return fmt.Errorf("chart %q does not exist", chartPath)
			}

			if lockFile == "" {
				f, err := chartutils.GetImageLockFilePath(chartPath)
				if err != nil {
					return fmt.Errorf("failed to find Images.lock file for Helm chart %q: %v", chartPath, err)
				}
				lockFile = f
			}

			if err := l.ExecuteStep("Verifying Images.lock", func() error {
				return Lock(chartPath, lockFile, Config{Insecure: cfg.Insecure, AnnotationsKey: cfg.AnnotationsKey})
			}); err != nil {
				return l.Failf("failed to verify %q lock: %w", chartPath, err)
			}

			l.Successf("Helm chart %q lock is valid", chartPath)
			return nil
		},
	}
	cmd.PersistentFlags().StringVar(&lockFile, "imagelock-file", lockFile, "location of the Images.lock YAML file")
	return cmd
}
