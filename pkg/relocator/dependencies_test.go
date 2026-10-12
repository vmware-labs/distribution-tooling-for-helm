package relocator

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"helm.sh/helm/v3/pkg/chart"
	"helm.sh/helm/v3/pkg/chartutil"
	sigsyaml "sigs.k8s.io/yaml"
)

const testChartYaml = `apiVersion: v2
name: parent
version: 1.0.0
# keep this comment
dependencies:
  - name: vendored
    version: 1.x.x
    repository: oci://source.example.com/charts
  - name: vendored-tgz
    version: 2.0.0
    repository: oci://source.example.com/charts
  - name: local
    version: 3.0.0
    repository: file://../local
  - name: missing
    version: 4.0.0
    repository: oci://source.example.com/charts
`

func writeSubchart(t *testing.T, dir, name, version string) {
	t.Helper()
	require.NoError(t, os.MkdirAll(dir, 0755))
	require.NoError(t, os.WriteFile(filepath.Join(dir, "Chart.yaml"),
		[]byte("apiVersion: v2\nname: "+name+"\nversion: "+version+"\n"), 0644))
}

func TestRelocateChartDirLocalizeDependencies(t *testing.T) {
	setup := func(t *testing.T, withLock bool) string {
		dir := sb.TempFile()
		require.NoError(t, os.MkdirAll(dir, 0755))
		require.NoError(t, os.WriteFile(filepath.Join(dir, "Chart.yaml"), []byte(testChartYaml), 0644))
		require.NoError(t, os.WriteFile(filepath.Join(dir, "values.yaml"), []byte("{}\n"), 0644))
		writeSubchart(t, filepath.Join(dir, "charts", "vendored"), "vendored", "1.2.3")
		writeSubchart(t, filepath.Join(dir, "charts", "local"), "local", "3.0.0")
		// tgz-vendored dependency
		_, err := chartutil.Save(&chart.Chart{Metadata: &chart.Metadata{APIVersion: "v2", Name: "vendored-tgz", Version: "2.0.0"}}, filepath.Join(dir, "charts"))
		require.NoError(t, err)

		if withLock {
			req := []*chart.Dependency{
				{Name: "vendored", Version: "1.x.x", Repository: "oci://source.example.com/charts"},
				{Name: "vendored-tgz", Version: "2.0.0", Repository: "oci://source.example.com/charts"},
				{Name: "local", Version: "3.0.0", Repository: "file://../local"},
				{Name: "missing", Version: "4.0.0", Repository: "oci://source.example.com/charts"},
			}
			locked := []*chart.Dependency{
				{Name: "vendored", Version: "1.2.3", Repository: "oci://source.example.com/charts"},
				{Name: "vendored-tgz", Version: "2.0.0", Repository: "oci://source.example.com/charts"},
				{Name: "local", Version: "3.0.0", Repository: "file://../local"},
				{Name: "missing", Version: "4.0.0", Repository: "oci://source.example.com/charts"},
			}
			digest, err := hashDependencies(req, locked)
			require.NoError(t, err)
			data, err := sigsyaml.Marshal(&chart.Lock{Dependencies: locked, Digest: digest})
			require.NoError(t, err)
			require.NoError(t, os.WriteFile(filepath.Join(dir, "Chart.lock"), data, 0644))
		}
		return dir
	}

	repos := func(t *testing.T, file string) map[string]string {
		data, err := os.ReadFile(file)
		require.NoError(t, err)
		var parsed struct {
			Dependencies []chart.Dependency `json:"dependencies"`
		}
		require.NoError(t, sigsyaml.Unmarshal(data, &parsed))
		res := map[string]string{}
		for _, d := range parsed.Dependencies {
			res[d.Name] = d.Repository
		}
		return res
	}

	t.Run("disabled by default", func(t *testing.T) {
		dir := setup(t, false)
		require.NoError(t, RelocateChartDir(dir, "test.example.com/airgap"))
		assert.Equal(t, "oci://source.example.com/charts", repos(t, filepath.Join(dir, "Chart.yaml"))["vendored"])
	})

	t.Run("vendored dependencies are localized", func(t *testing.T) {
		dir := setup(t, true)
		require.NoError(t, RelocateChartDir(dir, "test.example.com/airgap", WithLocalizeDependencies(true)))

		got := repos(t, filepath.Join(dir, "Chart.yaml"))
		assert.Equal(t, map[string]string{
			"vendored":     "",
			"vendored-tgz": "",
			"local":        "",
			"missing":      "oci://source.example.com/charts",
		}, got)

		data, err := os.ReadFile(filepath.Join(dir, "Chart.yaml"))
		require.NoError(t, err)
		assert.Contains(t, string(data), "# keep this comment")

		// The lock must be consistent with Chart.yaml: same repositories and a matching digest
		lockData, err := os.ReadFile(filepath.Join(dir, "Chart.lock"))
		require.NoError(t, err)
		var lock chart.Lock
		require.NoError(t, sigsyaml.Unmarshal(lockData, &lock))
		var meta chart.Metadata
		require.NoError(t, sigsyaml.Unmarshal(data, &meta))

		for _, d := range lock.Dependencies {
			assert.Equal(t, got[d.Name], d.Repository, d.Name)
		}
		digest, err := hashDependencies(meta.Dependencies, lock.Dependencies)
		require.NoError(t, err)
		assert.Equal(t, digest, lock.Digest)
	})

	t.Run("works with SkipImageRelocation", func(t *testing.T) {
		dir := setup(t, false)
		require.NoError(t, RelocateChartDir(dir, "test.example.com/airgap", WithSkipImageRelocation(true), WithLocalizeDependencies(true)))
		assert.Equal(t, "", repos(t, filepath.Join(dir, "Chart.yaml"))["vendored"])
	})
}
