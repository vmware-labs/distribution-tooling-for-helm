package main

import (
	"context"
	"fmt"

	"github.com/spf13/cobra"
	"github.com/vmware-labs/distribution-tooling-for-helm/internal/log"
	"github.com/vmware-labs/distribution-tooling-for-helm/pkg/chartutils"
	"github.com/vmware-labs/distribution-tooling-for-helm/pkg/imagelock"
	"github.com/vmware-labs/distribution-tooling-for-helm/pkg/utils"
	"github.com/vmware-labs/distribution-tooling-for-helm/pkg/wrapping"
)

var pullCmd = newPullCommand()

func pullChartImages(chart wrapping.Lockable, opts ...chartutils.Option) error {
	imagesDir := chart.ImagesDir()
	lockFile := chart.LockFilePath()

	lock, err := imagelock.FromYAMLFile(lockFile)
	if err != nil {
		return fmt.Errorf("failed to read Images.lock file")
	}
	if err := chartutils.PullImages(lock, imagesDir,
		opts...,
	); err != nil {
		return fmt.Errorf("failed to pull images: %v", err)
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
			if err := l.Section(fmt.Sprintf("Pulling images into %q", chart.ImagesDir()), func(childLog log.SectionLogger) error {
				if err := pullChartImages(
					chart,
					chartutils.WithLog(childLog),
					chartutils.WithContext(ctx),
					chartutils.WithProgressBar(childLog.ProgressBar()),
					chartutils.WithArtifactsDir(chart.ImageArtifactsDir()),
				); err != nil {
					return childLog.Failf("%v", err)
				}
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

			l.Printf(terminalSpacer)
			l.Successf(successMessage)

			return nil
		},
	}
	cmd.PersistentFlags().StringVar(&outputFile, "output-file", outputFile, "generate a tar.gz with the output of the pull operation")
	return cmd
}
