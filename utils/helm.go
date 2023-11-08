package utils

import (
	"crypto/tls"
	"fmt"
	"net/http"
	"os"
	"path/filepath"

	"helm.sh/helm/v3/pkg/action"
	"helm.sh/helm/v3/pkg/cli"
	"helm.sh/helm/v3/pkg/registry"
)

// RegistryClientConfig defines how the client communicates with the remote server
type RegistryClientConfig struct {
	UsePlainHTTP     bool
	UseInsecureHTTPS bool
}

// Option defines a RegistryClientConfig setting
type Option func(*RegistryClientConfig)

// Insecure asks the tool to allow insecure HTTPS connections to the remote server.
func Insecure(c *RegistryClientConfig) {
	c.UseInsecureHTTPS = true
}

// WithInsecure configures the InsecureMode of the Config
func WithInsecure(insecure bool) func(c *RegistryClientConfig) {
	return func(c *RegistryClientConfig) {
		c.UseInsecureHTTPS = insecure
	}
}

// WithPlainHTTP configures the InsecureMode of the Config
func WithPlainHTTP(usePlain bool) func(c *RegistryClientConfig) {
	return func(c *RegistryClientConfig) {
		c.UsePlainHTTP = usePlain
	}
}

// NewRegistryClientConfig returns a new RegistryClientConfig with default values
func NewRegistryClientConfig(opts ...Option) *RegistryClientConfig {
	cfg := &RegistryClientConfig{
		UsePlainHTTP:     false,
		UseInsecureHTTPS: false,
	}

	for _, opt := range opts {
		opt(cfg)
	}
	return cfg
}

func getRegistryClient(cfg *RegistryClientConfig) (*registry.Client, error) {
	opts := []registry.ClientOption{}
	if cfg.UsePlainHTTP {
		opts = append(opts, registry.ClientOptPlainHTTP())
	} else {
		if cfg.UseInsecureHTTPS { // #nosec G402
			httpClient := &http.Client{
				Transport: &http.Transport{
					TLSClientConfig: &tls.Config{
						InsecureSkipVerify: true,
					},
				},
			}
			opts = append(opts, registry.ClientOptHTTPClient(httpClient))
		}
	}
	return registry.NewClient(opts...)
}

// PullChart retrieves the specified chart
func PullChart(chartURL, version string, destDir string, opts ...Option) (string, error) {
	dir, err := os.MkdirTemp(destDir, "chart-*")
	if err != nil {
		return "", fmt.Errorf("failed to upload Helm chart: failed to create temp directory: %w", err)
	}

	cfg := &action.Configuration{}
	client := action.NewPullWithOpts(action.WithConfig(cfg))
	client.Settings = cli.New()
	client.DestDir = dir
	client.Untar = true

	reg, err := getRegistryClient(NewRegistryClientConfig(opts...))
	if err != nil {
		return "", fmt.Errorf("missing registry client: %w", err)
	}

	client.SetRegistryClient(reg)
	client.Version = version
	_, err = client.Run(chartURL)
	if err != nil {
		return "", fmt.Errorf("failed to pull Helm chart: %w", err)
	}

	charts, err := filepath.Glob(filepath.Join(dir, "*/Chart.yaml"))
	if err != nil {
		return "", fmt.Errorf("failed to located fetched Helm charts: %w", err)
	}
	if len(charts) == 0 {
		return "", fmt.Errorf("cannot find any Helm chart")
	}
	if len(charts) > 1 {
		return "", fmt.Errorf("found multiple Helm charts")
	}
	return filepath.Dir(charts[0]), nil
}

// PushChart pushes the local chart tarFile to the remote URL provided
func PushChart(tarFile string, pushChartURL string, opts ...Option) error {
	cfg := &action.Configuration{}
	reg, err := getRegistryClient(NewRegistryClientConfig(opts...))
	if err != nil {
		return fmt.Errorf("missing registry client: %w", err)
	}
	cfg.RegistryClient = reg

	client := action.NewPushWithOpts(action.WithPushConfig(cfg))

	client.Settings = cli.New()

	if _, err := client.Run(tarFile, pushChartURL); err != nil {
		return fmt.Errorf("failed to push Helm chart: %w", err)
	}
	return nil
}

func showRemoteHelmChart(chartURL string, version string, cfg *RegistryClientConfig) (string, error) {
	client := action.NewShowWithConfig(action.ShowChart, &action.Configuration{})
	reg, err := getRegistryClient(cfg)
	if err != nil {
		return "", fmt.Errorf("missing registry client: %w", err)
	}
	client.SetRegistryClient(reg)
	client.Version = version
	cp, err := client.ChartPathOptions.LocateChart(chartURL, cli.New())

	if err != nil {
		return "", err
	}
	return client.Run(cp)
}

// RemoteChartExist checks if the provided chart exists
func RemoteChartExist(chartURL string, version string, opts ...Option) bool {
	_, err := showRemoteHelmChart(chartURL, version, NewRegistryClientConfig(opts...))
	return err == nil
}
