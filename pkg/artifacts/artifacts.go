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
	// ArtifactsFolder defines the path of the chart artifacts, relative to the bundle root
	ArtifactsFolder = "artifacts"

	// HelmChartArtifactMetadataDir defines the relative path to the chart metadata inside the chart root
	HelmChartArtifactMetadataDir = ArtifactsFolder + "/chart/metadata"
)

var (
	// ErrTagDoesNotExist defines an error locating a remote tag because it does not exist
	ErrTagDoesNotExist = errors.New("tag does not exist")
	// ErrLocalArtifactNotExist defines an error locating a local artifact because it does not exist
	ErrLocalArtifactNotExist = errors.New("local artifact does not exist")
)

// Auth defines the authentication information to access the container registry
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

// sanitizeArch converts an arch string (e.g. "linux/amd64") into a filesystem-safe
// segment by replacing "/" with "_" (e.g. "linux_amd64").
func sanitizeArch(arch string) string {
	return strings.ReplaceAll(arch, "/", "_")
}

// getImageArchArtifactsDir returns the local path for a per-architecture artifact.
// The path follows the convention <destDir>/<chart>/<name>/<tag>.<sanitizedArch>.<suffix>/,
// keeping each platform's artifact distinct from the manifest-list level artifact.
func getImageArchArtifactsDir(image *imagelock.ChartImage, destDir string, suffix string, arch string, opts ...Option) (string, error) {
	imgTag, _, err := getImageTagAndDigest(image.Image, opts...)
	if err != nil {
		return "", fmt.Errorf("failed to parse image reference: %w", err)
	}

	return filepath.Join(destDir, image.Chart, image.Name,
		fmt.Sprintf("%s.%s.%s", imgTag, sanitizeArch(arch), suffix)), nil
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

// pushArtifactWithHex pushes the local oci-layout artifact at dest to the registry under the
// tag sha256-<hex>.<tagSuffix>, using the provided hex directly rather than resolving the
// image reference against the remote registry. This is used to push signatures under
// content-addressed (inner platform manifest) digest tags that remain stable across registries.
func pushArtifactWithHex(ctx context.Context, image string, dest string, hex string, tagSuffix string, opts ...Option) (string, error) {
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

	tag := fmt.Sprintf("sha256-%s.%s", hex, tagSuffix)
	artifact, err := loadImage(dest)
	if err != nil {
		return "", err
	}

	newImg := fmt.Sprintf("%s:%s", repo, tag)

	switch t := artifact.(type) {
	case v1.Image:
		return tag, crane.Push(t, newImg, craneOpts...)
	default:
		return "", fmt.Errorf("unsupported image type %T", t)
	}
}

// pushMetadataTags pushes the same local oci-layout artifact to the registry
// under both the sha256-based tag (sha256-<hex>.<suffix>) and the image-tag-based
// tag (<imgTag>-<suffix>). It returns the sha256 tag so callers can reference it
// when chaining further artifacts (e.g. the metadata signature).
func pushMetadataTags(ctx context.Context, image string, dest string, tagSuffix string, opts ...Option) (sha256Tag string, err error) {
	sha256Tag, err = pushArtifact(ctx, image, dest, tagSuffix, append(opts, WithResolveReference(true))...)
	if err != nil {
		return "", err
	}
	_, err = pushArtifact(ctx, image, dest, tagSuffix, append(opts, WithResolveReference(false))...)
	if err != nil {
		return sha256Tag, err
	}
	return sha256Tag, nil
}

func pushAssetMetadata(ctx context.Context, imageRef string, destDir string, opts ...Option) error {
	sha256Tag, err := pushMetadataTags(ctx, imageRef, destDir, "metadata", opts...)
	if err != nil {
		return err
	}
	repo, err := getImageRepository(imageRef)
	if err != nil {
		return fmt.Errorf("failed to get image repository: %w", err)
	}
	// The metadata signature is always referenced via the sha256 tag of the metadata artifact.
	metadataImg := fmt.Sprintf("%s:%s", repo, sha256Tag)

	metadataSigDir := fmt.Sprintf("%s.sig", destDir)
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

// PushImageSignatures pushes all locally stored cosign signatures for an image to the
// target registry. It handles two separate cases independently:
//
//  1. Manifest-list level signature: if <tag>.sig/ exists locally, it is pushed under the
//     sha256-<resolved-target-manifest-list-digest>.sig tag.
//
//  2. Per-architecture inner manifest signatures: for each platform digest in image.Digests,
//     if <tag>.<sanitizedArch>.sig/ exists locally (e.g. latest.linux_amd64.sig/), it is
//     pushed under sha256-<inner-digest>.sig.
//
// Returns ErrLocalArtifactNotExist when no local signature exists for any of the above cases.
func PushImageSignatures(ctx context.Context, image *imagelock.ChartImage, destDir string, opts ...Option) error {
	imageRef := image.Image
	pushedAny := false

	// 1. Push manifest-list level sig under the resolved target manifest-list digest tag.
	manifestListDir, err := getImageArtifactsDir(image, destDir, "sig", opts...)
	if err != nil {
		return fmt.Errorf("failed to obtain signature location: %v", err)
	}
	if utils.FileExists(manifestListDir) {
		if _, err := pushArtifact(ctx, imageRef, manifestListDir, "sig", opts...); err != nil {
			return err
		}
		pushedAny = true
	}

	// 2. Push each per-architecture inner manifest sig under sha256-<inner-digest>.sig.
	// Each arch sig has its own correct payload digest, so it is only pushed under the tag
	// that matches its payload — no cross-platform sig duplication.
	for _, dgst := range image.Digests {
		archDir, err := getImageArchArtifactsDir(image, destDir, "sig", dgst.Arch, opts...)
		if err != nil || !utils.FileExists(archDir) {
			continue
		}
		innerHex := dgst.Digest.Hex()
		if _, err := pushArtifactWithHex(ctx, imageRef, archDir, innerHex, "sig", opts...); err != nil {
			return err
		}
		pushedAny = true
	}

	if !pushedAny {
		return ErrLocalArtifactNotExist
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

// pullArchSignatures pulls the per-architecture inner manifest signatures for all platforms
// listed in image.Digests, saving each to <tag>.<sanitizedArch>.sig/. It returns true when
// at least one sig was saved successfully.
func pullArchSignatures(ctx context.Context, image *imagelock.ChartImage, destDir string, opts ...Option) bool {
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

	repo, err := getImageRepository(image.Image)
	if err != nil {
		return false
	}

	foundAny := false
	for _, dgst := range image.Digests {
		innerHex := dgst.Digest.Hex()
		tag := fmt.Sprintf("sha256-%s.sig", innerHex)

		exists, err := TagExist(ctx, repo, tag, o)
		if err != nil || !exists {
			continue
		}
		archDir, err := getImageArchArtifactsDir(image, destDir, "sig", dgst.Arch, opts...)
		if err != nil {
			continue
		}
		rmt, err := imagelock.GetImageRemoteDescriptor(fmt.Sprintf("%s:%s", repo, tag), craneOpts...)
		if err != nil {
			continue
		}
		img, err := rmt.Image()
		if err != nil {
			continue
		}
		if err := saveImage(img, archDir); err != nil {
			continue
		}
		foundAny = true
	}
	return foundAny
}

// PullImageSignatures pulls all available cosign signatures for an image and stores them
// locally as OCI layouts. It handles two separate cases independently:
//
//  1. Manifest-list level signature: if sha256-<manifest-list-digest>.sig exists on the
//     source, it is saved to <tag>.sig/.
//
//  2. Per-architecture inner manifest signatures: for each platform digest in image.Digests,
//     if sha256-<inner-digest>.sig exists on the source, it is saved to
//     <tag>.<sanitizedArch>.sig/ (e.g. latest.linux_amd64.sig/).
//
// Returns ErrTagDoesNotExist only when no signature at all was found.
func PullImageSignatures(ctx context.Context, image *imagelock.ChartImage, destDir string, opts ...Option) error {
	// 1. Manifest-list level sig → <tag>.sig/
	manifestListDir, err := getImageArtifactsDir(image, destDir, "sig", opts...)
	if err != nil {
		return fmt.Errorf("failed to obtain signature location: %v", err)
	}
	_, primaryErr := pullArtifact(ctx, image.Image, manifestListDir, "sig", opts...)
	if primaryErr != nil && primaryErr != ErrTagDoesNotExist {
		return primaryErr
	}

	// 2. Per-architecture inner manifest sigs → <tag>.<arch>.sig/
	archFound := pullArchSignatures(ctx, image, destDir, opts...)

	if primaryErr != nil && !archFound {
		return ErrTagDoesNotExist
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
