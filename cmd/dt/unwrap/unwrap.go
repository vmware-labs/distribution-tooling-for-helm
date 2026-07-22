// Package unwrap implements the unwrap command
package unwrap

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"strings"

	"github.com/spf13/cobra"
	"github.com/vmware-labs/distribution-tooling-for-helm/cmd/dt/config"
	"github.com/vmware-labs/distribution-tooling-for-helm/cmd/dt/push"
	"github.com/vmware-labs/distribution-tooling-for-helm/cmd/dt/verify"
	"github.com/vmware-labs/distribution-tooling-for-helm/cmd/dt/wrap"
	"github.com/vmware-labs/distribution-tooling-for-helm/internal/widgets"
	"github.com/vmware-labs/distribution-tooling-for-helm/pkg/artifacts"
	"github.com/vmware-labs/distribution-tooling-for-helm/pkg/chartutils"
	"github.com/vmware-labs/distribution-tooling-for-helm/pkg/dtlog"
	"github.com/vmware-labs/distribution-tooling-for-helm/pkg/dtlog/silent"
	"github.com/vmware-labs/distribution-tooling-for-helm/pkg/imagelock"

	"github.com/vmware-labs/distribution-tooling-for-helm/pkg/dtlog/logrus"

	"github.com/vmware-labs/distribution-tooling-for-helm/pkg/relocator"
	"github.com/vmware-labs/distribution-tooling-for-helm/pkg/utils"
	"github.com/vmware-labs/distribution-tooling-for-helm/pkg/wrapping"
)

var (
	maxRetries = 3
)

// Config defines the configuration for the Wrap/Unwrap command
type Config struct {
	Context               context.Context
	AnnotationsKey        string
	UsePlainHTTP          bool
	Insecure              bool
	Platforms             []string
	logger                dtlog.SectionLogger
	TempDirectory         string
	Version               string
	Carvelize             bool
	SkipImageRelocation   bool
	SkipPullImages        bool
	KeepArtifacts         bool
	FetchArtifacts        bool
	Auth                  Auth
	ContainerRegistryAuth Auth
	ValuesFiles           []string
	PreserveRepository    bool
	PreserveDigest        bool

	// Interactive enables interacting with the user
	Interactive bool
	SayYes      bool
}

// Auth defines the authentication information to access the container registry
type Auth struct {
	Username string
	Password string
}

// WithAuth configures the Auth of the unwrap Config
func WithAuth(username, password string) func(c *Config) {
	return func(c *Config) {
		c.Auth = Auth{
			Username: username,
			Password: password,
		}
	}
}

// WithContainerRegistryAuth configures the ContainerRegistryAuth of the unwrap Config
func WithContainerRegistryAuth(username, password string) func(c *Config) {
	return func(c *Config) {
		c.ContainerRegistryAuth = Auth{
			Username: username,
			Password: password,
		}
	}
}

// WithSayYes configures the SayYes of the WrapConfig
func WithSayYes(sayYes bool) func(c *Config) {
	return func(c *Config) {
		c.SayYes = sayYes
	}
}

// WithKeepArtifacts configures the KeepArtifacts of the WrapConfig
func WithKeepArtifacts(keepArtifacts bool) func(c *Config) {
	return func(c *Config) {
		c.KeepArtifacts = keepArtifacts
	}
}

// WithInteractive configures the Interactive of the WrapConfig
func WithInteractive(interactive bool) func(c *Config) {
	return func(c *Config) {
		c.Interactive = interactive
	}
}

// ShouldFetchChartArtifacts returns true if the chart artifacts should be fetched
func (c *Config) ShouldFetchChartArtifacts(inputChart string) bool {
	if chartutils.IsRemoteChart(inputChart) {
		return c.FetchArtifacts
	}
	return false
}

// Option defines a WrapOpts setting
type Option func(*Config)

// WithInsecure configures the InsecureMode of the WrapConfig
func WithInsecure(insecure bool) func(c *Config) {
	return func(c *Config) {
		c.Insecure = insecure
	}
}

// WithUsePlainHTTP configures the UsePlainHTTP of the WrapConfig
func WithUsePlainHTTP(usePlainHTTP bool) func(c *Config) {
	return func(c *Config) {
		c.UsePlainHTTP = usePlainHTTP
	}
}

