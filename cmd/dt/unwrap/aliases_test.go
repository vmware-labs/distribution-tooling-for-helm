package unwrap_test

import (
	"fmt"
	"io"
	"log"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"sync/atomic"
	"testing"

	"github.com/google/go-containerregistry/pkg/crane"
	"github.com/google/go-containerregistry/pkg/name"
	"github.com/google/go-containerregistry/pkg/registry"
	v1 "github.com/google/go-containerregistry/pkg/v1"
	"github.com/google/go-containerregistry/pkg/v1/empty"
	"github.com/google/go-containerregistry/pkg/v1/mutate"
	"github.com/google/go-containerregistry/pkg/v1/remote"
	"github.com/google/go-containerregistry/pkg/v1/types"
	"github.com/stretchr/testify/require"
	"github.com/vmware-labs/distribution-tooling-for-helm/cmd/dt/unwrap"
	"github.com/vmware-labs/distribution-tooling-for-helm/cmd/dt/wrap"
	"github.com/vmware-labs/distribution-tooling-for-helm/pkg/imagelock"
	"github.com/vmware-labs/distribution-tooling-for-helm/pkg/utils"
)

const aliasRevision = "18.6.0-debian-12-r13"

var imageAliases = []string{"18", "18-debian-12", "18.6.0", "latest"}

func aliasRegistry(t *testing.T, handler func(http.Handler) http.Handler) *httptest.Server {
	t.Helper()
	h := registry.New(registry.Logger(log.New(io.Discard, "", 0)))
	if handler != nil {
		h = handler(h)
	}
	s := httptest.NewServer(h)
	t.Cleanup(s.Close)
	return s
}

func aliasIndex(t *testing.T, revision string) v1.ImageIndex {
	t.Helper()
	idx := mutate.IndexMediaType(empty.Index, types.OCIImageIndex)
	for _, arch := range []string{"amd64", "arm64"} {
		img, err := mutate.ConfigFile(empty.Image, &v1.ConfigFile{
			OS: "linux", Architecture: arch,
			Config: v1.Config{Labels: map[string]string{"revision": revision}},
		})
		require.NoError(t, err)
		idx = mutate.AppendManifests(idx, mutate.IndexAddendum{Add: img,
			Descriptor: v1.Descriptor{Platform: &v1.Platform{OS: "linux", Architecture: arch}}})
	}
	return idx
}

func seedAliasImage(t *testing.T, repo string, idx v1.ImageIndex, tags ...string) {
	t.Helper()
	for _, tag := range tags {
		ref, err := name.NewTag(repo + ":" + tag)
		require.NoError(t, err)
		require.NoError(t, remote.WriteIndex(ref, idx))
	}
}

func aliasDigest(t *testing.T, ref string) string {
	t.Helper()
	d, err := crane.Digest(ref)
	require.NoError(t, err)
	return d
}

