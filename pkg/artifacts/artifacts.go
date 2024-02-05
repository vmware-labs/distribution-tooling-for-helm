// Package artifacts implements support to pushing and pulling artifacts to an OCI registry
package artifacts

import (
	"context"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/google/go-containerregistry/pkg/authn"

	"github.com/google/go-containerregistry/pkg/name"
	v1 "github.com/google/go-containerregistry/pkg/v1"
	"github.com/google/go-containerregistry/pkg/v1/layout"
	"github.com/google/go-containerregistry/pkg/v1/partial"
	"github.com/google/go-containerregistry/pkg/v1/remote"
	"github.com/opencontainers/go-digest"
	"golang.org/x/exp/slices"

	"github.com/google/go-containerregistry/pkg/crane"
	"github.com/vmware-labs/distribution-tooling-for-helm/pkg/imagelock"
	"github.com/vmware-labs/distribution-tooling-for-helm/pkg/utils"
)

const (
	// HelmArtifactsFolder defines the path of the chart artifacts, relative to the bundle root
	HelmArtifactsFolder = "artifacts"

	// HelmChartArtifactMetadataDir defines the relative path to the chart metadata inside the chart root
	HelmChartArtifactMetadataDir = HelmArtifactsFolder + "/chart/metadata"
)

var (
	// ErrTagDoesNotExist defines an error locating a remote tag because it does not exist
	ErrTagDoesNotExist = errors.New("tag does not exist")
	// ErrLocalArtifactNotExist defines an error locating a local artifact because it does not exist
	ErrLocalArtifactNotExist = errors.New("local artifact does not exist")
)

type Auth struct {
	Username string
	Password string
}

// Config defines the configuration when pulling/pushing artifacts to a registry
type Config struct {
	ResolveReference bool
	InsecureMode     bool
	Auth             Auth
}

// Option defines a Config option
type Option func(*Config)

// WithAuth configures the Auth
func WithAuth(username, password string) func(cfg *Config) {
	return func(cfg *Config) {
		cfg.Auth = Auth{
			Username: username,
			Password: password,
		}
	}
}

// WithInsecureMode configures Insecure transport
func WithInsecureMode(insecure bool) func(cfg *Config) {
	return func(cfg *Config) {
		cfg.InsecureMode = insecure
	}
}

// WithResolveReference configures the ResolveReference setting
func WithResolveReference(v bool) func(cfg *Config) {
	return func(cfg *Config) {
		cfg.ResolveReference = v
	}
}

// NewConfig creates a new Config
func NewConfig(opts ...Option) *Config {
	cfg := &Config{ResolveReference: true, InsecureMode: false}
	for _, opt := range opts {
		opt(cfg)
	}
	return cfg
}

func getImageTagAndDigest(image string, opts ...Option) (string, string, error) {
	ref, err := name.ParseReference(image)
	if err != nil {
		return "", "", fmt.Errorf("failed to parse image reference: %w", err)
	}

	var hex string
	var imgTag string

	switch v := ref.(type) {
	case name.Tag:
		cfg := NewConfig(opts...)
		craneOpts := make([]crane.Option, 0)
		if cfg.InsecureMode {
			craneOpts = append(craneOpts, crane.Insecure)
		}
		if cfg.Auth.Password != "" && cfg.Auth.Username != "" {
			craneOpts = append(craneOpts, crane.WithAuth(&authn.Basic{
				Username: cfg.Auth.Username,
				Password: cfg.Auth.Password,
			}))
		}
		desc, err := imagelock.GetImageRemoteDescriptor(image, craneOpts...)
		if err != nil {
			return "", "", fmt.Errorf("error getting descriptor: %w", err)
		}
		hex = desc.Digest.Hex
		imgTag = v.TagStr()
	case name.Digest:
		digestStr := v.DigestStr()
		prefix := digest.Canonical.String() + ":"
		if !strings.HasPrefix(digestStr, prefix) {
			return "", "", fmt.Errorf("unsupported digest algorithm: %s", digestStr)
		}
		hex = strings.TrimPrefix(digestStr, prefix)
		imgTag = strings.TrimPrefix(digestStr, prefix)
	default:
		return "", "", fmt.Errorf("unsupported reference type %T", v)
	}
	return imgTag, hex, nil
}

func getImageArtifactsDir(image *imagelock.ChartImage, destDir string, suffix string, opts ...Option) (string, error) {
	imgTag, _, err := getImageTagAndDigest(image.Image, opts...)
	if err != nil {
		return "", fmt.Errorf("failed to parse image reference: %w", err)
	}

	return filepath.Join(destDir, image.Chart, image.Name, fmt.Sprintf("%s.%s", imgTag, suffix)), nil
}

