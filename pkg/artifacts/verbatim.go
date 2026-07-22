package artifacts

import (
	"context"
	"errors"
	"fmt"
	"os"

	"github.com/google/go-containerregistry/pkg/authn"
	"github.com/google/go-containerregistry/pkg/crane"
	"github.com/google/go-containerregistry/pkg/name"
	v1 "github.com/google/go-containerregistry/pkg/v1"
	"github.com/google/go-containerregistry/pkg/v1/empty"
	"github.com/google/go-containerregistry/pkg/v1/layout"
	"github.com/google/go-containerregistry/pkg/v1/remote"
	digest "github.com/opencontainers/go-digest"

	"github.com/vmware-labs/distribution-tooling-for-helm/pkg/imagelock"
)

func verbatimCraneOptions(ctx context.Context, cfg *Config) []crane.Option {
	opts := []crane.Option{crane.WithContext(ctx)}
	if cfg.InsecureMode {
		opts = append(opts, crane.Insecure)
	}
	if cfg.Auth.Username != "" && cfg.Auth.Password != "" {
		opts = append(opts, crane.WithAuth(&authn.Basic{
			Username: cfg.Auth.Username,
			Password: cfg.Auth.Password,
		}))
	}
	return opts
}

// PullVerbatim fetches the OCI artifact referenced by ref exactly as served by
// the registry - a single manifest or a full, unfiltered index, whichever it
// is - and stores it as a single-entry OCI layout directory at destDir. It
// never goes through pkg/v1/mutate, so the artifact's manifest/index digest,
// and every nested per-platform manifest and layer digest, are preserved
// byte-for-byte. The returned descriptor carries the digest(s) actually found
// upstream, for callers that want to cross-check them (see
// VerifyLockedDigests).
func PullVerbatim(ctx context.Context, ref string, destDir string, opts ...Option) (*remote.Descriptor, error) {
	cfg := NewConfig(opts...)
	craneOpts := verbatimCraneOptions(ctx, cfg)

	desc, err := imagelock.GetImageRemoteDescriptor(ref, craneOpts...)
	if err != nil {
		return nil, fmt.Errorf("failed to fetch %q: %w", ref, err)
	}

	if err = os.MkdirAll(destDir, 0755); err != nil {
		return nil, fmt.Errorf("failed to create %q: %w", destDir, err)
	}

	p, err := layout.Write(destDir, empty.Index)
	if err != nil {
		return nil, fmt.Errorf("failed to initialize OCI layout at %q: %w", destDir, err)
	}

	switch {
	case desc.MediaType.IsIndex():
		idx, err := desc.ImageIndex()
		if err != nil {
			return nil, fmt.Errorf("failed to read image index for %q: %w", ref, err)
		}
		if err := p.AppendIndex(idx); err != nil {
			return nil, fmt.Errorf("failed to store image index for %q: %w", ref, err)
		}
	case desc.MediaType.IsImage():
		img, err := desc.Image()
		if err != nil {
			return nil, fmt.Errorf("failed to read image for %q: %w", ref, err)
		}
		if err := p.AppendImage(img); err != nil {
			return nil, fmt.Errorf("failed to store image for %q: %w", ref, err)
		}
	default:
		return nil, fmt.Errorf("unsupported media type %q for %q", desc.MediaType, ref)
	}
	return desc, nil
}

// loadVerbatimEntry loads the single top-level entry (a v1.Image or a
// v1.ImageIndex) previously written by PullVerbatim from the OCI layout
// directory at dir, without re-serializing it - RawManifest() on the returned
// object returns the exact bytes originally fetched from the source registry.
func loadVerbatimEntry(dir string) (any, error) {
	l, err := layout.FromPath(dir)
	if err != nil {
		return nil, fmt.Errorf("loading %q as OCI layout: %w", dir, err)
	}
	rootIdx, err := l.ImageIndex()
	if err != nil {
		return nil, err
	}
	im, err := rootIdx.IndexManifest()
	if err != nil {
		return nil, err
	}
	if len(im.Manifests) != 1 {
		return nil, fmt.Errorf("layout %q contains %d entries, expected exactly 1", dir, len(im.Manifests))
	}
	desc := im.Manifests[0]
	switch {
	case desc.MediaType.IsIndex():
		return rootIdx.ImageIndex(desc.Digest)
	case desc.MediaType.IsImage():
		return rootIdx.Image(desc.Digest)
	default:
		return nil, fmt.Errorf("layout %q contains unsupported media type %q", dir, desc.MediaType)
	}
}