// Both bundle formats must carry aliases independently of source access, use
// the actual destination index after filtering/reconstruction, and leave
// unrelated images and revision tags alone.
func TestAliasBundleRoundTrip(t *testing.T) {
	for _, chart := range []bool{false, true} {
		for _, mode := range []string{"rebuilt", "filtered", "preserved"} {
			t.Run(fmt.Sprintf("chart=%t/%s", chart, mode), func(t *testing.T) {
				src := aliasRegistry(t, nil)
				dst := aliasRegistry(t, nil)
				srcRepo := strings.TrimPrefix(src.URL, "http://") + "/postgresql"
				dstPrefix := strings.TrimPrefix(dst.URL, "http://") + "/mirror"
				dstRepo := dstPrefix + "/postgresql"
				fresh, old := aliasIndex(t, "new"), aliasIndex(t, "old")
				seedAliasImage(t, srcRepo, fresh, append(imageAliases, aliasRevision, "18.6.0-debian-12-r12")...)
				seedAliasImage(t, srcRepo, old, "17", "17.0.0-debian-12-r1")
				// Sharing platform manifests is insufficient: aliases must match
				// the complete source index, including its media type.
				seedAliasImage(t, srcRepo, mutate.IndexMediaType(fresh, types.DockerManifestList), "18-other-index")
				seedAliasImage(t, dstRepo, old, append(imageAliases, "18.6.0-debian-12-r12")...)
				oldDigest, err := old.Digest()
				require.NoError(t, err)
				sourceDigest, err := fresh.Digest()
				require.NoError(t, err)
				// Include image metadata and its signature to prove artifact
				// publication can resolve digests before the revision tag exists.
				metadata, err := mutate.ConfigFile(empty.Image, &v1.ConfigFile{OS: "linux", Architecture: "amd64", Config: v1.Config{Labels: map[string]string{"kind": "metadata"}}})
				require.NoError(t, err)
				metadataDigest, err := metadata.Digest()
				require.NoError(t, err)
				require.NoError(t, crane.Push(empty.Image, srcRepo+":sha256-"+sourceDigest.Hex+".sig"))
				require.NoError(t, crane.Push(metadata, srcRepo+":sha256-"+sourceDigest.Hex+".metadata"))
				require.NoError(t, crane.Push(empty.Image, srcRepo+":sha256-"+metadataDigest.Hex+".sig"))

				bundle := filepath.Join(t.TempDir(), "image.wrap.tgz")
				opts := []wrap.Option{wrap.WithFetchArtifacts(true), wrap.WithSyncAliasTags(true), wrap.WithOutputFile(bundle),
					wrap.WithTempDirectory(t.TempDir()), wrap.WithLogger(silentLogger()), wrap.WithPreserveDigest(mode == "preserved")}
				if mode == "filtered" {
					opts = append(opts, wrap.WithPlatforms([]string{"linux/amd64"}))
				}
				if chart {
					chartDir := t.TempDir()
					chartYAML := fmt.Sprintf("apiVersion: v2\nname: example\nversion: 1.0.0\nannotations:\n  images: |\n    - name: postgresql\n      image: %s:%s\n", srcRepo, aliasRevision)
					require.NoError(t, os.WriteFile(filepath.Join(chartDir, "Chart.yaml"), []byte(chartYAML), 0600))
					values := fmt.Sprintf("image:\n  registry: %s\n  repository: postgresql\n  tag: %s\n", strings.TrimPrefix(src.URL, "http://"), aliasRevision)
					require.NoError(t, os.WriteFile(filepath.Join(chartDir, "values.yaml"), []byte(values), 0600))
					// Exercise existing locks, including refreshing stale alias metadata.
					if mode != "filtered" {
						lock, lockErr := imagelock.GenerateFromChart(chartDir)
						require.NoError(t, lockErr)
						lock.Images[0].Aliases = []string{"stale"}
						f, lockErr := os.Create(filepath.Join(chartDir, "Images.lock"))
						require.NoError(t, lockErr)
						require.NoError(t, lock.ToYAML(f))
						require.NoError(t, f.Close())
					}
					tgz := filepath.Join(t.TempDir(), "example.tgz")
					require.NoError(t, utils.Tar(chartDir, tgz, utils.TarConfig{Prefix: "example"}))
					_, err = wrap.Chart(tgz, opts...)
					require.NoError(t, err)
				} else {
					_, err = wrap.Container(srcRepo+":"+aliasRevision, opts...)
					require.NoError(t, err)
				}
				extracted := t.TempDir()
				require.NoError(t, utils.Untar(bundle, extracted, utils.TarConfig{StripComponents: 1}))
				lockPath := filepath.Join(extracted, "Images.lock")
				if chart {
					lockPath = filepath.Join(extracted, "chart", "Images.lock")
				}
				lock, err := imagelock.FromYAMLFile(lockPath)
				require.NoError(t, err)
				require.Len(t, lock.Images, 1)
				require.Equal(t, imageAliases, lock.Images[0].Aliases)
				// The import must never need access to the source registry.
				src.Close()
				unwrapOpts := []unwrap.Option{unwrap.WithSyncAliasTags(true), unwrap.WithSayYes(true),
					unwrap.WithUsePlainHTTP(true), unwrap.WithTempDirectory(t.TempDir()), unwrap.WithLogger(silentLogger()),
					unwrap.WithPreserveDigest(mode == "preserved")}
				if chart {
					_, err = unwrap.Chart(bundle, dstPrefix, dstPrefix+"/charts", unwrapOpts...)
				} else {
					_, err = unwrap.Container(bundle, dstPrefix, unwrapOpts...)
				}
				require.NoError(t, err)
				published := aliasDigest(t, dstRepo+":"+aliasRevision)
				publishedHex := strings.TrimPrefix(published, "sha256:")
				sigDigest, err := empty.Image.Digest()
				require.NoError(t, err)
				require.Equal(t, sigDigest.String(), aliasDigest(t, dstRepo+":sha256-"+publishedHex+".sig"))
				require.Equal(t, metadataDigest.String(), aliasDigest(t, dstRepo+":sha256-"+publishedHex+".metadata"))
				require.Equal(t, metadataDigest.String(), aliasDigest(t, dstRepo+":"+aliasRevision+"-metadata"))
				require.Equal(t, sigDigest.String(), aliasDigest(t, dstRepo+":sha256-"+metadataDigest.Hex+".sig"))

				for _, alias := range imageAliases {
					require.Equal(t, published, aliasDigest(t, dstRepo+":"+alias))
				}
				require.Equal(t, oldDigest.String(), aliasDigest(t, dstRepo+":18.6.0-debian-12-r12"))
				_, err = crane.Digest(dstRepo + ":17")
				require.Error(t, err)
				_, err = crane.Digest(dstRepo + ":18-other-index")
				require.Error(t, err)
				if mode == "preserved" {
					require.Equal(t, sourceDigest.String(), published)
				} else {
					require.NotEqual(t, sourceDigest.String(), published)
				}
			})
		}
	}
}