func pushArtifact(ctx context.Context, image string, dest string, tagSuffix string, opts ...Option) (string, error) {
	cfg := NewConfig(opts...)
	if !utils.FileExists(dest) {
		return "", ErrLocalArtifactNotExist
	}
	craneOpts := []crane.Option{crane.WithContext(ctx)}

	if cfg.Auth.Password != "" && cfg.Auth.Username != "" {
		craneOpts = append(craneOpts, crane.WithAuth(&authn.Basic{
			Username: cfg.Auth.Username,
			Password: cfg.Auth.Password,
		}))
	}
	repo, err := getImageRepository(image)
	if err != nil {
		return "", fmt.Errorf("failed to get image repository: %w", err)
	}

	imgTag, hex, err := getImageTagAndDigest(image, opts...)
	if err != nil {
		return "", err
	}

	var tag string
	if cfg.ResolveReference {
		tag = fmt.Sprintf("sha256-%s.%s", hex, tagSuffix)
	} else {
		tag = fmt.Sprintf("%s-%s", imgTag, tagSuffix)
	}
	img, err := loadImage(dest)
	if err != nil {
		return "", err
	}

	newImg := fmt.Sprintf("%s:%s", repo, tag)

	switch t := img.(type) {
	case v1.Image:
		return tag, crane.Push(t, newImg, craneOpts...)
	default:
		return "", fmt.Errorf("unsupported image type %T", t)
	}
}

func pushAssetMetadata(ctx context.Context, imageRef string, destDir string, opts ...Option) error {
	tag, err := pushArtifact(ctx, imageRef, destDir, "metadata", opts...)
	if err != nil {
		return err
	}
	repo, err := getImageRepository(imageRef)
	if err != nil {
		return fmt.Errorf("failed to get image repository: %w", err)
	}
	metadataImg := fmt.Sprintf("%s:%s", repo, tag)

	metadataSigDir := fmt.Sprintf("%s.sig", destDir)
	// For the metadata pull, we may want to not resolve the tag to the shasum, but for the signature, we need to do it,
	// so we enfoce it here
	_, err = pushArtifact(ctx, metadataImg, metadataSigDir, "sig", append(opts, WithResolveReference(true))...)
	if err != nil {
		return err
	}
	return nil
}

// PushImageMetadata pushes a oci-layout directory to the registry as the image metadata
func PushImageMetadata(ctx context.Context, image *imagelock.ChartImage, destDir string, opts ...Option) error {
	imageRef := image.Image

	dir, err := getImageArtifactsDir(image, destDir, "metadata", opts...)
	if err != nil {
		return fmt.Errorf("failed to obtain metadata location: %v", err)
	}

	return pushAssetMetadata(ctx, imageRef, dir, opts...)
}

// PushImageSignatures pushes a oci-layout directory to the registry as the image signature
func PushImageSignatures(ctx context.Context, image *imagelock.ChartImage, destDir string, opts ...Option) error {
	imageRef := image.Image
	dir, err := getImageArtifactsDir(image, destDir, "sig", opts...)
	if err != nil {
		return fmt.Errorf("failed to obtain signature location: %v", err)
	}
	_, err = pushArtifact(ctx, imageRef, dir, "sig", opts...)
	if err != nil {
		return err
	}
	return nil
}

func getImageRepository(image string) (string, error) {
	ref, err := name.ParseReference(image)
	if err != nil {
		return "", fmt.Errorf("failed to parse image reference: %w", err)
	}
	return ref.Context().Name(), nil
}

func pullArtifact(ctx context.Context, image string, destDir string, tagSuffix string, opts ...Option) (string, error) {
	cfg := NewConfig(opts...)

	craneOpts := []crane.Option{crane.WithContext(ctx)}
	if cfg.InsecureMode {
		craneOpts = append(craneOpts, crane.Insecure)
	}
	if cfg.Auth.Password != "" && cfg.Auth.Username != "" {
		craneOpts = append(craneOpts, crane.WithAuth(&authn.Basic{
			Username: cfg.Auth.Username,
			Password: cfg.Auth.Password,
		}))
	}
	o := crane.GetOptions(craneOpts...)

	repo, err := getImageRepository(image)
	if err != nil {
		return "", fmt.Errorf("failed to get image repository: %w", err)
	}

	var tag string
	imgTag, hex, err := getImageTagAndDigest(image, opts...)
	if err != nil {
		return "", err
	}

	if cfg.ResolveReference {
		tag = fmt.Sprintf("sha256-%s.%s", hex, tagSuffix)
	} else {
		tag = fmt.Sprintf("%s-%s", imgTag, tagSuffix)
	}

	exist, err := TagExist(ctx, repo, tag, o)
	if err != nil {
		return "", fmt.Errorf("failed to check tag %q: %w", tag, err)
	}
	if !exist {
		return "", ErrTagDoesNotExist
	}

	newImg := fmt.Sprintf("%s:%s", repo, tag)
	rmt, err := imagelock.GetImageRemoteDescriptor(newImg, craneOpts...)
	if err != nil {
		return "", err
	}
	img, err := rmt.Image()
	if err != nil {
		return "", err
	}
	if err := saveImage(img, destDir); err != nil {
		return "", err
	}
	return tag, nil
}

