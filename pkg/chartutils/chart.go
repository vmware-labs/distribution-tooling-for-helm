// Package chartutils implements helper functions to manipulate helm Charts
package chartutils

import (
	"fmt"
	"path/filepath"

	"github.com/vmware-labs/distribution-tooling-for-helm/pkg/artifacts"
	"github.com/vmware-labs/distribution-tooling-for-helm/pkg/imagelock"
	"github.com/vmware-labs/distribution-tooling-for-helm/pkg/utils"

	"helm.sh/helm/v3/pkg/chart"
	"helm.sh/helm/v3/pkg/chart/loader"
)

// Chart defines a helm Chart with extra functionalities
type Chart struct {
	chart          *chart.Chart
	rootDir        string
	annotationsKey string
	valuesFiles    []string
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

// VerifyLock verifies the Images.lock file for the chart
func (c *Chart) VerifyLock(opts ...imagelock.Option) error {
	chartPath := c.ChartDir()
	if !utils.FileExists(chartPath) {
		return fmt.Errorf("Helm chart %q does not exist", chartPath)
	}

	currentLock, err := c.GetImagesLock()
	if err != nil {
		return fmt.Errorf("failed to load Images.lock: %w", err)
	}
	calculatedLock, err := imagelock.GenerateFromChart(chartPath,
		opts...,
	)

	if err != nil {
		return fmt.Errorf("failed to re-create Images.lock from Helm chart %q: %v", chartPath, err)
	}

	if err := calculatedLock.Validate(currentLock.Images); err != nil {
		return fmt.Errorf("Images.lock does not validate:\n%v", err)
	}
	return nil
}

// Chart returns the Chart object (required to implement wrapping.Unwrapable)
func (c *Chart) Chart() *Chart {
	return c
}

// LockFilePath returns the absolute path to the chart Images.lock
func (c *Chart) LockFilePath() string {
	return c.AbsFilePath(imagelock.DefaultImagesLockFileName)
}

// GetImagesLock returns the chart's ImagesLock object
func (c *Chart) GetImagesLock() (*imagelock.ImagesLock, error) {
	lockFile := c.LockFilePath()

	lock, err := imagelock.FromYAMLFile(lockFile)
	if err != nil {
		return nil, err
	}

	return lock, nil
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

// ValuesFiles returns all the values chart.File
func (c *Chart) ValuesFiles() []*chart.File {
	files := make([]*chart.File, 0, len(c.valuesFiles))
	for _, valuesFile := range c.valuesFiles {
		files = append(files, c.File(valuesFile))
	}
	return files
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
	cfg := NewConfiguration(WithAnnotationsKey(c.annotationsKey), WithValuesFiles(c.valuesFiles...))
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
	return &Chart{
		chart:          c,
		rootDir:        chartRoot,
		annotationsKey: cfg.AnnotationsKey,
		valuesFiles:    cfg.ValuesFiles,
	}
}
