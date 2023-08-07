package main

import (
	"fmt"

	"github.com/spf13/cobra"
	"github.com/vmware-labs/distribution-tooling-for-helm/relocator"
)

var relocateCmd = newRelocateCmd()

func relocateChart(chartPath, repository string, opts ...relocator.RelocateOption) error {
	baseOpts := []relocator.RelocateOption{
		relocator.Recursive,
		relocator.WithAnnotationsKey(getAnnotationsKey()),
	}
	if err := relocator.RelocateChartDir(
		chartPath,
		repository,
		append(baseOpts, opts...)...,
	); err != nil {
		return fmt.Errorf("failed to relocate Helm chart: %v", err)
	}
	return nil
}
func newRelocateCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "relocate CHART_PATH OCI_URI",
		Short: "Relocates a Helm chart",
		Long:  "Relocates a Helm chart into a new OCI registry. This command will replace the existing registry references with the new registry both in the Images.lock and values.yaml files",
		Example: `  # Relocate a chart from DockerHub into demo Harbor
  $ dt charts relocate examples/mariadb oci://demo.goharbor.io/test_repo`,
		Args:          cobra.ExactArgs(2),
		SilenceUsage:  true,
		SilenceErrors: true,
		RunE: func(cmd *cobra.Command, args []string) error {
			chartPath, repository := args[0], args[1]
			if repository == "" {
				return fmt.Errorf("repository cannot be empty")
			}
			l := getLogger()
			if err := l.ExecuteStep(fmt.Sprintf("Relocating %q with prefix %q", chartPath, repository), func() error {
				return relocateChart(chartPath, repository, relocator.WithLog(l))
			}); err != nil {
				return l.Failf("failed to relocate %q: %w", chartPath, err)
			}

			l.Successf("Helm chart relocated successfully")
			return nil
		},
	}
}
