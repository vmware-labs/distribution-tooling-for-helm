package main

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"strings"

	"github.com/spf13/cobra"
	"github.com/vmware-labs/distribution-tooling-for-helm/chartutils"
	"github.com/vmware-labs/distribution-tooling-for-helm/imagelock"
	"github.com/vmware-labs/distribution-tooling-for-helm/internal/log"
	"github.com/vmware-labs/distribution-tooling-for-helm/internal/widgets"
	"github.com/vmware-labs/distribution-tooling-for-helm/relocator"
	"github.com/vmware-labs/distribution-tooling-for-helm/utils"
)

var unwrapCmd = newUnwrapCommand()

func newUnwrapCommand() *cobra.Command {
	var (
		sayYes       bool
		pushChartURL string
		maxRetries   = 3
		version      string
	)

	successMessage := "Helm chart unwrapped successfully"

	cmd := &cobra.Command{
		Use:   "unwrap FILE OCI_URI",
		Short: "Unwraps a wrapped Helm chart",
		Long:  "Unwraps a wrapped package and moves it into a target OCI registry. This command will read a wrap tarball and push all its container images and Helm chart into the target OCI registry",
		Example: `  # Unwrap a Helm chart and push it into a Harbor repository
  $ dt unwrap mariadb-12.2.8.wrap.tgz oci://demo.goharbor.io/test_repo
`,
		SilenceUsage:  true,
		SilenceErrors: true,
		Args:          cobra.ExactArgs(2),
		RunE: func(cmd *cobra.Command, args []string) error {
			inputChart, registryURL := args[0], args[1]

			if registryURL == "" {
				return fmt.Errorf("the registry cannot be empty")
			}

			parentLog := getLogger()

			ctx, cancel := contextWithSigterm(context.Background())
			defer cancel()

			l := parentLog.StartSection(fmt.Sprintf("Unwrapping Helm chart %q", inputChart))

			tempDir, err := getGlobalTempWorkDir()
			if err != nil {
				return fmt.Errorf("failed to create temporary directory: %v", err)
			}
			if keepArtifacts {
				l.Debugf("Temporary assets kept at %q", tempDir)
			}

			chartPath, err := resolveInputChartPath(inputChart, l, cmd.Flags())
			if err != nil {
				return err
			}

			chart, err := chartutils.LoadChart(chartPath)
			if err != nil {
				return l.Failf("failed to load Helm chart %q: %w", chartPath, err)
			}

			if err := l.ExecuteStep(fmt.Sprintf("Relocating %q with prefix %q", chartPath, registryURL), func() error {
				return relocateChart(chartPath, registryURL, relocator.WithLog(l))
			}); err != nil {
				return l.Failf("failed to relocate %q: %w", chartPath, err)
			}
			l.Infof("Helm chart relocated successfully")

			lenImages := showImagesSummary(chart, l)

			if lenImages > 0 && (sayYes || widgets.ShowYesNoQuestion(l.PrefixText("Do you want to push the wrapped images to the OCI registry?"))) {
				if err := l.Section("Pushing Images", func(subLog log.SectionLogger) error {
					return pushChartImagesAndVerify(ctx, chartPath, subLog)
				}); err != nil {
					return l.Failf("Failed to push images: %w", err)
				}
				l.Printf(terminalSpacer)
			}

			if sayYes || widgets.ShowYesNoQuestion(l.PrefixText("Do you want to push the Helm chart to the OCI registry?")) {

				if pushChartURL == "" {
					pushChartURL = registryURL
				}
				pushChartURL = normalizeOCIURL(pushChartURL)
				fullChartURL := fmt.Sprintf("%s/%s", pushChartURL, chart.Name())

				if err := l.ExecuteStep(fmt.Sprintf("Pushing Helm chart to %q", pushChartURL), func() error {
					return utils.ExecuteWithRetry(maxRetries, func(try int, prevErr error) error {
						if try > 0 {
							l.Debugf("Failed to push Helm chart: %v", prevErr)
						}
						return pushChart(chart, pushChartURL)
					})
				}); err != nil {
					return l.Failf("Failed to push Helm chart: %w", err)
				}
				l.Infof("Helm chart successfully pushed")

				successMessage = fmt.Sprintf(`%s: You can use it now by running "helm install %s --generate-name"`, successMessage, fullChartURL)
			}

			l.Printf(terminalSpacer)

			parentLog.Successf(successMessage)

			return nil
		},
	}

	cmd.PersistentFlags().StringVar(&version, "version", version, "when unwrapping remote Helm charts from OCI, version to request")
	cmd.PersistentFlags().StringVar(&pushChartURL, "push-chart-url", pushChartURL, "push the unwrapped Helm chart to the given URL")
	cmd.PersistentFlags().BoolVar(&sayYes, "yes", sayYes, "respond 'yes' to any yes/no question")

	return cmd
}

