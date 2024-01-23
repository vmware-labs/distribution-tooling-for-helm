// Package info implements the dt info command
package info

import (
	"fmt"
	"os"
	"strings"

	"github.com/spf13/cobra"
	"github.com/vmware-labs/distribution-tooling-for-helm/cmd/dt/config"
	"github.com/vmware-labs/distribution-tooling-for-helm/pkg/chartutils"
	"github.com/vmware-labs/distribution-tooling-for-helm/pkg/log"
	"github.com/vmware-labs/distribution-tooling-for-helm/pkg/utils"
)

// NewCmd returns a new dt info command
func NewCmd(cfg *config.Config) *cobra.Command {
	var yamlFormat bool
	var showDetails bool

	cmd := &cobra.Command{
		Use:   "info FILE",
		Short: "shows info of a wrapped chart",
		Long:  `Shows information of a wrapped Helm chart, including the bundled images and chart metadata`,
		Example: `  # Show information of a wrapped Helm chart
  $ dt info mariadb-12.2.8.wrap.tgz`,
		SilenceUsage:  true,
		SilenceErrors: true,
		Args:          cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			chartPath := args[0]
			l := cfg.Logger()
			_, _ = chartPath, l
			if !utils.FileExists(chartPath) {
				return fmt.Errorf("wrap file %q does not exist", chartPath)
			}
			lock, err := chartutils.ReadLockFromChart(chartPath)
			if err != nil {
				return fmt.Errorf("failed to load Images.lock: %v", err)
			}
			if yamlFormat {
				if err := lock.ToYAML(os.Stdout); err != nil {
					return fmt.Errorf("failed to write Images.lock yaml representation: %v", err)
				}
			} else {

				_ = l.Section("Wrap Information", func(l log.SectionLogger) error {
					l.Printf("Chart: %s", lock.Chart.Name)
					l.Printf("Version: %s", lock.Chart.Version)
					l.Printf("App Version: %s", lock.Chart.AppVersion)
					_ = l.Section("Metadata", func(l log.SectionLogger) error {
						for k, v := range lock.Metadata {
							l.Printf("- %s: %s", k, v)

						}
						return nil
					})
					_ = l.Section("Images", func(l log.SectionLogger) error {
						for _, img := range lock.Images {
							if showDetails {
								_ = l.Section(fmt.Sprintf("%s/%s", img.Chart, img.Name), func(l log.SectionLogger) error {
									l.Printf("Image: %s", img.Image)
									if showDetails {
										l.Printf("Digests")
										for _, digest := range img.Digests {
											l.Printf("- Arch: %s", digest.Arch)
											l.Printf("  Digest: %s", digest.Digest)
										}
									}
									return nil
								})
							} else {
								platforms := make([]string, 0)
								for _, digest := range img.Digests {
									platforms = append(platforms, digest.Arch)
								}
								l.Printf("%s (%s)", img.Image, strings.Join(platforms, ", "))
							}
						}
						return nil
					})
					return nil
				})
			}
			return nil
		},
	}
	cmd.PersistentFlags().BoolVar(&yamlFormat, "yaml", yamlFormat, "Show report in YAML format")
	cmd.PersistentFlags().BoolVar(&showDetails, "detailed", showDetails, "When using the printable report, add more details about the bundled images")

	return cmd
}
