// Package chartutils implements helper functions to manipulate helm Charts
package chartutils

import (
	"fmt"
	"path/filepath"

	"github.com/vmware-labs/distribution-tooling-for-helm/imagelock"

	"helm.sh/helm/v3/pkg/chart"
	"helm.sh/helm/v3/pkg/chart/loader"
)

// Chart defines a helm Chart with extra functionalities
type Chart struct {
	*chart.Chart
	rootDir        string
	annotationsKey string
}

// RootDir returns the Chart root directory
func (c *Chart) RootDir() string {
	return c.rootDir
}

// ImagesDir returns the images directory inside the chart root directory
func (c *Chart) ImagesDir() string {
	return filepath.Join(c.RootDir(), "images")
}

// File returns the chart.File for the provided name or nil if not found
func (c *Chart) File(name string) *chart.File {
	return getChartFile(c.Chart, name)
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
		c.Chart,
		imagelock.NewImagesLockConfig(
			imagelock.WithAnnotationsKey(c.annotationsKey),
		),
	)
}

// Dependencies returns the chart dependencies
func (c *Chart) Dependencies() []*Chart {
	cfg := NewConfiguration(WithAnnotationsKey(c.annotationsKey))
	deps := make([]*Chart, 0)

	for _, dep := range c.Chart.Dependencies() {
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
	return &Chart{Chart: c, rootDir: chartRoot, annotationsKey: cfg.AnnotationsKey}
}
