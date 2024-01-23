// Package imagelock implements utility routines for manipulating Images.lock
// files
package imagelock

import (
	"errors"
	"fmt"
	"io"
	"os"
	"time"

	"gopkg.in/yaml.v3"

	"helm.sh/helm/v3/pkg/chart"
	"helm.sh/helm/v3/pkg/chart/loader"
)

// APIVersionV0 is the initial version of the API
const APIVersionV0 = "v0"

// DefaultImagesLockFileName is the default lock file name
const DefaultImagesLockFileName = "Images.lock"

// DefaultAnnotationsKey is the default annotations key used to include the images metadata
const DefaultAnnotationsKey = "images"

// ImagesLock represents the lock file containing information about the included images.
type ImagesLock struct {
	APIVersion string            `yaml:"apiVersion"` // The version of the API used for the lock file.
	Kind       string            // The type of object represented by the lock file.
	Metadata   map[string]string // Additional metadata associated with the lock file.

	Chart struct {
		Name       string // The name of the chart.
		Version    string // The version of the chart.
		AppVersion string `yaml:"appVersion"` // The version of the app contained in the chart
	} // Information about the chart associated with the lock file.

	Images ImageList // List of included images
}

// FindImageByName finds a included Image based on its name and containing chart
func (il *ImagesLock) FindImageByName(chartName string, imageName string) (*ChartImage, error) {
	return il.findImage(chartName, imageName)
}

// findImage finds a included Image based on its name and containing chart and optionally, Image URL
func (il *ImagesLock) findImage(chartName string, imageName string, extra ...string) (*ChartImage, error) {
	matchImageURL := false
	imageURL := ""
	if len(extra) > 0 {
		imageURL = extra[0]
		matchImageURL = true
	}
	for _, img := range il.Images {
		if img.Chart == chartName && img.Name == imageName && (!matchImageURL || img.Image == imageURL) {
			return img, nil
		}
	}
	return nil, fmt.Errorf("cannot find image %q", imageName)
}

// Validate checks if the provided list of images matches the contained set
func (il *ImagesLock) Validate(expectedImages ImageList) error {
	var allErrors error
	if len(il.Images) != len(expectedImages) {
		allErrors = errors.Join(allErrors, fmt.Errorf("number of images differs: %d != %d", len(il.Images), len(expectedImages)))
	}
	for _, img := range expectedImages {
		existingImg, err := il.findImage(img.Chart, img.Name, img.Image)
		if err != nil {
			allErrors = errors.Join(allErrors, fmt.Errorf("chart %q: %v", img.Chart, err))
			continue
		}
		if err := existingImg.Diff(img); err != nil {
			allErrors = errors.Join(allErrors, err)
		}
	}
	return allErrors
}

// ToYAML writes the serialized YAML representation of the ImagesLock to w
func (il *ImagesLock) ToYAML(w io.Writer) error {
	enc := yaml.NewEncoder(w)
	enc.SetIndent(2)

	return enc.Encode(il)
}

// NewImagesLock creates a new empty ImagesLock
func NewImagesLock() *ImagesLock {
	return &ImagesLock{
		APIVersion: APIVersionV0,
		Kind:       "ImagesLock",
		Metadata:   map[string]string{"generatedAt": time.Now().UTC().Format("2006-01-02T15:04:05.999999999Z"), "generatedBy": "Distribution Tooling for Helm"},
		Images:     make([]*ChartImage, 0),
	}
}

// FromYAML reads a ImagesLock from the YAML read from r
func FromYAML(r io.Reader) (*ImagesLock, error) {
	il := NewImagesLock()
	dec := yaml.NewDecoder(r)
	if err := dec.Decode(il); err != nil {
		return nil, fmt.Errorf("failed to load image-lock: %v", err)
	}

	return il, nil
}

// FromYAMLFile reads a ImagesLock from the YAML file
func FromYAMLFile(file string) (*ImagesLock, error) {
	fh, err := os.Open(file)
	if err != nil {
		return nil, fmt.Errorf("failed to open Images.lock file: %w", err)
	}
	defer fh.Close()
	return FromYAML(fh)
}

// GenerateFromChart creates a ImagesLock from the Chart at chartPath
func GenerateFromChart(chartPath string, opts ...Option) (*ImagesLock, error) {
	cfg := NewImagesLockConfig(opts...)

	chart, err := loader.Load(chartPath)
	if err != nil {
		return nil, fmt.Errorf("failed to load Helm chart: %v", err)
	}

	imgLock := NewImagesLock()

	imgLock.Chart.Name = chart.Name()
	imgLock.Chart.Version = chart.Metadata.Version
	imgLock.Chart.AppVersion = chart.Metadata.AppVersion

	if err := populateImagesFromChart(imgLock, chart, cfg); err != nil {
		return nil, err
	}

	return imgLock, nil
}

// populateImagesFromChart populates the ImagesLock with images and digests from the given chart and its dependencies.
func populateImagesFromChart(imgLock *ImagesLock, chart *chart.Chart, cfg *Config) error {

	images, err := getDigestedImagesFromChartAnnotations(chart, cfg)
	if err != nil {
		return fmt.Errorf("failed to process Helm chart %q images: %v", chart.Name(), err)
	}

	imgLock.Images = append(imgLock.Images, images...)

	if len(chart.Dependencies()) == 0 && len(chart.Metadata.Dependencies) > 0 {
		return fmt.Errorf("the Helm chart defines dependencies but they are not present in the charts directory")
	}
	var allErrors error

	for _, c := range chart.Dependencies() {
		err := populateImagesFromChart(imgLock, c, cfg)
		if err != nil {
			allErrors = errors.Join(allErrors, fmt.Errorf("failed to process Helm chart %q images: %v", c.Name(), err))
			continue
		}
	}
	imgLock.Images = imgLock.Images.Dedup()

	return allErrors
}