// PushVerbatim reads the single-entry OCI layout directory previously written
// by PullVerbatim at srcDir and pushes it to dstRef exactly as stored - again,
// never through pkg/v1/mutate - preserving its digest. After pushing, it
// re-fetches dstRef's digest from the registry and fails loudly on any
// mismatch against the digest of what was pushed, guarding against
// non-conformant registries that mutate manifests on write.
func PushVerbatim(ctx context.Context, srcDir string, dstRef string, opts ...Option) error {
	cfg := NewConfig(opts...)
	craneOpts := verbatimCraneOptions(ctx, cfg)
	o := crane.GetOptions(craneOpts...)

	entry, err := loadVerbatimEntry(srcDir)
	if err != nil {
		return fmt.Errorf("failed to load OCI layout at %q: %w", srcDir, err)
	}

	ref, err := name.ParseReference(dstRef, o.Name...)
	if err != nil {
		return fmt.Errorf("failed to parse reference %q: %w", dstRef, err)
	}

	var wantDigest v1.Hash
	switch t := entry.(type) {
	case v1.ImageIndex:
		if wantDigest, err = t.Digest(); err != nil {
			return fmt.Errorf("failed to compute digest for %q: %w", dstRef, err)
		}
		if err = remote.WriteIndex(ref, t, o.Remote...); err != nil {
			return fmt.Errorf("failed to push %q: %w", dstRef, err)
		}
	case v1.Image:
		if wantDigest, err = t.Digest(); err != nil {
			return fmt.Errorf("failed to compute digest for %q: %w", dstRef, err)
		}
		if err = remote.Write(ref, t, o.Remote...); err != nil {
			return fmt.Errorf("failed to push %q: %w", dstRef, err)
		}
	default:
		return fmt.Errorf("unsupported artifact type %T for %q", t, dstRef)
	}

	got, err := remote.Head(ref, o.Remote...)
	if err != nil {
		return fmt.Errorf("failed to verify pushed digest for %q: %w", dstRef, err)
	}
	if got.Digest != wantDigest {
		return fmt.Errorf("digest mismatch after pushing %q: pushed %s, registry reports %s", dstRef, wantDigest, got.Digest)
	}
	return nil
}

// VerifyLockedDigests cross-checks the per-platform digests recorded in
// image.Digests (as generated when Images.lock was written) against the
// digests actually present in desc, a descriptor freshly fetched from the
// source registry (e.g. via PullVerbatim). It returns an error on any
// mismatch or missing platform.
//
// This is the only safe use of Images.lock's per-platform digest list in a
// digest-preserving transfer: Images.lock is a lossy projection of the source
// index (imagelock.readDigestsInfoFromIndex deliberately drops
// attestation-manifest entries), so it can never be used to *reconstruct* an
// index byte-for-byte - only to sanity-check, after an independent verbatim
// fetch, that the tag referenced by image.Image still points at the content
// that was locked. This guards against a TOCTOU where the upstream tag moved
// between "Images.lock generation" and "mirror" time.
func VerifyLockedDigests(image *imagelock.ChartImage, desc *remote.Descriptor) error {
	switch {
	case desc.MediaType.IsIndex():
		idx, err := desc.ImageIndex()
		if err != nil {
			return fmt.Errorf("failed to read image index for %q: %w", image.Image, err)
		}
		im, err := idx.IndexManifest()
		if err != nil {
			return fmt.Errorf("failed to read index manifest for %q: %w", image.Image, err)
		}
		found := make(map[string]digest.Digest)
		for _, m := range im.Manifests {
			if !m.MediaType.IsImage() || m.Platform == nil {
				continue
			}
			arch := fmt.Sprintf("%s/%s", m.Platform.OS, m.Platform.Architecture)
			found[arch] = digest.Digest(m.Digest.String())
		}
		var allErrors error
		for _, want := range image.Digests {
			got, ok := found[want.Arch]
			if !ok {
				allErrors = errors.Join(allErrors, fmt.Errorf(
					"locked digest for %q arch %q not found in remote index (upstream tag may have moved)",
					image.Image, want.Arch))
				continue
			}
			if got != want.Digest {
				allErrors = errors.Join(allErrors, fmt.Errorf(
					"digest mismatch for %q arch %q: locked %s, remote %s",
					image.Image, want.Arch, want.Digest, got))
			}
		}
		return allErrors
	case desc.MediaType.IsImage():
		if len(image.Digests) != 1 {
			return fmt.Errorf("expected exactly one locked digest for single-manifest image %q, got %d", image.Image, len(image.Digests))
		}
		got := digest.Digest(desc.Digest.String())
		if got != image.Digests[0].Digest {
			return fmt.Errorf("digest mismatch for %q: locked %s, remote %s", image.Image, image.Digests[0].Digest, got)
		}
		return nil
	default:
		return fmt.Errorf("unsupported media type %q for %q", desc.MediaType, image.Image)
	}
}
