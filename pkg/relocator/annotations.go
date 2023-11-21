package relocator

import (
	"fmt"

	"github.com/vmware-labs/distribution-tooling-for-helm/pkg/chartutils"
	cu "github.com/vmware-labs/distribution-tooling-for-helm/pkg/chartutils"
)

// RelocateAnnotations rewrites the image urls in the chart annotations using the provided prefix
func RelocateAnnotations(chartDir string, prefix string, opts ...chartutils.Option) (string, error) {
	c, err := chartutils.LoadChart(chartDir, opts...)
	if err != nil {
		return "", fmt.Errorf("failed to relocate annotations: %w", err)
	}
	res, err := relocateAnnotations(c, prefix)
	if err != nil {
		return "", fmt.Errorf("failed to relocate annotations: %w", err)
	}
	return string(res.Data), nil
}

func relocateAnnotations(c *cu.Chart, prefix string) (*RelocationResult, error) {
	images, err := c.GetAnnotatedImages()
	if err != nil {
		return nil, fmt.Errorf("failed to read images from annotations: %v", err)
	}
	count, err := relocateImages(images, prefix)
	if err != nil {
		return nil, fmt.Errorf("failed to relocate annotations: %v", err)
	}

	data, err := images.ToAnnotation()
	if err != nil {
		return nil, fmt.Errorf("failed to relocate annotations: %v", err)
	}

	result := &RelocationResult{Data: data, Count: count}
	return result, nil
}
