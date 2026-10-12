package relocator

import (
	"github.com/vmware-labs/distribution-tooling-for-helm/pkg/dtlog"
	silentLog "github.com/vmware-labs/distribution-tooling-for-helm/pkg/dtlog/silent"

	"github.com/vmware-labs/distribution-tooling-for-helm/pkg/imagelock"
)

// RelocateConfig defines the configuration used in the relocator functions
type RelocateConfig struct {
	ImageLockConfig     imagelock.Config
	Log                 dtlog.Logger
	RelocateLockFile    bool
	Recursive           bool
	SkipImageRelocation bool
	ValuesFiles         []string
	PreserveRepository  bool
	// LocalizeDependencies clears the repository of the Chart.yaml dependencies vendored in charts/
	LocalizeDependencies bool
}

// NewRelocateConfig returns a new RelocateConfig with default settings
func NewRelocateConfig(opts ...RelocateOption) *RelocateConfig {
	cfg := &RelocateConfig{
		Log:                 silentLog.NewLogger(),
		SkipImageRelocation: false,
		RelocateLockFile:    true,
		PreserveRepository:  true,
		ImageLockConfig:     *imagelock.NewImagesLockConfig(),
		ValuesFiles:         []string{"values.yaml"},
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

// WithSkipImageRelocation configures the SkipImageRelocation configuration
func WithSkipImageRelocation(skipImageRelocation bool) func(rc *RelocateConfig) {
	return func(rc *RelocateConfig) {
		rc.SkipImageRelocation = skipImageRelocation
	}
}

// WithRelocateLockFile configures the RelocateLockFile configuration
func WithRelocateLockFile(relocateLock bool) func(rc *RelocateConfig) {
	return func(rc *RelocateConfig) {
		rc.RelocateLockFile = relocateLock
	}
}

// WithLog customizes the log used by the tool
func WithLog(l dtlog.Logger) func(rc *RelocateConfig) {
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

// WithPreserveRepository controls whether the source repository path is preserved in the
// relocated URL. When true, the last part of the repository is preserved (e.g., "bitnami/wordpress").
// When false, only the image base name is kept (e.g., "wordpress").
// Use false for respecting destination URLs that already include repository structure.
func WithPreserveRepository(preserve bool) func(rc *RelocateConfig) {
	return func(rc *RelocateConfig) {
		rc.PreserveRepository = preserve
	}
}

// WithLocalizeDependencies controls whether the repository of the dependencies that are already
// vendored under charts/ is cleared in Chart.yaml (and Chart.lock), so Helm treats them as local
// subcharts instead of trying to fetch them from the original repository.
func WithLocalizeDependencies(localize bool) func(rc *RelocateConfig) {
	return func(rc *RelocateConfig) {
		rc.LocalizeDependencies = localize
	}
}
