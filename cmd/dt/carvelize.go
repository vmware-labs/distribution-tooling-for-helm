package main

import (
	"bytes"
	"fmt"
	"os"
	"strings"

	"github.com/spf13/cobra"
	"github.com/vmware-labs/distribution-tooling-for-helm/chartutils"
	"github.com/vmware-labs/distribution-tooling-for-helm/internal/log"
	"github.com/vmware-labs/distribution-tooling-for-helm/utils"
	"github.com/vmware-tanzu/carvel-imgpkg/pkg/imgpkg/lockconfig"
)

var carvelizeCmd = newCarvelizeCmd()

func newCarvelizeCmd() *cobra.Command {
	var yamlFormat bool
	var showDetails bool

	cmd := &cobra.Command{
		Use:   "carvelize FILE",
		Short: "Adds a Carvel bundle to the Helm chart (Experimental)",
		Long:  `Experimental. Adds a Carvel bundle to an existing Helm chart`,
		Example: `  # Adds a Carvel bundle to a Helm chart
  $ dt charts carvelize examples/mariadb`,
		SilenceUsage:  true,
		SilenceErrors: true,
		Args:          cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			chartPath := args[0]
			l := getLogger()
			// Allows silencing called methods
			silentLog := log.SilentLog

			lockFile, err := getImageLockFilePath(chartPath)
			if err != nil {
				return fmt.Errorf("failed to determine Images.lock file location: %w", err)
			}

			if utils.FileExists(lockFile) {
				if err := l.ExecuteStep("Verifying Images.lock", func() error {
					return verifyLock(chartPath, lockFile)
				}); err != nil {
					return l.Failf("Failed to verify lock: %w", err)
				}
				l.Infof("Helm chart %q lock is valid", chartPath)

			} else {
				err := l.ExecuteStep(
					"Images.lock file does not exist. Generating it from annotations...",
					func() error {
						return createImagesLock(chartPath,
							lockFile, silentLog,
						)
					},
				)
				if err != nil {
					return l.Failf("Failed to generate lock: %w", err)
				}
				l.Infof("Images.lock file written to %q", lockFile)
			}
			if err := l.Section(fmt.Sprintf("Generating Carvel bundle for Helm chart %q", chartPath), func(childLog log.SectionLogger) error {
				if err := generateCarvelBundle(
					chartPath,
					chartutils.WithLog(childLog),
				); err != nil {
					return childLog.Failf("%v", err)
				}
				return nil
			}); err != nil {
				return l.Failf("%w", err)
			}
			l.Successf("Carvel bundle created successfully")
			return nil
		},
	}
	cmd.PersistentFlags().BoolVar(&yamlFormat, "yaml", yamlFormat, "Show report in YAML format")
	cmd.PersistentFlags().BoolVar(&showDetails, "detailed", showDetails, "When using the printable report, add more details about the bundled images")

	return cmd
}

func generateCarvelBundle(chartPath string, opts ...chartutils.Option) error {

	cfg := chartutils.NewConfiguration(opts...)
	l := cfg.Log

	lock, err := readLockFromWrap(chartPath)
	if err != nil {
		return fmt.Errorf("failed to load Images.lock: %v", err)
	}

	imgPkgPath := chartPath + "/.imgpkg"
	if !utils.FileExists(imgPkgPath) {
		err := os.Mkdir(imgPkgPath, os.FileMode(0755))
		if err != nil {
			return fmt.Errorf("failed to create .imgpkg directory: %w", err)
		}
	}

	_, _ = chartPath, l
	if !utils.FileExists(chartPath) {
		return fmt.Errorf("wrap file %q does not exist", chartPath)
	}

	bundleMetadata := chartutils.NewCarvelBundle()

	chart, err := chartutils.LoadChart(chartPath)
	if err != nil {
		return fmt.Errorf("failed to load chart: %w", err)
	}

	for _, maintainer := range chart.Metadata.Maintainers {
		author := chartutils.Author{
			Name: maintainer.Name,
		}
		if maintainer.Email != "" {
			author.Email = maintainer.Email
		}
		bundleMetadata.Authors = append(bundleMetadata.Authors, author)
	}
	for _, source := range chart.Metadata.Sources {
		website := chartutils.Website{
			URL: source,
		}
		bundleMetadata.Websites = append(bundleMetadata.Websites, website)
	}

	bundleMetadata.Metadata["name"] = lock.Chart.Name
	for key, value := range chart.Metadata.Annotations {
		if key != "images" {
			bundleMetadata.Metadata[key] = value
		}
	}

	imagesLock := lockconfig.ImagesLock{
		LockVersion: lockconfig.LockVersion{
			APIVersion: lockconfig.ImagesLockAPIVersion,
			Kind:       lockconfig.ImagesLockKind,
		},
	}
	for _, img := range lock.Images {
		// Carvel does not seem to support multi-arch. Grab amd64 digest
		name := img.Image
		i := strings.LastIndex(img.Image, ":")
		if i > -1 {
			name = img.Image[0:i]

		}
		for _, digest := range img.Digests {
			if digest.Arch == "linux/amd64" {
				name = name + "@" + digest.Digest.String()
				break
			}
		}
		imageRef := lockconfig.ImageRef{
			Image: name,
			Annotations: map[string]string{
				"kbld.carvel.dev/id": img.Image,
			},
		}
		imagesLock.AddImageRef(imageRef)
	}
	l.Infof("Validating Carvel images lock")
	imagesLock.Validate()

	path := imgPkgPath + "/images.yml"
	err = imagesLock.WriteToPath(path)
	if err != nil {
		return fmt.Errorf("Could not write image lock: %v", err)
	}
	l.Infof("Carvel images lock written to %q", path)

	buff := &bytes.Buffer{}
	if err = bundleMetadata.ToYAML(buff); err != nil {
		return fmt.Errorf("failed to write bundle metadata file: %v", err)
	}

	path = imgPkgPath + "/bundle.yml"
	if err := os.WriteFile(path, buff.Bytes(), 0666); err != nil {
		return fmt.Errorf("failed to write Carvel bundle metadata to %q: %w", path, err)
	}
	l.Infof("Carvel metadata written to %q", path)
	return nil
}

func init() {
	rootCmd.AddCommand(infoCmd)
}
