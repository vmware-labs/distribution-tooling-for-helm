// Package wrap implements the command to wrap a Helm chart
package wrap

import (
	"context"
	"fmt"
	"os"
	"path/filepath"

	"github.com/spf13/cobra"
	"github.com/vmware-labs/distribution-tooling-for-helm/cmd/dt/carvelize"
	"github.com/vmware-labs/distribution-tooling-for-helm/cmd/dt/config"
	"github.com/vmware-labs/distribution-tooling-for-helm/cmd/dt/lock"
	"github.com/vmware-labs/distribution-tooling-for-helm/cmd/dt/pull"
	"github.com/vmware-labs/distribution-tooling-for-helm/internal/widgets"
	"github.com/vmware-labs/distribution-tooling-for-helm/pkg/artifacts"
	"github.com/vmware-labs/distribution-tooling-for-helm/pkg/chartutils"
	"github.com/vmware-labs/distribution-tooling-for-helm/pkg/imagelock"
	"github.com/vmware-labs/distribution-tooling-for-helm/pkg/log"
	"github.com/vmware-labs/distribution-tooling-for-helm/pkg/log/silent"

	"github.com/vmware-labs/distribution-tooling-for-helm/pkg/log/logrus"

	"github.com/vmware-labs/distribution-tooling-for-helm/pkg/utils"
	"github.com/vmware-labs/distribution-tooling-for-helm/pkg/wrapping"
)

// Auth defines the authentication information to access the container registry
type Auth struct {
	Username string
	Password string
}

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
	SkipImages            bool
	Auth                  Auth
	ContainerRegistryAuth Auth
	OutputFile            string
}

// WithKeepArtifacts configures the KeepArtifacts of the WrapConfig
func WithKeepArtifacts(keepArtifacts bool) func(c *Config) {
	return func(c *Config) {
		c.KeepArtifacts = keepArtifacts
	}
}

// WithOutputFile configures the OutputFile of the WrapConfig
func WithOutputFile(outputFile string) func(c *Config) {
	return func(c *Config) {
		c.OutputFile = outputFile
	}
}

// WithAuth configures the Auth of the wrap Config
func WithAuth(username, password string) func(c *Config) {
	return func(c *Config) {
		c.Auth = Auth{
			Username: username,
			Password: password,
		}
	}
}