func TestLatestAliasDoesNotFollowUnselectedImage(t *testing.T) {
	src, dst := aliasRegistry(t, nil), aliasRegistry(t, nil)
	srcRepo := strings.TrimPrefix(src.URL, "http://") + "/postgresql"
	dstPrefix := strings.TrimPrefix(dst.URL, "http://") + "/mirror"
	dstRepo := dstPrefix + "/postgresql"
	selected, other := aliasIndex(t, "selected"), aliasIndex(t, "other")
	seedAliasImage(t, srcRepo, selected, aliasRevision, "18")
	seedAliasImage(t, srcRepo, other, "latest")
	seedAliasImage(t, dstRepo, other, "latest")

	bundle := filepath.Join(t.TempDir(), "image.wrap.tgz")
	_, err := wrap.Container(srcRepo+":"+aliasRevision, wrap.WithSyncAliasTags(true), wrap.WithOutputFile(bundle),
		wrap.WithTempDirectory(t.TempDir()), wrap.WithLogger(silentLogger()))
	require.NoError(t, err)
	extracted := t.TempDir()
	require.NoError(t, utils.Untar(bundle, extracted, utils.TarConfig{StripComponents: 1}))
	lock, err := imagelock.FromYAMLFile(filepath.Join(extracted, "Images.lock"))
	require.NoError(t, err)
	require.Len(t, lock.Images, 1)
	require.Equal(t, []string{"18"}, lock.Images[0].Aliases)
	src.Close()
	_, err = unwrap.Container(bundle, dstPrefix, unwrap.WithSyncAliasTags(true), unwrap.WithSayYes(true),
		unwrap.WithTempDirectory(t.TempDir()), unwrap.WithLogger(silentLogger()))
	require.NoError(t, err)
	otherDigest, err := other.Digest()
	require.NoError(t, err)
	require.Equal(t, otherDigest.String(), aliasDigest(t, dstRepo+":latest"))
	require.Equal(t, aliasDigest(t, dstRepo+":"+aliasRevision), aliasDigest(t, dstRepo+":18"))
}

