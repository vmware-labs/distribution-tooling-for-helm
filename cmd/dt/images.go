package main

import (
	"path/filepath"

	"github.com/spf13/cobra"
	"github.com/vmware-labs/distribution-tooling-for-helm/chartutils"
	"github.com/vmware-labs/distribution-tooling-for-helm/imagelock"
)

var imagesCmd = &cobra.Command{
	Use:           "images",
	SilenceUsage:  true,
	SilenceErrors: true,
	Short:         "Container image management commands",
	Run: func(cmd *cobra.Command, args []string) {
		_ = cmd.Help()
	},
}

func getImageLockFilePath(chartPath string) (string, error) {
	chartRoot, err := chartutils.GetChartRoot(chartPath)
	if err != nil {
		return "", err
	}

	return filepath.Join(chartRoot, imagelock.DefaultImagesLockFileName), nil
}

func init() {
	imagesCmd.AddCommand(lockCmd, verifyCmd, pullCmd, pushCmd)
}
