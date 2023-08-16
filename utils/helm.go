package utils

import (
	"fmt"
	"os"
	"path/filepath"

	"helm.sh/helm/v3/pkg/action"
	"helm.sh/helm/v3/pkg/cli"
	"helm.sh/helm/v3/pkg/registry"
)

// FetchRemoteChart retrieves the specified chart
func FetchRemoteChart(chartURL, version string, destDir string) (string, error) {
	dir, err := os.MkdirTemp(destDir, "chart-*")
	if err != nil {
		return "", fmt.Errorf("failed to upload Helm chart: failed to create temp directory: %w", err)
	}

	cfg := &action.Configuration{}
	client := action.NewPullWithOpts(action.WithConfig(cfg))
	client.Settings = cli.New()
	client.DestDir = dir
	client.Untar = true
	reg, err := registry.NewClient()
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
func PushChart(tarFile string, pushChartURL string) error {
	cfg := &action.Configuration{}

	reg, err := registry.NewClient()
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

func showRemoteHelmChart(chartURL string, version string) (string, error) {
	cfg := &action.Configuration{}

	client := action.NewShowWithConfig(action.ShowChart, cfg)
	reg, err := registry.NewClient()
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
func RemoteChartExist(chartURL string, version string) bool {
	_, err := showRemoteHelmChart(chartURL, version)
	return err == nil
}
