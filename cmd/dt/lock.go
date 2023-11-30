package main

import (
	"bytes"
	"fmt"
	"os"

	"github.com/spf13/cobra"
	"github.com/vmware-labs/distribution-tooling-for-helm/internal/log"
	"github.com/vmware-labs/distribution-tooling-for-helm/pkg/imagelock"
)

var lockCmd = newLockCommand()

func createImagesLock(chartPath string, outputFile string, l log.Logger, opts ...imagelock.Option) error {
	l.Infof("Generating images lock for Helm chart %q", chartPath)

	allOpts := append([]imagelock.Option{
		imagelock.WithAnnotationsKey(getAnnotationsKey()),
		imagelock.WithInsecure(insecure),
	}, opts...)

	lock, err := imagelock.GenerateFromChart(chartPath, allOpts...)

	if err != nil {
		return fmt.Errorf("failed to load Helm chart: %v", err)
	}

	if len(lock.Images) == 0 {
		l.Warnf("Did not find any image annotations at Helm chart %q", chartPath)
	}

	buff := &bytes.Buffer{}
	if err = lock.ToYAML(buff); err != nil {
		return fmt.Errorf("failed to write Images.lock file: %v", err)
	}

	if err := os.WriteFile(outputFile, buff.Bytes(), 0666); err != nil {
		return fmt.Errorf("failed to write lock to %q: %w", outputFile, err)
	}

	l.Infof("Images.lock file written to %q", outputFile)
	return nil
}

func newLockCommand() *cobra.Command {
	var platforms []string
	var outputFile string
	getOutputFilename := func(chartPath string) (string, error) {
		if outputFile != "" {
			return outputFile, nil
		}
		return getImageLockFilePath(chartPath)
	}

	cmd := &cobra.Command{
		Use:   "lock CHART_PATH",
		Short: "Creates the lock file",
		Long:  "Creates the Images.lock file for the given Helm chart associating all the images at the time of locking",
		Example: `  # Create the Images.lock for a Helm Chart
  $ dt images lock examples/mariadb
  
  # Create the Images.lock from a Helm chart that uses a different annotation for specifying images
  $ dt images lock examples/mariadb --annotations-key artifacthub.io/images`,
		SilenceUsage:  true,
		SilenceErrors: true,
		Args:          cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			l := getLogger()

			chartPath := args[0]

			outputFile, err := getOutputFilename(chartPath)
			if err != nil {
				return fmt.Errorf("failed to obtain Images.lock location: %w", err)
			}
			if err := l.ExecuteStep("Generating Images.lock from annotations...", func() error {
				return createImagesLock(chartPath, outputFile, log.SilentLog, imagelock.WithPlatforms(platforms))
			}); err != nil {
				return l.Failf("Failed to genereate lock: %w", err)
			}
			l.Successf("Images.lock file written to %q", outputFile)
			return nil
		},
	}
	cmd.PersistentFlags().StringVar(&outputFile, "output-file", outputFile, "output file where to write the Images Lock. If empty, writes to stdout")
	cmd.PersistentFlags().StringSliceVar(&platforms, "platforms", platforms, "platforms to include in the Images.lock file")

	return cmd
}
