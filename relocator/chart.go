// Package relocator implements the functionality to rewrite image URLs
// in Charts
package relocator

import (
	"errors"
	"fmt"
	"os"
	"strings"

	cu "github.com/vmware-labs/distribution-tooling-for-helm/chartutils"
	"github.com/vmware-labs/distribution-tooling-for-helm/imagelock"
	"github.com/vmware-labs/distribution-tooling-for-helm/utils"
	"github.com/vmware-tanzu/carvel-imgpkg/pkg/imgpkg/lockconfig"
)

// RelocationResult describes the result of performing a relocation
type RelocationResult struct {
	// Data is the relocated data
	Data []byte
	// Count is the number of relocated images
	Count int
}

func relocateChart(chart *cu.Chart, prefix string, cfg *RelocateConfig) error {
	valuesReplRes, err := relocateValues(chart, prefix)
	if err != nil {
		return fmt.Errorf("failed to relocate values.yaml: %v", err)
	}
	if valuesReplRes.Count > 0 {
		if err := os.WriteFile(chart.AbsFilePath("values.yaml"), valuesReplRes.Data, 0644); err != nil {
			return fmt.Errorf("failed to write values.yaml: %v", err)
		}
	}

	var allErrors error

	// TODO: Compare annotations with values replacements
	annotationsRelocResult, err := relocateAnnotations(chart, prefix)
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

	lockFile := chart.AbsFilePath(imagelock.DefaultImagesLockFileName)
	if utils.FileExists(lockFile) {
		err = RelocateLockFile(lockFile, prefix)
		if err != nil {
			allErrors = errors.Join(allErrors, fmt.Errorf("failed to relocate Images.lock file: %v", err))
		}
	}

	return allErrors
}

// RelocateChartDir relocates the chart (Chart.yaml annotations, Images.lock and values.yaml) specified
// by chartPath using the provided prefix
func RelocateChartDir(chartPath string, prefix string, opts ...RelocateOption) error {
	prefix = normalizeRelocateURL(prefix)

	cfg := NewRelocateConfig()
	for _, opt := range opts {
		opt(cfg)
	}
	chart, err := cu.LoadChart(chartPath, cu.WithAnnotationsKey(cfg.ImageLockConfig.AnnotationsKey))
	if err != nil {
		return fmt.Errorf("failed to load Helm chart: %v", err)
	}

	err = relocateChart(chart, prefix, cfg)
	if err != nil {
		return err
	}
	err = relocateCarvelBundle(chartPath, prefix)
	if err != nil {
		return err
	}

	var allErrors error

	if cfg.Recursive {
		for _, dep := range chart.Dependencies() {
			if err := relocateChart(dep, prefix, cfg); err != nil {
				allErrors = errors.Join(allErrors, fmt.Errorf("failed to reloacte Helm SubChart %q: %v", dep.ChartFullPath(), err))
			}
		}
	}
	return allErrors
}

func relocateCarvelBundle(chartRoot string, prefix string) error {

	//TODO: Do better detection here, imgpkg probably has something
	lockFile := chartRoot + "/.imgpkg/images.yml"
	if !utils.FileExists(lockFile) {
		fmt.Printf("Did not find Carvel images bundle at %s. Ignoring", lockFile)
		return nil
	}

	lock, err := lockconfig.NewImagesLockFromPath(lockFile)
	if err != nil {
		return fmt.Errorf("failed to load Carvel images lock: %v", err)
	}
	result, err := RelocateCarvelImagesLock(&lock, prefix)
	if err != nil {
		return err
	}
	if result.Count == 0 {
		return nil
	}
	if err := utils.SafeWriteFile(lockFile, result.Data, 0600); err != nil {
		return fmt.Errorf("failed to overwrite Carvel images lock file: %v", err)
	}
	return nil
}

// RelocateCarvelImagesLock rewrites the images urls in the provided lock using prefix
func RelocateCarvelImagesLock(lock *lockconfig.ImagesLock, prefix string) (*RelocationResult, error) {

	count, err := relocateCarvelImages(lock.Images, prefix)
	if err != nil {
		return nil, fmt.Errorf("failed to relocate Carvel images lock file: %v", err)
	}

	if buff, err := lock.AsBytes(); err != nil {
		return nil, fmt.Errorf("failed to write Images.lock file: %v", err)
	} else {
		return &RelocationResult{Data: buff, Count: count}, nil
	}
}

func relocateCarvelImages(images []lockconfig.ImageRef, prefix string) (count int, err error) {
	var allErrors error
	for i, img := range images {
		norm, err := utils.RelocateImageURL(img.Image, prefix, true)
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
