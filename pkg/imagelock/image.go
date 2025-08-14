package imagelock

import (
	"bytes"
	"errors"
	"fmt"
	"strings"

	"golang.org/x/exp/slices"
	"gopkg.in/yaml.v3"

	"helm.sh/helm/v3/pkg/chart"
)

// ChartImage represents an chart image with its associated information.
type ChartImage struct {
	Name    string       // The name of the image.
	Image   string       // The image reference.
	Chart   string       // The chart containing the image.
	Digests []DigestInfo // List of image digests associated with the image.
}

// ImageList defines a list of images
type ImageList []*ChartImage

// Dedup returns a cleaned ImageSet removing duplicates
func (imgs ImageList) Dedup() ImageList {
	newImages := make([]*ChartImage, 0)
	done := make(map[string]struct{})
	for _, img := range imgs {
		id := fmt.Sprintf("%s:%s:%s", img.Chart, img.Name, img.Image)
		if _, found := done[id]; found {
			continue
		}
		done[id] = struct{}{}
		newImages = append(newImages, img)
	}
	return newImages
}

// ToAnnotation returns the annotation text describing the list of
// named images, ready to be inserted in the Chart.yaml file
func (imgs ImageList) ToAnnotation() ([]byte, error) {
	type rawDataElem struct {
		Name  string
		Image string
	}
	rawData := make([]rawDataElem, 0)
	for _, img := range imgs {
		rawData = append(rawData, rawDataElem{Name: img.Name, Image: img.Image})
	}
	buf := &bytes.Buffer{}
	enc := yaml.NewEncoder(buf)
	if err := enc.Encode(rawData); err != nil {
		return nil, fmt.Errorf("failed to serialize as annotation yaml: %v", err)
	}
	return buf.Bytes(), nil
}

// Diff returns an error if the Image is not equivalent to the provided one
func (i *ChartImage) Diff(other *ChartImage) error {
	var allErrors error
	if i.Image != other.Image {
		return fmt.Errorf("images do not match")
	}
	for _, digest := range other.Digests {
		existingDigest, err := i.GetDigestForArch(digest.Arch)
		if err != nil {
			allErrors = errors.Join(allErrors, fmt.Errorf("chart %q: image %q: %v", other.Chart, other.Image, err))
			continue
		}
		if existingDigest.Digest != digest.Digest {
			allErrors = errors.Join(allErrors,
				fmt.Errorf("chart %q: image %q: digests do not match:\n- %s\n+ %s",
					other.Chart, other.Image, digest.Digest, existingDigest.Digest))
			continue
		}
	}
	return allErrors
}

// GetDigestForArch returns the image digest for the specified architecture.
// It searches through the image's digests and returns the first digest that matches the given architecture.
// If no matching digest is found, it returns an error.
func (i *ChartImage) GetDigestForArch(arch string) (*DigestInfo, error) {
	for _, digest := range i.Digests {
		if digest.Arch == arch {
			return &digest, nil
		}
	}
	return nil, fmt.Errorf("failed to find digest for arch %q", arch)
}

// FetchDigests fetches the image digests for the image from upstream.
// It updates the Image's Digests field with the fetched digests.
// If an error occurs during the fetch, it returns the error.
func (i *ChartImage) FetchDigests(cfg *Config) error {
	digests, err := fetchImageDigests(i.Image, cfg)
	if err != nil {
		return err
	}
	filteredDigests := filterDigestsByPlatforms(digests, cfg.Platforms)
	if len(filteredDigests) == 0 {
		return fmt.Errorf("got empty list of digests after applying platforms filter %q", strings.Join(cfg.Platforms, ", "))
	}
	i.Digests = filteredDigests
	return nil
}

// GetImagesFromChartAnnotations reads the images annotation from the chart (if present) and returns a list of
// ChartImage
func GetImagesFromChartAnnotations(c *chart.Chart, cfg *Config) (ImageList, error) {
	images := make([]*ChartImage, 0)

	annotationsKey := cfg.AnnotationsKey
	if annotationsKey == "" {
		annotationsKey = DefaultAnnotationsKey
	}

	imgsData, ok := c.Metadata.Annotations[annotationsKey]

	// Is perfectly fine to just return an empty list
	// if the key is not there
	if !ok {
		return images, nil
	}

	err := yaml.Unmarshal([]byte(imgsData), &images)
	if err != nil {
		return images, fmt.Errorf("failed to parse images metadata: %v", err)
	}

	// Fill up chart ownership
	for _, img := range images {
		img.Chart = c.Name()
	}

	return images, nil
}

// getDigestedImagesFromChartAnnotations reads the images from the chart annotations and fills up
// the per-architecture digests for the images based on the remote registry
func getDigestedImagesFromChartAnnotations(c *chart.Chart, cfg *Config) (ImageList, error) {
	var allErrors error
	images, err := GetImagesFromChartAnnotations(c, cfg)
	if err != nil {
		return nil, fmt.Errorf("failed to get image list: %w", err)
	}
	for _, image := range images {
		if err := image.FetchDigests(cfg); err != nil {
			allErrors = errors.Join(allErrors, fmt.Errorf("failed to fetch image %q digests: %w", image.Name, err))
		}
	}
	return images, allErrors
}

func filterDigestsByPlatforms(digests []DigestInfo, platforms []string) []DigestInfo {
	// If we do not ask for anything, we get all
	if len(platforms) == 0 {
		return digests
	}

	filteredDigests := make([]DigestInfo, 0)
	for _, d := range digests {
		if slices.Contains(platforms, d.Arch) {
			filteredDigests = append(filteredDigests, d)
		}
	}
	return filteredDigests
}