func pushChartImagesAndVerify(ctx context.Context, chartPath string, l log.SectionLogger) error {
	lockFile, err := getImageLockFilePath(chartPath)
	if err != nil {
		return fmt.Errorf("failed to determine Images.lock file location: %w", err)
	}
	if !utils.FileExists(lockFile) {
		return fmt.Errorf("lock file %q does not exist", lockFile)
	}
	if err := pushChartImages(
		chartPath,
		chartutils.WithLog(log.SilentLog),
		chartutils.WithContext(ctx),
		chartutils.WithProgressBar(l.ProgressBar()),
	); err != nil {
		return err
	}
	l.Infof("All images pushed successfully")
	if err := l.ExecuteStep("Verifying Images.lock", func() error {
		return verifyLock(chartPath, lockFile)
	}); err != nil {
		return fmt.Errorf("failed to verify Helm chart Images.lock: %w", err)
	}
	l.Infof("Chart %q lock is valid", chartPath)
	return nil
}

func showImagesSummary(chart *chartutils.Chart, l log.SectionLogger) int {
	lock, err := imagelock.FromYAMLFile(filepath.Join(chart.RootDir(), imagelock.DefaultImagesLockFileName))
	if err != nil {
		l.Debugf("failed to load list of images: failed to load lock file: %v", err)
		return 0
	}
	if len(lock.Images) == 0 {
		l.Warnf("The bundle does not include any image")
		return 0
	}
	_ = l.Section(fmt.Sprintf("The wrap includes the following %d images:\n", len(lock.Images)), func(log.SectionLogger) error {
		for _, img := range lock.Images {
			l.Printf(img.Image)
		}
		l.Printf(terminalSpacer)
		return nil
	})
	return len(lock.Images)
}

func untarChart(chartFile string, dir string) (string, error) {
	sandboxDir, err := os.MkdirTemp(dir, "at-wrap*")
	if err != nil {
		return "", fmt.Errorf("failed to create sandbox directory")
	}
	if err := utils.Untar(chartFile, sandboxDir, utils.TarConfig{StripComponents: 1}); err != nil {
		return "", err
	}
	return sandboxDir, nil
}

func normalizeOCIURL(url string) string {
	schemeRe := regexp.MustCompile(`([a-z][a-z0-9+\-.]*)://`)
	if !schemeRe.MatchString(url) {
		return fmt.Sprintf("oci://%s", url)
	}
	return url
}

func pushChart(chart *chartutils.Chart, pushChartURL string) error {
	chartPath := chart.RootDir()
	tmpDir, err := getGlobalTempWorkDir()
	if err != nil {
		return err
	}
	dir, err := os.MkdirTemp(tmpDir, "chart-*")

	if err != nil {
		return fmt.Errorf("failed to upload Helm chart: failed to create temp directory: %w", err)
	}
	tempTarFile := filepath.Join(dir, fmt.Sprintf("%s.tgz", chart.Name()))
	if err := utils.Tar(chartPath, tempTarFile, utils.TarConfig{
		Prefix: chart.Name(),
		Skip:   func(f string) bool { return strings.HasPrefix(f, "/images/") },
	}); err != nil {
		return fmt.Errorf("failed to untar filename %q: %w", chartPath, err)
	}
	return utils.PushChart(tempTarFile, pushChartURL)
}

func init() {
	rootCmd.AddCommand(unwrapCmd)
}
