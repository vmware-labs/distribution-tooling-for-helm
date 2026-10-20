package chartutils

import (
	"errors"
	"fmt"
	"net/http"
	"regexp"
	"sort"
	"strings"

	"github.com/google/go-containerregistry/pkg/crane"
	"github.com/google/go-containerregistry/pkg/name"
	v1 "github.com/google/go-containerregistry/pkg/v1"
	"github.com/google/go-containerregistry/pkg/v1/remote"
	"github.com/google/go-containerregistry/pkg/v1/remote/transport"
	"github.com/vmware-labs/distribution-tooling-for-helm/pkg/artifacts"
	"github.com/vmware-labs/distribution-tooling-for-helm/pkg/imagelock"
)

var (
	// Exclude revision tags from aliases even if their version/distro format
	// differs from charts-syncer's standalone image discovery formats.
	revisionTag       = regexp.MustCompile(`-r[0-9]+$`)
	validAliasTag     = regexp.MustCompile(`^[a-zA-Z0-9_][a-zA-Z0-9_.-]{0,127}$`)
	digestArtifactTag = regexp.MustCompile(`^sha256-[0-9a-f]{64}\.(sig|metadata)$`)
)

// artifactTag recognizes DT's signature and metadata naming conventions.
// These artifacts are copied separately and need no alias-discovery requests.
func artifactTag(tag string) bool {
	return digestArtifactTag.MatchString(tag) || strings.HasSuffix(tag, "-metadata")
}

func manifestNotFound(err error) bool {
	var transportErr *transport.Error
	return errors.As(err, &transportErr) && transportErr.StatusCode == http.StatusNotFound
}

// aliasDiscovery caches a repository's tag snapshot for one pull operation.
// Only selected images are downloaded; other tags are inspected for identity.
type aliasDiscovery map[string]map[string]v1.Hash

func (discovery aliasDiscovery) capture(img *imagelock.ChartImage, o crane.Options) (name.Digest, error) {
	ref, err := name.ParseReference(img.Image, o.Name...)
	if err != nil {
		return name.Digest{}, err
	}
	desc, err := remote.Get(ref, o.Remote...)
	if err != nil {
		return name.Digest{}, err
	}
	if err := artifacts.VerifyLockedDigests(img, desc); err != nil {
		return name.Digest{}, err
	}
	repo := ref.Context()
	tagDigests, ok := discovery[repo.Name()]
	if !ok {
		tags, err := remote.List(repo, o.Remote...)
		if err != nil {
			return name.Digest{}, fmt.Errorf("listing aliases for %q: %w", repo, err)
		}
		tagDigests = make(map[string]v1.Hash)
		for _, tag := range tags {
			if revisionTag.MatchString(tag) || artifactTag(tag) {
				continue
			}
			if !validAliasTag.MatchString(tag) {
				return name.Digest{}, fmt.Errorf("invalid alias tag %q in %q", tag, repo)
			}
			d, err := remote.Head(repo.Tag(tag), o.Remote...)
			if manifestNotFound(err) { // A tag can disappear between list and HEAD.
				continue
			}
			if err != nil {
				return name.Digest{}, fmt.Errorf("resolving alias %q: %w", repo.Tag(tag), err)
			}
			tagDigests[tag] = d.Digest
		}
		discovery[repo.Name()] = tagDigests
	}
	// Replace any snapshot inherited from a source chart's existing lock.
	img.Aliases = nil
	for tag, digest := range tagDigests {
		// Exact manifest identity also excludes artifacts with unfamiliar tag
		// names, including artifacts that use the OCI image manifest format.
		if digest == desc.Digest && tag != ref.Identifier() {
			img.Aliases = append(img.Aliases, tag)
		}
	}
	sort.Strings(img.Aliases)
	return repo.Digest(desc.Digest.String()), nil
}

