package unwrap_test

import (
	"fmt"
	"io"
	"log"
	"net/http/httptest"
	"net/url"
	"path/filepath"
	"strings"
	"testing"

	"github.com/google/go-containerregistry/pkg/crane"
	"github.com/google/go-containerregistry/pkg/registry"
	"github.com/stretchr/testify/require"

	"github.com/vmware-labs/distribution-tooling-for-helm/cmd/dt/unwrap"
	"github.com/vmware-labs/distribution-tooling-for-helm/cmd/dt/wrap"
	tu "github.com/vmware-labs/distribution-tooling-for-helm/internal/testutil"
	"github.com/vmware-labs/distribution-tooling-for-helm/pkg/artifacts"
	"github.com/vmware-labs/distribution-tooling-for-helm/pkg/dtlog/logrus"
	"github.com/vmware-labs/distribution-tooling-for-helm/pkg/utils"
)

func silentLogger() *logrus.SectionLogger {
	l := logrus.NewSectionLogger()
	l.SetWriter(io.Discard)
	return l
}

// TestPreserveDigestChartManifest proves the core promise of --preserve-digest:
// a chart originally published to an OCI registry keeps the exact same
// manifest digest after being wrapped and unwrapped into a different
// registry, even though today's default wrap/unwrap round trip (untar, copy,
// re-tar, re-push through Helm's registry client) would change it.
func TestPreserveDigestChartManifest(t *testing.T) {
	silentStdLog := log.New(io.Discard, "", 0)
	s := httptest.NewServer(registry.New(registry.Logger(silentStdLog)))
	defer s.Close()
	u, err := url.Parse(s.URL)
	require.NoError(t, err)
	serverURL := u.Host

	sb := tu.NewSandbox()
	defer sb.Cleanup()

	chartName := "test"
	version := "1.0.0"

	// Set up a chart with no images so this test stays focused purely on the
	// chart artifact's own digest, and publish it to the source registry via
	// Helm's OWN push action - exactly as if some third party had already
	// published and (conceptually) signed it.
	chartDir := sb.TempFile()
	require.NoError(t, tu.RenderScenario("../../../testdata/scenarios/no-images-chart", chartDir,
		map[string]any{"Name": chartName, "Version": version},
	))

	chartTgz := sb.TempFile() + ".tgz"
	require.NoError(t, utils.Tar(chartDir, chartTgz, utils.TarConfig{Prefix: chartName}))

	srcRepo := "oci://" + serverURL + "/src"
	require.NoError(t, artifacts.PushChart(chartTgz, srcRepo,
		artifacts.WithPlainHTTP(true),
		artifacts.WithTempDir(sb.TempFile()),
	))

	srcRef := strings.TrimPrefix(srcRepo, "oci://") + "/" + chartName + ":" + version
	srcDigest, err := crane.Digest(srcRef)
	require.NoError(t, err)

	// Wrap directly from the OCI source with --preserve-digest.
	bundleFile := sb.TempFile() + ".wrap.tgz"
	_, err = wrap.Chart(srcRepo+"/"+chartName,
		wrap.WithLogger(silentLogger()),
		wrap.WithUsePlainHTTP(true),
		wrap.WithVersion(version),
		wrap.WithPreserveDigest(true),
		wrap.WithOutputFile(bundleFile),
	)
	require.NoError(t, err)
	require.FileExists(t, bundleFile)

	// The bundle must carry the raw captured OCI artifact, not just the
	// exploded chart tree.
	bundleExtractDir := sb.TempFile()
	require.NoError(t, utils.Untar(bundleFile, bundleExtractDir, utils.TarConfig{StripComponents: 1}))
	require.FileExists(t, filepath.Join(bundleExtractDir, "chart.oci", "index.json"))

	// Unwrap into a different registry namespace with --preserve-digest.
	dstRepo := "oci://" + serverURL + "/dst"
	_, err = unwrap.Chart(bundleFile, dstRepo, "",
		unwrap.WithLogger(silentLogger()),
		unwrap.WithUsePlainHTTP(true),
		unwrap.WithSayYes(true),
		unwrap.WithPreserveDigest(true),
	)
	require.NoError(t, err)

	dstRef := strings.TrimPrefix(dstRepo, "oci://") + "/" + chartName + ":" + version
	dstDigest, err := crane.Digest(dstRef)
	require.NoError(t, err)

	require.Equal(t, srcDigest, dstDigest, "chart manifest digest must be preserved end to end")
}

