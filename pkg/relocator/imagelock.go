package relocator

import (
	"bytes"
	"errors"
	"fmt"

	"github.com/vmware-labs/distribution-tooling-for-helm/pkg/imagelock"
	"github.com/vmware-labs/distribution-tooling-for-helm/pkg/utils"
)

func relocateImages(images imagelock.ImageList, prefix string) (count int, err error) {
	var allErrors error
	for _, img := range images {
		norm, err := utils.RelocateImageURL(img.Image, prefix, true)
		if err != nil {
			allErrors = errors.Join(allErrors, err)
			continue
		}
		img.Image = norm
		count++
	}
	return count, allErrors
}

// RelocateLock rewrites the images urls in the provided lock using prefix
func RelocateLock(lock *imagelock.ImagesLock, prefix string) (*RelocationResult, error) {
	count, err := relocateImages(lock.Images, prefix)
	if err != nil {
		return nil, fmt.Errorf("failed to relocate Images.lock file: %v", err)
	}
	buff := &bytes.Buffer{}
	if err := lock.ToYAML(buff); err != nil {
		return nil, fmt.Errorf("failed to write Images.lock file: %v", err)
	}
	return &RelocationResult{Data: buff.Bytes(), Count: count}, nil
}

// RelocateLockFile relocates images urls in the provided Images.lock using prefix
func RelocateLockFile(file string, prefix string) error {
	lock, err := imagelock.FromYAMLFile(file)
	if err != nil {
		return fmt.Errorf("failed to load Images.lock: %v", err)
	}
	result, err := RelocateLock(lock, prefix)
	if err != nil {
		return err
	}
	if result.Count == 0 {
		return nil
	}
	if err := utils.SafeWriteFile(file, result.Data, 0600); err != nil {
		return fmt.Errorf("failed to overwrite Images.lock file: %v", err)
	}
	return nil
}
