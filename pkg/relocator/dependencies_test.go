package relocator

import (
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	tu "github.com/vmware-labs/distribution-tooling-for-helm/internal/testutil"
	cu "github.com/vmware-labs/distribution-tooling-for-helm/pkg/chartutils"
	"helm.sh/helm/v3/pkg/chart"
	"helm.sh/helm/v3/pkg/chartutil"
	"helm.sh/helm/v3/pkg/cli"
	"helm.sh/helm/v3/pkg/downloader"
	"helm.sh/helm/v3/pkg/getter"
	sigsyaml "sigs.k8s.io/yaml"
)

const sourceRepo = "oci://source.example.com/charts"

type testDep struct {
	name       string
	constraint string
	repository string
	// vendored is one of "dir", "tgz" or "" (not vendored). The vendored version is the one that
	// satisfies the constraint, unless vendoredVersion is set
	vendored        string
	vendoredVersion string
	// vendoredAs overrides the name of the vendored subchart (i.e. to simulate a chart whose name
	// merely starts with the name of the dependency)
	vendoredAs string
}

// newTestChart creates a chart with the specified dependencies and returns its directory.
// If withLock is set, a Chart.lock matching the Chart.yaml is created.
func newTestChart(t *testing.T, chartYamlExtra string, deps []testDep, withLock bool) string {
	t.Helper()
	dir := sb.TempFile()
	require.NoError(t, os.MkdirAll(filepath.Join(dir, "charts"), 0755))

	var chartYaml strings.Builder
	chartYaml.WriteString("apiVersion: v2\nname: parent\nversion: 1.0.0\n")
	chartYaml.WriteString(chartYamlExtra)
	chartYaml.WriteString("dependencies:\n")

	req := make([]*chart.Dependency, 0, len(deps))
	locked := make([]*chart.Dependency, 0, len(deps))
	for _, d := range deps {
		fmt.Fprintf(&chartYaml, "  - name: %s\n    version: %s\n    repository: %s\n", d.name, d.constraint, d.repository)
		req = append(req, &chart.Dependency{Name: d.name, Version: d.constraint, Repository: d.repository})

		version := d.vendoredVersion
		if version == "" {
			version = strings.NewReplacer("x", "0", "*", "0").Replace(d.constraint)
		}
		locked = append(locked, &chart.Dependency{Name: d.name, Version: version, Repository: d.repository})

		if d.vendored == "" {
			continue
		}
		name := d.name
		if d.vendoredAs != "" {
			name = d.vendoredAs
		}
		subchart := &chart.Chart{Metadata: &chart.Metadata{APIVersion: "v2", Name: name, Version: version}}
		switch d.vendored {
		case "dir":
			require.NoError(t, chartutil.SaveDir(subchart, filepath.Join(dir, "charts")))
		case "tgz":
			_, err := chartutil.Save(subchart, filepath.Join(dir, "charts"))
			require.NoError(t, err)
		default:
			t.Fatalf("unknown vendored mode %q", d.vendored)
		}
	}
	require.NoError(t, os.WriteFile(filepath.Join(dir, "Chart.yaml"), []byte(chartYaml.String()), 0644))
	require.NoError(t, os.WriteFile(filepath.Join(dir, "values.yaml"), []byte("{}\n"), 0644))

	if withLock {
		digest, err := hashDependencies(req, locked)
		require.NoError(t, err)
		data, err := sigsyaml.Marshal(&chart.Lock{Dependencies: locked, Digest: digest})
		require.NoError(t, err)
		require.NoError(t, os.WriteFile(filepath.Join(dir, "Chart.lock"), data, 0644))
	}
	return dir
}

func chartRepositories(t *testing.T, chartDir string) map[string]string {
	t.Helper()
	data, err := os.ReadFile(filepath.Join(chartDir, "Chart.yaml"))
	require.NoError(t, err)
	var meta chart.Metadata
	require.NoError(t, sigsyaml.Unmarshal(data, &meta))
	res := map[string]string{}
	for _, d := range meta.Dependencies {
		res[d.Name] = d.Repository
	}
	return res
}

