// Package relocate implements the dt relocate command
package relocate

import (
	"fmt"

	"github.com/spf13/cobra"
	"github.com/vmware-labs/distribution-tooling-for-helm/cmd/dt/config"
	"github.com/vmware-labs/distribution-tooling-for-helm/pkg/relocator"
)

// NewCmd builds a new relocate command
func NewCmd(cfg *config.Config) *cobra.Command {
	valuesFiles := []string{"values.yaml"}
	cmd := &cobra.Command{
		Use:   "relocate CHART_PATH OCI_URI",
		Short: "Relocates a Helm chart",
		Long:  "Relocates a Helm chart into a new OCI registry. This command will replace the existing registry references with the new registry both in the Images.lock and values.yaml files",
		Example: `  # Relocate a chart from DockerHub into demo Harbor
  $ dt charts relocate examples/mariadb oci://demo.goharbor.io/test_repo`,
		Args:          cobra.ExactArgs(2),
		SilenceUsage:  true,
		SilenceErrors: true,
		RunE: func(_ *cobra.Command, args []string) error {
			chartPath, repository := args[0], args[1]
			if repository == "" {
				return fmt.Errorf("repository cannot be empty")
			}
			l := cfg.Logger()

			if err := l.ExecuteStep(fmt.Sprintf("Relocating %q with prefix %q", chartPath, repository), func() error {
				return relocator.RelocateChartDir(
					chartPath,
					repository,
					relocator.WithLog(l), relocator.Recursive,
					relocator.WithAnnotationsKey(cfg.AnnotationsKey),
					relocator.WithValuesFiles(valuesFiles...),
				)
			}); err != nil {
				return l.Failf("failed to relocate Helm chart %q: %w", chartPath, err)
			}

			l.Successf("Helm chart relocated successfully")
			return nil
		},
	}

	cmd.PersistentFlags().StringSliceVar(&valuesFiles, "values", valuesFiles, "values files to relocate images (can specify multiple)")

	return cmd
}