// WithAnnotationsKey configures the AnnotationsKey of the WrapConfig
func WithAnnotationsKey(annotationsKey string) func(c *Config) {
	return func(c *Config) {
		c.AnnotationsKey = annotationsKey
	}
}

// WithCarvelize configures the Carvelize of the WrapConfig
func WithCarvelize(carvelize bool) func(c *Config) {
	return func(c *Config) {
		c.Carvelize = carvelize
	}
}

// WithSkipImageRelocation configures the WithSkipImageRelocation of the WrapConfig
func WithSkipImageRelocation(skipImageRelocation bool) func(c *Config) {
	return func(c *Config) {
		c.SkipImageRelocation = skipImageRelocation
	}
}

// WithSkipPullImages configures the WithSkipPullImages of the WrapConfig
func WithSkipPullImages(skipPullImages bool) func(c *Config) {
	return func(c *Config) {
		c.SkipPullImages = skipPullImages
	}
}

// WithFetchArtifacts configures the FetchArtifacts of the WrapConfig
func WithFetchArtifacts(fetchArtifacts bool) func(c *Config) {
	return func(c *Config) {
		c.FetchArtifacts = fetchArtifacts
	}
}

// WithVersion configures the Version of the WrapConfig
func WithVersion(version string) func(c *Config) {
	return func(c *Config) {
		c.Version = version
	}
}

// WithLogger configures the Logger of the WrapConfig
func WithLogger(logger dtlog.SectionLogger) func(c *Config) {
	return func(c *Config) {
		c.logger = logger
	}
}

// WithContext configures the Context of the WrapConfig
func WithContext(ctx context.Context) func(c *Config) {
	return func(c *Config) {
		c.Context = ctx
	}
}

// GetTemporaryDirectory returns the temporary directory of the WrapConfig
func (c *Config) GetTemporaryDirectory() (string, error) {
	if c.TempDirectory != "" {
		return c.TempDirectory, nil
	}
	return config.GetGlobalTempWorkDir()
}

// GetLogger returns the logger of the WrapConfig
func (c *Config) GetLogger() dtlog.SectionLogger {
	if c.logger != nil {
		return c.logger
	}
	return logrus.NewSectionLogger()
}

// WithPlatforms configures the Platforms of the WrapConfig
func WithPlatforms(platforms []string) func(c *Config) {
	return func(c *Config) {
		c.Platforms = platforms
	}
}

// WithTempDirectory configures the TempDirectory of the WrapConfig
func WithTempDirectory(tempDir string) func(c *Config) {
	return func(c *Config) {
		c.TempDirectory = tempDir
	}
}

// WithValuesFiles configures the values files of the wrapped chart
func WithValuesFiles(files ...string) func(c *Config) {
	return func(c *Config) {
		c.ValuesFiles = files
	}
}

// WithPreserveRepository configures the PreserveRepository of the Config
func WithPreserveRepository(preserve bool) func(c *Config) {
	return func(c *Config) {
		c.PreserveRepository = preserve
	}
}

// WithPreserveDigest configures the PreserveDigest of the Config
func WithPreserveDigest(preserveDigest bool) func(c *Config) {
	return func(c *Config) {
		c.PreserveDigest = preserveDigest
	}
}

// NewConfig returns a new WrapConfig with default values
func NewConfig(opts ...Option) *Config {
	cfg := &Config{
		Context:            context.Background(),
		TempDirectory:      "",
		logger:             logrus.NewSectionLogger(),
		AnnotationsKey:     imagelock.DefaultAnnotationsKey,
		Platforms:          []string{},
		ValuesFiles:        []string{"values.yaml"},
		PreserveRepository: true,
	}

	for _, opt := range opts {
		opt(cfg)
	}
	return cfg
}

// Chart unwraps a Helm chart
func Chart(inputChart, registryURL, pushChartURL string, opts ...Option) (string, error) {
	return unwrapChart(inputChart, registryURL, pushChartURL, opts...)
}

// Container unwraps a container image
func Container(inputContainer, registryURL string, opts ...Option) (string, error) {
	return unwrapContainer(inputContainer, registryURL, opts...)
}

func askYesNoQuestion(msg string, cfg *Config) bool {
	if cfg.SayYes {
		return true
	}
	if !cfg.Interactive {
		return false
	}
	return widgets.ShowYesNoQuestion(msg)
}