// WithContainerRegistryAuth configures the Auth of the wrap Config
func WithContainerRegistryAuth(username, password string) func(c *Config) {
	return func(c *Config) {
		c.ContainerRegistryAuth = Auth{
			Username: username,
			Password: password,
		}
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

// WithSkipImages configures the WithSkipImages of the WrapConfig
func WithSkipImages(skipimages bool) func(c *Config) {
	return func(c *Config) {
		c.SkipImages = skipimages
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

	dir, err := os.MkdirTemp("", "chart-*")
	if err != nil {
		return "", err
	}
	c.TempDirectory = dir
	return dir, nil
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

// NewConfig returns a new WrapConfig with default values
func NewConfig(opts ...Option) *Config {
	cfg := &Config{
		Context:        context.Background(),
		TempDirectory:  "",
		logger:         logrus.NewSectionLogger(),
		AnnotationsKey: imagelock.DefaultAnnotationsKey,
		Platforms:      []string{},
	}

	for _, opt := range opts {
		opt(cfg)
	}
	return cfg
}

// Chart wraps a Helm chart
func Chart(inputPath string, opts ...Option) (string, error) {
	return wrapChart(inputPath, opts...)
}

// ResolveInputChartPath resolves the input chart into a local uncompressed chart path
func ResolveInputChartPath(inputPath string, cfg *Config) (string, error) {
	l := cfg.GetLogger()
	var chartPath string
	var err error

	tmpDir, err := cfg.GetTemporaryDirectory()
	if err != nil {
		return "", fmt.Errorf("failed to create temporary directory: %w", err)
	}

	if chartutils.IsRemoteChart(inputPath) {
		if err := l.ExecuteStep("Fetching remote Helm chart", func() error {
			version := cfg.Version

			chartPath, err = fetchRemoteChart(inputPath, version, tmpDir, cfg)
			if err != nil {
				return err
			}
			return nil
		}); err != nil {
			return "", l.Failf("Failed to download Helm chart: %w", err)
		}
		l.Infof("Helm chart downloaded to %q", chartPath)
	} else if isTar, _ := utils.IsTarFile(inputPath); isTar {
		if err := l.ExecuteStep("Uncompressing Helm chart", func() error {
			var err error
			chartPath, err = untarChart(inputPath, tmpDir)
			return err
		}); err != nil {
			return "", l.Failf("Failed to uncompress %q: %w", inputPath, err)
		}
		l.Infof("Helm chart uncompressed to %q", chartPath)
	} else {
		chartPath = inputPath
	}

	return chartPath, nil
}

func untarChart(chartFile string, dir string) (string, error) {
	sandboxDir, err := os.MkdirTemp(dir, "dt-wrap*")
	if err != nil {
		return "", fmt.Errorf("failed to create sandbox directory")
	}
	if err := utils.Untar(chartFile, sandboxDir, utils.TarConfig{StripComponents: 1}); err != nil {
		return "", err
	}
	return sandboxDir, nil
}

func fetchRemoteChart(chartURL string, version string, dir string, cfg *Config) (string, error) {
	d, err := cfg.GetTemporaryDirectory()
	if err != nil {
		return "", err
	}
	chartPath, err := artifacts.PullChart(
		chartURL, version, dir,
		artifacts.WithInsecure(cfg.Insecure),
		artifacts.WithPlainHTTP(cfg.UsePlainHTTP),
		artifacts.WithRegistryAuth(cfg.Auth.Username, cfg.Auth.Password),
		artifacts.WithTempDir(d),
	)
	if err != nil {
		return "", err
	}
	return chartPath, nil
}

func validateWrapLock(wrap wrapping.Wrap, cfg *Config) error {
	l := cfg.GetLogger()
	chart := wrap.Chart()

	lockFile := wrap.LockFilePath()
	if utils.FileExists(lockFile) {
		if err := l.ExecuteStep("Verifying Images.lock", func() error {
			return wrap.VerifyLock(imagelock.WithAnnotationsKey(cfg.AnnotationsKey),
				imagelock.WithContext(cfg.Context),
				imagelock.WithAuth(cfg.ContainerRegistryAuth.Username, cfg.ContainerRegistryAuth.Password),
				imagelock.WithInsecure(cfg.Insecure))
		}); err != nil {
			return l.Failf("Failed to verify lock: %w", err)
		}
		l.Infof("Helm chart %q lock is valid", chart.RootDir())
	} else {
		if err := l.ExecuteStep(
			"Images.lock file does not exist. Generating it from annotations...",
			func() error {
				return lock.Create(chart.RootDir(), lockFile, silent.NewLogger(),
					imagelock.WithAnnotationsKey(cfg.AnnotationsKey),
					imagelock.WithInsecure(cfg.Insecure),
					imagelock.WithAuth(cfg.ContainerRegistryAuth.Username, cfg.ContainerRegistryAuth.Password),
					imagelock.WithPlatforms(cfg.Platforms),
					imagelock.WithContext(cfg.Context),
				)
			},
		); err != nil {
			return l.Failf("Failed to generate lock: %w", err)
		}
		l.Infof("Images.lock file written to %q", lockFile)
	}
	return nil
}

func fetchArtifacts(chartURL string, destDir string, cfg *Config) error {
	if err := artifacts.FetchChartMetadata(
		context.Background(), chartURL,
		destDir, artifacts.WithAuth(cfg.Auth.Username, cfg.Auth.Password),
	); err != nil && err != artifacts.ErrTagDoesNotExist {
		return fmt.Errorf("failed to fetch chart remote metadata: %w", err)
	}
	return nil
}

func pullImages(wrap wrapping.Wrap, cfg *Config) error {
	l := cfg.GetLogger()

	lock, err := wrap.GetImagesLock()
	if err != nil {
		return l.Failf("Failed to load Images.lock: %v", err)
	}
	if len(lock.Images) == 0 {
		l.Warnf("No images found in Images.lock")
	} else {
		return l.Section(fmt.Sprintf("Pulling images into %q", wrap.ImagesDir()), func(childLog log.SectionLogger) error {
			if err := pull.ChartImages(
				wrap,
				wrap.ImagesDir(),
				chartutils.WithLog(childLog),
				chartutils.WithContext(cfg.Context),
				chartutils.WithFetchArtifacts(cfg.FetchArtifacts),
				chartutils.WithAuth(cfg.ContainerRegistryAuth.Username, cfg.ContainerRegistryAuth.Password),
				chartutils.WithArtifactsDir(wrap.ImageArtifactsDir()),
				chartutils.WithProgressBar(childLog.ProgressBar()),
				chartutils.WithInsecureMode(cfg.Insecure),
			); err != nil {
				return childLog.Failf("%v", err)
			}
			childLog.Infof("All images pulled successfully")
			return nil
		})
	}
	return nil
}

func wrapChart(inputPath string, opts ...Option) (string, error) {
	cfg := NewConfig(opts...)

	ctx := cfg.Context
	parentLog := cfg.GetLogger()

	l := parentLog.StartSection(fmt.Sprintf("Wrapping Helm chart %q", inputPath))

	subCfg := NewConfig(append(opts, WithLogger(l))...)

	chartPath, err := ResolveInputChartPath(inputPath, subCfg)
	if err != nil {
		return "", err
	}
	tmpDir, err := cfg.GetTemporaryDirectory()
	if err != nil {
		return "", fmt.Errorf("failed to create temporary directory: %w", err)
	}
	wrap, err := wrapping.Create(chartPath, filepath.Join(tmpDir, "wrap"),
		chartutils.WithAnnotationsKey(cfg.AnnotationsKey),
	)
	if err != nil {
		return "", l.Failf("failed to create wrap: %v", err)
	}

	chart := wrap.Chart()

	if cfg.ShouldFetchChartArtifacts(inputPath) {
		chartURL := fmt.Sprintf("%s:%s", inputPath, chart.Version())
		if err := fetchArtifacts(chartURL, filepath.Join(wrap.RootDir(), artifacts.HelmChartArtifactMetadataDir), subCfg); err != nil {
			return "", err
		}
	}

	chartRoot := chart.RootDir()
	if err := validateWrapLock(wrap, subCfg); err != nil {
		return "", err
	}

	outputFile := cfg.OutputFile

	if outputFile == "" {
		outputBaseName := fmt.Sprintf("%s-%s.wrap.tgz", chart.Name(), chart.Version())
		if outputFile, err = filepath.Abs(outputBaseName); err != nil {
			l.Debugf("failed to normalize output file: %v", err)
			outputFile = filepath.Join(filepath.Dir(chartRoot), outputBaseName)
		}
	}
	if !cfg.SkipImages {
		if err := pullImages(wrap, subCfg); err != nil {
			return "", err
		}
	}
	if cfg.Carvelize {
		if err := l.Section(fmt.Sprintf("Generating Carvel bundle for Helm chart %q", chartPath), func(childLog log.SectionLogger) error {
			return carvelize.GenerateBundle(
				chartRoot,
				chartutils.WithAnnotationsKey(cfg.AnnotationsKey),
				chartutils.WithLog(childLog),
			)
		}); err != nil {
			return "", l.Failf("%w", err)
		}
		l.Infof("Carvel bundle created successfully")
	}

	if err := l.ExecuteStep(
		"Compressing Helm chart...",
		func() error {
			return utils.TarContext(ctx, wrap.RootDir(), outputFile, utils.TarConfig{
				Prefix: fmt.Sprintf("%s-%s", chart.Name(), chart.Version()),
			})
		},
	); err != nil {
		return "", l.Failf("failed to wrap Helm chart: %w", err)
	}
	l.Infof("Compressed into %q", outputFile)

	return outputFile, nil
}

// NewCmd builds a new wrap command
func NewCmd(cfg *config.Config) *cobra.Command {
	var outputFile string
	var version string
	var platforms []string
	var fetchArtifacts bool
	var carvelize bool
	var skipImages bool
	var examples = `  # Wrap a Helm chart from a local folder
  $ dt wrap examples/mariadb

  # Wrap a Helm chart in an OCI registry
  $ dt wrap oci://docker.io/bitnamicharts/mariadb
	`
	cmd := &cobra.Command{
		Use:   "wrap CHART_PATH|OCI_URI",
		Short: "Wraps a Helm chart",
		Long: `Wraps a Helm chart either local or remote into a distributable package.
This command will pull all the container images and wrap it into a single tarball along with the Images.lock and metadata`,
		Example:       examples,
		SilenceUsage:  true,
		SilenceErrors: true,
		Args:          cobra.ExactArgs(1),
		RunE: func(_ *cobra.Command, args []string) error {
			chartPath := args[0]

			ctx, cancel := cfg.ContextWithSigterm()
			defer cancel()

			tmpDir, err := config.GetGlobalTempWorkDir()
			if err != nil {
				return err
			}

			parentLog := cfg.Logger()

			wrappedChart, err := wrapChart(chartPath,
				WithLogger(parentLog),
				WithAnnotationsKey(cfg.AnnotationsKey), WithContext(ctx),
				WithPlatforms(platforms), WithVersion(version),
				WithFetchArtifacts(fetchArtifacts), WithCarvelize(carvelize),
				WithUsePlainHTTP(cfg.UsePlainHTTP), WithInsecure(cfg.Insecure),
				WithOutputFile(outputFile),
				WithTempDirectory(tmpDir),
				WithSkipImages(skipImages),
			)
			if err != nil {
				if _, ok := err.(*log.LoggedError); ok {
					// We already logged it, lets be less verbose
					return fmt.Errorf("failed to wrap Helm chart: %v", err)
				}
				return err
			}

			parentLog.Printf(widgets.TerminalSpacer)
			parentLog.Successf("Helm chart wrapped into %q", wrappedChart)

			return nil
		},
	}

	cmd.PersistentFlags().StringVar(&version, "version", version, "when wrapping remote Helm charts from OCI, version to request")
	cmd.PersistentFlags().StringVar(&outputFile, "output-file", outputFile, "generate a tar.gz with the output of the pull operation")
	cmd.PersistentFlags().StringSliceVar(&platforms, "platforms", platforms, "platforms to include in the Images.lock file")
	cmd.PersistentFlags().BoolVar(&carvelize, "add-carvel-bundle", carvelize, "whether the wrap should include a Carvel bundle or not")
	cmd.PersistentFlags().BoolVar(&fetchArtifacts, "fetch-artifacts", fetchArtifacts, "fetch remote metadata and signature artifacts")
	cmd.PersistentFlags().BoolVar(&skipImages, "skip-images", skipImages, "skip fetching images")

	return cmd
}
