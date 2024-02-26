package artifacts

import (
	"context"
	"crypto/tls"
	"fmt"
	"net/http"
	"net/url"
	"os"
	"path/filepath"
	"strings"

	"helm.sh/helm/v3/pkg/action"
	"helm.sh/helm/v3/pkg/cli"
	"helm.sh/helm/v3/pkg/registry"
)

// RegistryClientConfig defines how the client communicates with the remote server
type RegistryClientConfig struct {
	UsePlainHTTP     bool
	UseInsecureHTTPS bool
	Auth             Auth
	TempDir          string
}

// RegistryClientOption defines a RegistryClientConfig setting
type RegistryClientOption func(*RegistryClientConfig)

type registryClientWrap struct {
	client          *registry.Client
	credentialsFile string
}

// WithRegistryAuth configures the Auth of the RegistryClientConfig
func WithRegistryAuth(username, password string) func(c *RegistryClientConfig) {
	return func(c *RegistryClientConfig) {
		c.Auth = Auth{Username: username, Password: password}
	}
}

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

// WithTempDir configures the directory in which to place the temporary credentials file
func WithTempDir(dir string) func(c *RegistryClientConfig) {
	return func(c *RegistryClientConfig) {
		c.TempDir = dir
	}
}

// NewRegistryClientConfig returns a new RegistryClientConfig with default values
func NewRegistryClientConfig(opts ...RegistryClientOption) *RegistryClientConfig {
	cfg := &RegistryClientConfig{
		UsePlainHTTP:     false,
		UseInsecureHTTPS: false,
	}

	for _, opt := range opts {
		opt(cfg)
	}
	return cfg
}

func getRegistryClientWrap(cfg *RegistryClientConfig) (*registryClientWrap, error) {
	wrap := &registryClientWrap{}
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
	if cfg.Auth.Username != "" && cfg.Auth.Password != "" {
		f, err := os.CreateTemp(cfg.TempDir, "dt-config-*.json")
		if err != nil {
			return nil, fmt.Errorf("error creating credentials file: %w", err)
		}
		wrap.credentialsFile = f.Name()
		err = f.Close()
		if err != nil {
			return nil, fmt.Errorf("error closing credentials file: %w", err)
		}
		opts = append(opts, registry.ClientOptCredentialsFile(f.Name()))

		if cfg.UsePlainHTTP {
			opts = append(opts, registry.ClientOptPlainHTTP())
		}
	}
	r, err := registry.NewClient(opts...)
	if err != nil {
		return nil, err
	}
	wrap.client = r

	return wrap, nil

}

// PullChart retrieves the specified chart
func PullChart(chartURL, version string, destDir string, opts ...RegistryClientOption) (string, error) {
	u, err := url.Parse(chartURL)
	if err != nil {
		return "", fmt.Errorf("invalid url: %w", err)
	}
	cfg := &action.Configuration{}
	cc := NewRegistryClientConfig(opts...)
	reg, err := getRegistryClientWrap(cc)
	if err != nil {
		return "", fmt.Errorf("missing registry client: %w", err)
	}
	cfg.RegistryClient = reg.client
	if cc.Auth.Username != "" && cc.Auth.Password != "" && reg.credentialsFile != "" {
		if err := reg.client.Login(u.Host, registry.LoginOptBasicAuth(cc.Auth.Username, cc.Auth.Password)); err != nil {
			return "", fmt.Errorf("error logging in to %s: %w", u.Host, err)
		}
		defer func() {
			_ = reg.client.Logout(u.Host)
			_ = os.Remove(reg.credentialsFile)
		}()
	}
	client := action.NewPullWithOpts(action.WithConfig(cfg))

	dir, err := os.MkdirTemp(destDir, "chart-*")
	if err != nil {
		return "", fmt.Errorf("failed to upload Helm chart: failed to create temp directory: %w", err)
	}
	client.Settings = cli.New()
	client.DestDir = dir
	client.Untar = true
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
func PushChart(tarFile string, pushChartURL string, opts ...RegistryClientOption) error {
	cfg := &action.Configuration{}
	reg, err := getRegistryClientWrap(NewRegistryClientConfig(opts...))
	if err != nil {
		return fmt.Errorf("missing registry client: %w", err)
	}
	cfg.RegistryClient = reg.client
	client := action.NewPushWithOpts(action.WithPushConfig(cfg))

	client.Settings = cli.New()

	if _, err := client.Run(tarFile, pushChartURL); err != nil {
		return fmt.Errorf("failed to push Helm chart: %w", err)
	}

	return nil
}

func showRemoteHelmChart(chartURL string, version string, cfg *RegistryClientConfig) (string, error) {
	client := action.NewShowWithConfig(action.ShowChart, &action.Configuration{})
	reg, err := getRegistryClientWrap(cfg)
	if err != nil {
		return "", fmt.Errorf("missing registry client: %w", err)
	}
	client.SetRegistryClient(reg.client)
	client.Version = version
	cp, err := client.ChartPathOptions.LocateChart(chartURL, cli.New())

	if err != nil {
		return "", err
	}
	return client.Run(cp)
}

// RemoteChartExist checks if the provided chart exists
func RemoteChartExist(chartURL string, version string, opts ...RegistryClientOption) bool {
	_, err := showRemoteHelmChart(chartURL, version, NewRegistryClientConfig(opts...))
	return err == nil
}

// FetchChartMetadata retrieves the chart metadata artifact from the registry
func FetchChartMetadata(ctx context.Context, url string, destination string, opts ...Option) error {
	reference := strings.TrimPrefix(url, "oci://")
	allOpts := append(opts, WithResolveReference(false))
	return pullAssetMetadata(ctx, reference, destination, allOpts...)
}

// PushChartMetadata pushes the chart metadata artifact to the registry
func PushChartMetadata(ctx context.Context, url string, chartDir string, opts ...Option) error {
	reference := strings.TrimPrefix(url, "oci://")
	allOpts := append(opts, WithResolveReference(false))

	return pushAssetMetadata(ctx, reference, chartDir, allOpts...)
}
