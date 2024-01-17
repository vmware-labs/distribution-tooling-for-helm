package main

import (
	"context"
	"errors"
	"fmt"

	"github.com/spf13/cobra"
	"github.com/vmware-labs/distribution-tooling-for-helm/internal/log"
	"github.com/vmware-labs/distribution-tooling-for-helm/pkg/chartutils"
	"github.com/vmware-labs/distribution-tooling-for-helm/pkg/imagelock"
	"github.com/vmware-labs/distribution-tooling-for-helm/pkg/utils"
	"github.com/vmware-labs/distribution-tooling-for-helm/pkg/wrapping"
)

var pullCmd = newPullCommand()

func pullChartImages(chart wrapping.Lockable, imagesDir string, opts ...chartutils.Option) error {
	lockFile := chart.LockFilePath()

	lock, err := imagelock.FromYAMLFile(lockFile)
	if err != nil {
		return fmt.Errorf("failed to read Images.lock file")
	}
	if err := chartutils.PullImages(lock, imagesDir,
		opts...,
	); err != nil {
		return fmt.Errorf("failed to pull images: %w", err)
	}
	return nil
}

func compressChart(ctx context.Context, dir, prefix, outputFile string) error {
	return utils.TarContext(ctx, dir, outputFile, utils.TarConfig{
		Prefix: prefix,
	})
}

func newPullCommand() *cobra.Command {
	var outputFile string
	var imagesDir string

	cmd := &cobra.Command{
		Use:   "pull CHART_PATH",
		Short: "Pulls the images from the Images.lock",
		Long:  "Pulls all the images that are defined within the Images.lock from the given Helm chart",
		Example: `  # Pull images from a Helm Chart in a local folder
  $ dt images pull examples/mariadb`,
		Args:          cobra.ExactArgs(1),
		SilenceUsage:  true,
		SilenceErrors: true,
		RunE: func(cmd *cobra.Command, args []string) error {
			chartPath := args[0]
			l := getLogger()

			// TODO: Implement timeout
			ctx, cancel := contextWithSigterm(context.Background())
			defer cancel()

			chart, err := chartutils.LoadChart(chartPath)
			if err != nil {
				return fmt.Errorf("failed to load chart: %w", err)
			}
			if imagesDir == "" {
				imagesDir = chart.ImagesDir()
			}

			var imagesPulled bool
			if err := l.Section(fmt.Sprintf("Pulling images into %q", chart.ImagesDir()), func(childLog log.SectionLogger) error {
				if err := pullChartImages(
					chart,
					imagesDir,
					chartutils.WithLog(childLog),
					chartutils.WithContext(ctx),
					chartutils.WithProgressBar(childLog.ProgressBar()),
					chartutils.WithArtifactsDir(chart.ImageArtifactsDir()),
				); err != nil {
					if errors.Is(err, chartutils.ErrNoImagesFound) {
						childLog.Warnf("No images found in Images.lock")
						return nil
					}
					return childLog.Failf("%v", err)
				}
				imagesPulled = true
				childLog.Infof("All images pulled successfully")
				return nil
			}); err != nil {
				return l.Failf("%w", err)
			}

			if outputFile != "" {
				if err := l.ExecuteStep(
					fmt.Sprintf("Compressing chart into %q", outputFile),
					func() error {
						return compressChart(ctx, chart.RootDir(), fmt.Sprintf("%s-%s", chart.Name(), chart.Version()), outputFile)
					},
				); err != nil {
					return l.Failf("failed to compress chart: %w", err)
				}

				l.Infof("Helm chart compressed to %q", outputFile)
			}

			var successMessage string
			if outputFile != "" {
				successMessage = fmt.Sprintf("All images pulled successfully and chart compressed into %q", outputFile)
			} else {
				successMessage = fmt.Sprintf("All images pulled successfully into %q", chart.ImagesDir())
			}

			var warningMessage string
			if outputFile != "" {
				warningMessage = fmt.Sprintf("No images found in Images.lock. Chart compressed into %q", outputFile)
			} else {
				warningMessage = "No images found in Images.lock"
			}

			l.Printf(terminalSpacer)

			if imagesPulled {
				l.Successf(successMessage)
			} else {
				l.Warnf(warningMessage)
			}

			return nil
		},
	}
	cmd.PersistentFlags().StringVar(&outputFile, "output-file", outputFile, "generate a tar.gz with the output of the pull operation")
	cmd.PersistentFlags().StringVar(&imagesDir, "images-dir", imagesDir,
		"directory where the images will be pulled to. If not empty, it overrides the default images directory inside the chart directory")
	return cmd
}