// stageImageWithAliases uploads content by digest first. Its completion step
// publishes aliases and then the primary reference, after the caller has also
// pushed artifacts. A failed alias write therefore cannot leave a new revision
// tag that charts-syncer would treat as completed on its next run.
func stageImageWithAliases(img *imagelock.ChartImage, imagesDir string, cfg *Configuration, o crane.Options) (*stagedImage, error) {
	ref, err := name.ParseReference(img.Image, o.Name...)
	if err != nil {
		return nil, err
	}
	aliases := make([]string, 0, len(img.Aliases))
	seen := make(map[string]bool)
	for _, alias := range img.Aliases {
		if !validAliasTag.MatchString(alias) {
			return nil, fmt.Errorf("invalid alias tag %q for %q", alias, img.Image)
		}
		// Artifact tags never come from discovery; skip them so a hand-edited
		// lock cannot overwrite signatures or metadata at the destination.
		if revisionTag.MatchString(alias) || artifactTag(alias) || alias == ref.Identifier() || seen[alias] {
			continue
		}
		seen[alias] = true
		aliases = append(aliases, alias)
	}
	sort.Strings(aliases)

	digest, upload, err := prepareImageUpload(img, imagesDir, cfg, o)
	if err != nil {
		return nil, err
	}

	immutableRef := ref.Context().Digest(digest.String())
	if err = upload(immutableRef); err != nil {
		return nil, err
	}
	published, err := remote.Get(immutableRef, o.Remote...)
	if err != nil {
		return nil, fmt.Errorf("reading published image %q: %w", immutableRef, err)
	}
	if published.Digest != digest {
		return nil, fmt.Errorf("published image %q has unexpected digest %s", immutableRef, published.Digest)
	}

	return &stagedImage{digest: published.Digest, publishTags: func() error {
		if err := publishAliases(ref.Context(), aliases, published, cfg, o); err != nil {
			return err
		}
		return publishPrimary(ref, published, o)
	}}, nil
}

func prepareImageUpload(img *imagelock.ChartImage, imagesDir string, cfg *Configuration, o crane.Options) (v1.Hash, func(name.Reference) error, error) {
	var digest v1.Hash
	var err error
	var upload func(name.Reference) error
	if cfg.PreserveDigest {
		dir := verbatimImageDir(imagesDir, img)
		digest, err = artifacts.VerbatimDigest(dir)
		upload = func(dst name.Reference) error {
			return artifacts.PushVerbatim(cfg.Context, dir, dst.Name(),
				artifacts.WithAuth(cfg.Auth.Username, cfg.Auth.Password),
				artifacts.WithInsecureMode(cfg.InsecureMode))
		}
	} else {
		var idx v1.ImageIndex
		idx, err = buildImageIndex(img, imagesDir)
		if err == nil {
			digest, err = idx.Digest()
		}
		upload = func(dst name.Reference) error { return remote.WriteIndex(dst, idx, o.Remote...) }
	}
	if err != nil {
		return v1.Hash{}, nil, err
	}

	return digest, upload, nil
}

func publishAliases(repo name.Repository, aliases []string, published *remote.Descriptor, cfg *Configuration, o crane.Options) error {
	for _, alias := range aliases {
		tag := repo.Tag(alias)
		current, err := remote.Head(tag, o.Remote...)
		if err == nil && current.Digest == published.Digest {
			continue
		}
		if err != nil && !manifestNotFound(err) {
			return fmt.Errorf("checking destination alias %q: %w", tag, err)
		}
		if err := remote.Tag(tag, published, o.Remote...); err != nil {
			return fmt.Errorf("publishing alias %q: %w", tag, err)
		}
		cfg.Log.Infof("Synced alias %q to %s", tag, published.Digest)
	}
	return nil
}

func publishPrimary(ref name.Reference, published *remote.Descriptor, o crane.Options) error {
	// The primary reference was explicitly selected for copying, so retain
	// PushImages' overwrite behavior regardless of alias synchronization.
	current, err := remote.Head(ref, o.Remote...)
	if err == nil && current.Digest == published.Digest {
		return nil
	}
	if err != nil && !manifestNotFound(err) {
		return fmt.Errorf("checking destination image %q: %w", ref, err)
	}
	return remote.Put(ref, published, o.Remote...)
}

type stagedImage struct {
	digest      v1.Hash
	publishTags func() error
}
