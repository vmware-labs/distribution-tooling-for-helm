// Package chartutils implements helper functions to manipulate helm Charts
package chartutils

import (
	"fmt"
	"os"
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
		return fmt.Errorf("chart %q does not exist", chartPath)
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
		return fmt.Errorf("validation failed for Images.lock:\n%v", err)
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
	return filepath.Join(c.RootDir(), artifacts.ArtifactsFolder, "images")
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

// resolveDependencyPath is the shared implementation for resolving dependency paths
func resolveDependencyPath(chartRoot string, dep *chart.Chart) (string, error) {
	chartsDir := filepath.Join(chartRoot, "charts")
	depName := dep.Name()

	// First, check for directory-based dependency (traditional format)
	dirPath := filepath.Join(chartsDir, depName)
	if info, err := os.Stat(dirPath); err == nil && info.IsDir() {
		return dirPath, nil
	}

	// Second, check for compressed dependency (.tgz format)
	// Try common patterns: dep-name-version.tgz or dep-name.tgz
	tgzPatterns := []string{
		fmt.Sprintf("%s-%s.tgz", depName, dep.Metadata.Version),
		fmt.Sprintf("%s.tgz", depName),
	}

	for _, pattern := range tgzPatterns {
		tgzPath := filepath.Join(chartsDir, pattern)
		if info, err := os.Stat(tgzPath); err == nil && !info.IsDir() {
			// Found a compressed dependency, extract it
			extractedPath, err := extractCompressedDependency(tgzPath, depName)
			if err != nil {
				return "", fmt.Errorf("failed to extract compressed dependency %q: %w", tgzPath, err)
			}
			return extractedPath, nil
		}
	}

	// If neither format exists, return the directory path (for backward compatibility)
	// This will allow the error to be handled by the loader
	return dirPath, nil
}

// extractCompressedDependency is the shared implementation for extracting compressed dependencies
func extractCompressedDependency(tgzPath, depName string) (string, error) {
	chartsDir := filepath.Dir(tgzPath)
	extractDir := filepath.Join(chartsDir, depName)

	// Check if already extracted (directory exists)
	if info, err := os.Stat(extractDir); err == nil && info.IsDir() {
		return extractDir, nil
	}

	// Extract directly to the final location using existing utils
	// StripComponents: 1 removes the top-level directory from the tar (e.g., "chart-name/")
	if err := utils.Untar(tgzPath, extractDir, utils.TarConfig{StripComponents: 1}); err != nil {
		return "", fmt.Errorf("failed to extract tgz: %w", err)
	}

	// Verify the extraction worked by checking for Chart.yaml
	chartYaml := filepath.Join(extractDir, "Chart.yaml")
	if !utils.FileExists(chartYaml) {
		return "", fmt.Errorf("extracted directory does not contain Chart.yaml: %s", extractDir)
	}

	// Remove the original .tgz file after successful extraction
	if err := os.Remove(tgzPath); err != nil {
		return "", fmt.Errorf("failed to remove %s dependency tarball: %w", tgzPath, err)
	}

	return extractDir, nil
}

// Dependencies returns the chart dependencies
func (c *Chart) Dependencies() ([]*Chart, error) {
	cfg := NewConfiguration(WithAnnotationsKey(c.annotationsKey), WithValuesFiles(c.valuesFiles...))
	deps := make([]*Chart, 0)

	for _, dep := range c.chart.Dependencies() {
		subChartPath, err := resolveDependencyPath(c.RootDir(), dep)
		if err != nil {
			return nil, fmt.Errorf("failed to resolve dependency path for %q: %w", dep.Name(), err)
		}
		deps = append(deps, newChart(dep, subChartPath, cfg))
	}
	return deps, nil
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
