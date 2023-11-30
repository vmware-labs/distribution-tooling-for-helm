// Package chartutils implements helper functions to manipulate helm Charts
package chartutils

import (
	"fmt"
	"path/filepath"

	"github.com/vmware-labs/distribution-tooling-for-helm/pkg/artifacts"
	"github.com/vmware-labs/distribution-tooling-for-helm/pkg/imagelock"

	"helm.sh/helm/v3/pkg/chart"
	"helm.sh/helm/v3/pkg/chart/loader"
)

// Chart defines a helm Chart with extra functionalities
type Chart struct {
	chart          *chart.Chart
	rootDir        string
	annotationsKey string
}

// ChartFullPath returns the wrapped chart ChartFullPath
func (c *Chart) ChartFullPath() string {
	return c.chart.ChartFullPath()
}

// Name returns the name of the chart
func (c *Chart) Name() string {
	return c.chart.Name()
}

// Version returns the version of the chart
func (c *Chart) Version() string {
	return c.chart.Metadata.Version
}

// Metadata returns the metadata of the chart
func (c *Chart) Metadata() *chart.Metadata {
	return c.chart.Metadata
}

// RootDir returns the Chart root directory
func (c *Chart) RootDir() string {
	return c.rootDir
}

// ChartDir returns the Chart root directory (required to implement wrapping.Unwrapable)
func (c *Chart) ChartDir() string {
	return c.RootDir()
}

// Chart returns the Chart object (required to implement wrapping.Unwrapable)
func (c *Chart) Chart() *Chart {
	return c
}

// LockFilePath returns the absolute path to the chart Images.lock
func (c *Chart) LockFilePath() string {
	return c.AbsFilePath(imagelock.DefaultImagesLockFileName)
}

// ImageArtifactsDir returns the imags artifacts directory
func (c *Chart) ImageArtifactsDir() string {
	return filepath.Join(c.RootDir(), artifacts.HelmArtifactsFolder, "images")
}

// ImagesDir returns the images directory inside the chart root directory
func (c *Chart) ImagesDir() string {
	return filepath.Join(c.RootDir(), "images")
}

// File returns the chart.File for the provided name or nil if not found
func (c *Chart) File(name string) *chart.File {
	return getChartFile(c.chart, name)
}

// ValuesFile returns the values.yaml chart.File
func (c *Chart) ValuesFile() *chart.File {
	return c.File("values.yaml")
}

// AbsFilePath returns the absolute path to the Chart relative file name
func (c *Chart) AbsFilePath(name string) string {
	return filepath.Join(c.rootDir, name)
}

// GetAnnotatedImages returns the chart images specified in the annotations
func (c *Chart) GetAnnotatedImages() (imagelock.ImageList, error) {
	return imagelock.GetImagesFromChartAnnotations(
		c.chart,
		imagelock.NewImagesLockConfig(
			imagelock.WithAnnotationsKey(c.annotationsKey),
		),
	)
}

// Dependencies returns the chart dependencies
func (c *Chart) Dependencies() []*Chart {
	cfg := NewConfiguration(WithAnnotationsKey(c.annotationsKey))
	deps := make([]*Chart, 0)

	for _, dep := range c.chart.Dependencies() {
		subChart := filepath.Join(c.RootDir(), "charts", dep.Name())
		deps = append(deps, newChart(dep, subChart, cfg))
	}
	return deps
}

// LoadChart returns the Chart defined by path
func LoadChart(path string, opts ...Option) (*Chart, error) {
	cfg := NewConfiguration(opts...)

	chart, err := loader.Load(path)
	if err != nil {
		return nil, fmt.Errorf("failed to load Helm chart: %v", err)
	}
	chartRoot, err := GetChartRoot(path)
	if err != nil {
		return nil, fmt.Errorf("cannot determine Helm chart root: %v", err)
	}
	return newChart(chart, chartRoot, cfg), nil
}

func newChart(c *chart.Chart, chartRoot string, cfg *Configuration) *Chart {
	return &Chart{chart: c, rootDir: chartRoot, annotationsKey: cfg.AnnotationsKey}
}