func lockContents(t *testing.T, chartDir string) *chart.Lock {
	t.Helper()
	data, err := os.ReadFile(filepath.Join(chartDir, "Chart.lock"))
	require.NoError(t, err)
	var lock chart.Lock
	require.NoError(t, sigsyaml.Unmarshal(data, &lock))
	return &lock
}

// helmManager returns a Helm dependency manager that is not able to reach any repository,
// so any operation that would need to download a dependency fails
func helmManager(t *testing.T, chartDir string) *downloader.Manager {
	t.Helper()
	return &downloader.Manager{
		Out:              io.Discard,
		ChartPath:        chartDir,
		Getters:          getter.All(cli.New()),
		RepositoryConfig: filepath.Join(sb.TempFile(), "repositories.yaml"),
		RepositoryCache:  sb.TempFile(),
		SkipUpdate:       true,
	}
}

func copyDir(t *testing.T, src string) string {
	t.Helper()
	dst := sb.TempFile()
	require.NoError(t, os.CopyFS(dst, os.DirFS(src)))
	return dst
}

func relocateOpts(extra ...RelocateOption) []RelocateOption {
	return append([]RelocateOption{WithSkipImageRelocation(true), Recursive}, extra...)
}

func TestRelocateChartDirLocalizeDependencies(t *testing.T) {
	t.Run("is disabled by default", func(t *testing.T) {
		dir := newTestChart(t, "", []testDep{{name: "child", constraint: "1.x.x", repository: sourceRepo, vendored: "dir"}}, true)
		before, err := os.ReadFile(filepath.Join(dir, "Chart.yaml"))
		require.NoError(t, err)
		lockBefore, err := os.ReadFile(filepath.Join(dir, "Chart.lock"))
		require.NoError(t, err)

		require.NoError(t, RelocateChartDir(dir, "test.example.com/airgap", relocateOpts()...))

		after, err := os.ReadFile(filepath.Join(dir, "Chart.yaml"))
		require.NoError(t, err)
		lockAfter, err := os.ReadFile(filepath.Join(dir, "Chart.lock"))
		require.NoError(t, err)
		assert.Equal(t, string(before), string(after))
		assert.Equal(t, string(lockBefore), string(lockAfter))
	})

	for _, withLock := range []bool{false, true} {
		for _, mode := range []string{"dir", "tgz"} {
			t.Run(fmt.Sprintf("vendored %s dependencies work offline with Helm (lock=%v)", mode, withLock), func(t *testing.T) {
				dir := newTestChart(t, "", []testDep{
					{name: "child", constraint: "1.x.x", repository: sourceRepo, vendored: mode},
					{name: "other", constraint: "2.0.0", repository: sourceRepo, vendored: mode},
				}, withLock)

				require.NoError(t, RelocateChartDir(dir, "test.example.com/airgap", relocateOpts(WithLocalizeDependencies(true))...))

				assert.Equal(t, map[string]string{"child": "", "other": ""}, chartRepositories(t, dir))

				// Helm itself must be happy with the result without reaching any repository: it
				// validates the Chart.lock digest and loads the subcharts from charts/
				require.NoError(t, helmManager(t, dir).Build())
				assert.DirExists(t, filepath.Join(dir, "charts", "child"))
			})
		}
	}

	t.Run("works without Recursive (tgz archives are unpacked)", func(t *testing.T) {
		dir := newTestChart(t, "", []testDep{{name: "child", constraint: "1.x.x", repository: sourceRepo, vendored: "tgz"}}, true)

		require.NoError(t, RelocateChartDir(dir, "test.example.com/airgap", WithSkipImageRelocation(true), WithLocalizeDependencies(true)))

		assert.Equal(t, map[string]string{"child": ""}, chartRepositories(t, dir))
		require.NoError(t, helmManager(t, dir).Build())
	})

	t.Run("lock matches what Helm generates", func(t *testing.T) {
		dir := newTestChart(t, "", []testDep{
			{name: "child", constraint: "1.x.x", repository: sourceRepo, vendored: "dir", vendoredVersion: "1.2.3"},
		}, true)
		require.NoError(t, RelocateChartDir(dir, "test.example.com/airgap", relocateOpts(WithLocalizeDependencies(true))...))
		ours := lockContents(t, dir)

		// Let Helm regenerate the lock from scratch for the localized chart
		helmDir := copyDir(t, dir)
		require.NoError(t, os.Remove(filepath.Join(helmDir, "Chart.lock")))
		require.NoError(t, helmManager(t, helmDir).Update())
		theirs := lockContents(t, helmDir)

		assert.Equal(t, theirs.Dependencies, ours.Dependencies)
		assert.Equal(t, theirs.Digest, ours.Digest)
		// For dependencies without repository, Helm records the constraint, not the resolved version
		assert.Equal(t, "1.x.x", ours.Dependencies[0].Version)
	})

	t.Run("only the vendored dependencies are localized", func(t *testing.T) {
		dir := newTestChart(t, "", []testDep{
			{name: "vendored", constraint: "1.x.x", repository: sourceRepo, vendored: "dir"},
			{name: "vendored-tgz", constraint: "2.0.0", repository: sourceRepo, vendored: "tgz"},
			{name: "local", constraint: "3.0.0", repository: "file://../local", vendored: "dir"},
			{name: "missing", constraint: "4.0.0", repository: sourceRepo},
		}, true)

		require.NoError(t, RelocateChartDir(dir, "test.example.com/airgap", relocateOpts(WithLocalizeDependencies(true))...))

		assert.Equal(t, map[string]string{
			"vendored":     "",
			"vendored-tgz": "",
			"local":        "",
			"missing":      sourceRepo,
		}, chartRepositories(t, dir))

		lock := lockContents(t, dir)
		for _, d := range lock.Dependencies {
			assert.Equal(t, chartRepositories(t, dir)[d.Name], d.Repository, d.Name)
		}
	})

	t.Run("does not mistake other charts with a similar name for the dependency", func(t *testing.T) {
		for _, tc := range []struct{ dep, vendored string }{
			{"redis", "redis-cluster"},
			{"postgresql", "postgresql-ha"},
			{"common", "common-utils"},
		} {
			t.Run(tc.dep, func(t *testing.T) {
				dir := newTestChart(t, "", []testDep{
					{name: tc.dep, constraint: "1.x.x", repository: sourceRepo, vendored: "tgz", vendoredAs: tc.vendored},
				}, true)

				require.NoError(t, RelocateChartDir(dir, "test.example.com/airgap", relocateOpts(WithLocalizeDependencies(true))...))

				assert.Equal(t, map[string]string{tc.dep: sourceRepo}, chartRepositories(t, dir))
				assert.Equal(t, sourceRepo, lockContents(t, dir).Dependencies[0].Repository)
			})
		}
	})

	t.Run("does not localize a vendored dependency that does not satisfy the version constraint", func(t *testing.T) {
		dir := newTestChart(t, "", []testDep{
			{name: "child", constraint: "1.x.x", repository: sourceRepo, vendored: "dir", vendoredVersion: "2.0.0"},
		}, true)

		require.NoError(t, RelocateChartDir(dir, "test.example.com/airgap", relocateOpts(WithLocalizeDependencies(true))...))

		assert.Equal(t, map[string]string{"child": sourceRepo}, chartRepositories(t, dir))
	})

	t.Run("keeps the rest of Chart.yaml", func(t *testing.T) {
		dir := newTestChart(t, "# keep this comment\ndescription: \"quoted description\"\n",
			[]testDep{{name: "child", constraint: "1.x.x", repository: sourceRepo, vendored: "dir"}}, false)

		require.NoError(t, RelocateChartDir(dir, "test.example.com/airgap", relocateOpts(WithLocalizeDependencies(true))...))

		data, err := os.ReadFile(filepath.Join(dir, "Chart.yaml"))
		require.NoError(t, err)
		assert.Contains(t, string(data), "# keep this comment")
		assert.Contains(t, string(data), `description: "quoted description"`)
		assert.Contains(t, string(data), `repository: ""`)
		assert.Equal(t, map[string]string{"child": ""}, chartRepositories(t, dir))
	})

	t.Run("is applied when skipping image relocation too", func(t *testing.T) {
		dir := newTestChart(t, "", []testDep{{name: "child", constraint: "1.x.x", repository: sourceRepo, vendored: "dir"}}, false)

		require.NoError(t, RelocateChartDir(dir, "test.example.com/airgap", WithSkipImageRelocation(true), WithLocalizeDependencies(true)))

		assert.Equal(t, map[string]string{"child": ""}, chartRepositories(t, dir))
	})

	t.Run("is applied to the subcharts too", func(t *testing.T) {
		dir := newTestChart(t, "", []testDep{{name: "child", constraint: "1.x.x", repository: sourceRepo, vendored: "dir"}}, false)
		// Make the child chart have a vendored dependency of its own
		childDir := filepath.Join(dir, "charts", "child")
		require.NoError(t, os.WriteFile(filepath.Join(childDir, "Chart.yaml"), []byte(
			"apiVersion: v2\nname: child\nversion: 0.0.0\ndependencies:\n  - name: grandchild\n    version: 1.0.0\n    repository: "+sourceRepo+"\n"), 0644))
		require.NoError(t, chartutil.SaveDir(&chart.Chart{Metadata: &chart.Metadata{APIVersion: "v2", Name: "grandchild", Version: "1.0.0"}}, filepath.Join(childDir, "charts")))

		require.NoError(t, RelocateChartDir(dir, "test.example.com/airgap", relocateOpts(WithLocalizeDependencies(true))...))

		assert.Equal(t, map[string]string{"grandchild": ""}, chartRepositories(t, childDir))
	})
}

