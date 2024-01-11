package main

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"regexp"

	"github.com/spf13/cobra"
	"github.com/vmware-labs/distribution-tooling-for-helm/internal/log"
	"github.com/vmware-labs/distribution-tooling-for-helm/internal/widgets"
	"github.com/vmware-labs/distribution-tooling-for-helm/pkg/artifacts"
	"github.com/vmware-labs/distribution-tooling-for-helm/pkg/chartutils"
	"github.com/vmware-labs/distribution-tooling-for-helm/pkg/imagelock"
	"github.com/vmware-labs/distribution-tooling-for-helm/pkg/relocator"
	"github.com/vmware-labs/distribution-tooling-for-helm/pkg/utils"
	"github.com/vmware-labs/distribution-tooling-for-helm/pkg/wrapping"
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

			wrap, err := wrapping.Load(chartPath)
			if err != nil {
				return err
			}
			if err := l.ExecuteStep(fmt.Sprintf("Relocating %q with prefix %q", wrap.ChartDir(), registryURL), func() error {
				return relocateChart(wrap.ChartDir(), registryURL, relocator.WithLog(l))
			}); err != nil {
				return l.Failf("failed to relocate %q: %w", chartPath, err)
			}
			l.Infof("Helm chart relocated successfully")

			lenImages := showImagesSummary(wrap, l)

			if lenImages > 0 && (sayYes || widgets.ShowYesNoQuestion(l.PrefixText("Do you want to push the wrapped images to the OCI registry?"))) {
				if err := l.Section("Pushing Images", func(subLog log.SectionLogger) error {
					return pushChartImagesAndVerify(ctx, wrap, subLog)
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
				fullChartURL := fmt.Sprintf("%s/%s", pushChartURL, wrap.Chart().Name())

				if err := l.ExecuteStep(fmt.Sprintf("Pushing Helm chart to %q", pushChartURL), func() error {
					return utils.ExecuteWithRetry(maxRetries, func(try int, prevErr error) error {
						if try > 0 {
							l.Debugf("Failed to push Helm chart: %v", prevErr)
						}
						return pushChart(ctx, wrap, pushChartURL)
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

func pushChartImagesAndVerify(ctx context.Context, wrap wrapping.Wrap, l log.SectionLogger) error {
	lockFile := wrap.LockFilePath()

	if !utils.FileExists(lockFile) {
		return fmt.Errorf("lock file %q does not exist", lockFile)
	}
	if err := pushChartImages(
		wrap,
		wrap.ImagesDir(),
		chartutils.WithLog(log.SilentLog),
		chartutils.WithContext(ctx),
		chartutils.WithArtifactsDir(wrap.ImageArtifactsDir()),
		chartutils.WithProgressBar(l.ProgressBar()),
		chartutils.WithInsecureMode(insecure),
	); err != nil {
		return err
	}
	l.Infof("All images pushed successfully")
	if err := l.ExecuteStep("Verifying Images.lock", func() error {
		return verifyLock(wrap.ChartDir(), lockFile)
	}); err != nil {
		return fmt.Errorf("failed to verify Helm chart Images.lock: %w", err)
	}
	l.Infof("Chart %q lock is valid", wrap.ChartDir())
	return nil
}

func showImagesSummary(wrap wrapping.Lockable, l log.SectionLogger) int {
	lock, err := imagelock.FromYAMLFile(wrap.LockFilePath())
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

func pushChart(ctx context.Context, wrap wrapping.Wrap, pushChartURL string) error {
	chart := wrap.Chart()
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
	}); err != nil {
		return fmt.Errorf("failed to untar filename %q: %w", chartPath, err)
	}
	if err := artifacts.PushChart(tempTarFile, pushChartURL, artifacts.WithInsecure(insecure), artifacts.WithPlainHTTP(usePlainHTTP)); err != nil {
		return err
	}
	fullChartURL := fmt.Sprintf("%s/%s", pushChartURL, chart.Name())

	metadataArtifactDir := filepath.Join(chart.RootDir(), artifacts.HelmChartArtifactMetadataDir)
	if utils.FileExists(metadataArtifactDir) {
		return artifacts.PushChartMetadata(ctx, fmt.Sprintf("%s:%s", fullChartURL, chart.Version()), metadataArtifactDir)
	}
	return nil
}

func init() {
	rootCmd.AddCommand(unwrapCmd)
}
