package wrapping

import (
	"github.com/vmware-labs/distribution-tooling-for-helm/pkg/chartutils"
	"github.com/vmware-labs/distribution-tooling-for-helm/pkg/imagelock"
)

// Lockable defines the interface to support getting images locked
type Lockable interface {
	LockFilePath() string
	ImagesDir() string
	GetImagesLock() (*imagelock.ImagesLock, error)
}

// Wrap defines the interface to implement a Helm chart wrap
type Wrap interface {
	Lockable
	VerifyLock(...imagelock.Option) error

	Chart() *chartutils.Chart
	RootDir() string
	ChartDir() string
	ImageArtifactsDir() string
}

// WrapContainer defines the interface to implement a container image wrap
type WrapContainer interface {
	Lockable

	RootDir() string
	ImageArtifactsDir() string
}
