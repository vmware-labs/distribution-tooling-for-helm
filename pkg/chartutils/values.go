package chartutils

import (
	"bytes"
	"fmt"
	"path/filepath"
	"slices"

	"github.com/google/go-containerregistry/pkg/name"
	"github.com/vmware-labs/distribution-tooling-for-helm/pkg/utils"
	"gopkg.in/yaml.v2"

	"helm.sh/helm/v3/pkg/chart/loader"
)

var (
	imageElementKeys = []string{"registry", "repository", "tag", "digest"}
)

// ValuesImageElement defines a docker image element definition found
// when parsing values.yaml
type ValuesImageElement struct {
	locationPath string
	Registry     string
	Repository   string
	Digest       string
	Tag          string

	foundFields []string
}

// YamlLocationPath returns the jsonpath-like location of the element in values.yaml
func (v *ValuesImageElement) YamlLocationPath() string {
	return v.locationPath
}

// Name returns the image name
func (v *ValuesImageElement) Name() string {
	return filepath.Base(v.Repository)
}

// ValuesImageElementList defines a list of ValuesImageElement
type ValuesImageElementList []*ValuesImageElement

func (imgs ValuesImageElementList) Len() int      { return len(imgs) }
func (imgs ValuesImageElementList) Swap(i, j int) { imgs[i], imgs[j] = imgs[j], imgs[i] }
func (imgs ValuesImageElementList) Less(i, j int) bool {
	return imgs[i].Name() < imgs[j].Name()
}

// ToAnnotation returns the annotation text representation of the ValuesImageElementList
func (imgs ValuesImageElementList) ToAnnotation() ([]byte, error) {
	done := make(map[string]struct{}, 0)
	rawData := make([]map[string]string, 0)
	for _, img := range imgs {
		url := img.URL()
		if _, alreadyDone := done[url]; alreadyDone {
			continue
		}
		rawData = append(rawData, map[string]string{
			"name":  img.Name(),
			"image": url,
		})
		done[url] = struct{}{}
	}
	buf := &bytes.Buffer{}
	enc := yaml.NewEncoder(buf)
	if err := enc.Encode(rawData); err != nil {
		return nil, fmt.Errorf("failed to serialize as annotation yaml: %v", err)
	}
	return buf.Bytes(), nil
}

// URL returns the full URL to the image
func (v *ValuesImageElement) URL() string {
	var url string
	if v.Registry != "" {
		url = fmt.Sprintf("%s/%s", v.Registry, v.Repository)
	} else {
		url = v.Repository
	}
	if v.Tag != "" {
		url = fmt.Sprintf("%s:%s", url, v.Tag)
	}
	if v.Digest != "" {
		url = fmt.Sprintf("%s@%s", url, v.Digest)
	}
	return url
}

// ToMap returns the map[string]string representation of the ValuesImageElement
func (v *ValuesImageElement) ToMap() map[string]string {
	return map[string]string{
		"registry":   v.Registry,
		"repository": v.Repository,
		"digest":     v.Digest,
		"tag":        v.Tag,
	}
}

// YamlReplaceMap returns the yaml paths to the different image definition elements
// and the current value
func (v *ValuesImageElement) YamlReplaceMap() map[string]string {
	data := make(map[string]string, 0)
	fullMap := v.ToMap()
	// We should only write back what we found
	for _, key := range v.foundFields {
		value := fullMap[key]
		p := fmt.Sprintf("%s.%s", v.YamlLocationPath(), key)
		data[p] = value
	}
	return data
}

// Relocate modifies the ValuesImageElement Registry and Repository based on the provided prefix
func (v *ValuesImageElement) Relocate(prefix string) error {
	newURL, err := utils.RelocateImageURL(v.URL(), prefix, false)
	if err != nil {
		return fmt.Errorf("failed to relocate")
	}

	newRef, err := name.ParseReference(newURL)
	if err != nil {
		return fmt.Errorf("failed to parse relocated URL: %v", err)
	}

	if slices.Contains(v.foundFields, "registry") {
		v.Registry = newRef.Context().Registry.RegistryStr()
		v.Repository = newRef.Context().RepositoryStr()
	} else {
		v.Repository = newRef.Context().Name()
	}
	return nil
}

// FindImageElementsInValuesMap parses the provided data looking for ValuesImageElement and returns the list
func FindImageElementsInValuesMap(data map[string]interface{}) (ValuesImageElementList, error) {
	return findImageElementsInMap(data, "$"), nil
}

// FindImageElementsInValuesFile looks for a list of ValuesImageElement in the
// values.yaml for the specified chartPath
func FindImageElementsInValuesFile(chartPath string) (ValuesImageElementList, error) {
	c, err := loader.Load(chartPath)
	if err != nil {
		return nil, fmt.Errorf("failed to load Helm chart: %v", err)
	}
	return FindImageElementsInValuesMap(c.Values)
}

func valuesImageElementFromMap(elemData map[string]string) *ValuesImageElement {
	foundFields := make([]string, 0)
	for _, k := range imageElementKeys {
		if _, ok := elemData[k]; !ok {
			elemData[k] = ""
		} else {
			foundFields = append(foundFields, k)
		}
	}
	return &ValuesImageElement{
		foundFields: foundFields,
		Registry:    elemData["registry"],
		Repository:  elemData["repository"],
		Digest:      elemData["digest"],
		Tag:         elemData["tag"],
	}
}

func findImageElementsInMap(data map[string]interface{}, id string) []*ValuesImageElement {
	elements := make([]*ValuesImageElement, 0)
	if elem := parseValuesImageElement(data); elem != nil {
		elem.locationPath = id
		elements = append(elements, elem)
	}

	for k, v := range data {
		if v, ok := v.(map[string]interface{}); ok {
			elements = append(elements, findImageElementsInMap(v, fmt.Sprintf("%s.%s", id, k))...)
		}
	}
	return elements
}

func parseValuesImageElement(data map[string]interface{}) *ValuesImageElement {
	elemData := make(map[string]string)
	for _, k := range imageElementKeys {
		v, ok := data[k]
		if !ok {
			// digest is optional
			if k == "digest" {
				continue
			}
			// repository element might contain registry url
			if k == "registry" {
				continue
			}
			// tag may be implicitly set to .Chart.appVersion
			if k == "tag" {
				continue
			}
			return nil
		}
		vStr, ok := v.(string)
		if !ok {
			return nil
		}
		elemData[k] = vStr
	}
	// An empty registry may be acceptable, but not an empty repository
	if elemData["repository"] == "" {
		return nil
	}
	return valuesImageElementFromMap(elemData)
}
