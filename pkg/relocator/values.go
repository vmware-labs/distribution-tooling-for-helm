package relocator

import (
	"fmt"

	cu "github.com/vmware-labs/distribution-tooling-for-helm/pkg/chartutils"
	"github.com/vmware-labs/distribution-tooling-for-helm/pkg/utils"

	"helm.sh/helm/v3/pkg/chartutil"
)

// RelocateValues rewrites the images urls in the chart.yaml file using the provided prefix
func RelocateValues(chartDir string, prefix string, opts ...cu.Option) (string, error) {
	c, err := cu.LoadChart(chartDir, opts...)
	if err != nil {
		return "", fmt.Errorf("failed to relocate values: %w", err)
	}
	res, err := relocateValues(c, prefix)
	if err != nil {
		return "", fmt.Errorf("failed to relocate values: %w", err)
	}
	return string(res.Data), nil
}

func relocateValuesData(valuesData []byte, prefix string) (*RelocationResult, error) {
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
		err := e.Relocate(prefix)
		if err != nil {
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
	return &RelocationResult{Data: relocatedData, Count: len(imageElems)}, nil
}

func relocateValues(c *cu.Chart, prefix string) (*RelocationResult, error) {
	valuesFile := c.ValuesFile()
	if valuesFile == nil {
		return &RelocationResult{}, nil
	}
	return relocateValuesData(valuesFile.Data, prefix)
}
