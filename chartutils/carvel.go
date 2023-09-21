// Package chartutils implements helper functions to manipulate helm Charts
package chartutils

import (
	"fmt"
	"io"
	"os"

	"gopkg.in/yaml.v3"
)

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

type BundleVersion struct {
	APIVersion string `json:"apiVersion"` // This generated yaml, but due to lib we need to use `json`
	Kind       string `json:"kind"`       // This generated yaml, but due to lib we need to use `json`
}
type Metadata struct {
	Version  BundleVersion
	Metadata map[string]string `json:"metadata,omitempty"`
	Authors  []Author          `json:"authors,omitempty"`
	Websites []Website         `json:"websites,omitempty"`
}

func (il *Metadata) ToYAML(w io.Writer) error {
	enc := yaml.NewEncoder(w)
	enc.SetIndent(2)

	return enc.Encode(il)
}

func CarvelBundleFromYAMLFile(file string) (*Metadata, error) {
	fh, err := os.Open(file)
	if err != nil {
		return nil, fmt.Errorf("failed to open Images.lock file: %v", err)
	}
	defer fh.Close()
	return CarvelBundleFromYAML(fh)
}

// reads a Carvel metadata bundled from the YAML read from r
func CarvelBundleFromYAML(r io.Reader) (*Metadata, error) {
	metadata := &Metadata{
		Version: BundleVersion{
			APIVersion: BundleAPIVersion,
			Kind:       BundleKind,
		},
		Metadata: map[string]string{},
		Authors:  []Author{},
		Websites: []Website{},
	}
	dec := yaml.NewDecoder(r)
	if err := dec.Decode(metadata); err != nil {
		return nil, fmt.Errorf("failed to load Carvel bundle: %v", err)
	}

	return metadata, nil
}

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