func TestLatestPrimaryCanBeUpdated(t *testing.T) {
	src, dst := aliasRegistry(t, nil), aliasRegistry(t, nil)
	srcRepo := strings.TrimPrefix(src.URL, "http://") + "/postgresql"
	dstPrefix := strings.TrimPrefix(dst.URL, "http://") + "/mirror"
	dstRepo := dstPrefix + "/postgresql"
	fresh, old := aliasIndex(t, "new"), aliasIndex(t, "old")
	seedAliasImage(t, srcRepo, fresh, "latest", "18")
	seedAliasImage(t, dstRepo, old, "latest")

	bundle := filepath.Join(t.TempDir(), "image.wrap.tgz")
	_, err := wrap.Container(srcRepo+":latest", wrap.WithSyncAliasTags(true), wrap.WithPreserveDigest(true), wrap.WithOutputFile(bundle),
		wrap.WithTempDirectory(t.TempDir()), wrap.WithLogger(silentLogger()))
	require.NoError(t, err)
	src.Close()
	_, err = unwrap.Container(bundle, dstPrefix, unwrap.WithSyncAliasTags(true), unwrap.WithPreserveDigest(true), unwrap.WithSayYes(true),
		unwrap.WithTempDirectory(t.TempDir()), unwrap.WithLogger(silentLogger()))
	require.NoError(t, err)
	want, err := fresh.Digest()
	require.NoError(t, err)
	require.Equal(t, want.String(), aliasDigest(t, dstRepo+":latest"))
	require.Equal(t, want.String(), aliasDigest(t, dstRepo+":18"))
}

func TestAliasDiscoverySkipsArtifactRequests(t *testing.T) {
	var recording atomic.Bool
	var mu sync.Mutex
	heads := make(map[string]int)
	src := aliasRegistry(t, func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			if recording.Load() && r.Method == http.MethodHead {
				if tag, ok := strings.CutPrefix(r.URL.Path, "/v2/postgresql/manifests/"); ok {
					mu.Lock()
					heads[tag]++
					mu.Unlock()
				}
			}
			next.ServeHTTP(w, r)
		})
	})
	repo := strings.TrimPrefix(src.URL, "http://") + "/postgresql"
	fresh := aliasIndex(t, "new")
	digest, err := fresh.Digest()
	require.NoError(t, err)
	aliases := []string{"18", "18.6.0", "sha256-custom", "sha256-" + digest.Hex,
		"sha256-short.sig", "sha256-" + digest.Hex + ".custom", "release-metadata-preview"}
	seedAliasImage(t, repo, fresh, append(aliases, aliasRevision)...)
	seedAliasImage(t, repo, aliasIndex(t, "old"), "17")

	metadataDigest, err := empty.Image.Digest()
	require.NoError(t, err)
	artifactTags := []string{
		"sha256-" + digest.Hex + ".sig",
		"sha256-" + digest.Hex + ".metadata",
		"sha256-" + metadataDigest.Hex + ".sig",
		aliasRevision + "-metadata",
		"18-metadata",
	}
	manifest, err := fresh.IndexManifest()
	require.NoError(t, err)
	for _, platform := range manifest.Manifests {
		artifactTags = append(artifactTags, "sha256-"+platform.Digest.Hex+".sig")
	}
	for _, tag := range artifactTags {
		require.NoError(t, crane.Push(empty.Image, repo+":"+tag))
	}

	bundle := filepath.Join(t.TempDir(), "image.wrap.tgz")
	// Isolate discovery from the legitimate requests made when fetching artifacts.
	recording.Store(true)
	_, err = wrap.Container(repo+":"+aliasRevision, wrap.WithSyncAliasTags(true), wrap.WithFetchArtifacts(false),
		wrap.WithOutputFile(bundle), wrap.WithTempDirectory(t.TempDir()), wrap.WithLogger(silentLogger()))
	recording.Store(false)
	require.NoError(t, err)
	extracted := t.TempDir()
	require.NoError(t, utils.Untar(bundle, extracted, utils.TarConfig{StripComponents: 1}))
	lock, err := imagelock.FromYAMLFile(filepath.Join(extracted, "Images.lock"))
	require.NoError(t, err)
	require.Len(t, lock.Images, 1)
	require.ElementsMatch(t, aliases, lock.Images[0].Aliases)

	mu.Lock()
	defer mu.Unlock()
	for _, tag := range artifactTags {
		require.Zero(t, heads[tag], "artifact tag %q must not be resolved during alias discovery", tag)
	}
	for _, tag := range aliases {
		require.Equal(t, 1, heads[tag], "image alias %q must still be resolved", tag)
	}
	require.Equal(t, 1, heads["17"], "other image tags still need digest comparison")
}

