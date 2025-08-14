package relocator

import (
	"fmt"

	cu "github.com/vmware-labs/distribution-tooling-for-helm/pkg/chartutils"
	"github.com/vmware-labs/distribution-tooling-for-helm/pkg/utils"

	"helm.sh/helm/v3/pkg/chartutil"
)

func relocateValuesData(valuesFile string, valuesData []byte, prefix string) (*RelocationResult, error) {
	valuesMap, err := chartutil.ReadValues(valuesData)
	if err != nil {
		return nil, fmt.Errorf("failed to parse Helm chart values: %v", err)
	}
	imageElems, err := cu.FindImageElementsInValuesMap(valuesMap)
	if err != nil {
		return nil, fmt.Errorf("failed to find Helm chart image elements from values.yaml: %v", err)
	}
	if len(imageElems) == 0 {
		return &RelocationResult{Data: valuesData, Count: 0}, nil
	}

	data := make(map[string]string, 0)
	for _, e := range imageElems {
		if err = e.Relocate(prefix); err != nil {
			return nil, fmt.Errorf("unexpected error relocating: %v", err)
		}
		for k, v := range e.YamlReplaceMap() {
			data[k] = v
		}
	}
	relocatedData, err := utils.YamlSet(valuesData, data)
	if err != nil {
		return nil, fmt.Errorf("unexpected error relocating: %v", err)
	}
	return &RelocationResult{Name: valuesFile, Data: relocatedData, Count: len(imageElems)}, nil
}

func relocateValues(c *cu.Chart, prefix string) ([]*RelocationResult, error) {
	result := make([]*RelocationResult, 0, len(c.ValuesFiles()))
	for _, values := range c.ValuesFiles() {
		if values == nil {
			result = append(result, &RelocationResult{})
			continue
		}
		res, err := relocateValuesData(values.Name, values.Data, prefix)
		if err != nil {
			return nil, err
		}
		result = append(result, res)
	}
	return result, nil
}
