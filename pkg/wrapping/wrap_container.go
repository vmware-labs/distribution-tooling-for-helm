package wrapping

import (
	"fmt"
	"os"
	"path/filepath"

	"github.com/vmware-labs/distribution-tooling-for-helm/pkg/artifacts"
	"github.com/vmware-labs/distribution-tooling-for-helm/pkg/imagelock"
)

// wrapContainer defines a wrapped container image
type wrapContainer struct {
	rootDir string
}

// RootDir returns the path to the Wrap root directory
func (w *wrapContainer) RootDir() string {
	return w.rootDir
}

// LockFilePath returns the absolute path to the container Images.lock
func (w *wrapContainer) LockFilePath() string {
	return filepath.Join(w.RootDir(), imagelock.DefaultImagesLockFileName)
}

// ImageArtifactsDir returns the images artifacts directory
func (w *wrapContainer) ImageArtifactsDir() string {
	return filepath.Join(w.RootDir(), artifacts.ArtifactsFolder, "images")
}

// ImagesDir returns the images directory inside the container root directory
func (w *wrapContainer) ImagesDir() string {
	return w.absFilePath("images")
}

// GetImagesLock returns the container's ImagesLock object
func (w *wrapContainer) GetImagesLock() (*imagelock.ImagesLock, error) {
	return imagelock.FromYAMLFile(w.LockFilePath())
}

func (w *wrapContainer) absFilePath(name string) string {
	return filepath.Join(w.rootDir, name)
}

// LoadContainer loads a directory containing a wrapped container and returns a WrapContainer
func LoadContainer(dir string) (WrapContainer, error) {
	return &wrapContainer{rootDir: dir}, nil
}

// CreateContainer creates a new empty container wrap directory structure at destDir and returns a WrapContainer
func CreateContainer(destDir string) (WrapContainer, error) {
	if err := os.MkdirAll(destDir, 0o755); err != nil {
		return nil, fmt.Errorf("failed to create container wrap root directory: %w", err)
	}
	return &wrapContainer{rootDir: destDir}, nil
}
