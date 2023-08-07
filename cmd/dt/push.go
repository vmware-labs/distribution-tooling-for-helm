package main

import (
	"context"
	"fmt"
	"os"
	"path/filepath"

	"github.com/spf13/cobra"
	"github.com/vmware-labs/distribution-tooling-for-helm/chartutils"
	"github.com/vmware-labs/distribution-tooling-for-helm/imagelock"
	"github.com/vmware-labs/distribution-tooling-for-helm/internal/log"
)

var pushCmd = newPushCmd()

func pushChartImages(chartPath string, opts ...chartutils.Option) error {
	chartRoot, err := chartutils.GetChartRoot(chartPath)
	if err != nil {
		return fmt.Errorf("cannot determine Helm chart root for %q: %v", chartPath, err)
	}
	imagesDir := filepath.Join(chartRoot, "images")

	lockFile := filepath.Join(chartRoot, imagelock.DefaultImagesLockFileName)

	fh, err := os.Open(lockFile)
	if err != nil {
		return fmt.Errorf("failed to open Images.lock file: %v", err)
	}
	defer fh.Close()

	lock, err := imagelock.FromYAML(fh)
	if err != nil {
		return fmt.Errorf("failed to load Images.lock: %v", err)
	}

	return chartutils.PushImages(lock, imagesDir, opts...)
}

func newPushCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "push CHART_PATH OCI_URI",
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

			if err := l.Section("Pushing Images", func(subLog log.SectionLogger) error {
				if err := pushChartImages(
					chartPath,
					chartutils.WithLog(log.SilentLog),
					chartutils.WithContext(ctx),
					chartutils.WithProgressBar(subLog.ProgressBar()),
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
}
