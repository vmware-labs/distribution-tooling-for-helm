// Package wrap implements the command to wrap a Helm chart or container image
package wrap

import (
	"bytes"
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
	"github.com/vmware-labs/distribution-tooling-for-helm/pkg/dtlog"
	"github.com/vmware-labs/distribution-tooling-for-helm/pkg/dtlog/silent"
	"github.com/vmware-labs/distribution-tooling-for-helm/pkg/imagelock"

	"github.com/vmware-labs/distribution-tooling-for-helm/pkg/dtlog/logrus"

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
	logger                dtlog.SectionLogger
	TempDirectory         string
	Version               string
	Carvelize             bool
	KeepArtifacts         bool
	FetchArtifacts        bool
	SkipPullImages        bool
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

// WithSkipPullImages configures the WithSkipPullImages of the WrapConfig
func WithSkipPullImages(skipPullImages bool) func(c *Config) {
	return func(c *Config) {
		c.SkipPullImages = skipPullImages
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

	dir, err := os.MkdirTemp("", "chart-*")
	if err != nil {
		return "", err
	}
	c.TempDirectory = dir
	return dir, nil
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

	tmpDir, err := cfg.GetTemporaryDirectory()
	if err != nil {
		return "", fmt.Errorf("failed to create temporary directory: %w", err)
	}

	if chartutils.IsRemoteChart(inputPath) {
		if err = l.ExecuteStep("Fetching remote Helm chart", func() error {
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
			chartPath, err = untar(inputPath, tmpDir)
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

// ResolveInputContainerPath resolves the input container into a local uncompressed container path
func ResolveInputContainerPath(inputPath string, cfg *Config) (string, error) {
	l := cfg.GetLogger()
	var chartPath string

	tmpDir, err := cfg.GetTemporaryDirectory()
	if err != nil {
		return "", fmt.Errorf("failed to create temporary directory: %w", err)
	}

	if isTar, _ := utils.IsTarFile(inputPath); isTar {
		if err := l.ExecuteStep("Uncompressing container image", func() error {
			var err error
			chartPath, err = untar(inputPath, tmpDir)
			return err
		}); err != nil {
			return "", l.Failf("Failed to uncompress %q: %w", inputPath, err)
		}
		l.Infof("Container image uncompressed to %q", chartPath)
	} else {
		chartPath = inputPath
	}

	return chartPath, nil
}

func untar(chartFile string, dir string) (string, error) {
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
					imagelock.WithSkipImageDigestResolution(cfg.SkipPullImages),
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
		return l.Section(fmt.Sprintf("Pulling images into %q", wrap.ImagesDir()), func(childLog dtlog.SectionLogger) error {
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
		if err = fetchArtifacts(chartURL, filepath.Join(wrap.RootDir(), artifacts.HelmChartArtifactMetadataDir), subCfg); err != nil {
			return "", err
		}
	}

	chartRoot := chart.RootDir()
	if err = validateWrapLock(wrap, subCfg); err != nil {
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

	if !cfg.SkipPullImages {
		if err := pullImages(wrap, subCfg); err != nil {
			return "", err
		}
	}
	if cfg.Carvelize {
		if err := l.Section(fmt.Sprintf("Generating Carvel bundle for Helm chart %q", chartPath), func(childLog dtlog.SectionLogger) error {
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
	var skipPullImages bool
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
				WithSkipPullImages(skipPullImages),
			)
			if err != nil {
				if _, ok := err.(*dtlog.LoggedError); ok {
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
	cmd.PersistentFlags().BoolVar(&skipPullImages, "skip-pull-images", skipPullImages, "skip pulling images when wrapping a Helm Chart")

	return cmd
}

// Container wraps a single container image into a portable tarball
func Container(imageRef string, opts ...Option) (string, error) {
	return wrapContainer(imageRef, opts...)
}

func wrapContainer(imageRef string, opts ...Option) (string, error) {
	cfg := NewConfig(opts...)

	ctx := cfg.Context
	l := cfg.GetLogger().StartSection(fmt.Sprintf("Wrapping container image %q", imageRef))
	cfg.logger = l

	tmpDir, err := cfg.GetTemporaryDirectory()
	if err != nil {
		return "", fmt.Errorf("failed to create temporary directory: %w", err)
	}

	wrapDir := filepath.Join(tmpDir, "wrap")
	wc, err := wrapping.CreateContainer(wrapDir)
	if err != nil {
		return "", l.Failf("failed to create container wrap: %v", err)
	}

	// Generate Images.lock from the remote image reference
	var lock *imagelock.ImagesLock
	err = l.ExecuteStep("Generating Images.lock from container image...", func() error {
		var genErr error
		lock, genErr = imagelock.GenerateFromContainerRef(
			imageRef,
			imagelock.WithPlatforms(cfg.Platforms),
			imagelock.WithInsecure(cfg.Insecure),
			imagelock.WithSkipImageDigestResolution(cfg.SkipPullImages),
			imagelock.WithContext(ctx),
			imagelock.WithAuth(cfg.ContainerRegistryAuth.Username, cfg.ContainerRegistryAuth.Password),
		)
		return genErr
	})
	if err != nil {
		return "", l.Failf("failed to generate Images.lock: %w", err)
	}

	// Write the lock file
	lockBuf := &bytes.Buffer{}
	err = lock.ToYAML(lockBuf)
	if err != nil {
		return "", l.Failf("failed to serialize Images.lock: %w", err)
	}
	err = os.WriteFile(wc.LockFilePath(), lockBuf.Bytes(), 0600)
	if err != nil {
		return "", l.Failf("failed to write Images.lock: %w", err)
	}
	l.Infof("Images.lock written to %q", wc.LockFilePath())

	if !cfg.SkipPullImages {
		err = l.Section(fmt.Sprintf("Pulling container image into %q", wc.ImagesDir()), func(childLog dtlog.SectionLogger) error {
			return chartutils.PullImages(
				lock,
				wc.ImagesDir(),
				chartutils.WithLog(childLog),
				chartutils.WithContext(ctx),
				chartutils.WithFetchArtifacts(cfg.FetchArtifacts),
				chartutils.WithAuth(cfg.ContainerRegistryAuth.Username, cfg.ContainerRegistryAuth.Password),
				chartutils.WithArtifactsDir(wc.ImageArtifactsDir()),
				chartutils.WithProgressBar(childLog.ProgressBar()),
				chartutils.WithInsecureMode(cfg.Insecure),
			)
		})
		if err != nil {
			return "", l.Failf("failed to pull container image: %w", err)
		}
		l.Infof("Container image pulled successfully")
	}

	baseName, tag, digest := utils.ParseImageReference(imageRef)
	// Build a short identifier for use in file names: prefer tag
	identifier := tag
	if identifier == "" {
		identifier = digest
	}
	outputFile := cfg.OutputFile
	if outputFile == "" {
		outputBaseName := fmt.Sprintf("%s-%s.container.wrap.tgz", baseName, identifier)
		if outputFile, err = filepath.Abs(outputBaseName); err != nil {
			l.Debugf("failed to normalize output file: %v", err)
			outputFile = outputBaseName
		}
	}

	if err := l.ExecuteStep("Compressing container image wrap...", func() error {
		return utils.TarContext(ctx, wc.RootDir(), outputFile, utils.TarConfig{
			Prefix: fmt.Sprintf("%s-%s", baseName, identifier),
		})
	}); err != nil {
		return "", l.Failf("failed to wrap container image: %w", err)
	}
	l.Infof("Compressed into %q", outputFile)

	return outputFile, nil
}

// NewContainerCmd builds a new container wrap command
func NewContainerCmd(cfg *config.Config) *cobra.Command {
	var outputFile string
	var platforms []string
	var fetchArtifacts bool

	cmd := &cobra.Command{
		Use:   "wrap OCI_REF",
		Short: "Wraps a container image",
		Long: `Wraps a single container image into a portable tarball suitable for air-gapped distribution.
This command pulls the container image and its metadata/signature artifacts and packages them together
with an Images.lock file into a single tarball.`,
		Example: `  # Wrap a container image from Docker Hub
  $ dt images wrap docker.io/library/nginx:1.25

  # Wrap a container image for specific platforms
  $ dt images wrap docker.io/library/nginx:1.25 --platforms linux/amd64,linux/arm64

  # Wrap a container image including its signatures and metadata artifacts
  $ dt images wrap docker.io/library/nginx:1.25 --fetch-artifacts
`,
		SilenceUsage:  true,
		SilenceErrors: true,
		Args:          cobra.ExactArgs(1),
		RunE: func(_ *cobra.Command, args []string) error {
			imageRef := args[0]

			ctx, cancel := cfg.ContextWithSigterm()
			defer cancel()

			tmpDir, err := cfg.GetTemporaryDirectory()
			if err != nil {
				return fmt.Errorf("failed to create temporary directory: %v", err)
			}

			parentLog := cfg.Logger()

			wrappedContainer, err := wrapContainer(imageRef,
				WithLogger(parentLog),
				WithContext(ctx),
				WithPlatforms(platforms),
				WithFetchArtifacts(fetchArtifacts),
				WithUsePlainHTTP(cfg.UsePlainHTTP),
				WithInsecure(cfg.Insecure),
				WithOutputFile(outputFile),
				WithTempDirectory(tmpDir),
			)
			if err != nil {
				if _, ok := err.(*dtlog.LoggedError); ok {
					return fmt.Errorf("failed to wrap container image: %v", err)
				}
				return err
			}

			parentLog.Printf(widgets.TerminalSpacer)
			parentLog.Successf("Container image wrapped into %q", wrappedContainer)

			return nil
		},
	}

	cmd.PersistentFlags().StringVar(&outputFile, "output-file", outputFile, "output tarball path (defaults to <name>-<tag>.container.wrap.tgz)")
	cmd.PersistentFlags().StringSliceVar(&platforms, "platforms", platforms, "platforms to include in the Images.lock file (e.g. linux/amd64,linux/arm64)")
	cmd.PersistentFlags().BoolVar(&fetchArtifacts, "fetch-artifacts", fetchArtifacts, "fetch remote metadata and signature artifacts")

	return cmd
}
