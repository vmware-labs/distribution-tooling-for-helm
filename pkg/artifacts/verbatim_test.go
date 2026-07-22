package artifacts

import (
	"context"
	"fmt"
	"io"
	"log"
	"net/http/httptest"
	"net/url"
	"path/filepath"
	"testing"

	"github.com/google/go-containerregistry/pkg/crane"
	"github.com/google/go-containerregistry/pkg/registry"
	"github.com/opencontainers/go-digest"
	"github.com/stretchr/testify/require"

	"github.com/vmware-labs/distribution-tooling-for-helm/pkg/imagelock"
)

func newTestRegistry(t *testing.T) string {
	t.Helper()
	silentLog := log.New(io.Discard, "", 0)
	s := httptest.NewServer(registry.New(registry.Logger(silentLog)))
	t.Cleanup(s.Close)
	u, err := url.Parse(s.URL)
	require.NoError(t, err)
	return u.Host
}

func TestPullPushVerbatimSingleImage(t *testing.T) {
	serverURL := newTestRegistry(t)
	ctx := context.Background()

	img, err := crane.Image(map[string][]byte{"file.txt": []byte("hello")})
	require.NoError(t, err)

	srcRef := fmt.Sprintf("%s/single:latest", serverURL)
	require.NoError(t, crane.Push(img, srcRef))

	srcDigest, err := crane.Digest(srcRef)
	require.NoError(t, err)

	dir := t.TempDir()
	destDir := filepath.Join(dir, "layout")

	desc, err := PullVerbatim(ctx, srcRef, destDir)
	require.NoError(t, err)
	require.True(t, desc.MediaType.IsImage())
	require.Equal(t, srcDigest, desc.Digest.String())

	dstRef := fmt.Sprintf("%s/single-copy:latest", serverURL)
	require.NoError(t, PushVerbatim(ctx, destDir, dstRef))

	dstDigest, err := crane.Digest(dstRef)
	require.NoError(t, err)
	require.Equal(t, srcDigest, dstDigest, "digest must be preserved across PullVerbatim+PushVerbatim")
}

func TestVerifyLockedDigestsSingleManifest(t *testing.T) {
	serverURL := newTestRegistry(t)

	img, err := crane.Image(map[string][]byte{"file.txt": []byte("hello")})
	require.NoError(t, err)

	ref := fmt.Sprintf("%s/single:latest", serverURL)
	require.NoError(t, crane.Push(img, ref))

	desc, err := imagelock.GetImageRemoteDescriptor(ref)
	require.NoError(t, err)

	image := &imagelock.ChartImage{
		Image: ref,
		Digests: []imagelock.DigestInfo{
			{Arch: "linux/amd64", Digest: digest.Digest(desc.Digest.String())},
		},
	}
	require.NoError(t, VerifyLockedDigests(image, desc))

	tampered := &imagelock.ChartImage{
		Image: ref,
		Digests: []imagelock.DigestInfo{
			{Arch: "linux/amd64", Digest: digest.Digest("sha256:0000000000000000000000000000000000000000000000000000000000000000")},
		},
	}
	require.Error(t, VerifyLockedDigests(tampered, desc), "a locked digest that does not match the remote content must be rejected")
}