// PullImageMetadata pulls the image metadata and stores it locally as an oci-layout
func PullImageMetadata(ctx context.Context, image *imagelock.ChartImage, destDir string, opts ...Option) error {
	imageRef := image.Image

	dir, err := getImageArtifactsDir(image, destDir, "metadata", opts...)
	if err != nil {
		return fmt.Errorf("failed to obtain metadata location: %v", err)
	}

	return pullAssetMetadata(ctx, imageRef, dir, opts...)
}

func pullAssetMetadata(ctx context.Context, imageRef string, dir string, opts ...Option) error {
	tag, err := pullArtifact(ctx, imageRef, dir, "metadata", opts...)
	if err != nil {
		return err
	}
	repo, err := getImageRepository(imageRef)
	if err != nil {
		return fmt.Errorf("failed to get image repository: %w", err)
	}
	metadataImg := fmt.Sprintf("%s:%s", repo, tag)

	// For the metadata pull, we may want to not resolve the tag to the shasum, but for the signature, we need to do it,
	// so we enfoce it here
	metadataSigDir := fmt.Sprintf("%s.sig", dir)
	_, err = pullArtifact(ctx, metadataImg, metadataSigDir, "sig", append(opts, WithResolveReference(true))...)
	if err != nil {
		return err
	}
	return nil
}

// PullImageSignatures pulls the image signature and stores it locally as an oci-layout
func PullImageSignatures(ctx context.Context, image *imagelock.ChartImage, destDir string, opts ...Option) error {
	imageRef := image.Image
	dir, err := getImageArtifactsDir(image, destDir, "sig", opts...)
	if err != nil {
		return fmt.Errorf("failed to obtain signature location: %v", err)
	}
	_, err = pullArtifact(ctx, imageRef, dir, "sig", opts...)
	if err != nil {
		return err
	}
	return nil
}

// TagExist checks if a given tag exist in the provided repository
func TagExist(ctx context.Context, src string, tag string, o crane.Options) (bool, error) {
	result, err := listTags(ctx, src, o)
	if err != nil {
		return false, err
	}
	return slices.Contains(result, tag), nil
}

// ListTags lists the defined tags in the repository
func ListTags(ctx context.Context, src string, opts ...crane.Option) ([]string, error) {
	o := crane.GetOptions(opts...)
	return listTags(ctx, src, o)
}

func listTags(ctx context.Context, src string, o crane.Options) ([]string, error) {
	result := make([]string, 0)
	repo, err := name.NewRepository(src, o.Name...)
	if err != nil {
		return nil, fmt.Errorf("parsing repo %q: %w", src, err)
	}

	puller, err := remote.NewPuller(o.Remote...)
	if err != nil {
		return nil, err
	}

	lister, err := puller.Lister(ctx, repo)
	if err != nil {
		return nil, fmt.Errorf("reading tags for %s: %w", repo, err)
	}

	for lister.HasNext() {
		tags, err := lister.Next(ctx)
		if err != nil {
			return result, err
		}
		result = append(result, tags.Tags...)
	}
	return result, nil
}

func saveImage(img v1.Image, dir string) error {
	if err := crane.SaveOCI(img, dir); err != nil {
		return fmt.Errorf("failed to save image: %v", err)
	}
	return nil
}

func loadImage(path string) (partial.WithRawManifest, error) {
	stat, err := os.Stat(path)
	if err != nil {
		return nil, err
	}

	if !stat.IsDir() {
		return nil, fmt.Errorf("expected %q to be a directory", path)
	}

	l, err := layout.ImageIndexFromPath(path)
	if err != nil {
		return nil, fmt.Errorf("loading %s as OCI layout: %w", path, err)
	}

	m, err := l.IndexManifest()
	if err != nil {
		return nil, err
	}
	if len(m.Manifests) != 1 {
		return nil, fmt.Errorf("layout contains multiple entries (%d)", len(m.Manifests))
	}

	desc := m.Manifests[0]
	if desc.MediaType.IsImage() {
		return l.Image(desc.Digest)
	} else if desc.MediaType.IsIndex() {
		return l.ImageIndex(desc.Digest)
	}
	return nil, fmt.Errorf("layout contains non-image (mediaType: %q)", desc.MediaType)
}