func unwrapChart(inputChart, registryURL, pushChartURL string, opts ...Option) (string, error) {

	cfg := NewConfig(opts...)

	ctx := cfg.Context
	parentLog := cfg.GetLogger()

	if registryURL == "" {
		return "", fmt.Errorf("the registry cannot be empty")
	}

	tempDir, err := cfg.GetTemporaryDirectory()
	if err != nil {
		return "", fmt.Errorf("failed to create temporary directory: %w", err)
	}

	l := parentLog.StartSection(fmt.Sprintf("Unwrapping Helm chart %q", inputChart))

	if cfg.KeepArtifacts {
		l.Debugf("Temporary assets kept at %q", tempDir)
	}

	if cfg.PreserveDigest && !cfg.SkipImageRelocation {
		l.Debugf("--preserve-digest implies --skip-image-relocation: no chart content will be rewritten")
		cfg.SkipImageRelocation = true
	}

	chartPath, err := wrap.ResolveInputChartPath(
		inputChart,
		wrap.NewConfig(
			wrap.WithTempDirectory(cfg.TempDirectory),
			wrap.WithLogger(l),
			wrap.WithVersion(cfg.Version),
			wrap.WithInsecure(cfg.Insecure),
			wrap.WithUsePlainHTTP(cfg.UsePlainHTTP),
		),
	)
	if err != nil {
		return "", fmt.Errorf("failed to resolve input chart path: %w", err)
	}

	wrap, err := wrapping.Load(chartPath)
	if err != nil {
		return "", err
	}
	if err := l.ExecuteStep(fmt.Sprintf("Relocating %q with prefix %q", wrap.ChartDir(), registryURL), func() error {
		return relocator.RelocateChartDir(
			wrap.ChartDir(), registryURL, relocator.WithLog(l),
			relocator.Recursive, relocator.WithAnnotationsKey(cfg.AnnotationsKey), relocator.WithValuesFiles(cfg.ValuesFiles...),
			relocator.WithSkipImageRelocation(cfg.SkipImageRelocation),
			relocator.WithPreserveRepository(cfg.PreserveRepository),
		)
	}); err != nil {
		return "", l.Failf("failed to relocate %q: %w", chartPath, err)
	}
	l.Infof("Helm chart relocated successfully")

	images := getImageList(wrap, l)

	if len(images) > 0 && !cfg.SkipPullImages {
		// If we are not in interactive mode, we do not show the list of images
		if cfg.Interactive {
			showImagesSummary(images, l)
		}
		if askYesNoQuestion(l.PrefixText("Do you want to push the wrapped images to the OCI registry?"), cfg) {
			if err := l.Section("Pushing Images", func(subLog dtlog.SectionLogger) error {
				return pushChartImagesAndVerify(ctx, wrap, registryURL, NewConfig(append(opts, WithLogger(subLog))...))
			}); err != nil {
				return "", l.Failf("Failed to push images: %w", err)
			}
			l.Printf(widgets.TerminalSpacer)
		}
	}

	if askYesNoQuestion(l.PrefixText("Do you want to push the Helm chart to the OCI registry?"), cfg) {
		if pushChartURL == "" {
			pushChartURL = registryURL
			// we will push the chart to the same registry as the containers
			cfg.Auth = cfg.ContainerRegistryAuth
		}
		pushChartURL = normalizeOCIURL(pushChartURL)
		fullChartURL := fmt.Sprintf("%s/%s", pushChartURL, wrap.Chart().Name())

		if err := l.ExecuteStep(fmt.Sprintf("Pushing Helm chart to %q", pushChartURL), func() error {
			return utils.ExecuteWithRetry(maxRetries, func(try int, prevErr error) error {
				if try > 0 {
					l.Debugf("Failed to push Helm chart: %v", prevErr)
				}
				return pushChart(ctx, wrap, pushChartURL, cfg)
			})
		}); err != nil {
			return "", l.Failf("Failed to push Helm chart: %w", err)
		}

		l.Infof("Helm chart successfully pushed")
		return fullChartURL, nil
	}
	return "", nil
}

