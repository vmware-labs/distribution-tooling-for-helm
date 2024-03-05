// Package pull implements the pull command
package pull

import (
	"fmt"

	"github.com/spf13/cobra"
	"github.com/vmware-labs/distribution-tooling-for-helm/cmd/dt/config"
	"github.com/vmware-labs/distribution-tooling-for-helm/internal/widgets"
	"github.com/vmware-labs/distribution-tooling-for-helm/pkg/chartutils"
	"github.com/vmware-labs/distribution-tooling-for-helm/pkg/log"
	"github.com/vmware-labs/distribution-tooling-for-helm/pkg/utils"
	"github.com/vmware-labs/distribution-tooling-for-helm/pkg/wrapping"
)

// ChartImages pulls the images of a Helm chart
func ChartImages(wrap wrapping.Wrap, imagesDir string, opts ...chartutils.Option) error {
	return pullImages(wrap, imagesDir, opts...)
}

// NewCmd builds a new pull command
func NewCmd(cfg *config.Config) *cobra.Command {
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
		RunE: func(_ *cobra.Command, args []string) error {
			chartPath := args[0]
			l := cfg.Logger()

			// TODO: Implement timeout

			ctx, cancel := cfg.ContextWithSigterm()
			defer cancel()

			chart, err := chartutils.LoadChart(chartPath)
			if err != nil {
				return fmt.Errorf("failed to load chart: %w", err)
			}
			if imagesDir == "" {
				imagesDir = chart.ImagesDir()
			}
			lock, err := chart.GetImagesLock()
			if err != nil {
				return l.Failf("Failed to load Images.lock: %v", err)
			}
			if len(lock.Images) == 0 {
				l.Warnf("No images found in Images.lock")
			} else {
				if err := l.Section(fmt.Sprintf("Pulling images into %q", chart.ImagesDir()), func(childLog log.SectionLogger) error {
					if err := pullImages(
						chart,
						imagesDir,
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
			}

			if outputFile != "" {
				if err := l.ExecuteStep(
					fmt.Sprintf("Compressing chart into %q", outputFile),
					func() error {
						return utils.TarContext(ctx, chart.RootDir(), outputFile, utils.TarConfig{
							Prefix: fmt.Sprintf("%s-%s", chart.Name(), chart.Version()),
						})
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

			l.Printf(widgets.TerminalSpacer)
			l.Successf(successMessage)

			return nil
		},
	}
	cmd.PersistentFlags().StringVar(&outputFile, "output-file", outputFile, "generate a tar.gz with the output of the pull operation")
	cmd.PersistentFlags().StringVar(&imagesDir, "images-dir", imagesDir,
		"directory where the images will be pulled to. If not empty, it overrides the default images directory inside the chart directory")
	return cmd
}

func pullImages(chart wrapping.Lockable, imagesDir string, opts ...chartutils.Option) error {
	lock, err := chart.GetImagesLock()

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
