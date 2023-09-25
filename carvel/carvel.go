// Package carvel implements experimental Carvel support
package carvel

import (
	"fmt"
	"io"
	"strings"

	"github.com/vmware-labs/distribution-tooling-for-helm/chartutils"
	"github.com/vmware-labs/distribution-tooling-for-helm/imagelock"
	"github.com/vmware-tanzu/carvel-imgpkg/pkg/imgpkg/lockconfig"

	"gopkg.in/yaml.v3"
)

// CarvelBundleFilePath represents the usual bundle file for Carvel packaging
const CarvelBundleFilePath = ".imgpkg/bundle.yml"

// CarvelImagesFilePath represents the usual images file for Carvel packaging
const CarvelImagesFilePath = ".imgpkg/images.yml"

const carvelID = "kbld.carvel.dev/id"

// Somehow there is no data structure for a bundle in Carvel. Copying some basics from the describe command.

// Author information from a Bundle
type Author struct {
	Name  string `json:"name,omitempty"`
	Email string `json:"email,omitempty"`
}

// Website URL where more information of the Bundle can be found
type Website struct {
	URL string `json:"url,omitempty"`
}

// Bundle Metadata
const (
	BundleAPIVersion = "imgpkg.carvel.dev/v1alpha1"
	BundleKind       = "Bundle"
)

// BundleVersion with detailsa bout the Carvel bundle version
type BundleVersion struct {
	APIVersion string `json:"apiVersion"` // This generated yaml, but due to lib we need to use `json`
	Kind       string `json:"kind"`       // This generated yaml, but due to lib we need to use `json`
}

// Metadata for a Carvel bundle
type Metadata struct {
	Version  BundleVersion
	Metadata map[string]string `json:"metadata,omitempty"`
	Authors  []Author          `json:"authors,omitempty"`
	Websites []Website         `json:"websites,omitempty"`
}

// ToYAML serializes the Carvel bundle into YAML
func (il *Metadata) ToYAML(w io.Writer) error {
	enc := yaml.NewEncoder(w)
	enc.SetIndent(2)

	return enc.Encode(il)
}

// NewCarvelBundle returns a new carvel bundle Metadata instance
func NewCarvelBundle() *Metadata {
	return &Metadata{
		Version: BundleVersion{
			APIVersion: BundleAPIVersion,
			Kind:       BundleKind,
		},
		Metadata: map[string]string{},
		Authors:  []Author{},
		Websites: []Website{},
	}
}

// CreateBundleMetadata builds and sets a new Carvel bundle struct
func CreateBundleMetadata(chartPath string, lock *imagelock.ImagesLock, cfg *chartutils.Configuration) (*Metadata, error) {
	bundleMetadata := NewCarvelBundle()

	chart, err := chartutils.LoadChart(chartPath)
	if err != nil {
		return nil, fmt.Errorf("failed to load chart: %w", err)
	}

	for _, maintainer := range chart.Metadata.Maintainers {
		author := Author{
			Name: maintainer.Name,
		}
		author.Email = maintainer.Email
		bundleMetadata.Authors = append(bundleMetadata.Authors, author)
	}
	for _, source := range chart.Metadata.Sources {
		website := Website{
			URL: source,
		}
		bundleMetadata.Websites = append(bundleMetadata.Websites, website)
	}

	bundleMetadata.Metadata["name"] = lock.Chart.Name
	for key, value := range chart.Metadata.Annotations {
		annotationsKey := cfg.AnnotationsKey
		if annotationsKey == "" {
			annotationsKey = imagelock.DefaultAnnotationsKey
		}
		if key != annotationsKey {
			bundleMetadata.Metadata[key] = value
		}
	}
	return bundleMetadata, nil
}

// CreateImagesLock builds and set a new Carvel images lock struct
func CreateImagesLock(lock *imagelock.ImagesLock) (lockconfig.ImagesLock, error) {
	imagesLock := lockconfig.ImagesLock{
		LockVersion: lockconfig.LockVersion{
			APIVersion: lockconfig.ImagesLockAPIVersion,
			Kind:       lockconfig.ImagesLockKind,
		},
	}
	for _, img := range lock.Images {
		// Carvel does not seem to support multi-arch. Grab amd64 digest

		name := img.Image
		i := strings.LastIndex(img.Image, ":")
		if i > -1 {
			name = img.Image[0:i]
		}
		//TODO: Clarify with Carvel community their multi-arch support
		//for the time being we stick to amd64
		imageWithDigest := getIntelImageWithDigest(name, img)
		if imageWithDigest == "" {
			// See above. Skip
			break
		}
		imageRef := lockconfig.ImageRef{
			Image: imageWithDigest,
			Annotations: map[string]string{
				carvelID: img.Image,
			},
		}
		imagesLock.AddImageRef(imageRef)
	}
	return imagesLock, nil
}

func getIntelImageWithDigest(name string, img *imagelock.ChartImage) string {

	for _, digest := range img.Digests {
		if digest.Arch == "linux/amd64" {
			return fmt.Sprintf("%s@%s", name, digest.Digest.String())
		}
	}
	return ""
}
