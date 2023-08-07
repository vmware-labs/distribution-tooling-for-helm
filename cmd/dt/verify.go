package main

import (
	"context"
	"fmt"
	"os"

	"github.com/spf13/cobra"
	"github.com/vmware-labs/distribution-tooling-for-helm/imagelock"
	"github.com/vmware-labs/distribution-tooling-for-helm/utils"
)

var verifyCmd = newVerifyCmd()

func verifyLock(chartPath string, lockFile string) error {
	if !utils.FileExists(chartPath) {
		return fmt.Errorf("Helm chart %q does not exist", chartPath)
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
		imagelock.WithAnnotationsKey(getAnnotationsKey()),
		imagelock.WithContext(context.Background()),
		imagelock.WithInsecure(insecure),
	)
	if err != nil {
		return fmt.Errorf("failed to re-create Images.lock from Helm chart %q: %v", chartPath, err)
	}

	if err := calculatedLock.Validate(currentLock.Images); err != nil {
		return fmt.Errorf("Images.lock does not validate:\n%v", err)
	}
	return nil
}

func newVerifyCmd() *cobra.Command {
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
		RunE: func(cmd *cobra.Command, args []string) error {
			chartPath := args[0]

			l := getLogger()

			if !utils.FileExists(chartPath) {
				return fmt.Errorf("Helm chart %q does not exist", chartPath)
			}

			if lockFile == "" {
				f, err := getImageLockFilePath(chartPath)
				if err != nil {
					return fmt.Errorf("failed to find Images.lock file for Helm chart %q: %v", chartPath, err)
				}
				lockFile = f
			}

			if err := l.ExecuteStep("Verifying Images.lock", func() error {
				return verifyLock(chartPath, lockFile)
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
