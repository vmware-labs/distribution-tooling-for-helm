// Package annotate implements the dt charts annotate command
package annotate

import (
	"errors"
	"fmt"

	"github.com/spf13/cobra"
	"github.com/vmware-labs/distribution-tooling-for-helm/cmd/dt/config"
	"github.com/vmware-labs/distribution-tooling-for-helm/pkg/chartutils"
)

// NewCmd builds a new annotate command
func NewCmd(cfg *config.Config) *cobra.Command {
	return &cobra.Command{
		Use:   "annotate CHART_PATH",
		Short: "Annotates a Helm chart (Experimental)",
		Long: `Experimental. Tries to annotate a Helm chart by guesing the container images from the information at values.yaml.

Use it cautiously. Very often the complete list of images cannot be guessed from information in values.yaml`,
		Example: `  # Annotate an example Helm chart
  $ dt charts annotate examples/mongodb`,
		SilenceUsage:  true,
		SilenceErrors: true,
		Args:          cobra.ExactArgs(1),
		RunE: func(_ *cobra.Command, args []string) error {
			chartPath := args[0]
			l := cfg.Logger()

			err := l.ExecuteStep(fmt.Sprintf("Annotating Helm chart %q", chartPath), func() error {
				return chartutils.AnnotateChart(chartPath,
					chartutils.WithAnnotationsKey(cfg.AnnotationsKey),
					chartutils.WithLog(l),
				)

			})

			if err != nil {
				if errors.Is(err, chartutils.ErrNoImagesToAnnotate) {
					l.Warnf("No container images found to be annotated")
					return nil
				}
				return l.Failf("failed to annotate Helm chart %q: %v", chartPath, err)
			}

			l.Successf("Helm chart annotated successfully")

			return nil
		},
	}
}
