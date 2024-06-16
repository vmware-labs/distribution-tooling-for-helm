// Package relocator implements the functionality to rewrite image URLs
// in Charts
package relocator

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/vmware-labs/distribution-tooling-for-helm/pkg/carvel"
	cu "github.com/vmware-labs/distribution-tooling-for-helm/pkg/chartutils"
	"github.com/vmware-labs/distribution-tooling-for-helm/pkg/utils"
	"github.com/vmware-tanzu/carvel-imgpkg/pkg/imgpkg/lockconfig"
)

// RelocationResult describes the result of performing a relocation
type RelocationResult struct {
	// Name is the name of the values file
	Name string
	// Data is the relocated data
	Data []byte
	// Count is the number of relocated images
	Count int
}

func relocateChart(chart *cu.Chart, newRegistry string, cfg *RelocateConfig) error {
	valuesReplRes, err := relocateValues(chart, newRegistry)
	if err != nil {
		return fmt.Errorf("failed to relocate chart: %v", err)
	}

	for _, result := range valuesReplRes {
		if result.Count > 0 {
			if err := os.WriteFile(chart.AbsFilePath(result.Name), result.Data, 0644); err != nil {
				return fmt.Errorf("failed to write %s: %v", result.Name, err)
			}
		}
	}

	var allErrors error

	// TODO: Compare annotations with values replacements
	annotationsRelocResult, err := relocateAnnotations(chart, newRegistry)
	if err != nil {
		allErrors = errors.Join(allErrors, fmt.Errorf("failed to relocate Helm chart: %v", err))
	} else {
		if annotationsRelocResult.Count > 0 {
			annotationsKeyPath := fmt.Sprintf("$.annotations['%s']", cfg.ImageLockConfig.AnnotationsKey)
			if err := utils.YamlFileSet(chart.AbsFilePath("Chart.yaml"), map[string]string{
				annotationsKeyPath: string(annotationsRelocResult.Data),
			}); err != nil {
				allErrors = errors.Join(allErrors, fmt.Errorf("failed to relocate Helm chart: failed to write annotations: %v", err))
			}
		}
	}

	lockFile := chart.LockFilePath()
	if utils.FileExists(lockFile) {
		err = RelocateLockFile(lockFile, newRegistry)
		if err != nil {
			allErrors = errors.Join(allErrors, fmt.Errorf("failed to relocate Images.lock file: %v", err))
		}
	}

	if cfg.Recursive {
		for _, dep := range chart.Dependencies() {
			if err := relocateChart(dep, newRegistry, cfg); err != nil {
				allErrors = errors.Join(allErrors, fmt.Errorf("failed to relocate Helm SubChart %q: %v", dep.Chart().ChartFullPath(), err))
			}
		}
	}

	return allErrors
}

// RelocateChartDir relocates the chart (Chart.yaml annotations, Images.lock and values.yaml) specified
// by chartPath using the provided prefix
func RelocateChartDir(chartPath string, newRegistry string, opts ...RelocateOption) error {
	newRegistry = normalizeRelocateURL(newRegistry)

	cfg := NewRelocateConfig(opts...)

	chart, err := cu.LoadChart(chartPath, cu.WithAnnotationsKey(cfg.ImageLockConfig.AnnotationsKey), cu.WithValuesFiles(cfg.ValuesFiles...))
	if err != nil {
		return fmt.Errorf("failed to load Helm chart: %v", err)
	}

	err = relocateChart(chart, newRegistry, cfg)
	if err != nil {
		return err
	}
	if utils.FileExists(filepath.Join(chartPath, carvel.CarvelImagesFilePath)) {
		err = relocateCarvelBundle(chartPath, newRegistry)

		if err != nil {
			return err
		}
	}

	return err
}

func relocateCarvelBundle(chartRoot string, newRegistry string) error {

	//TODO: Do better detection here, imgpkg probably has something
	carvelImagesFile := filepath.Join(chartRoot, carvel.CarvelImagesFilePath)
	lock, err := lockconfig.NewImagesLockFromPath(carvelImagesFile)
	if err != nil {
		return fmt.Errorf("failed to load Carvel images lock: %v", err)
	}
	result, err := RelocateCarvelImagesLock(&lock, newRegistry)
	if err != nil {
		return err
	}
	if result.Count == 0 {
		return nil
	}
	if err := utils.SafeWriteFile(carvelImagesFile, result.Data, 0600); err != nil {
		return fmt.Errorf("failed to overwrite Carvel images lock file: %v", err)
	}
	return nil
}

// RelocateCarvelImagesLock rewrites the images urls in the provided lock using prefix
func RelocateCarvelImagesLock(lock *lockconfig.ImagesLock, newRegistry string) (*RelocationResult, error) {

	count, err := relocateCarvelImages(lock.Images, newRegistry)
	if err != nil {
		return nil, fmt.Errorf("failed to relocate Carvel images lock file: %v", err)
	}

	buff, err := lock.AsBytes()
	if err != nil {
		return nil, fmt.Errorf("failed to write Images.lock file: %v", err)
	}

	return &RelocationResult{Data: buff, Count: count}, nil

}

func relocateCarvelImages(images []lockconfig.ImageRef, newRegistry string) (count int, err error) {
	var allErrors error
	for i, img := range images {
		norm, err := utils.RelocateImageURL(img.Image, newRegistry, true)
		if err != nil {
			allErrors = errors.Join(allErrors, err)
			continue
		}
		images[i].Image = norm
		count++
	}
	return count, allErrors
}

func normalizeRelocateURL(url string) string {
	ociPrefix := "oci://"
	// crane gets confused with the oci schema, so we
	// strip it
	return strings.TrimPrefix(url, ociPrefix)
}
