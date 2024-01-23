package chartutils

import (
	"context"

	"github.com/vmware-labs/distribution-tooling-for-helm/pkg/log"

	"github.com/vmware-labs/distribution-tooling-for-helm/internal/widgets"
	"github.com/vmware-labs/distribution-tooling-for-helm/pkg/imagelock"
)

// Configuration defines configuration settings used in chartutils functions
type Configuration struct {
	AnnotationsKey string
	Log            log.Logger
	Context        context.Context
	ProgressBar    widgets.ProgressBar
	ArtifactsDir   string
	FetchArtifacts bool
	MaxRetries     int
	InsecureMode   bool
}

// WithInsecureMode configures Insecure transport
func WithInsecureMode(insecure bool) func(cfg *Configuration) {
	return func(cfg *Configuration) {
		cfg.InsecureMode = insecure
	}
}

// WithArtifactsDir configures the ArtifactsDir
func WithArtifactsDir(dir string) func(cfg *Configuration) {
	return func(cfg *Configuration) {
		cfg.ArtifactsDir = dir
	}
}

// WithFetchArtifacts configures the FetchArtifacts setting
func WithFetchArtifacts(fetch bool) func(cfg *Configuration) {
	return func(cfg *Configuration) {
		cfg.FetchArtifacts = fetch
	}
}

// WithContext provides an execution context
func WithContext(ctx context.Context) func(cfg *Configuration) {
	return func(cfg *Configuration) {
		cfg.Context = ctx
	}
}

// WithMaxRetries configures the number of retries on error
func WithMaxRetries(retries int) func(cfg *Configuration) {
	return func(cfg *Configuration) {
		cfg.MaxRetries = retries
	}
}

// WithProgressBar provides a ProgressBar for long running operations
func WithProgressBar(pb widgets.ProgressBar) func(cfg *Configuration) {
	return func(cfg *Configuration) {
		cfg.ProgressBar = pb
	}
}

// NewConfiguration returns a new Configuration
func NewConfiguration(opts ...Option) *Configuration {
	cfg := &Configuration{
		AnnotationsKey: imagelock.DefaultAnnotationsKey,
		Context:        context.Background(),
		ProgressBar:    widgets.NewSilentProgressBar(),
		ArtifactsDir:   "",
		FetchArtifacts: false,
		MaxRetries:     3,
		Log:            log.NewSilentLogger(),
		InsecureMode:   false,
	}
	for _, opt := range opts {
		opt(cfg)
	}
	return cfg
}

// Option defines a configuration option
type Option func(c *Configuration)

// WithLog provides a log to use
func WithLog(l log.Logger) func(cfg *Configuration) {
	return func(cfg *Configuration) {
		cfg.Log = l
	}
}

// WithAnnotationsKey customizes the annotations key to use when reading/writing images
// to the Chart.yaml
func WithAnnotationsKey(str string) func(cfg *Configuration) {
	return func(cfg *Configuration) {
		cfg.AnnotationsKey = str
	}
}