func unwrapContainer(inputContainer, registryURL string, opts ...Option) (string, error) {
	cfg := NewConfig(opts...)

	ctx := cfg.Context
	parentLog := cfg.GetLogger()

	if registryURL == "" {
		return "", fmt.Errorf("the registry cannot be empty")
	}

	tempDir, err := cfg.GetTemporaryDirectory()
	if err != nil {
		return "", fmt.Errorf("failed to create temporary directory: %w", err)
	}

	l := parentLog.StartSection(fmt.Sprintf("Unwrapping container image %q", inputContainer))

	if cfg.KeepArtifacts {
		l.Debugf("Temporary assets kept at %q", tempDir)
	}

	containerPath, err := wrap.ResolveInputContainerPath(
		inputContainer,
		wrap.NewConfig(
			wrap.WithTempDirectory(cfg.TempDirectory),
			wrap.WithLogger(l),
			wrap.WithVersion(cfg.Version),
			wrap.WithInsecure(cfg.Insecure),
			wrap.WithUsePlainHTTP(cfg.UsePlainHTTP),
		),
	)
	if err != nil {
		return "", fmt.Errorf("failed to resolve input container path: %w", err)
	}

	wrapContainer, err := wrapping.LoadContainer(containerPath)
	if err != nil {
		return "", err
	}
	images := getImageList(wrapContainer, l)

	lockFile := wrapContainer.LockFilePath()
	if utils.FileExists(lockFile) {
		// For standalone container images we relocate to just <registry>/<image-name>,
		// dropping any source repository path. Unlike chart wraps, there is no meaningful
		// namespace to preserve from the original registry.
		err = relocator.RelocateLockFile(lockFile, registryURL, false)
		if err != nil {
			return "", fmt.Errorf("failed to relocate Images.lock file: %v", err)
		}
	}

	if len(images) > 0 && !cfg.SkipPullImages {
		// If we are not in interactive mode, we do not show the list of images
		if cfg.Interactive {
			showImagesSummary(images, l)
		}
		if askYesNoQuestion(l.PrefixText("Do you want to push the wrapped images to the OCI registry?"), cfg) {
			if err := l.Section("Pushing Images", func(subLog dtlog.SectionLogger) error {
				return pushImages(ctx, wrapContainer, NewConfig(append(opts, WithLogger(subLog))...))
			}); err != nil {
				return "", l.Failf("Failed to push images: %w", err)
			}
			l.Printf(widgets.TerminalSpacer)
		}
	}
	return "", nil
}

func pushChartImagesAndVerify(ctx context.Context, wrap wrapping.Wrap, registryURL string, cfg *Config) error {
	lockFile := wrap.LockFilePath()

	l := cfg.GetLogger()
	if !utils.FileExists(lockFile) {
		return fmt.Errorf("lock file %q does not exist", lockFile)
	}

	if cfg.PreserveDigest {
		// --preserve-digest forces SkipImageRelocation, which also skips
		// relocating Images.lock itself (both live behind the same gate in
		// relocator.relocateChart). Images still need to land at
		// registryURL, so relocate the lock file directly here, mirroring
		// what unwrapContainer already does unconditionally.
		if err := relocator.RelocateLockFile(lockFile, strings.TrimPrefix(registryURL, "oci://"), cfg.PreserveRepository); err != nil {
			return fmt.Errorf("failed to relocate Images.lock file: %w", err)
		}
	}

	if err := push.ChartImages(
		wrap,
		wrap.ImagesDir(),
		chartutils.WithLog(silent.NewLogger()),
		chartutils.WithContext(ctx),
		chartutils.WithArtifactsDir(wrap.ImageArtifactsDir()),
		chartutils.WithProgressBar(l.ProgressBar()),
		chartutils.WithInsecureMode(cfg.Insecure),
		chartutils.WithAuth(cfg.ContainerRegistryAuth.Username, cfg.ContainerRegistryAuth.Password),
		chartutils.WithPreserveDigest(cfg.PreserveDigest),
	); err != nil {
		return err
	}
	l.Infof("All images pushed successfully")

	if cfg.PreserveDigest {
		// The chart's own declared image references were deliberately left
		// unrelocated to preserve its byte-for-byte digest, so they no
		// longer match the Images.lock just relocated above. Digest
		// correctness against the source was already checked at pull time
		// (pullImageVerbatim -> VerifyLockedDigests), so the usual
		// values.yaml-vs-Images.lock cross-check doesn't apply here.
		return nil
	}

	if err := l.ExecuteStep("Verifying Images.lock", func() error {

		return verify.Lock(wrap.ChartDir(), lockFile, verify.Config{
			Insecure: cfg.Insecure, AnnotationsKey: cfg.AnnotationsKey,
			PreserveRepository: cfg.PreserveRepository,
			Auth:               verify.Auth{Username: cfg.ContainerRegistryAuth.Username, Password: cfg.ContainerRegistryAuth.Password},
		})
	}); err != nil {
		return fmt.Errorf("failed to verify Helm chart Images.lock: %w", err)
	}
	l.Infof("Chart %q lock is valid", wrap.ChartDir())
	return nil
}

