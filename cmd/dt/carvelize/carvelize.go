// Package carvelize provides the carvelize command
package carvelize

import (
	"bytes"
	"fmt"
	"os"
	"path/filepath"

	"github.com/spf13/cobra"
	"github.com/vmware-labs/distribution-tooling-for-helm/cmd/dt/config"
	"github.com/vmware-labs/distribution-tooling-for-helm/cmd/dt/lock"
	"github.com/vmware-labs/distribution-tooling-for-helm/cmd/dt/verify"
	"github.com/vmware-labs/distribution-tooling-for-helm/pkg/carvel"
	"github.com/vmware-labs/distribution-tooling-for-helm/pkg/chartutils"
	"github.com/vmware-labs/distribution-tooling-for-helm/pkg/imagelock"
	"github.com/vmware-labs/distribution-tooling-for-helm/pkg/log"
	"github.com/vmware-labs/distribution-tooling-for-helm/pkg/log/silent"

	"github.com/vmware-labs/distribution-tooling-for-helm/pkg/utils"
)

// NewCmd builds a new carvelize command
func NewCmd(cfg *config.Config) *cobra.Command {
	var yamlFormat bool
	var showDetails bool

	cmd := &cobra.Command{
		Use:   "carvelize FILE",
		Short: "Adds a Carvel bundle to the Helm chart (Experimental)",
		Long:  `Experimental. Adds a Carvel bundle to an existing Helm chart`,
		Example: `  # Adds a Carvel bundle to a Helm chart
  $ dt charts carvelize examples/mariadb`,
		SilenceUsage:  true,
		SilenceErrors: true,
		Args:          cobra.ExactArgs(1),
		RunE: func(_ *cobra.Command, args []string) error {
			chartPath := args[0]
			l := cfg.Logger()
			// Allows silencing called methods
			silentLog := silent.NewLogger()

			lockFile, err := chartutils.GetImageLockFilePath(chartPath)
			if err != nil {
				return fmt.Errorf("failed to determine Images.lock file location: %w", err)
			}

			if utils.FileExists(lockFile) {
				if err := l.ExecuteStep("Verifying Images.lock", func() error {
					return verify.Lock(chartPath, lockFile, verify.Config{Insecure: cfg.Insecure, AnnotationsKey: cfg.AnnotationsKey})
				}); err != nil {
					return l.Failf("Failed to verify lock: %w", err)
				}
				l.Infof("Helm chart %q lock is valid", chartPath)

			} else {
				err := l.ExecuteStep(
					"Images.lock file does not exist. Generating it from annotations...",
					func() error {
						return lock.Create(chartPath,
							lockFile, silentLog, imagelock.WithAnnotationsKey(cfg.AnnotationsKey), imagelock.WithInsecure(cfg.Insecure),
						)
					},
				)
				if err != nil {
					return l.Failf("Failed to generate lock: %w", err)
				}
				l.Infof("Images.lock file written to %q", lockFile)
			}
			if err := l.Section(fmt.Sprintf("Generating Carvel bundle for Helm chart %q", chartPath), func(childLog log.SectionLogger) error {
				if err := GenerateBundle(
					chartPath,
					chartutils.WithAnnotationsKey(cfg.AnnotationsKey),
					chartutils.WithLog(childLog),
				); err != nil {
					return childLog.Failf("%v", err)
				}
				return nil
			}); err != nil {
				return l.Failf("%w", err)
			}
			l.Successf("Carvel bundle created successfully")
			return nil
		},
	}
	cmd.PersistentFlags().BoolVar(&yamlFormat, "yaml", yamlFormat, "Show report in YAML format")
	cmd.PersistentFlags().BoolVar(&showDetails, "detailed", showDetails, "When using the printable report, add more details about the bundled images")

	return cmd
}

// GenerateBundle generates a Carvel bundle for a Helm chart
func GenerateBundle(chartPath string, opts ...chartutils.Option) error {
	cfg := chartutils.NewConfiguration(opts...)
	l := cfg.Log

	lock, err := chartutils.ReadLockFromChart(chartPath)
	if err != nil {
		return fmt.Errorf("failed to load Images.lock: %v", err)
	}

	imgPkgPath := filepath.Join(chartPath, ".imgpkg")
	if !utils.FileExists(imgPkgPath) {
		err := os.Mkdir(imgPkgPath, os.FileMode(0755))
		if err != nil {
			return fmt.Errorf("failed to create .imgpkg directory: %w", err)
		}
	}

	bundleMetadata, err := carvel.CreateBundleMetadata(chartPath, lock, cfg)
	if err != nil {
		return fmt.Errorf("failed to prepare Carvel bundle: %w", err)
	}

	carvelImagesLock, err := carvel.CreateImagesLock(lock)
	if err != nil {
		return fmt.Errorf("failed to prepare Carvel images lock: %w", err)
	}
	l.Infof("Validating Carvel images lock")

	err = carvelImagesLock.Validate()
	if err != nil {
		return fmt.Errorf("failed to validate Carvel images lock: %w", err)
	}

	path := filepath.Join(imgPkgPath, "images.yml")
	err = carvelImagesLock.WriteToPath(path)
	if err != nil {
		return fmt.Errorf("Could not write image lock: %v", err)
	}
	l.Infof("Carvel images lock written to %q", path)

	buff := &bytes.Buffer{}
	if err = bundleMetadata.ToYAML(buff); err != nil {
		return fmt.Errorf("failed to write bundle metadata file: %v", err)
	}

	path = imgPkgPath + "/bundle.yml"
	if err := os.WriteFile(path, buff.Bytes(), 0666); err != nil {
		return fmt.Errorf("failed to write Carvel bundle metadata to %q: %w", path, err)
	}
	l.Infof("Carvel metadata written to %q", path)
	return nil
}
