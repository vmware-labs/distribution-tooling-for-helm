package imagelock

import (
	"encoding/json"
	"errors"
	"fmt"

	"github.com/google/go-containerregistry/pkg/crane"
	"github.com/google/go-containerregistry/pkg/name"
	v1 "github.com/google/go-containerregistry/pkg/v1"
	"github.com/google/go-containerregistry/pkg/v1/remote"
	"github.com/google/go-containerregistry/pkg/v1/types"
	"github.com/opencontainers/go-digest"
)

// DigestInfo defines the digest information for an Architecture
type DigestInfo struct {
	Digest digest.Digest
	Arch   string
}

func fetchImageDigests(r string, cfg *Config) ([]DigestInfo, error) {
	desc, err := getRemoteDescriptor(r, cfg)
	if err != nil {
		return nil, fmt.Errorf("failed to get descriptor: %v", err)
	}

	switch desc.MediaType {

	case types.OCIImageIndex, types.DockerManifestList:
		var idx v1.IndexManifest
		if err := json.Unmarshal(desc.Manifest, &idx); err != nil {
			return nil, fmt.Errorf("failed to parse images data")
		}
		digests, err := readDigestsInfoFromIndex(idx)
		if err != nil {
			return nil, fmt.Errorf("failed to parse multi-arch image digests from remote descriptor: %w", err)
		}
		return digests, nil
	case types.OCIManifestSchema1, types.DockerManifestSchema2:
		img, err := desc.Image()
		if err != nil {
			return nil, fmt.Errorf("faild to get image from descriptor: %w", err)
		}
		digest, err := readDigestInfoFromImage(img)
		if err != nil {
			return nil, fmt.Errorf("failed to parse image digest from remote descriptor: %w", err)
		}
		return []DigestInfo{digest}, nil

	default:
		return nil, fmt.Errorf("unknown media type %q", desc.MediaType)
	}
}

func getRemoteDescriptor(r string, cfg *Config) (*remote.Descriptor, error) {
	opts := make([]crane.Option, 0)
	if cfg.InsecureMode {
		opts = append(opts, crane.Insecure)
	}
	opts = append(opts, crane.WithContext(cfg.Context))

	o := crane.GetOptions(opts...)

	ref, err := name.ParseReference(r, o.Name...)

	if err != nil {
		return nil, fmt.Errorf("failed to parse reference %q: %w", r, err)
	}
	return remote.Get(ref, o.Remote...)
}

func readDigestsInfoFromIndex(idx v1.IndexManifest) ([]DigestInfo, error) {
	digests := make([]DigestInfo, 0)

	var allErrors error

	for _, img := range idx.Manifests {
		// Skip attestations
		if img.Annotations["vnd.docker.reference.type"] == "attestation-manifest" {
			continue
		}
		switch img.MediaType {
		case types.OCIManifestSchema1, types.DockerManifestSchema2:
			imgDigest := DigestInfo{
				Digest: digest.Digest(img.Digest.String()),
				Arch:   fmt.Sprintf("%s/%s", img.Platform.OS, img.Platform.Architecture),
			}
			digests = append(digests, imgDigest)
		default:
			allErrors = errors.Join(allErrors, fmt.Errorf("unknown media type %q", img.MediaType))
			continue
		}
	}
	return digests, allErrors
}

func readDigestInfoFromImage(img v1.Image) (DigestInfo, error) {
	conf, err := img.ConfigFile()
	if err != nil {
		return DigestInfo{}, fmt.Errorf("faild to get image config: %w", err)
	}

	platform := conf.Platform()
	if platform == nil {
		return DigestInfo{}, fmt.Errorf("failed to obtain image platform")
	}

	digestData, err := img.Digest()
	if err != nil {
		return DigestInfo{}, fmt.Errorf("failed to get image digest: %w", err)
	}

	return DigestInfo{
		Arch:   fmt.Sprintf("%s/%s", platform.OS, platform.Architecture),
		Digest: digest.Digest(digestData.String()),
	}, nil
}