func pushImages(ctx context.Context, wrap wrapping.WrapContainer, cfg *Config) error {
	lockFile := wrap.LockFilePath()

	l := cfg.GetLogger()
	if !utils.FileExists(lockFile) {
		return fmt.Errorf("lock file %q does not exist", lockFile)
	}
	lock, err := wrap.GetImagesLock()
	if err != nil {
		return err
	}
	return chartutils.PushImages(lock, wrap.ImagesDir(), chartutils.WithLog(l),
		chartutils.WithContext(ctx),
		chartutils.WithArtifactsDir(wrap.ImageArtifactsDir()),
		chartutils.WithProgressBar(l.ProgressBar()),
		chartutils.WithInsecureMode(cfg.Insecure),
		chartutils.WithAuth(cfg.ContainerRegistryAuth.Username, cfg.ContainerRegistryAuth.Password),
		chartutils.WithPreserveDigest(cfg.PreserveDigest))
}

func getImageList(wrap wrapping.Lockable, l dtlog.SectionLogger) imagelock.ImageList {
	lock, err := wrap.GetImagesLock()

	if err != nil {
		l.Debugf("failed to load list of images: failed to load lock file: %v", err)
		return imagelock.ImageList{}
	}
	if len(lock.Images) == 0 {
		l.Warnf("The bundle does not include any image")
		return imagelock.ImageList{}
	}
	return lock.Images
}

func showImagesSummary(images imagelock.ImageList, l dtlog.SectionLogger) {
	_ = l.Section(fmt.Sprintf("The wrap includes the following %d images:\n", len(images)), func(dtlog.SectionLogger) error {
		for _, img := range images {
			l.Printf(img.Image)
		}
		l.Printf(widgets.TerminalSpacer)
		return nil
	})
}

func normalizeOCIURL(url string) string {
	schemeRe := regexp.MustCompile(`([a-z][a-z0-9+\-.]*)://`)
	if !schemeRe.MatchString(url) {
		return fmt.Sprintf("oci://%s", url)
	}
	return url
}

func pushChart(ctx context.Context, wrap wrapping.Wrap, pushChartURL string, cfg *Config) error {
	if cfg.PreserveDigest {
		return pushPreservedChart(ctx, wrap, pushChartURL, cfg)
	}

	var tmpDir, dir string
	var err error
	tmpDir, err = cfg.GetTemporaryDirectory()
	if err != nil {
		return fmt.Errorf("failed to get temp dir: %w", err)
	}

	dir, err = os.MkdirTemp(tmpDir, "chart-*")
	if err != nil {
		return fmt.Errorf("failed to create temp directory: %w", err)
	}

	chart := wrap.Chart()
	chartPath := chart.RootDir()
	tempTarFile := filepath.Join(dir, fmt.Sprintf("%s.tgz", chart.Name()))
	if err = utils.Tar(chartPath, tempTarFile, utils.TarConfig{
		Prefix: chart.Name(),
	}); err != nil {
		return fmt.Errorf("failed to untar filename %q: %w", chartPath, err)
	}

	tmpDir, err = cfg.GetTemporaryDirectory()
	if err != nil {
		return fmt.Errorf("failed to get temp dir: %w", err)
	}

	if err = artifacts.PushChart(tempTarFile, pushChartURL,
		artifacts.WithInsecure(cfg.Insecure),
		artifacts.WithPlainHTTP(cfg.UsePlainHTTP),
		artifacts.WithRegistryAuth(cfg.Auth.Username, cfg.Auth.Password),
		artifacts.WithTempDir(tmpDir),
	); err != nil {
		return err
	}
	fullChartURL := fmt.Sprintf("%s/%s", pushChartURL, chart.Name())

	metadataArtifactDir := filepath.Join(chart.RootDir(), artifacts.HelmChartArtifactMetadataDir)
	if utils.FileExists(metadataArtifactDir) {
		return artifacts.PushChartMetadata(ctx, fmt.Sprintf("%s:%s", fullChartURL, chart.Version()), metadataArtifactDir, artifacts.WithAuth(cfg.Auth.Username, cfg.Auth.Password))
	}
	return nil
}