// TestPreserveDigestChartImagesRelocated proves that --preserve-digest still
// routes the chart's images to the destination registry passed to
// `dt unwrap`, instead of silently re-pushing them back to their original
// source location. This regresses if Images.lock relocation is skipped
// along with chart content relocation, since both used to live behind the
// same SkipImageRelocation gate.
func TestPreserveDigestChartImagesRelocated(t *testing.T) {
	silentStdLog := log.New(io.Discard, "", 0)

	srcSrv := httptest.NewServer(registry.New(registry.Logger(silentStdLog)))
	defer srcSrv.Close()
	srcURL, err := url.Parse(srcSrv.URL)
	require.NoError(t, err)
	srcHost := srcURL.Host

	dstSrv := httptest.NewServer(registry.New(registry.Logger(silentStdLog)))
	defer dstSrv.Close()
	dstURL, err := url.Parse(dstSrv.URL)
	require.NoError(t, err)
	dstHost := dstURL.Host

	sb := tu.NewSandbox()
	defer sb.Cleanup()

	chartName := "test"
	version := "1.0.0"
	imageName := "sample-image"

	images, err := tu.AddSampleImagesToRegistry(imageName, srcHost)
	require.NoError(t, err)
	require.Len(t, images, 1)

	chartDir := sb.TempFile()
	require.NoError(t, tu.RenderScenario("../../../testdata/scenarios/complete-chart", chartDir,
		map[string]any{"Name": chartName, "Version": version, "ServerURL": srcHost, "RepositoryURL": srcHost, "Images": images},
	))

	chartTgz := sb.TempFile() + ".tgz"
	require.NoError(t, utils.Tar(chartDir, chartTgz, utils.TarConfig{Prefix: chartName}))

	srcRepo := "oci://" + srcHost
	require.NoError(t, artifacts.PushChart(chartTgz, srcRepo,
		artifacts.WithPlainHTTP(true),
		artifacts.WithTempDir(sb.TempFile()),
	))

	srcImageRef := fmt.Sprintf("%s/%s", srcHost, images[0].Image)
	srcImageDigest, err := crane.Digest(srcImageRef)
	require.NoError(t, err)

	// Wrap directly from the OCI source with --preserve-digest.
	bundleFile := sb.TempFile() + ".wrap.tgz"
	_, err = wrap.Chart(srcRepo+"/"+chartName,
		wrap.WithLogger(silentLogger()),
		wrap.WithUsePlainHTTP(true),
		wrap.WithVersion(version),
		wrap.WithPreserveDigest(true),
		wrap.WithOutputFile(bundleFile),
	)
	require.NoError(t, err)

	// Unwrap into a completely different registry with --preserve-digest.
	dstRepo := "oci://" + dstHost
	_, err = unwrap.Chart(bundleFile, dstRepo, "",
		unwrap.WithLogger(silentLogger()),
		unwrap.WithUsePlainHTTP(true),
		unwrap.WithSayYes(true),
		unwrap.WithPreserveDigest(true),
	)
	require.NoError(t, err)

	dstImageRef := fmt.Sprintf("%s/%s", dstHost, images[0].Image)
	dstImageDigest, err := crane.Digest(dstImageRef)
	require.NoError(t, err, "image should have been relocated and pushed to the destination registry")
	require.Equal(t, srcImageDigest, dstImageDigest, "image manifest digest must be preserved end to end")
}
