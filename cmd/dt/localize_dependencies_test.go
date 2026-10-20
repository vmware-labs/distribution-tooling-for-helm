package main

import (
	"fmt"
	"io"
	"log"
	"net/http/httptest"
	"net/url"
	"os"
	"path/filepath"
	"testing"

	"github.com/google/go-containerregistry/pkg/registry"
	"github.com/stretchr/testify/require"
	"github.com/vmware-labs/distribution-tooling-for-helm/cmd/dt/wrap"
	tu "github.com/vmware-labs/distribution-tooling-for-helm/internal/testutil"
	"github.com/vmware-labs/distribution-tooling-for-helm/pkg/artifacts"
	"github.com/vmware-labs/distribution-tooling-for-helm/pkg/dtlog/logrus"
	"gopkg.in/yaml.v3"
	"helm.sh/helm/v3/pkg/chart"
	"helm.sh/helm/v3/pkg/chartutil"
	"helm.sh/helm/v3/pkg/cli"
	"helm.sh/helm/v3/pkg/downloader"
	"helm.sh/helm/v3/pkg/getter"
)

const localizeSourceRepo = "oci://source.example.com/charts"

// renderChartWithVendoredDependency renders a chart with a dependency pointing to a remote
// repository, which is bundled in charts/ as a .tgz archive (as published charts are)
func renderChartWithVendoredDependency(t *testing.T, sb *tu.Sandbox) string {
	t.Helper()
	chartDir := sb.TempFile()
	require.NoError(t, tu.RenderScenario("../../testdata/scenarios/no-images-chart", chartDir, map[string]interface{}{
		"Name":    "parent",
		"Version": "1.0.0",
		"Dependencies": []map[string]string{
			{"Name": "child", "Repository": localizeSourceRepo, "Version": "1.x.x"},
		},
	}))
	_, err := chartutil.Save(
		&chart.Chart{Metadata: &chart.Metadata{APIVersion: "v2", Name: "child", Version: "1.2.3"}},
		filepath.Join(chartDir, "charts"),
	)
	require.NoError(t, err)
	return chartDir
}

func dependencyRepository(t *testing.T, chartDir, name string) string {
	t.Helper()
	data, err := os.ReadFile(filepath.Join(chartDir, "Chart.yaml"))
	require.NoError(t, err)
	var meta struct {
		Dependencies []struct {
			Name       string `yaml:"name"`
			Repository string `yaml:"repository"`
		} `yaml:"dependencies"`
	}
	require.NoError(t, yaml.Unmarshal(data, &meta))
	for _, d := range meta.Dependencies {
		if d.Name == name {
			return d.Repository
		}
	}
	require.Failf(t, "dependency not found", "%q not found in %s", name, chartDir)
	return ""
}

// assertHelmBuildsOffline checks Helm is able to build the dependencies of the chart without
// reaching any repository
func assertHelmBuildsOffline(t *testing.T, sb *tu.Sandbox, chartDir string) {
	t.Helper()
	m := &downloader.Manager{
		Out:              io.Discard,
		ChartPath:        chartDir,
		Getters:          getter.All(cli.New()),
		RepositoryConfig: filepath.Join(sb.TempFile(), "repositories.yaml"),
		RepositoryCache:  sb.TempFile(),
		SkipUpdate:       true,
	}
	require.NoError(t, m.Build())
}

func (suite *CmdSuite) TestRelocateLocalizeDependencies() {
	t := suite.T()
	sb := suite.sb

	t.Run("flag clears the repository of vendored dependencies", func(t *testing.T) {
		chartDir := renderChartWithVendoredDependency(t, sb)

		dt("charts", "relocate", "--skip-relocation", "--localize-dependencies", chartDir, "custom.repo.example.com").AssertSuccess(t)

		require.Equal(t, "", dependencyRepository(t, chartDir, "child"))
		assertHelmBuildsOffline(t, sb, chartDir)
	})

	t.Run("dependencies are left untouched without the flag", func(t *testing.T) {
		chartDir := renderChartWithVendoredDependency(t, sb)

		dt("charts", "relocate", "--skip-relocation", chartDir, "custom.repo.example.com").AssertSuccess(t)

		require.Equal(t, localizeSourceRepo, dependencyRepository(t, chartDir, "child"))
	})
}

func (suite *CmdSuite) TestUnwrapLocalizeDependencies() {
	t := suite.T()
	sb := suite.sb

	silentStdLog := log.New(io.Discard, "", 0)
	s := httptest.NewServer(registry.New(registry.Logger(silentStdLog)))
	defer s.Close()
	u, err := url.Parse(s.URL)
	suite.Require().NoError(err)
	serverURL := u.Host

	l := logrus.NewSectionLogger()
	l.SetWriter(io.Discard)

	wrapChart := func(t *testing.T) string {
		bundle := sb.TempFile() + ".wrap.tgz"
		_, wErr := wrap.Chart(renderChartWithVendoredDependency(t, sb),
			wrap.WithLogger(l),
			wrap.WithOutputFile(bundle),
		)
		require.NoError(t, wErr)
		return bundle
	}
	pullChart := func(t *testing.T, repo string) string {
		destDir, mErr := sb.Mkdir(sb.TempFile(), 0755)
		require.NoError(t, mErr)
		dir, pErr := artifacts.PullChart(fmt.Sprintf("oci://%s/parent", repo), "1.0.0", destDir, artifacts.WithPlainHTTP(true))
		require.NoError(t, pErr)
		return dir
	}

	t.Run("flag clears the repository of vendored dependencies", func(t *testing.T) {
		repo := serverURL + "/with-flag"
		dt("unwrap", "--plain", "--yes", "--use-plain-http", "--localize-dependencies", wrapChart(t), "oci://"+repo).AssertSuccess(t)

		chartDir := pullChart(t, repo)
		require.Equal(t, "", dependencyRepository(t, chartDir, "child"))
		assertHelmBuildsOffline(t, sb, chartDir)
	})

	t.Run("dependencies are left untouched without the flag", func(t *testing.T) {
		repo := serverURL + "/without-flag"
		dt("unwrap", "--plain", "--yes", "--use-plain-http", wrapChart(t), "oci://"+repo).AssertSuccess(t)

		require.Equal(t, localizeSourceRepo, dependencyRepository(t, pullChart(t, repo), "child"))
	})
}