func TestAliasDiscoveryIsOptInAndReportsErrors(t *testing.T) {
	src := aliasRegistry(t, func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			if strings.HasSuffix(r.URL.Path, "/tags/list") {
				http.Error(w, "tag listing denied", http.StatusForbidden)
				return
			}
			next.ServeHTTP(w, r)
		})
	})
	repo := strings.TrimPrefix(src.URL, "http://") + "/postgresql"
	seedAliasImage(t, repo, aliasIndex(t, "new"), aliasRevision)
	for _, enabled := range []bool{false, true} {
		_, err := wrap.Container(repo+":"+aliasRevision, wrap.WithSyncAliasTags(enabled),
			wrap.WithOutputFile(filepath.Join(t.TempDir(), "image.wrap.tgz")),
			wrap.WithTempDirectory(t.TempDir()), wrap.WithLogger(silentLogger()))
		if enabled {
			require.ErrorContains(t, err, "listing aliases")
		} else {
			require.NoError(t, err)
		}
	}
}

func TestAliasImportOptInAndProtectedTags(t *testing.T) {
	src, dst := aliasRegistry(t, nil), aliasRegistry(t, nil)
	srcRepo := strings.TrimPrefix(src.URL, "http://") + "/postgresql"
	dstHost := strings.TrimPrefix(dst.URL, "http://")
	fresh, old := aliasIndex(t, "new"), aliasIndex(t, "old")
	seedAliasImage(t, srcRepo, fresh, aliasRevision, "18", "latest")
	bundle := filepath.Join(t.TempDir(), "image.wrap.tgz")
	_, err := wrap.Container(srcRepo+":"+aliasRevision, wrap.WithSyncAliasTags(true), wrap.WithOutputFile(bundle),
		wrap.WithTempDirectory(t.TempDir()), wrap.WithLogger(silentLogger()))
	require.NoError(t, err)

	// Import remains opt-in even if a bundle contains aliases.
	_, err = unwrap.Container(bundle, dstHost+"/disabled", unwrap.WithSayYes(true),
		unwrap.WithTempDirectory(t.TempDir()), unwrap.WithLogger(silentLogger()))
	require.NoError(t, err)
	for _, alias := range []string{"18", "latest"} {
		_, err = crane.Digest(dstHost + "/disabled/postgresql:" + alias)
		require.Error(t, err)
	}

	// Even if supplied in an external bundle, revision and artifact
	// tags must never be treated as aliases.
	oldDigest, err := old.Digest()
	require.NoError(t, err)
	protectedTags := []string{"18.6.0-debian-12-r12",
		"sha256-" + oldDigest.Hex + ".sig", "sha256-" + oldDigest.Hex + ".metadata", aliasRevision + "-metadata"}
	extracted := t.TempDir()
	require.NoError(t, utils.Untar(bundle, extracted, utils.TarConfig{StripComponents: 1}))
	lockPath := filepath.Join(extracted, "Images.lock")
	lock, err := imagelock.FromYAMLFile(lockPath)
	require.NoError(t, err)
	lock.Images[0].Aliases = append([]string{"18", "latest", aliasRevision}, protectedTags...)
	f, err := os.Create(lockPath)
	require.NoError(t, err)
	require.NoError(t, lock.ToYAML(f))
	require.NoError(t, f.Close())
	modified := filepath.Join(t.TempDir(), "image.wrap.tgz")
	require.NoError(t, utils.Tar(extracted, modified, utils.TarConfig{Prefix: "postgresql"}))
	protectedRepo := dstHost + "/protected/postgresql"
	seedAliasImage(t, protectedRepo, old, protectedTags...)
	_, err = unwrap.Container(modified, dstHost+"/protected", unwrap.WithSyncAliasTags(true), unwrap.WithSayYes(true),
		unwrap.WithTempDirectory(t.TempDir()), unwrap.WithLogger(silentLogger()))
	require.NoError(t, err)
	for _, tag := range protectedTags {
		require.Equal(t, oldDigest.String(), aliasDigest(t, protectedRepo+":"+tag))
	}
	for _, alias := range []string{"18", "latest"} {
		require.Equal(t, aliasDigest(t, protectedRepo+":"+aliasRevision), aliasDigest(t, protectedRepo+":"+alias))
	}

	// An existing primary revision must also be protected, including in the
	// chart image push path which normally permits replacing primary tags.
	conflictRepo := dstHost + "/conflict/postgresql"
	seedAliasImage(t, conflictRepo, old, aliasRevision, "18")
	_, err = unwrap.Container(bundle, dstHost+"/conflict", unwrap.WithSyncAliasTags(true), unwrap.WithSayYes(true),
		unwrap.WithTempDirectory(t.TempDir()), unwrap.WithLogger(silentLogger()))
	require.ErrorContains(t, err, "refusing to overwrite protected destination tag")
	for _, tag := range []string{aliasRevision, "18"} {
		require.Equal(t, oldDigest.String(), aliasDigest(t, conflictRepo+":"+tag))
	}
}