// pushPreservedChart pushes the chart artifact captured by
// `dt wrap --preserve-digest` (see capturePreservedChart in cmd/dt/wrap)
// exactly as stored, bypassing Helm's push action entirely when a raw OCI
// manifest was captured. This matters because Helm's own registry.Client.Push
// always stamps a fresh creation-time annotation into a freshly-built
// manifest on every push, so pushing through it can never reproduce the
// source's original manifest digest, even given byte-identical tgz content.
func pushPreservedChart(ctx context.Context, wrap wrapping.Wrap, pushChartURL string, cfg *Config) error {
	chart := wrap.Chart()
	fullChartURL := fmt.Sprintf("%s/%s", pushChartURL, chart.Name())
	dstRef := fmt.Sprintf("%s:%s", strings.TrimPrefix(fullChartURL, "oci://"), chart.Version())

	chartOCIDir := filepath.Join(wrap.RootDir(), "chart.oci")
	chartTgzPath := filepath.Join(wrap.RootDir(), "chart.tgz")

	switch {
	case utils.FileExists(chartOCIDir):
		// Source was oci://: push the exact original manifest+config+layer
		// bytes, reproducing the source's manifest digest.
		if err := artifacts.PushVerbatim(ctx, chartOCIDir, dstRef,
			artifacts.WithAuth(cfg.Auth.Username, cfg.Auth.Password),
			artifacts.WithInsecureMode(cfg.Insecure)); err != nil {
			return fmt.Errorf("failed to push preserved chart artifact: %w", err)
		}
	case utils.FileExists(chartTgzPath):
		// Source was a local .tgz: there is no pre-existing OCI manifest to
		// preserve, but the tgz blob itself is the pristine original bytes.
		tmpDir, err := cfg.GetTemporaryDirectory()
		if err != nil {
			return fmt.Errorf("failed to get temp dir: %w", err)
		}
		if err := artifacts.PushChart(chartTgzPath, pushChartURL,
			artifacts.WithInsecure(cfg.Insecure),
			artifacts.WithPlainHTTP(cfg.UsePlainHTTP),
			artifacts.WithRegistryAuth(cfg.Auth.Username, cfg.Auth.Password),
			artifacts.WithTempDir(tmpDir),
		); err != nil {
			return err
		}
	default:
		return fmt.Errorf(
			"bundle %q does not contain a preserved chart artifact; re-run \"dt wrap --preserve-digest\" to produce a compatible bundle",
			wrap.RootDir())
	}

	metadataArtifactDir := filepath.Join(chart.RootDir(), artifacts.HelmChartArtifactMetadataDir)
	if utils.FileExists(metadataArtifactDir) {
		return artifacts.PushChartMetadata(ctx, fmt.Sprintf("%s:%s", fullChartURL, chart.Version()), metadataArtifactDir, artifacts.WithAuth(cfg.Auth.Username, cfg.Auth.Password))
	}
	return nil
}

