package relocator

import (
	"github.com/vmware-labs/distribution-tooling-for-helm/pkg/log"

	"github.com/vmware-labs/distribution-tooling-for-helm/pkg/imagelock"
)

// RelocateConfig defines the configuration used in the relocator functions
type RelocateConfig struct {
	ImageLockConfig  imagelock.Config
	Log              log.Logger
	RelocateLockFile bool
	Recursive        bool
}

// NewRelocateConfig returns a new RelocateConfig with default settings
func NewRelocateConfig() *RelocateConfig {
	return &RelocateConfig{
		Log:              log.SilentLog,
		RelocateLockFile: true,
		ImageLockConfig:  *imagelock.NewImagesLockConfig(),
	}
}

// RelocateOption defines a RelocateConfig option
type RelocateOption func(*RelocateConfig)

// Recursive asks relocation functions to apply to the chart dependencies recursively
func Recursive(c *RelocateConfig) {
	c.Recursive = true
}

// WithAnnotationsKey customizes the annotations key used in Chart.yaml
func WithAnnotationsKey(str string) func(rc *RelocateConfig) {
	return func(rc *RelocateConfig) {
		rc.ImageLockConfig.AnnotationsKey = str
	}
}

// WithRelocateLockFile configures the RelocateLockFile configuration
func WithRelocateLockFile(relocateLock bool) func(rc *RelocateConfig) {
	return func(rc *RelocateConfig) {
		rc.RelocateLockFile = relocateLock
	}
}

// WithLog customizes the log used by the tool
func WithLog(l log.Logger) func(rc *RelocateConfig) {
	return func(rc *RelocateConfig) {
		rc.Log = l
	}
}
