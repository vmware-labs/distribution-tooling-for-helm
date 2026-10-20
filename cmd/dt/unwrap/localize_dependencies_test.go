package unwrap_test

import (
	"bytes"
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
	"helm.sh/helm/v3/pkg/chart"
	"helm.sh/helm/v3/pkg/chartutil"

	"github.com/vmware-labs/distribution-tooling-for-helm/cmd/dt/unwrap"
	"github.com/vmware-labs/distribution-tooling-for-helm/cmd/dt/wrap"
	tu "github.com/vmware-labs/distribution-tooling-for-helm/internal/testutil"
	"github.com/vmware-labs/distribution-tooling-for-helm/pkg/artifacts"
	"github.com/vmware-labs/distribution-tooling-for-helm/pkg/dtlog"
	"github.com/vmware-labs/distribution-tooling-for-helm/pkg/dtlog/logrus"
	"github.com/vmware-labs/distribution-tooling-for-helm/pkg/utils"
)

// TestPreserveDigestIgnoresLocalizeDependencies proves that asking to localize the dependencies
// does not break --preserve-digest: the chart is pushed byte-for-byte, so its digest must remain
// unchanged and the unwrap must not fail because of the localization.
func TestPreserveDigestIgnoresLocalizeDependencies(t *testing.T) {
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

	chartDir := sb.TempFile()
	require.NoError(t, tu.RenderScenario("../../../testdata/scenarios/no-images-chart", chartDir, map[string]any{
		"Name":    chartName,
		"Version": version,
		"Dependencies": []map[string]string{
			{"Name": "child", "Repository": "oci://source.example.com/charts", "Version": "1.x.x"},
		},
	}))
	_, err = chartutil.Save(
		&chart.Chart{Metadata: &chart.Metadata{APIVersion: "v2", Name: "child", Version: "1.2.3"}},
		filepath.Join(chartDir, "charts"),
	)
	require.NoError(t, err)

	chartTgz := sb.TempFile() + ".tgz"
	require.NoError(t, utils.Tar(chartDir, chartTgz, utils.TarConfig{Prefix: chartName}))

	srcRepo := "oci://" + serverURL + "/src"
	require.NoError(t, artifacts.PushChart(chartTgz, srcRepo,
		artifacts.WithPlainHTTP(true),
		artifacts.WithTempDir(sb.TempFile()),
	))
	srcDigest, err := crane.Digest(strings.TrimPrefix(srcRepo, "oci://") + "/" + chartName + ":" + version)
	require.NoError(t, err)

	bundleFile := sb.TempFile() + ".wrap.tgz"
	_, err = wrap.Chart(srcRepo+"/"+chartName,
		wrap.WithLogger(silentLogger()),
		wrap.WithUsePlainHTTP(true),
		wrap.WithVersion(version),
		wrap.WithPreserveDigest(true),
		wrap.WithOutputFile(bundleFile),
	)
	require.NoError(t, err)

	var logs bytes.Buffer
	l := logrus.NewSectionLogger()
	l.SetWriter(&logs)
	l.SetLevel(dtlog.DebugLevel)

	dstRepo := "oci://" + serverURL + "/dst"
	_, err = unwrap.Chart(bundleFile, dstRepo, "",
		unwrap.WithLogger(l),
		unwrap.WithUsePlainHTTP(true),
		unwrap.WithSayYes(true),
		unwrap.WithPreserveDigest(true),
		unwrap.WithLocalizeDependencies(true),
	)
	require.NoError(t, err)

	dstDigest, err := crane.Digest(strings.TrimPrefix(dstRepo, "oci://") + "/" + chartName + ":" + version)
	require.NoError(t, err)
	require.Equal(t, srcDigest, dstDigest, "chart manifest digest must be preserved")
	// No chart content is rewritten when preserving the digest, so the localization is not attempted
	require.NotContains(t, logs.String(), "Localizing the dependencies")
}
