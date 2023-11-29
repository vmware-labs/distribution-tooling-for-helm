// Package wrapping defines methods to handle Helm chart wraps
package wrapping

import (
	"fmt"
	"os"
	"path/filepath"

	"github.com/vmware-labs/distribution-tooling-for-helm/pkg/artifacts"
	"github.com/vmware-labs/distribution-tooling-for-helm/pkg/chartutils"
	"github.com/vmware-labs/distribution-tooling-for-helm/pkg/imagelock"
	"github.com/vmware-labs/distribution-tooling-for-helm/pkg/utils"

	"helm.sh/helm/v3/pkg/chart/loader"
)

// Lockable defines the interface to support getting images locked
type Lockable interface {
	LockFilePath() string
	ImagesDir() string
}

// Wrap defines the interface to implement a Helm chart wrap
type Wrap interface {
	Lockable
	Chart() *chartutils.Chart
	RootDir() string
	ChartDir() string
	ImageArtifactsDir() string
}

// wrap defines a wrapped chart
type wrap struct {
	rootDir string
	chart   *chartutils.Chart
}

// RootDir returns the path to the Wrap root directory
func (w *wrap) RootDir() string {
	return w.rootDir
}

// LockFilePath returns the absolute path to the chart Images.lock
func (w *wrap) LockFilePath() string {
	return filepath.Join(w.ChartDir(), imagelock.DefaultImagesLockFileName)
}

// ImageArtifactsDir returns the imags artifacts directory
func (w *wrap) ImageArtifactsDir() string {
	return filepath.Join(w.RootDir(), artifacts.HelmArtifactsFolder, "images")
}

// ImagesDir returns the images directory inside the chart root directory
func (w *wrap) ImagesDir() string {
	return w.AbsFilePath("images")
}

// AbsFilePath returns the absolute path to the Chart relative file name
func (w *wrap) AbsFilePath(name string) string {
	return filepath.Join(w.rootDir, name)
}

// ChartDir returns the path to the Helm chart
func (w *wrap) ChartDir() string {
	return w.chart.RootDir()
}

// Chart returns the Chart object
func (w *wrap) Chart() *chartutils.Chart {
	return w.chart
}

// Load loads a directory containing a wrapped chart and returns a Wrap
func Load(dir string, opts ...chartutils.Option) (Wrap, error) {
	chartDir := filepath.Join(dir, "chart")
	chart, err := chartutils.LoadChart(chartDir, opts...)
	if err != nil {
		return nil, err
	}

	return &wrap{rootDir: dir, chart: chart}, nil
}

// Create receives a path to a source Helm chart and a destination directory where to wrap it and returns a Wrap
func Create(chartSrc string, destDir string, opts ...chartutils.Option) (Wrap, error) {
	// Check we got a chart dir
	_, err := loader.Load(chartSrc)
	if err != nil {
		return nil, fmt.Errorf("failed to load Helm chart: %v", err)
	}

	if err := os.MkdirAll(destDir, 0755); err != nil {
		return nil, fmt.Errorf("failed to create wrap root directory: %w", err)
	}

	wrapChartDir := filepath.Join(destDir, "chart")
	if utils.FileExists(wrapChartDir) {
		return nil, fmt.Errorf("chart dir %q already exists", wrapChartDir)
	}

	if err := utils.CopyDir(chartSrc, wrapChartDir); err != nil {
		return nil, fmt.Errorf("failed to copy source chart: %w", err)
	}

	chart, err := chartutils.LoadChart(wrapChartDir, opts...)
	if err != nil {
		return nil, fmt.Errorf("failed to load Helm chart: %w", err)
	}
	return &wrap{rootDir: destDir, chart: chart}, nil
}
