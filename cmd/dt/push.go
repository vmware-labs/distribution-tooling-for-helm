package main

import (
	"context"
	"fmt"

	"github.com/spf13/cobra"
	"github.com/vmware-labs/distribution-tooling-for-helm/internal/log"
	"github.com/vmware-labs/distribution-tooling-for-helm/pkg/chartutils"
	"github.com/vmware-labs/distribution-tooling-for-helm/pkg/wrapping"
)

var pushCmd = newPushCmd()

func pushChartImages(wrap wrapping.Wrap, imagesDir string, opts ...chartutils.Option) error {
	lock, err := wrap.GetImagesLock()
	if err != nil {
		return fmt.Errorf("failed to load Images.lock: %v", err)
	}

	return chartutils.PushImages(lock, imagesDir, opts...)
}

func newPushCmd() *cobra.Command {
	var imagesDir string

	cmd := &cobra.Command{
		Use:   "push CHART_PATH",
		Short: "Pushes the images from Images.lock",
		Long:  "Pushes the images found on the Images.lock from the given Helm chart path into their current registries",
		Example: `  # Push images from a sample local Helm chart
  # Images are pushed to their registries, e.g. oci://docker.io/bitnami/kafka will be pushed to DockerHub, oci://demo.goharbor.io/bitnami/redis will be pushed to Harbor
  $ dt images push examples/mariadb`,
		Args:          cobra.ExactArgs(1),
		SilenceUsage:  true,
		SilenceErrors: true,
		RunE: func(cmd *cobra.Command, args []string) error {
			chartPath := args[0]

			ctx, cancel := contextWithSigterm(context.Background())
			defer cancel()
			l := getLogger()

			chart, err := chartutils.LoadChart(chartPath)
			if err != nil {
				return fmt.Errorf("failed to load chart: %w", err)
			}

			if imagesDir == "" {
				imagesDir = chart.ImagesDir()
			}
			if err := l.Section("Pushing Images", func(subLog log.SectionLogger) error {
				if err := pushChartImages(
					chart,
					imagesDir,
					chartutils.WithLog(log.SilentLog),
					chartutils.WithContext(ctx),
					chartutils.WithProgressBar(subLog.ProgressBar()),
					chartutils.WithArtifactsDir(chart.ImageArtifactsDir()),
					chartutils.WithInsecureMode(insecure),
				); err != nil {
					return subLog.Failf("Failed to push images: %w", err)
				}
				subLog.Infof("Images pushed successfully")
				return nil
			}); err != nil {
				return err
			}

			l.Printf(terminalSpacer)
			l.Successf("All images pushed successfully")
			return nil
		},
	}
	cmd.PersistentFlags().StringVar(&imagesDir, "images-dir", imagesDir,
		"directory containing the images to push. If not empty, it overrides the default images directory inside the chart directory")
	return cmd
}
