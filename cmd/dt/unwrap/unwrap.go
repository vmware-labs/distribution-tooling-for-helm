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
	"github.com/vmware-labs/distribution-tooling-for-helm/pkg/imagelock"
	"github.com/vmware-labs/distribution-tooling-for-helm/pkg/log"
	"github.com/vmware-labs/distribution-tooling-for-helm/pkg/log/logrus"
	"github.com/vmware-labs/distribution-tooling-for-helm/pkg/log/silent"
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
	logger                log.SectionLogger
	TempDirectory         string
	Version               string
	Carvelize             bool
	KeepArtifacts         bool
	FetchArtifacts        bool
	Auth                  Auth
	ContainerRegistryAuth Auth
	ValuesFiles           []string

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
func WithLogger(logger log.SectionLogger) func(c *Config) {
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
func (c *Config) GetLogger() log.SectionLogger {
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

// NewConfig returns a new WrapConfig with default values
func NewConfig(opts ...Option) *Config {
	cfg := &Config{
		Context:        context.Background(),
		TempDirectory:  "",
		logger:         logrus.NewSectionLogger(),
		AnnotationsKey: imagelock.DefaultAnnotationsKey,
		Platforms:      []string{},
		ValuesFiles:    []string{"values.yaml"},
	}

	for _, opt := range opts {
		opt(cfg)
	}
	return cfg
}

// Chart unwraps a Helm chart
func Chart(inputChart, registryURL, pushRepository string, pushChartURL string, opts ...Option) (string, *chartutils.Chart, error) {
	return unwrapChart(inputChart, registryURL, pushRepository, pushChartURL, opts...)
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

func unwrapChart(inputChart, registryURL, pushRepository string, pushChartURL string, opts ...Option) (string, *chartutils.Chart, error) {

	cfg := NewConfig(opts...)

	ctx := cfg.Context
	parentLog := cfg.GetLogger()

	if registryURL == "" {
		return "", nil, fmt.Errorf("the registry cannot be empty")
	}

	tempDir, err := cfg.GetTemporaryDirectory()
	if err != nil {
		return "", nil, fmt.Errorf("failed to create temporary directory: %w", err)
	}

	l := parentLog.StartSection(fmt.Sprintf("Unwrapping Helm chart %q", inputChart))

	if cfg.KeepArtifacts {
		l.Debugf("Temporary assets kept at %q", tempDir)
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
		return "", nil, err
	}

	wrap, err := wrapping.Load(chartPath)
	if err != nil {
		return "", nil, err
	}
	if err := l.ExecuteStep(fmt.Sprintf("Relocating %q with prefix %q", wrap.ChartDir(), registryURL), func() error {
		return relocator.RelocateChartDir(
			wrap.ChartDir(), registryURL, relocator.WithLog(l),
			relocator.Recursive, relocator.WithAnnotationsKey(cfg.AnnotationsKey), relocator.WithValuesFiles(cfg.ValuesFiles...),
		)
	}); err != nil {
		return "", nil, l.Failf("failed to relocate %q: %w", chartPath, err)
	}
	l.Infof("Helm chart relocated successfully")

	images := getImageList(wrap, l)

	if len(images) > 0 {
		// If we are not in interactive mode, we do not show the list of images
		if cfg.Interactive {
			showImagesSummary(images, l)
		}
		if askYesNoQuestion(l.PrefixText("Do you want to push the wrapped images to the OCI registry?"), cfg) {
			if err := l.Section("Pushing Images", func(subLog log.SectionLogger) error {
				return pushChartImagesAndVerify(ctx, wrap, registryURL, pushRepository, NewConfig(append(opts, WithLogger(subLog))...))
			}); err != nil {
				return "", nil, l.Failf("Failed to push images: %w", err)
			}
			l.Printf(widgets.TerminalSpacer)
		}
	}

	if askYesNoQuestion(l.PrefixText("Do you want to push the Helm chart to the OCI registry?"), cfg) {

		if pushChartURL == "" {
			if pushRepository == "" {
				// we will push the chart to the same registry as the containers
				pushChartURL = registryURL
			} else {
				// we will push the chart to the same registry as defined by pushRepository
				pushChartURL = pushRepository
			}
			cfg.Auth = cfg.ContainerRegistryAuth
		}
		pushChartURL = normalizeOCIURL(pushChartURL)
		if err := l.ExecuteStep(fmt.Sprintf("Pushing Helm chart to %q", pushChartURL), func() error {
			return utils.ExecuteWithRetry(maxRetries, func(try int, prevErr error) error {
				if try > 0 {
					l.Debugf("Failed to push Helm chart: %v", prevErr)
				}
				return pushChart(ctx, wrap, pushChartURL, cfg)
			})
		}); err != nil {
			return "", nil, l.Failf("Failed to push Helm chart: %w", err)
		}

		l.Infof("Helm chart successfully pushed")

		fullChartURL := fmt.Sprintf("%s/%s", pushChartURL, wrap.Chart().Name())

		// point chart url to the registryURL if pushRepository is set
		if pushRepository != "" && strings.Contains(pushChartURL, pushRepository) {
			fullChartURL = strings.Replace(fullChartURL, pushRepository, registryURL, 1)
		}

		return fullChartURL, wrap.Chart(), nil
	}
	return "", wrap.Chart(), nil
}

func pushChartImagesAndVerify(ctx context.Context, wrap wrapping.Wrap, registryURL string, pushRepository string, cfg *Config) error {
	lockFile := wrap.LockFilePath()

	l := cfg.GetLogger()
	if !utils.FileExists(lockFile) {
		return fmt.Errorf("lock file %q does not exist", lockFile)
	}
	if err := push.ChartImages(
		wrap,
		wrap.ImagesDir(),
		registryURL,
		pushRepository,
		chartutils.WithLog(silent.NewLogger()),
		chartutils.WithContext(ctx),
		chartutils.WithArtifactsDir(wrap.ImageArtifactsDir()),
		chartutils.WithProgressBar(l.ProgressBar()),
		chartutils.WithInsecureMode(cfg.Insecure),
		chartutils.WithAuth(cfg.ContainerRegistryAuth.Username, cfg.ContainerRegistryAuth.Password),
	); err != nil {
		return err
	}
	l.Infof("All images pushed successfully")
	if err := l.ExecuteStep("Verifying Images.lock", func() error {

		return verify.Lock(wrap.ChartDir(), lockFile, verify.Config{
			Insecure: cfg.Insecure, AnnotationsKey: cfg.AnnotationsKey,
			Auth: verify.Auth{Username: cfg.ContainerRegistryAuth.Username, Password: cfg.ContainerRegistryAuth.Password},
		})
	}); err != nil {
		return fmt.Errorf("failed to verify Helm chart Images.lock: %w", err)
	}
	l.Infof("Chart %q lock is valid", wrap.ChartDir())
	return nil
}

func getImageList(wrap wrapping.Lockable, l log.SectionLogger) imagelock.ImageList {
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

func showImagesSummary(images imagelock.ImageList, l log.SectionLogger) {
	_ = l.Section(fmt.Sprintf("The wrap includes the following %d images:\n", len(images)), func(log.SectionLogger) error {
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
	chart := wrap.Chart()
	chartPath := chart.RootDir()
	tmpDir, err := cfg.GetTemporaryDirectory()
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
	d, err := cfg.GetTemporaryDirectory()
	if err != nil {
		return fmt.Errorf("failed to get temp dir: %w", err)
	}
	if err := artifacts.PushChart(tempTarFile, pushChartURL,
		artifacts.WithInsecure(cfg.Insecure), artifacts.WithPlainHTTP(cfg.UsePlainHTTP),
		artifacts.WithRegistryAuth(cfg.Auth.Username, cfg.Auth.Password),
		artifacts.WithTempDir(d),
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

// NewCmd returns a new unwrap command
func NewCmd(cfg *config.Config) *cobra.Command {
	var (
		sayYes         bool
		pushChartURL   string
		version        string
		pushRepository string
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

			fullChartURL, chart, err := unwrapChart(inputChart, registryURL, pushRepository, pushChartURL,
				WithLogger(l),
				WithSayYes(sayYes),
				WithContext(ctx),
				WithVersion(version),
				WithInteractive(true),
				WithInsecure(cfg.Insecure),
				WithTempDirectory(tempDir),
				WithUsePlainHTTP(cfg.UsePlainHTTP),
				WithValuesFiles(valuesFiles...),
			)
			if err != nil {
				return err
			}

			var successMessage = "Helm chart unwrapped successfully"
			if fullChartURL != "" {
				//successMessage = fmt.Sprintf(`%s: You can use it now by running "helm install %s --generate-name"`, successMessage, fullChartURL)
				successMessage = fmt.Sprintf(`%s: "helm install %s --generate-name --version %s"`, successMessage, fullChartURL, chart.Version())
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
	cmd.PersistentFlags().StringVar(&pushRepository, "push-repository", pushRepository, "values repository to be used instead the write repo e.g (demo.goharbor.io/test_repo)")

	return cmd
}