func TestRelocateChartDirLocalizeDependenciesWithImages(t *testing.T) {
	scenarioDir := "../../testdata/scenarios/chart1"
	chartDir := sb.TempFile()
	require.NoError(t, tu.RenderScenario(scenarioDir, chartDir, map[string]interface{}{"ServerURL": "localhost"}))

	require.NoError(t, RelocateChartDir(chartDir, "test.example.com/airgap", Recursive, WithLocalizeDependencies(true)))

	// The images are still relocated
	values, err := os.ReadFile(filepath.Join(chartDir, "values.yaml"))
	require.NoError(t, err)
	assert.Contains(t, string(values), "test.example.com")

	// Both dependencies are bundled in charts/
	assert.Equal(t, map[string]string{"common": "", "mariadb": ""}, chartRepositories(t, chartDir))
}

func TestLocalizeDependenciesFailure(t *testing.T) {
	scenarioDir := "../../testdata/scenarios/chart1"
	chartDir := sb.TempFile()
	require.NoError(t, tu.RenderScenario(scenarioDir, chartDir, map[string]interface{}{"ServerURL": "localhost"}))

	c, err := cu.LoadChart(chartDir)
	require.NoError(t, err)

	chartYamlBefore, err := os.ReadFile(filepath.Join(chartDir, "Chart.yaml"))
	require.NoError(t, err)
	valuesBefore, err := os.ReadFile(filepath.Join(chartDir, "values.yaml"))
	require.NoError(t, err)

	// A Chart.lock that cannot be parsed. It is written once the chart is loaded, as Helm would
	// refuse to load it otherwise
	badLock := "dependencies: [this is not valid\n"
	require.NoError(t, os.WriteFile(filepath.Join(chartDir, "Chart.lock"), []byte(badLock), 0644))

	t.Run("does not leave Chart.yaml and Chart.lock out of sync", func(t *testing.T) {
		require.Error(t, localizeDependencies(c))

		chartYamlAfter, rErr := os.ReadFile(filepath.Join(chartDir, "Chart.yaml"))
		require.NoError(t, rErr)
		assert.Equal(t, string(chartYamlBefore), string(chartYamlAfter))
		lockAfter, rErr := os.ReadFile(filepath.Join(chartDir, "Chart.lock"))
		require.NoError(t, rErr)
		assert.Equal(t, badLock, string(lockAfter))
	})

	t.Run("does not carry on relocating the chart", func(t *testing.T) {
		rErr := relocateChart(c, "test.example.com/airgap", NewRelocateConfig(WithLocalizeDependencies(true)))
		require.Error(t, rErr)
		assert.Contains(t, rErr.Error(), "failed to localize chart dependencies")

		valuesAfter, rErr := os.ReadFile(filepath.Join(chartDir, "values.yaml"))
		require.NoError(t, rErr)
		assert.Equal(t, string(valuesBefore), string(valuesAfter), "values.yaml must not be relocated")
	})
}
