package relocator

import (
	"github.com/vmware-labs/distribution-tooling-for-helm/pkg/log"
	silentLog "github.com/vmware-labs/distribution-tooling-for-helm/pkg/log/silent"

	"github.com/vmware-labs/distribution-tooling-for-helm/pkg/imagelock"
)

// RelocateConfig defines the configuration used in the relocator functions
type RelocateConfig struct {
	ImageLockConfig  imagelock.Config
	Log              log.Logger
	RelocateLockFile bool
	Recursive        bool
	ValuesFiles      []string
}

// NewRelocateConfig returns a new RelocateConfig with default settings
func NewRelocateConfig(opts ...RelocateOption) *RelocateConfig {
	cfg := &RelocateConfig{
		Log:              silentLog.NewLogger(),
		RelocateLockFile: true,
		ImageLockConfig:  *imagelock.NewImagesLockConfig(),
		ValuesFiles:      []string{"values.yaml"},
	}
	for _, opt := range opts {
		opt(cfg)
	}

	return cfg
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

// WithValuesFiles configures the values files to use for relocation
func WithValuesFiles(files ...string) func(rc *RelocateConfig) {
	return func(rc *RelocateConfig) {
		rc.ValuesFiles = files
	}
}