func TestAliasFailureLeavesRevisionPending(t *testing.T) {
	src := aliasRegistry(t, nil)
	var reject atomic.Bool
	reject.Store(true)
	dst := aliasRegistry(t, func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			if reject.Load() && r.Method == http.MethodPut && strings.HasSuffix(r.URL.Path, "/manifests/18.6.0") {
				http.Error(w, "alias update denied", http.StatusForbidden)
				return
			}
			next.ServeHTTP(w, r)
		})
	})
	srcRepo := strings.TrimPrefix(src.URL, "http://") + "/postgresql"
	dstPrefix := strings.TrimPrefix(dst.URL, "http://") + "/mirror"
	dstRepo := dstPrefix + "/postgresql"
	seedAliasImage(t, srcRepo, aliasIndex(t, "new"), append(imageAliases, aliasRevision)...)
	bundle := filepath.Join(t.TempDir(), "image.wrap.tgz")
	_, err := wrap.Container(srcRepo+":"+aliasRevision, wrap.WithSyncAliasTags(true), wrap.WithOutputFile(bundle),
		wrap.WithTempDirectory(t.TempDir()), wrap.WithLogger(silentLogger()))
	require.NoError(t, err)
	opts := []unwrap.Option{unwrap.WithSyncAliasTags(true), unwrap.WithSayYes(true), unwrap.WithLogger(silentLogger())}
	_, err = unwrap.Container(bundle, dstPrefix, append(opts, unwrap.WithTempDirectory(t.TempDir()))...)
	require.ErrorContains(t, err, "publishing alias")
	_, err = crane.Digest(dstRepo + ":" + aliasRevision)
	require.Error(t, err, "a failed alias update must not publish the revision tag")
	// One alias was already published; repeating the operation must complete it.
	aliasDigest(t, dstRepo+":18")
	reject.Store(false)
	_, err = unwrap.Container(bundle, dstPrefix, append(opts, unwrap.WithTempDirectory(t.TempDir()))...)
	require.NoError(t, err)
	published := aliasDigest(t, dstRepo+":"+aliasRevision)
	for _, alias := range imageAliases {
		require.Equal(t, published, aliasDigest(t, dstRepo+":"+alias))
	}
}