// NewCmd returns a new unwrap command
func NewCmd(cfg *config.Config) *cobra.Command {
	var (
		sayYes              bool
		pushChartURL        string
		version             string
		skipImageRelocation bool
		skipPullImages      bool
		preserveDigest      bool
	)
	valuesFiles := []string{"values.yaml"}
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
		RunE: func(_ *cobra.Command, args []string) error {
			l := cfg.Logger()

			inputChart, registryURL := args[0], args[1]
			ctx, cancel := cfg.ContextWithSigterm()
			defer cancel()

			tempDir, err := cfg.GetTemporaryDirectory()
			if err != nil {
				return fmt.Errorf("failed to create temporary directory: %v", err)
			}
			fullChartURL, err := unwrapChart(inputChart, registryURL, pushChartURL,
				WithLogger(l),
				WithSayYes(sayYes),
				WithContext(ctx),
				WithVersion(version),
				WithInteractive(true),
				WithAnnotationsKey(cfg.AnnotationsKey),
				WithInsecure(cfg.Insecure),
				WithTempDirectory(tempDir),
				WithUsePlainHTTP(cfg.UsePlainHTTP),
				WithValuesFiles(valuesFiles...),
				WithSkipImageRelocation(skipImageRelocation),
				WithSkipPullImages(skipPullImages),
				WithPreserveDigest(preserveDigest),
			)
			if err != nil {
				return err
			}
			var successMessage = "Helm chart unwrapped successfully"
			if fullChartURL != "" {
				successMessage = fmt.Sprintf(`%s: You can use it now by running "helm install %s --generate-name"`, successMessage, fullChartURL)
			}
			l.Printf(widgets.TerminalSpacer)
			l.Successf(successMessage)
			return nil
		},
	}

	cmd.PersistentFlags().StringVar(&version, "version", version, "when unwrapping remote Helm charts from OCI, version to request")
	cmd.PersistentFlags().StringVar(&pushChartURL, "push-chart-url", pushChartURL, "push the unwrapped Helm chart to the given URL")
	cmd.PersistentFlags().BoolVar(&sayYes, "yes", sayYes, "respond 'yes' to any yes/no question")
	cmd.PersistentFlags().StringSliceVar(&valuesFiles, "values", valuesFiles, "values files to relocate images (can specify multiple)")
	cmd.PersistentFlags().BoolVar(&skipImageRelocation, "skip-image-relocation", skipImageRelocation, "Skip relocating image references in the different files")
	cmd.PersistentFlags().BoolVar(&skipPullImages, "skip-pull-images", skipPullImages, "Skip pulling images")
	cmd.PersistentFlags().BoolVar(&preserveDigest, "preserve-digest", preserveDigest,
		"push the chart and its images byte-for-byte so their digests are unchanged. "+
			"Implies --skip-image-relocation, and requires a bundle produced by \"dt wrap --preserve-digest\"")

	return cmd
}

// NewContainerCmd returns a new unwrap command for container images
func NewContainerCmd(cfg *config.Config) *cobra.Command {
	var sayYes bool
	var preserveDigest bool
	cmd := &cobra.Command{
		Use:   "unwrap FILE OCI_REF",
		Short: "Unwraps a wrapped container image",
		Long:  "Unwraps a container image wrap tarball and pushes the image and its artifacts into a target OCI registry",
		Example: `  # Unwrap a container image and push it into a registry
  $ dt images unwrap nginx-1.25.container.wrap.tgz oci://demo.goharbor.io/myrepo
`,
		SilenceUsage:  true,
		SilenceErrors: true,
		Args:          cobra.ExactArgs(2),
		RunE: func(_ *cobra.Command, args []string) error {
			l := cfg.Logger()

			inputContainer, registryURL := args[0], args[1]
			ctx, cancel := cfg.ContextWithSigterm()
			defer cancel()

			tempDir, err := cfg.GetTemporaryDirectory()
			if err != nil {
				return fmt.Errorf("failed to create temporary directory: %v", err)
			}
			_, err = unwrapContainer(inputContainer, registryURL,
				WithLogger(l),
				WithSayYes(sayYes),
				WithContext(ctx),
				WithInsecure(cfg.Insecure),
				WithTempDirectory(tempDir),
				WithUsePlainHTTP(cfg.UsePlainHTTP),
				WithInteractive(true),
				WithPreserveDigest(preserveDigest),
			)
			if err != nil {
				return err
			}
			l.Printf(widgets.TerminalSpacer)
			l.Successf("Container image unwrapped successfully")
			return nil
		},
	}

	cmd.PersistentFlags().BoolVar(&sayYes, "yes", sayYes, "respond 'yes' to any yes/no question")
	cmd.PersistentFlags().BoolVar(&preserveDigest, "preserve-digest", preserveDigest,
		"push the image byte-for-byte instead of relocating/re-packaging it, so its digest (and any signature made against it) remains unchanged")

	return cmd
}
