package imagelock

import "context"

// Config defines configuration options for ImageLock functions
type Config struct {
	InsecureMode   bool
	AnnotationsKey string
	Context        context.Context
	Platforms      []string
}

// NewImagesLockConfig returns a new ImageLockConfig with default values
func NewImagesLockConfig(opts ...Option) *Config {
	cfg := &Config{
		AnnotationsKey: DefaultAnnotationsKey,
		Context:        context.Background(),
		Platforms:      make([]string, 0),
	}

	for _, opt := range opts {
		opt(cfg)
	}
	return cfg
}

// Option defines a ImageLockConfig option
type Option func(*Config)

// Insecure asks the tool to allow insecure HTTPS connections to the remote server.
func Insecure(ic *Config) {
	ic.InsecureMode = true
}

// WithPlatforms configures the Platforms of the Config
func WithPlatforms(platforms []string) func(ic *Config) {
	return func(ic *Config) {
		ic.Platforms = platforms
	}
}

// WithInsecure configures the InsecureMode of the Config
func WithInsecure(insecure bool) func(ic *Config) {
	return func(ic *Config) {
		ic.InsecureMode = insecure
	}
}

// WithContext provides an execution context
func WithContext(ctx context.Context) func(ic *Config) {
	return func(ic *Config) {
		ic.Context = ctx
	}
}

// WithAnnotationsKey provides a custom annotation key to use when
// reading/writing the list of images
func WithAnnotationsKey(str string) func(ic *Config) {
	return func(ic *Config) {
		ic.AnnotationsKey = str
	}
}
